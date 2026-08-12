package postings_test

import (
	"context"
	"testing"

	"api/internal/app/events"
	"api/internal/app/store"
	"api/internal/routes/postings"
	"api/internal/routes/settings"
	"api/internal/testutil"
)

const now = "2026-08-11T09:00:00Z"

func newService(t *testing.T) (*postings.Service, *store.Store) {
	t.Helper()
	opened := testutil.NewStore(t)
	source := testutil.FixedClock(now)
	hub := events.NewHub(source)
	t.Cleanup(hub.Close)

	configured := settings.NewService(settings.Dependencies{
		Reads: opened.Reads(),
		Hub:   hub,
		Clock: source,
	})
	return postings.NewService(postings.Dependencies{
		Reads:        opened.Reads(),
		Transactions: opened,
		Settings:     configured,
		Hub:          hub,
		Clock:        source,
	}), opened
}

func harvested(id, company, title, location string) postings.Harvested {
	return postings.Harvested{
		ID:       id,
		Title:    title,
		Company:  company,
		Location: location,
		URL:      "https://example.invalid/" + id,
	}
}

func TestADuplicateAcrossTwoCitiesCollapses(t *testing.T) {
	service, _ := newService(t)
	ctx := context.Background()

	first, err := service.Ingest(ctx, []postings.Harvested{
		harvested("1001", "Acme GmbH", "Senior Go Engineer", "Berlin"),
	}, nil, "", 1)
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if first.Inserted != 1 || first.Collapsed != 0 {
		t.Fatalf("first ingest = %+v", first)
	}

	second, err := service.Ingest(ctx, []postings.Harvested{
		harvested("2002", "Acme GmbH", "Senior Go Engineer", "Munich"),
	}, nil, "", 1)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if second.Inserted != 0 || second.Collapsed != 1 {
		t.Fatalf("the second city did not collapse: %+v", second)
	}

	duplicates, err := service.List(ctx, postings.StatusDuplicate)
	if err != nil {
		t.Fatalf("list duplicates: %v", err)
	}
	if len(duplicates) != 1 || duplicates[0].ID != "2002" {
		t.Fatalf("duplicates = %+v", duplicates)
	}
	if duplicates[0].DupeOf == nil || *duplicates[0].DupeOf != "1001" {
		t.Fatalf("the duplicate does not point at the canonical posting: %+v", duplicates[0])
	}
}

func TestRepostingTheSameIdDoesNotInsertTwice(t *testing.T) {
	service, _ := newService(t)
	ctx := context.Background()
	batch := []postings.Harvested{harvested("3003", "Globex", "Backend Engineer", "Istanbul")}

	first, err := service.Ingest(ctx, batch, nil, "", 1)
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if first.Inserted != 1 {
		t.Fatalf("first ingest = %+v", first)
	}

	second, err := service.Ingest(ctx, batch, nil, "", 1)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if second.Inserted != 0 || second.Duplicates != 1 {
		t.Fatalf("a re-post inserted again: %+v", second)
	}

	all, err := service.List(ctx, postings.ListAll)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("the store holds %d rows, want 1", len(all))
	}
}

func TestAPostingAlreadyCarryingAVerdictNeverReachesABatch(t *testing.T) {
	service, opened := newService(t)
	ctx := context.Background()

	if _, err := service.Ingest(ctx, []postings.Harvested{
		harvested("4004", "Initech", "Platform Engineer", "Istanbul"),
	}, nil, "", 1); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := opened.Reads().ExecContext(
		ctx,
		"UPDATE jobs SET status = ? WHERE id = ?",
		postings.StatusApplied,
		"4004",
	); err != nil {
		t.Fatalf("record a verdict: %v", err)
	}

	result, err := service.Ingest(ctx, []postings.Harvested{
		harvested("4004", "Initech", "Platform Engineer", "Istanbul"),
		harvested("5005", "Initech", "Data Engineer", "Istanbul"),
	}, nil, "", 1)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Judged != 1 {
		t.Fatalf("judged = %d, want 1: %+v", result.Judged, result)
	}
	if result.Inserted != 1 {
		t.Fatalf("inserted = %d, want 1: %+v", result.Inserted, result)
	}
	for _, id := range result.NewIDs {
		if id == "4004" {
			t.Fatal("a judged posting reached the batch")
		}
	}

	applied, err := service.List(ctx, postings.StatusApplied)
	if err != nil {
		t.Fatalf("list applied: %v", err)
	}
	if len(applied) != 1 || applied[0].Status != postings.StatusApplied {
		t.Fatalf("the verdict was overwritten: %+v", applied)
	}
}

