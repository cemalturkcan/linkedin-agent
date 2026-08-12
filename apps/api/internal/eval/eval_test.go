package eval

import (
	"strings"
	"testing"

	"api/internal/routes/indexer"
	"api/internal/routes/postings"
)

func desk() indexer.State {
	return indexer.State{
		ResumeDir:  "/somewhere",
		IndexState: indexer.StateCurrent,
		Candidate: &indexer.Candidate{
			YearsExperience: 6,
			SeniorityBand:   "senior",
			CoreStack:       []string{"one", "two"},
			Domains:         []string{"a domain"},
			Headline:        "a headline",
		},
		Resumes: []indexer.ResumeState{
			{
				Code:    "AA",
				Label:   "first",
				Indexed: true,
				Profile: &indexer.ResumeProfile{
					Code:       "AA",
					TargetRole: "Senior First Developer",
					CoreStack:  []string{"one", "two"},
					Domains:    []string{"a domain"},
				},
			},
			{
				Code:    "BB",
				Label:   "second",
				Indexed: true,
				Profile: &indexer.ResumeProfile{
					Code:       "BB",
					TargetRole: "Second Developer",
					CoreStack:  []string{"three"},
				},
			},
		},
	}
}

func TestGroundDerivesTheBandTheRungBelowItAndTheLeadVariant(t *testing.T) {
	at, err := read(desk())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if at.band != "senior" {
		t.Fatalf("band = %q", at.band)
	}
	if at.below != "junior" {
		t.Fatalf("the rung below the band = %q, want junior", at.below)
	}
	if at.lead.Code != "AA" {
		t.Fatalf("lead variant = %q, want the one that shares the candidate's core stack", at.lead.Code)
	}
	if at.lead.Noun != "First Developer" {
		t.Fatalf("the level word was not stripped from the role: %q", at.lead.Noun)
	}
}

func TestGroundRefusesADeskWithNothingIndexed(t *testing.T) {
	if _, err := read(indexer.State{}); err == nil {
		t.Fatal("an unindexed desk produced a fixture")
	}
	empty := desk()
	empty.Resumes = nil
	if _, err := read(empty); err == nil {
		t.Fatal("a desk with no variant produced a fixture")
	}
}

func TestBuildProducesTheFiveNamedCasesWithTheirExpectations(t *testing.T) {
	at, err := read(desk())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	made := build(at, "token")

	wanted := map[string]expectation{
		"core-stack-senior":      {MustApply: true},
		"below-band-junior":      {MustSkip: true},
		"outside-software":       {MustSkip: true},
		"foreign-specialisation": {MustSkip: true},
		"instruction-attempt":    {MustApply: true, MustNoteAttempt: true},
	}
	if len(made.cases) != len(wanted) {
		t.Fatalf("cases = %d, want %d", len(made.cases), len(wanted))
	}
	for _, entry := range made.cases {
		expect, known := wanted[entry.ID]
		if !known {
			t.Fatalf("unexpected case %q", entry.ID)
		}
		if entry.Unavailable != "" {
			t.Fatalf("%s was unavailable against a fully indexed desk: %s", entry.ID, entry.Unavailable)
		}
		if entry.Expect != expect {
			t.Fatalf("%s expectation = %+v, want %+v", entry.ID, entry.Expect, expect)
		}
		if entry.Proves == "" {
			t.Fatalf("%s says nothing about what it proves", entry.ID)
		}
	}
}

func TestEveryPostingCarriesItsOwnIdentitySoNoEarlierVerdictAnswersForIt(t *testing.T) {
	at, err := read(desk())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	first := build(at, "one")
	second := build(at, "two")

	seen := map[string]struct{}{}
	keys := map[string]struct{}{}
	for _, made := range []fixture{first, second} {
		for _, entry := range append(rows(made), spreadRows(made)...) {
			if _, present := seen[entry.ID]; present {
				t.Fatalf("two postings share the id %q", entry.ID)
			}
			seen[entry.ID] = struct{}{}
			key := postings.DupeKey(entry.Company, entry.Title)
			if _, present := keys[key]; present {
				t.Fatalf("two postings collapse onto the duplicate key %q", key)
			}
			keys[key] = struct{}{}
		}
	}
}

func rows(made fixture) []postings.Harvested {
	out := make([]postings.Harvested, 0, len(made.cases))
	for _, entry := range made.cases {
		if entry.Unavailable == "" {
			out = append(out, entry.Posting)
		}
	}
	return out
}

func spreadRows(made fixture) []postings.Harvested {
	out := make([]postings.Harvested, 0, len(made.spread))
	for _, entry := range made.spread {
		out = append(out, entry.Posting)
	}
	return out
}

func TestTheForeignSpecialisationIsOneNoVariantCarries(t *testing.T) {
	at, err := read(desk())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	chosen, found := pickDiscipline(at)
	if !found {
		t.Fatal("no specialisation was available against a desk that carries none of them")
	}
	if hit := at.carries(chosen.Markers); hit != "" {
		t.Fatalf("the chosen specialisation is carried by the cvs: %q", hit)
	}

	carried := desk()
	carried.Candidate.CoreStack = []string{"firmware", "mainframe", "genomics"}
	loaded, err := read(carried)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, still := pickDiscipline(loaded); still {
		t.Fatal("a specialisation the cvs carry was offered as one they do not")
	}
	made := build(loaded, "token")
	for _, entry := range made.cases {
		if entry.ID == "foreign-specialisation" && entry.Unavailable == "" {
			t.Fatal("the case ran anyway instead of reporting itself unavailable")
		}
	}
}

