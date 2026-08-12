package planner_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"api/internal/app/claude"
	"api/internal/app/events"
	"api/internal/app/store"
	"api/internal/app/trace"
	"api/internal/routes/indexer"
	"api/internal/routes/planner"
	"api/internal/routes/plugin"
	"api/internal/routes/postings"
	"api/internal/routes/resumes"
	"api/internal/routes/settings"
	"api/internal/testutil"
)

const boot = "2026-08-11T09:00:00Z"

type countingTransport struct {
	calls atomic.Int64
}

func (t *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls.Add(1)
	return nil, errors.New("the model is not reachable from this test")
}

type stubToken struct{}

func (stubToken) AccessToken(context.Context) (string, error) {
	return "test-token", nil
}

type desk struct {
	planner  *planner.Service
	plugin   *plugin.Service
	settings *settings.Service
	store    *store.Store
	clock    *testutil.MovingClock
	model    *countingTransport
}

func newDesk(t *testing.T) *desk {
	t.Helper()
	opened := testutil.NewStore(t)
	source := testutil.NewMovingClock(boot)
	hub := events.NewHub(source)
	t.Cleanup(hub.Close)

	recorder := trace.NewRecorder(trace.Dependencies{
		Reads:        opened.Reads(),
		Transactions: opened,
		Hub:          hub,
		Clock:        source,
		Config: trace.Config{
			FlushInterval:  time.Second,
			MaxFrameChars:  1000,
			MaxStoredChars: 10000,
			WriteTimeout:   5 * time.Second,
		},
	})
	transport := &countingTransport{}
	model := claude.NewClient(
		stubToken{},
		recorder,
		&http.Client{Transport: transport},
		claude.Config{Model: claude.Model, MaxAttempts: 1},
		source,
	)

	folder := resumes.NewService(resumes.Dependencies{Reads: opened.Reads(), Clock: source})
	configured := settings.NewService(settings.Dependencies{
		Reads: opened.Reads(),
		Hub:   hub,
		Clock: source,
	})
	derived := indexer.NewService(indexer.Dependencies{
		Resumes: folder,
		Claude:  model,
		Reads:   opened.Reads(),
		Hub:     hub,
		Clock:   source,
		Config:  indexer.Config{RunBudget: time.Minute},
	})
	held := plugin.NewService(plugin.Dependencies{
		Reads:  opened.Reads(),
		Hub:    hub,
		Clock:  source,
		Config: plugin.Config{ConnectedWindow: plugin.ConnectedWindow},
	})
	t.Cleanup(held.Close)

	harvested := postings.NewService(postings.Dependencies{
		Reads:        opened.Reads(),
		Transactions: opened,
		Settings:     configured,
		Hub:          hub,
		Clock:        source,
	})
	rounds := planner.NewService(planner.Dependencies{
		Reads:        opened.Reads(),
		Transactions: opened,
		Claude:       model,
		Settings:     configured,
		Plugin:       held,
		Profiles:     derived,
		Digest:       harvested,
		Hub:          hub,
		Clock:        source,
		Config: planner.Config{
			KnownIDWindow: 14 * 24 * time.Hour,
			KnownIDCap:    100,
		},
	})
	harvested.UseRounds(rounds)

	return &desk{
		planner:  rounds,
		plugin:   held,
		settings: configured,
		store:    opened,
		clock:    source,
		model:    transport,
	}
}

func (d *desk) withIndexedCandidate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"backend.pdf", "go.pdf"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("%PDF-1.4\n"), 0o600); err != nil {
			t.Fatalf("write a cv fixture: %v", err)
		}
	}
	if _, err := d.store.Reads().ExecContext(
		context.Background(),
		"INSERT INTO resume_folder (id, path, updated_at) VALUES (1, ?, ?)",
		dir,
		boot,
	); err != nil {
		t.Fatalf("save the cv folder: %v", err)
	}
	if _, err := d.store.Reads().ExecContext(
		context.Background(),
		`INSERT INTO candidate_profile (id, profile, source_key, model, indexed_at)
		 VALUES (1, ?, 'seeded', 'claude-opus-5', ?)`,
		`{"yearsExperience":6,"seniorityBand":"senior","coreStack":["Go"],`+
			`"secondaryStack":[],"domains":[],"workLanguages":["en"],"places":[],`+
			`"headline":"a senior backend engineer"}`,
		boot,
	); err != nil {
		t.Fatalf("seed the candidate profile: %v", err)
	}
}