func TestDedupeOffKeepsBothPostings(t *testing.T) {
	service, opened := newService(t)
	ctx := context.Background()

	configured := settings.NewService(settings.Dependencies{
		Reads: opened.Reads(),
		Hub:   events.NewHub(testutil.FixedClock(now)),
		Clock: testutil.FixedClock(now),
	})
	if _, err := configured.Save(ctx, map[string]any{
		"companies": map[string]any{"dedupe": false},
	}); err != nil {
		t.Fatalf("turn dedupe off: %v", err)
	}

	if _, err := service.Ingest(ctx, []postings.Harvested{
		harvested("6006", "Umbrella", "Site Reliability Engineer", "Berlin"),
	}, nil, "", 1); err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	second, err := service.Ingest(ctx, []postings.Harvested{
		harvested("7007", "Umbrella", "Site Reliability Engineer", "Munich"),
	}, nil, "", 1)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if second.Inserted != 1 || second.Collapsed != 0 {
		t.Fatalf("dedupe stayed on: %+v", second)
	}
}

func TestTheJudgedDigestCarriesOnlyJudgedPostings(t *testing.T) {
	service, opened := newService(t)
	ctx := context.Background()

	if _, err := service.Ingest(ctx, []postings.Harvested{
		harvested("8008", "Hooli", "Backend Engineer", "Istanbul"),
		harvested("9009", "Pied Piper", "Backend Engineer", "Istanbul"),
	}, nil, "", 1); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := opened.Reads().ExecContext(
		ctx,
		"UPDATE jobs SET status = ? WHERE id = ?",
		postings.StatusSkipped,
		"8008",
	); err != nil {
		t.Fatalf("record a verdict: %v", err)
	}

	digest, err := service.RecentJudgedIDs(ctx, 14*24*60*60*1e9, 100)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	if len(digest) != 1 || digest[0] != "8008" {
		t.Fatalf("digest = %v, want only the judged posting", digest)
	}
}

