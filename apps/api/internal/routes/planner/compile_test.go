package planner

import (
	"strings"
	"testing"

	"api/internal/routes/plugin"
	"api/internal/routes/settings"
)

func testContext() planContext {
	return planContext{
		capabilities: plugin.Capabilities{
			Fetch: plugin.Fetch{
				Place:            true,
				Rings:            settings.PlaceRings,
				Ranges:           plugin.KnownRanges,
				EasyApply:        true,
				RemoteOnly:       true,
				Sorts:            plugin.KnownSorts,
				PageSize:         25,
				MaxPagesPerQuery: 8,
			},
			SeenSet: true,
		},
		maxQueries:   4,
		screenBudget: 40,
		maxWant:      50,
	}
}

func wellFormed(label, ring, place string) modelQuery {
	return modelQuery{
		Label:  label,
		Ring:   ring,
		Place:  place,
		Range:  "r86400",
		Want:   20,
		Reason: "the " + ring + " ring earns a slot because it is the nearest ground.",
	}
}

func TestAWorldwideQueryNamingAPlaceIsDropped(t *testing.T) {
	queries, rejected := compileQueries([]modelQuery{
		wellFormed("planet", settings.RingWorldwide, "Berlin"),
	}, testContext())

	if len(queries) != 0 {
		t.Fatalf("a worldwide query naming a place survived: %+v", queries)
	}
	if len(rejected) != 1 || !strings.Contains(rejected[0].Reason, "carries no place") {
		t.Fatalf("rejection = %v", rejected)
	}
}

func TestAPlacedRingWithoutAPlaceIsDropped(t *testing.T) {
	for _, ring := range []string{settings.RingCity, settings.RingCountry, settings.RingRegion} {
		queries, rejected := compileQueries([]modelQuery{wellFormed(ring+"-run", ring, "")},
			testContext())
		if len(queries) != 0 {
			t.Fatalf("the %s ring survived with no place: %+v", ring, queries)
		}
		if len(rejected) != 1 || !strings.Contains(rejected[0].Reason, "names no place") {
			t.Fatalf("rejection for %s = %v", ring, rejected)
		}
	}
}

func TestAWorldwideQueryCarryingNoPlaceSurvives(t *testing.T) {
	queries, rejected := compileQueries([]modelQuery{
		wellFormed("planet", settings.RingWorldwide, ""),
	}, testContext())

	if len(rejected) != 0 {
		t.Fatalf("a well formed worldwide query was rejected: %v", rejected)
	}
	if len(queries) != 1 || queries[0].Place != "" || queries[0].Ring != settings.RingWorldwide {
		t.Fatalf("query = %+v", queries)
	}
}

func TestABadQueryIsDroppedAndTheWellFormedOnesSurvive(t *testing.T) {
	queries, rejected := compileQueries([]modelQuery{
		wellFormed("home-city", settings.RingCity, "Istanbul"),
		wellFormed("planet", settings.RingWorldwide, "Berlin"),
		wellFormed("nowhere", settings.RingCity, ""),
		wellFormed("country-wide", settings.RingCountry, "Turkiye"),
	}, testContext())

	if len(queries) != 2 {
		t.Fatalf("survivors = %d, want 2: %+v", len(queries), queries)
	}
	if queries[0].Label != "home-city" || queries[1].Label != "country-wide" {
		t.Fatalf("survivors = %s, %s", queries[0].Label, queries[1].Label)
	}
	if len(rejected) != 2 {
		t.Fatalf("rejections = %v", rejected)
	}
}

func TestARingTheExecutorDoesNotWorkIsDropped(t *testing.T) {
	context := testContext()
	context.capabilities.Fetch.Rings = []string{settings.RingCity, settings.RingWorldwide}

	queries, rejected := compileQueries([]modelQuery{
		wellFormed("region-run", settings.RingRegion, "Europe"),
	}, context)
	if len(queries) != 0 {
		t.Fatalf("an unavailable ring survived: %+v", queries)
	}
	if len(rejected) != 1 || !strings.Contains(rejected[0].Reason, "the executor only works") {
		t.Fatalf("rejection = %v", rejected)
	}
}

func TestDuplicateLabelsAreDropped(t *testing.T) {
	queries, rejected := compileQueries([]modelQuery{
		wellFormed("same", settings.RingCity, "Istanbul"),
		wellFormed("same", settings.RingCountry, "Turkiye"),
	}, testContext())

	if len(queries) != 1 {
		t.Fatalf("survivors = %+v", queries)
	}
	if len(rejected) != 1 || !strings.Contains(rejected[0].Reason, "duplicate query label") {
		t.Fatalf("rejection = %v", rejected)
	}
}

