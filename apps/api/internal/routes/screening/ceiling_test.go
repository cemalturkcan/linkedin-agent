package screening

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
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

const cappedAt = "2026-08-11T09:00:00Z"

var seededID = regexp.MustCompile(`cap-\d+`)

type triageStub struct {
	calls atomic.Int64
}

func (t *triageStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.calls.Add(1)
	asked, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	seen := map[string]struct{}{}
	decisions := make([]map[string]any, 0)
	for _, id := range seededID.FindAllString(string(asked), -1) {
		if _, done := seen[id]; done {
			continue
		}
		seen[id] = struct{}{}
		decisions = append(decisions, map[string]any{
			"id":      id,
			"keep":    false,
			"promise": 10,
			"reason":  "the posting runs a stack the candidate does not carry",
		})
	}
	input, err := json.Marshal(map[string]any{"decisions": decisions})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	partial, err := json.Marshal(string(input))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	frames := []string{
		`{"type":"message_start","message":{"model":"claude-opus-5",` +
			`"usage":{"input_tokens":900,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":` +
			`{"type":"tool_use","id":"toolu_1","name":"triage_jobs","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":` +
			`{"type":"input_json_delta","partial_json":` + string(partial) + `}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":120}}`,
		`{"type":"message_stop"}`,
	}
	for _, frame := range frames {
		if _, err := io.WriteString(w, "data: "+frame+"\n\n"); err != nil {
			return
		}
	}
}

type cappedDesk struct {
	screening *Service
	postings  *postings.Service
	planner   *planner.Service
	settings  *settings.Service
	store     *store.Store
	model     *triageStub
}