func TestAPostingOpenedByHandIsHeldAgainstTheAgent(t *testing.T) {
	service, _ := newService(t)
	ctx := context.Background()

	if _, err := service.Ingest(ctx, []postings.Harvested{
		harvested("1111", "Hooli", "Backend Engineer", "Istanbul"),
	}, nil, "", 1); err != nil {
		t.Fatalf("seed: %v", err)
	}

	held, err := service.Open(ctx, []postings.Harvested{
		harvested("1111", "Hooli", "Backend Engineer", "Istanbul"),
	}, postings.Pick{Code: "GO", Lang: "en"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if held.Recorded != 1 || held.Held != 0 {
		t.Fatalf("open = %+v", held)
	}

	mine, err := service.Get(ctx, "1111")
	if err != nil {
		t.Fatalf("read the posting: %v", err)
	}
	if mine.Status != postings.StatusOpened || mine.Stage != postings.StageHand {
		t.Fatalf("the posting is %s at stage %s", mine.Status, mine.Stage)
	}
	if mine.ResumeCode == nil || *mine.ResumeCode != "GO" {
		t.Fatalf("the cv the person chose was not kept: %+v", mine.ResumeCode)
	}

	again, err := service.Ingest(ctx, []postings.Harvested{
		harvested("1111", "Hooli", "Backend Engineer", "Istanbul"),
	}, nil, "", 1)
	if err != nil {
		t.Fatalf("re-ingest: %v", err)
	}
	if again.Judged != 1 || again.Inserted != 0 {
		t.Fatalf("a hand-handled posting came back to the agent: %+v", again)
	}
}

func TestAPostingOpenedByHandIsRecordedEvenWhenTheAgentNeverSawIt(t *testing.T) {
	service, _ := newService(t)
	ctx := context.Background()

	held, err := service.Open(ctx, []postings.Harvested{
		harvested("2222", "Initech", "Platform Engineer", "Berlin"),
	}, postings.Pick{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if held.Recorded != 1 {
		t.Fatalf("open = %+v", held)
	}

	mine, err := service.List(ctx, postings.StatusOpened)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(mine) != 1 || mine[0].ID != "2222" || mine[0].ResumeCode != nil {
		t.Fatalf("opened = %+v", mine)
	}
}

func TestOpeningAPostingTwiceHoldsItOnce(t *testing.T) {
	service, _ := newService(t)
	ctx := context.Background()
	card := []postings.Harvested{harvested("3333", "Globex", "Data Engineer", "Berlin")}

	if _, err := service.Open(ctx, card, postings.Pick{}); err != nil {
		t.Fatalf("first open: %v", err)
	}
	second, err := service.Open(ctx, card, postings.Pick{})
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	if second.Held != 1 || second.Recorded != 0 {
		t.Fatalf("second open = %+v", second)
	}
}

func TestAPostingOpenedByHandStillCollapsesOntoTheRoleOnFile(t *testing.T) {
	service, _ := newService(t)
	ctx := context.Background()

	if _, err := service.Ingest(ctx, []postings.Harvested{
		harvested("4444", "Umbrella GmbH", "Senior Go Engineer", "Berlin"),
	}, nil, "", 1); err != nil {
		t.Fatalf("seed: %v", err)
	}

	held, err := service.Open(ctx, []postings.Harvested{
		harvested("5555", "Umbrella GmbH", "Senior Go Engineer", "Munich"),
	}, postings.Pick{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if held.Collapsed != 1 || held.Recorded != 0 {
		t.Fatalf("open = %+v", held)
	}

	duplicate, err := service.Get(ctx, "5555")
	if err != nil {
		t.Fatalf("read the duplicate: %v", err)
	}
	if duplicate.Status != postings.StatusDuplicate ||
		duplicate.DupeOf == nil || *duplicate.DupeOf != "4444" {
		t.Fatalf("duplicate = %+v", duplicate)
	}
}

func TestAPostingTheAgentAlreadyOwnsIsNotTakenByAnOpen(t *testing.T) {
	service, opened := newService(t)
	ctx := context.Background()

	if _, err := service.Ingest(ctx, []postings.Harvested{
		harvested("6666", "Initech", "Backend Engineer", "Istanbul"),
	}, nil, "", 1); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := opened.Reads().ExecContext(
		ctx,
		"UPDATE jobs SET status = ?, stage = 'deep', resume_code = 'JA' WHERE id = ?",
		postings.StatusQueue,
		"6666",
	); err != nil {
		t.Fatalf("queue it: %v", err)
	}

	held, err := service.Open(ctx, []postings.Harvested{
		harvested("6666", "Initech", "Backend Engineer", "Istanbul"),
	}, postings.Pick{Code: "GO", Lang: "en"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if held.Held != 1 || held.Recorded != 0 {
		t.Fatalf("open = %+v", held)
	}

	queued, err := service.Get(ctx, "6666")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if queued.Status != postings.StatusQueue || *queued.ResumeCode != "JA" {
		t.Fatalf("the agent's own row was overwritten: %+v", queued)
	}
}