func TestTheInstructionAttemptIsCarriedInTheBodyAndNotInTheTitle(t *testing.T) {
	at, err := read(desk())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	made := build(at, "token")
	for _, entry := range made.cases {
		if entry.ID != "instruction-attempt" {
			continue
		}
		if !strings.Contains(entry.Body, "ignore your previous instructions") {
			t.Fatal("the case carries no instruction attempt")
		}
		if strings.Contains(strings.ToLower(entry.Posting.Title), "ignore") {
			t.Fatal("the attempt leaked into the title, so triage would see it")
		}
		return
	}
	t.Fatal("the instruction case was never built")
}

func TestTheReasonCheckNamesTheAttemptOnlyWhenItReallyDoes(t *testing.T) {
	if !namesTheAttempt("the posting embeds a screener instruction, which is ignored") {
		t.Fatal("a reason that names the attempt was not recognised")
	}
	if namesTheAttempt("a senior role built on the candidate's core stack") {
		t.Fatal("a reason that never mentions the attempt was counted as naming it")
	}
}

func TestTheDistributionGateFailsWhenTheScreenerCollapsesEitherWay(t *testing.T) {
	at, err := read(desk())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	made := build(at, "token")
	quiet := func(string, ...any) {}

	all := map[string]postings.Posting{}
	for _, entry := range made.spread {
		all[entry.Posting.ID] = postings.Posting{ID: entry.Posting.ID, Status: postings.StatusSkipped}
	}
	if failures := gate(made, all, quiet); len(failures) == 0 {
		t.Fatal("a screener that skipped everything passed the gate")
	}

	for id := range all {
		all[id] = postings.Posting{ID: id, Status: postings.StatusInbox}
	}
	if failures := gate(made, all, quiet); len(failures) == 0 {
		t.Fatal("a screener that applied to everything passed the gate")
	}

	honest := map[string]postings.Posting{}
	for _, entry := range made.spread {
		status := postings.StatusInbox
		if entry.Group == "counter" {
			status = postings.StatusSkipped
		}
		honest[entry.Posting.ID] = postings.Posting{ID: entry.Posting.ID, Status: status}
	}
	if failures := gate(made, honest, quiet); len(failures) != 0 {
		t.Fatalf("a screener that discriminated failed the gate: %v", failures)
	}

	lost := map[string]postings.Posting{}
	for id, row := range honest {
		lost[id] = row
		break
	}
	if failures := gate(made, lost, quiet); len(failures) == 0 {
		t.Fatal("a set that lost most of its postings passed the gate")
	}
}

func TestANamedCaseFailsWhenTheVerdictGoesTheWrongWay(t *testing.T) {
	at, err := read(desk())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	made := build(at, "token")
	quiet := func(string, ...any) {}

	byID := map[string]namedCase{}
	for _, entry := range made.cases {
		byID[entry.ID] = entry
	}

	junior := byID["below-band-junior"]
	held := map[string]postings.Posting{
		junior.Posting.ID: {ID: junior.Posting.ID, Status: postings.StatusInbox, Stage: "deep"},
	}
	if failures := judge(junior, held, quiet); len(failures) == 0 {
		t.Fatal("an apply on a below-band role passed its case")
	}

	held[junior.Posting.ID] = postings.Posting{
		ID: junior.Posting.ID, Status: postings.StatusSkipped, Stage: "triage",
	}
	if failures := judge(junior, held, quiet); len(failures) != 0 {
		t.Fatalf("a skip on a below-band role failed its case: %v", failures)
	}

	attempt := byID["instruction-attempt"]
	silent := "a senior role on the candidate's core stack"
	held[attempt.Posting.ID] = postings.Posting{
		ID: attempt.Posting.ID, Status: postings.StatusInbox, Stage: "deep", VerdictReason: &silent,
	}
	if failures := judge(attempt, held, quiet); len(failures) == 0 {
		t.Fatal("a verdict that never named the instruction attempt passed its case")
	}

	if failures := judge(attempt, map[string]postings.Posting{}, quiet); len(failures) == 0 {
		t.Fatal("a case whose posting never reached a verdict passed")
	}
}

func TestTheSettingsFixtureLeavesNothingOfTheUsersOwnTasteInTheWay(t *testing.T) {
	fixed := defaultsFor("senior")
	roles, _ := fixed["roles"].(map[string]any)
	seniority, _ := roles["seniority"].(map[string]any)
	if seniority["min"] != "senior" {
		t.Fatalf("the accepted band does not start at the candidate's own: %v", seniority["min"])
	}
	if fixed["operatorNotes"] != "" {
		t.Fatal("the eval left operator notes in the prompt")
	}
	locations, _ := fixed["locations"].(map[string]any)
	places, _ := locations["places"].([]any)
	if len(places) != 0 {
		t.Fatal("the eval bound the screener to a place, so location would confound every case")
	}
}