func newCappedDesk(t *testing.T) *cappedDesk {
	t.Helper()
	opened := testutil.NewStore(t)
	source := testutil.FixedClock(cappedAt)
	hub := events.NewHub(source)
	t.Cleanup(hub.Close)

	recorder := trace.NewRecorder(trace.Dependencies{
		Reads:        opened.Reads(),
		Transactions: opened,
		Hub:          hub,
		Clock:        source,
		Config: trace.Config{
			FlushInterval:  time.Millisecond,
			MaxFrameChars:  1_000,
			MaxStoredChars: 100_000,
			WriteTimeout:   5 * time.Second,
		},
	})
	stub := &triageStub{}
	server := httptest.NewServer(stub)
	t.Cleanup(server.Close)

	model := claude.NewClient(
		stubToken{},
		recorder,
		server.Client(),
		claude.Config{Model: claude.Model, MaxAttempts: 1, Endpoint: server.URL},
		source,
	)

	folder := resumes.NewService(resumes.Dependencies{Reads: opened.Reads(), Clock: source})
	configured := settings.NewService(settings.Dependencies{
		Reads:     opened.Reads(),
		Hub:       hub,
		Languages: folder,
		Clock:     source,
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

	found := postings.NewService(postings.Dependencies{
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
		Digest:       found,
		Hub:          hub,
		Clock:        source,
		Config:       planner.Config{KnownIDWindow: time.Hour, KnownIDCap: 10},
	})
	found.UseRounds(rounds)

	screener := NewService(Dependencies{
		Reads:        opened.Reads(),
		Transactions: opened,
		Claude:       model,
		Settings:     configured,
		Profiles:     derived,
		Postings:     found,
		Rounds:       rounds,
		Hub:          hub,
		Clock:        source,
		Config:       Config{ScreenBudget: time.Minute},
	})

	desk := &cappedDesk{
		screening: screener,
		postings:  found,
		planner:   rounds,
		settings:  configured,
		store:     opened,
		model:     stub,
	}
	desk.withIndexedCandidate(t)
	return desk
}

type stubToken struct{}

func (stubToken) AccessToken(context.Context) (string, error) {
	return "sk-ant-oat01-TESTTOKENTESTTOKEN", nil
}

func (d *cappedDesk) withIndexedCandidate(t *testing.T) {
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
		cappedAt,
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
		cappedAt,
	); err != nil {
		t.Fatalf("seed the candidate profile: %v", err)
	}
}

func (d *cappedDesk) openRound(t *testing.T) int64 {
	t.Helper()
	outcome, err := d.store.Reads().ExecContext(
		context.Background(),
		`INSERT INTO cycles (started_at, status, rationale, screen_budget)
		 VALUES (?, 'open', 'seeded round', 200)`,
		cappedAt,
	)
	if err != nil {
		t.Fatalf("open a round: %v", err)
	}
	id, err := outcome.LastInsertId()
	if err != nil {
		t.Fatalf("read the round id: %v", err)
	}
	return id
}

func (d *cappedDesk) seedPostings(t *testing.T, count int) {
	t.Helper()
	harvested := make([]postings.Harvested, 0, count)
	for index := range count {
		id := "cap-" + itoa(index)
		harvested = append(harvested, postings.Harvested{
			ID:       id,
			Title:    "Backend Engineer " + itoa(index),
			Company:  "Company " + itoa(index),
			Location: "Berlin",
			URL:      "https://example.invalid/" + id,
		})
	}
	if _, err := d.postings.Ingest(context.Background(), harvested, nil, "", 1); err != nil {
		t.Fatalf("seed postings: %v", err)
	}
}

func (d *cappedDesk) setCeiling(t *testing.T, calls int) {
	t.Helper()
	if _, err := d.settings.Save(context.Background(), map[string]any{
		"budget": map[string]any{"maxModelCallsPerCycle": float64(calls)},
	}); err != nil {
		t.Fatalf("set the round ceiling: %v", err)
	}
}

func TestScreeningStopsAtABatchBoundaryWhenTheRoundCeilingIsReached(t *testing.T) {
	desk := newCappedDesk(t)
	desk.openRound(t)
	desk.setCeiling(t, 1)
	desk.seedPostings(t, 3*TriageBatch)

	result, err := desk.screening.Screen(context.Background(), 0)
	if err != nil {
		t.Fatalf("screen: %v", err)
	}

	if calls := desk.model.calls.Load(); calls != 1 {
		t.Fatalf("screening spent %d model calls under a ceiling of 1", calls)
	}
	if result.Stopped == "" {
		t.Fatal("screening stopped without recording why")
	}
	if !strings.Contains(result.Stopped, "1 model calls") {
		t.Fatalf("stopped = %q, want the ceiling named", result.Stopped)
	}
	if result.Triaged != TriageBatch {
		t.Fatalf("triaged = %d, want one whole batch of %d", result.Triaged, TriageBatch)
	}

	left, err := desk.screening.untriaged(context.Background(), 3*TriageBatch)
	if err != nil {
		t.Fatalf("untriaged: %v", err)
	}
	if len(left) != 2*TriageBatch {
		t.Fatalf("%d postings left unjudged, want %d", len(left), 2*TriageBatch)
	}
}

func TestTheRoundCeilingCountsEveryScreeningCallAgainstTheOpenRound(t *testing.T) {
	desk := newCappedDesk(t)
	id := desk.openRound(t)
	desk.setCeiling(t, 1)
	desk.seedPostings(t, TriageBatch)

	if _, err := desk.screening.Screen(context.Background(), 0); err != nil {
		t.Fatalf("screen: %v", err)
	}

	round, err := desk.planner.Round(context.Background(), id)
	if err != nil {
		t.Fatalf("read the round: %v", err)
	}
	if round.ModelCalls != 1 {
		t.Fatalf("the round carries %d model calls, want the triage call charged to it",
			round.ModelCalls)
	}
}

func TestAZeroRoundCeilingScreensEveryBatch(t *testing.T) {
	desk := newCappedDesk(t)
	desk.openRound(t)
	desk.seedPostings(t, 3*TriageBatch)

	result, err := desk.screening.Screen(context.Background(), 0)
	if err != nil {
		t.Fatalf("screen: %v", err)
	}

	if result.Stopped != "" {
		t.Fatalf("a ceiling of 0 stopped screening: %q", result.Stopped)
	}
	if calls := desk.model.calls.Load(); calls != 3 {
		t.Fatalf("screening spent %d model calls, want one per batch", calls)
	}
	if result.Triaged != 3*TriageBatch {
		t.Fatalf("triaged = %d, want every seeded posting", result.Triaged)
	}
}

func TestPostingsTheCeilingLeftUnjudgedAreScreenedInTheNextRound(t *testing.T) {
	desk := newCappedDesk(t)
	first := desk.openRound(t)
	desk.setCeiling(t, 1)
	desk.seedPostings(t, 2*TriageBatch)

	if _, err := desk.screening.Screen(context.Background(), 0); err != nil {
		t.Fatalf("first screen: %v", err)
	}
	if _, err := desk.store.Reads().ExecContext(
		context.Background(),
		"UPDATE cycles SET status = 'done', finished_at = ? WHERE id = ?",
		cappedAt,
		first,
	); err != nil {
		t.Fatalf("close the first round: %v", err)
	}
	desk.openRound(t)

	result, err := desk.screening.Screen(context.Background(), 0)
	if err != nil {
		t.Fatalf("second screen: %v", err)
	}
	if result.Triaged != TriageBatch {
		t.Fatalf("the next round triaged %d, want the %d the ceiling left",
			result.Triaged, TriageBatch)
	}

	left, err := desk.screening.untriaged(context.Background(), 2*TriageBatch)
	if err != nil {
		t.Fatalf("untriaged: %v", err)
	}
	if len(left) != 0 {
		t.Fatalf("%d postings are still unjudged after the next round", len(left))
	}
}