func (d *desk) hello(t *testing.T) {
	t.Helper()
	if _, err := d.plugin.Hello(context.Background(), plugin.Hello{
		ID:      strings.Repeat("a", 32),
		Version: "1.0.0",
	}); err != nil {
		t.Fatalf("hello: %v", err)
	}
}

func refusalOf(t *testing.T, err error) *planner.RefusalError {
	t.Helper()
	var refusal *planner.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a refusal", err)
	}
	return refusal
}

func TestPlanningRefusesAndSpendsNothingWhenNoExtensionEverCheckedIn(t *testing.T) {
	desk := newDesk(t)
	desk.withIndexedCandidate(t)

	_, err := desk.planner.Start(context.Background())
	refusal := refusalOf(t, err)

	if refusal.Reason != planner.ReasonExecutorOffline {
		t.Fatalf("reason = %q", refusal.Reason)
	}
	if refusal.OK {
		t.Fatal("a refusal reported ok")
	}
	if calls := desk.model.calls.Load(); calls != 0 {
		t.Fatalf("planning spent %d model calls before refusing", calls)
	}
}

func TestPlanningRefusesWhilePaused(t *testing.T) {
	desk := newDesk(t)
	desk.withIndexedCandidate(t)
	desk.hello(t)

	if _, err := desk.settings.Save(context.Background(), map[string]any{
		"apply": map[string]any{"paused": true},
	}); err != nil {
		t.Fatalf("pause: %v", err)
	}

	_, err := desk.planner.Start(context.Background())
	if refusal := refusalOf(t, err); refusal.Reason != planner.ReasonPaused {
		t.Fatalf("reason = %q", refusal.Reason)
	}
	if calls := desk.model.calls.Load(); calls != 0 {
		t.Fatalf("a paused desk spent %d model calls", calls)
	}
}

func TestPlanningRefusesAtTheDailyModelCallCap(t *testing.T) {
	desk := newDesk(t)
	desk.withIndexedCandidate(t)
	desk.hello(t)

	if _, err := desk.settings.Save(context.Background(), map[string]any{
		"budget": map[string]any{"dailyModelCallCap": float64(1)},
	}); err != nil {
		t.Fatalf("set the cap: %v", err)
	}
	if _, err := desk.store.Reads().ExecContext(
		context.Background(),
		"INSERT INTO model_usage (day, calls) VALUES (?, 1)",
		"2026-08-11",
	); err != nil {
		t.Fatalf("seed the usage: %v", err)
	}

	_, err := desk.planner.Start(context.Background())
	if refusal := refusalOf(t, err); refusal.Reason != planner.ReasonDailyCap {
		t.Fatalf("reason = %q", refusal.Reason)
	}
	if calls := desk.model.calls.Load(); calls != 0 {
		t.Fatalf("a spent budget still made %d model calls", calls)
	}
}

func TestPlanningRefusesAtTheRoundModelCallCeiling(t *testing.T) {
	desk := newDesk(t)
	desk.withIndexedCandidate(t)
	desk.hello(t)

	if _, err := desk.settings.Save(context.Background(), map[string]any{
		"budget": map[string]any{"maxModelCallsPerCycle": float64(2)},
	}); err != nil {
		t.Fatalf("set the round ceiling: %v", err)
	}
	id := desk.openRound(t)
	if _, err := desk.store.Reads().ExecContext(
		context.Background(),
		"UPDATE cycles SET model_calls = 2 WHERE id = ?",
		id,
	); err != nil {
		t.Fatalf("seed the round spend: %v", err)
	}

	_, err := desk.planner.Start(context.Background())
	refusal := refusalOf(t, err)
	if refusal.Reason != planner.ReasonRoundCap {
		t.Fatalf("reason = %q", refusal.Reason)
	}
	if !strings.Contains(refusal.Message, "all 2 model calls") {
		t.Fatalf("message = %q", refusal.Message)
	}
	if calls := desk.model.calls.Load(); calls != 0 {
		t.Fatalf("a spent round ceiling still made %d model calls", calls)
	}
}

func TestPlanningRefusesWhenTheExtensionHasGoneQuiet(t *testing.T) {
	desk := newDesk(t)
	desk.withIndexedCandidate(t)
	desk.hello(t)
	desk.clock.Advance(plugin.ConnectedWindow + time.Minute)

	_, err := desk.planner.Start(context.Background())
	refusal := refusalOf(t, err)
	if refusal.Reason != planner.ReasonExecutorOffline {
		t.Fatalf("reason = %q", refusal.Reason)
	}
	if !strings.Contains(refusal.Message, "last checked in") {
		t.Fatalf("message = %q", refusal.Message)
	}
	if calls := desk.model.calls.Load(); calls != 0 {
		t.Fatalf("a quiet executor still cost %d model calls", calls)
	}
}

