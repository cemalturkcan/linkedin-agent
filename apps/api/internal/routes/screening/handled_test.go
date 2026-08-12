package screening

import (
	"context"
	"testing"

	"api/internal/app/events"
	"api/internal/routes/postings"
	"api/internal/routes/settings"
	"api/internal/testutil"
)

const openedAt = "2026-08-11T09:00:00Z"

func handDesk(t *testing.T) (*postings.Service, *Service) {
	t.Helper()
	opened := testutil.NewStore(t)
	source := testutil.FixedClock(openedAt)
	hub := events.NewHub(source)
	t.Cleanup(hub.Close)

	configured := settings.NewService(settings.Dependencies{
		Reads: opened.Reads(),
		Hub:   hub,
		Clock: source,
	})
	found := postings.NewService(postings.Dependencies{
		Reads:        opened.Reads(),
		Transactions: opened,
		Settings:     configured,
		Hub:          hub,
		Clock:        source,
	})
	return found, NewService(Dependencies{
		Reads:        opened.Reads(),
		Transactions: opened,
		Settings:     configured,
		Postings:     found,
		Hub:          hub,
		Clock:        source,
	})
}

func card(id, company, title string) postings.Harvested {
	return postings.Harvested{
		ID:       id,
		Title:    title,
		Company:  company,
		Location: "Berlin",
		URL:      "https://example.invalid/" + id,
	}
}

func TestAPostingOpenedByHandNeverReachesABatch(t *testing.T) {
	found, screener := handDesk(t)
	ctx := context.Background()

	if _, err := found.Ingest(ctx, []postings.Harvested{
		card("1001", "Hooli", "Backend Engineer"),
		card("1002", "Initech", "Platform Engineer"),
	}, nil, "", 1); err != nil {
		t.Fatalf("seed: %v", err)
	}

	before, err := screener.untriaged(ctx, 10)
	if err != nil {
		t.Fatalf("untriaged: %v", err)
	}
	if len(before) != 2 {
		t.Fatalf("the screener sees %d postings, want 2", len(before))
	}

	if _, err := found.Open(ctx, []postings.Harvested{
		card("1001", "Hooli", "Backend Engineer"),
	}, postings.Pick{Code: "GO", Lang: "en"}); err != nil {
		t.Fatalf("open: %v", err)
	}

	after, err := screener.untriaged(ctx, 10)
	if err != nil {
		t.Fatalf("untriaged: %v", err)
	}
	if len(after) != 1 || after[0].ID != "1002" {
		t.Fatalf("the screener would still spend a call on what the person handled: %+v", after)
	}
}

func TestACompanyOpenedByHandRidesTheReapplyCooldown(t *testing.T) {
	found, screener := handDesk(t)
	ctx := context.Background()

	if _, err := found.Open(ctx, []postings.Harvested{
		card("2001", "Umbrella GmbH", "Data Engineer"),
	}, postings.Pick{}); err != nil {
		t.Fatalf("open: %v", err)
	}

	engaged, err := screener.engagedCompanies(ctx, 90)
	if err != nil {
		t.Fatalf("engaged: %v", err)
	}
	recent, present := engaged[postings.CompanyKey("Umbrella GmbH")]
	if !present {
		t.Fatalf("the company the person worked by hand is outside the cooldown: %+v", engaged)
	}
	if !recent.ByHand {
		t.Fatalf("the cooldown reads a hand-opened posting as an application: %+v", recent)
	}
}
