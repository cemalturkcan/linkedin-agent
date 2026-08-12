package indexer

import (
	"testing"
	"time"
)

const testMoment = "2026-08-11T00:00:00Z"

func at(t *testing.T) time.Time {
	t.Helper()
	moment, err := time.Parse(time.RFC3339, testMoment)
	if err != nil {
		t.Fatalf("parse %s: %v", testMoment, err)
	}
	return moment
}

func TestOneNarrowedVariantNeverShortensTheCareer(t *testing.T) {
	profiles := make([]ResumeProfile, 0, 13)
	for range 12 {
		profiles = append(profiles, ResumeProfile{
			YearsClaimed:     5,
			SeniorityClaimed: "senior",
			EarliestStart:    "2021-11",
		})
	}
	profiles = append(profiles, ResumeProfile{
		YearsClaimed:     3,
		SeniorityClaimed: "unknown",
		EarliestStart:    "2023-03",
	})

	if got := EvidencedYears(profiles, at(t), 3); got != 5 {
		t.Fatalf("years = %d, want 5: the floor across variants was the original bug", got)
	}
	if got := EvidencedBand(profiles, "mid"); got != "senior" {
		t.Fatalf("band = %q, want senior", got)
	}
}

func TestTheOrderOfTheVariantsNeverMovesTheYears(t *testing.T) {
	forward := []ResumeProfile{{YearsClaimed: 2}, {YearsClaimed: 7}, {YearsClaimed: 5}}
	backward := []ResumeProfile{{YearsClaimed: 5}, {YearsClaimed: 7}, {YearsClaimed: 2}}
	now := at(t)
	if EvidencedYears(forward, now, 0) != EvidencedYears(backward, now, 0) {
		t.Fatal("the same variants in another order produced another figure")
	}
}

func TestTheEarliestStartAnswersOnlyWhenNoVariantStatesATotal(t *testing.T) {
	now := at(t)
	profiles := []ResumeProfile{
		{EarliestStart: "2020-03"},
		{EarliestStart: "2018-09"},
		{EarliestStart: ""},
	}
	if got := EvidencedYears(profiles, now, 0); got != 7 {
		t.Fatalf("years = %d, want the span from 2018-09", got)
	}

	profiles = append(profiles, ResumeProfile{YearsClaimed: 9})
	if got := EvidencedYears(profiles, now, 0); got != 9 {
		t.Fatalf("years = %d, want the stated total to win", got)
	}
}

func TestSilentVariantsLeaveTheModelsOwnFigure(t *testing.T) {
	if got := EvidencedYears([]ResumeProfile{{}}, at(t), 4); got != 4 {
		t.Fatalf("years = %d", got)
	}
}

func TestTheHighestBandWinsAndUnknownCarriesNoWeight(t *testing.T) {
	profiles := []ResumeProfile{
		{SeniorityClaimed: "unknown"},
		{SeniorityClaimed: "mid"},
		{SeniorityClaimed: "senior"},
		{SeniorityClaimed: "unknown"},
	}
	if got := EvidencedBand(profiles, "junior"); got != "senior" {
		t.Fatalf("band = %q, want senior", got)
	}
	if got := EvidencedBand([]ResumeProfile{{SeniorityClaimed: "unknown"}}, "mid"); got != "mid" {
		t.Fatalf("band = %q, want the model's own answer when every variant is silent", got)
	}
}

func TestTheShapedCandidateIgnoresASmallerAnswerFromTheModel(t *testing.T) {
	profiles := []ResumeProfile{{YearsClaimed: 6, SeniorityClaimed: "senior"}, {YearsClaimed: 3}}
	shaped := shapeCandidate(
		Candidate{YearsExperience: 3, SeniorityBand: "mid", Headline: "  a  headline "},
		profiles,
		at(t),
	)
	if shaped.YearsExperience != 6 {
		t.Fatalf("yearsExperience = %d", shaped.YearsExperience)
	}
	if shaped.SeniorityBand != "senior" {
		t.Fatalf("seniorityBand = %q", shaped.SeniorityBand)
	}
	if shaped.Headline != "a headline" {
		t.Fatalf("headline = %q", shaped.Headline)
	}
}

func TestPlacesOutsideTheCityAndCountryRingsAreDropped(t *testing.T) {
	shaped := shapeResumeProfile(
		ResumeProfile{
			Places: []Place{
				{Name: "Istanbul", Ring: "city"},
				{Name: "Europe", Ring: "region"},
				{Name: "", Ring: "country"},
				{Name: "Turkiye", Ring: "COUNTRY"},
				{Name: "istanbul", Ring: "city"},
			},
			SeniorityClaimed: "wizard",
			YearsClaimed:     -4,
			EarliestStart:    "2018-13",
		},
		"GO",
		"Go",
		at(t),
	)
	if len(shaped.Places) != 2 {
		t.Fatalf("places = %+v", shaped.Places)
	}
	if shaped.Places[1].Ring != "country" {
		t.Fatalf("ring = %q", shaped.Places[1].Ring)
	}
	if shaped.SeniorityClaimed != "unknown" {
		t.Fatalf("seniorityClaimed = %q", shaped.SeniorityClaimed)
	}
	if shaped.YearsClaimed != 0 {
		t.Fatalf("yearsClaimed = %d", shaped.YearsClaimed)
	}
	if shaped.EarliestStart != "" {
		t.Fatalf("earliestStart = %q", shaped.EarliestStart)
	}
	if shaped.TargetRole != "Go" {
		t.Fatalf("targetRole = %q", shaped.TargetRole)
	}
}