func TestTheQuerySlateIsCutToTheConfiguredCeiling(t *testing.T) {
	context := testContext()
	context.maxQueries = 2

	raw := []modelQuery{
		wellFormed("one", settings.RingCity, "Istanbul"),
		wellFormed("two", settings.RingCountry, "Turkiye"),
		wellFormed("three", settings.RingWorldwide, ""),
	}
	queries, _ := compileQueries(raw, context)
	if len(queries) != 2 {
		t.Fatalf("kept %d queries, want 2", len(queries))
	}
}

func TestWantIsClampedToWhatOneQueryCanReturn(t *testing.T) {
	context := testContext()
	context.maxWant = 30

	greedy := wellFormed("greedy", settings.RingCity, "Istanbul")
	greedy.Want = 900

	query, err := compileQuery(greedy, "query-1", context, map[string]struct{}{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if query.Want != 30 {
		t.Fatalf("want = %d, want 30", query.Want)
	}
}

func TestAnUnknownRangeFallsBackToTheWidestTheExecutorOffers(t *testing.T) {
	odd := wellFormed("odd", settings.RingCity, "Istanbul")
	odd.Range = "r99"

	query, err := compileQuery(odd, "query-1", testContext(), map[string]struct{}{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if query.Range != plugin.KnownRanges[len(plugin.KnownRanges)-1] {
		t.Fatalf("range = %q", query.Range)
	}
}

func TestAKnobTheExecutorLacksIsNotCarried(t *testing.T) {
	context := testContext()
	context.capabilities.Fetch.EasyApply = false
	context.capabilities.Fetch.RemoteOnly = false

	asked := wellFormed("asked", settings.RingCity, "Istanbul")
	asked.EasyApply = true
	asked.RemoteOnly = true

	query, err := compileQuery(asked, "query-1", context, map[string]struct{}{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if query.EasyApply || query.RemoteOnly {
		t.Fatalf("query carried a knob the executor lacks: %+v", query)
	}
}

func TestTheScreenBudgetIsClampedToTheSetting(t *testing.T) {
	context := testContext()
	context.screenBudget = 12

	cases := []struct {
		name   string
		asked  int
		wanted int
	}{
		{"over the ceiling", 400, 12},
		{"under the ceiling", 5, 5},
		{"absent", 0, 12},
		{"below the floor", -3, 1},
	}
	for _, entry := range cases {
		t.Run(entry.name, func(t *testing.T) {
			got := clamp(entry.asked, 1, context.screenBudget, context.screenBudget)
			if got != entry.wanted {
				t.Fatalf("screen budget = %d, want %d", got, entry.wanted)
			}
		})
	}
}

func TestARejectedQueryIsKeptSoTheNextRoundReadsIt(t *testing.T) {
	context := testContext()
	context.capabilities.Fetch.Keyword = true
	ringless := wellFormed("europe-fullstack-week", "", "Europe")
	ringless.Keyword = "senior full stack engineer"

	queries, rejected := compileQueries([]modelQuery{ringless}, context)
	if len(queries) != 0 || len(rejected) != 1 {
		t.Fatalf("queries = %d, rejected = %d", len(queries), len(rejected))
	}
	if !strings.Contains(rejected[0].Reason, "unnamed") {
		t.Fatalf("reason = %q", rejected[0].Reason)
	}

	record := rejected[0].Record
	if record.Label != "europe-fullstack-week" {
		t.Fatalf("label = %q, the next round has to recognise its own query", record.Label)
	}
	if record.Place != "Europe" || record.Keyword != "senior full stack engineer" {
		t.Fatalf("record lost what the planner wrote: %+v", record)
	}
	if !strings.HasPrefix(RejectedBeforeRun, "rejected before it ran") {
		t.Fatalf("the reason must say the query was rejected, not that it never got a turn")
	}
}

func TestTwoQueriesRejectedUnderOneLabelStayApart(t *testing.T) {
	context := testContext()
	first := wellFormed("europe", "", "Europe")
	second := wellFormed("europe", "", "Europe")

	queries, rejected := compileQueries([]modelQuery{first, second}, context)
	if len(queries) != 0 || len(rejected) != 2 {
		t.Fatalf("queries = %d, rejected = %d", len(queries), len(rejected))
	}
	if rejected[0].Record.Label == rejected[1].Record.Label {
		t.Fatalf("both records claim the label %q, and the round stores one row per label",
			rejected[0].Record.Label)
	}
}