func (d *desk) openRound(t *testing.T) int64 {
	t.Helper()
	outcome, err := d.store.Reads().ExecContext(
		context.Background(),
		`INSERT INTO cycles (started_at, status, rationale, screen_budget)
		 VALUES (?, 'open', 'seeded round', 40)`,
		boot,
	)
	if err != nil {
		t.Fatalf("open a round: %v", err)
	}
	id, err := outcome.LastInsertId()
	if err != nil {
		t.Fatalf("read the round id: %v", err)
	}
	if _, err := d.store.Reads().ExecContext(
		context.Background(),
		`INSERT INTO cycle_queries (cycle_id, position, label, query)
		 VALUES (?, 0, 'home-city', '{"label":"home-city","ring":"city","place":"Istanbul"}')`,
		id,
	); err != nil {
		t.Fatalf("seed a round query: %v", err)
	}
	return id
}

func TestARoundInsideItsDeadlineIsResumed(t *testing.T) {
	desk := newDesk(t)
	desk.withIndexedCandidate(t)
	desk.hello(t)
	id := desk.openRound(t)

	plan, err := desk.planner.Start(context.Background())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !plan.Resumed || plan.CycleID != id {
		t.Fatalf("plan = %+v", plan)
	}
	if calls := desk.model.calls.Load(); calls != 0 {
		t.Fatalf("resuming a live round spent %d model calls", calls)
	}
}

func TestARoundPastItsDeadlineIsClosedRatherThanResumed(t *testing.T) {
	desk := newDesk(t)
	desk.withIndexedCandidate(t)
	desk.hello(t)
	id := desk.openRound(t)

	current, err := desk.settings.Load(context.Background())
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	desk.clock.Advance(time.Duration(current.Harvest.RoundDeadlineMs)*time.Millisecond + time.Minute)
	desk.hello(t)

	_, err = desk.planner.Start(context.Background())
	if refusal := refusalOf(t, err); refusal.Reason != planner.ReasonPlannerFailed {
		t.Fatalf("reason = %q, want the planner call to have been attempted", refusal.Reason)
	}

	closed, err := desk.planner.Round(context.Background(), id)
	if err != nil {
		t.Fatalf("read the round: %v", err)
	}
	if closed.Status != planner.StatusDone {
		t.Fatalf("status = %q, want the round closed", closed.Status)
	}
	if closed.TerminationReason == nil ||
		!strings.Contains(*closed.TerminationReason, "deadline passed") {
		t.Fatalf("termination reason = %v", closed.TerminationReason)
	}
	if closed.FinishedAt == nil {
		t.Fatal("a closed round carries no finish stamp")
	}
}

func TestNextRunAtFollowsTheAutoCycleSwitch(t *testing.T) {
	desk := newDesk(t)
	desk.withIndexedCandidate(t)
	desk.hello(t)

	if _, err := desk.settings.Save(context.Background(), map[string]any{
		"budget": map[string]any{"autoCycle": false},
	}); err != nil {
		t.Fatalf("switch autoCycle off: %v", err)
	}

	state, err := desk.planner.State(context.Background())
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.NextRunAt != nil {
		t.Fatalf("nextRunAt = %v with autoCycle off, want null", *state.NextRunAt)
	}

	if _, err := desk.settings.Save(context.Background(), map[string]any{
		"budget": map[string]any{"autoCycle": true, "cycleMinutes": float64(10)},
	}); err != nil {
		t.Fatalf("turn autoCycle on: %v", err)
	}
	desk.openRound(t)

	state, err = desk.planner.State(context.Background())
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.NextRunAt == nil {
		t.Fatal("nextRunAt is null with autoCycle on")
	}
	wanted := "2026-08-11T09:10:00Z"
	if got, err := time.Parse(time.RFC3339Nano, *state.NextRunAt); err != nil {
		t.Fatalf("nextRunAt = %q: %v", *state.NextRunAt, err)
	} else if !got.Equal(mustParse(t, wanted)) {
		t.Fatalf("nextRunAt = %q, want %q", *state.NextRunAt, wanted)
	}
}

func mustParse(t *testing.T, stamp string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		t.Fatalf("parse %q: %v", stamp, err)
	}
	return at
}
