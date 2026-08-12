package settings

import (
	"encoding/json"
	"slices"
	"testing"
)

func decode(t *testing.T, body string) map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return raw
}

func TestTheShippedDefaultsAreWideAndCarryNothingPersonal(t *testing.T) {
	current := Normalize(map[string]any{})

	workplace := current.Locations.Workplace
	if !workplace.Onsite || !workplace.Hybrid || !workplace.Remote {
		t.Fatalf("workplace = %+v", workplace)
	}
	if workplace.Scope != "global" {
		t.Fatalf("scope = %q", workplace.Scope)
	}
	if !current.Locations.Relocation.Open {
		t.Fatal("relocation is closed by default")
	}
	if len(current.Locations.Places) != 0 {
		t.Fatalf("places = %+v", current.Locations.Places)
	}
	if current.Locations.Authorization != "" {
		t.Fatalf("authorization = %q", current.Locations.Authorization)
	}
	if len(current.Roles.ExcludeStacks) != 0 || len(current.Roles.ExcludeIndustries) != 0 {
		t.Fatalf("roles ship exclusions: %+v", current.Roles)
	}
	if len(current.Apply.ResumeLanguages) != 0 {
		t.Fatalf("resume languages = %v", current.Apply.ResumeLanguages)
	}
	if current.Roles.MinCompensation.Amount != 0 || current.Roles.MinCompensation.Currency != "" {
		t.Fatalf("compensation = %+v", current.Roles.MinCompensation)
	}
	if current.Roles.Seniority.Min != "intern" || current.Roles.Seniority.Max != "principal" {
		t.Fatalf("seniority = %+v", current.Roles.Seniority)
	}
	if !slices.Equal(current.Roles.ContractTypes, ContractTypes) {
		t.Fatalf("contract types = %v", current.Roles.ContractTypes)
	}
}

func TestAnInvalidRingFallsBackRatherThanBeingStored(t *testing.T) {
	current := Normalize(decode(t, `{
		"locations": {"places": [
			{"name": "Istanbul", "ring": "galaxy", "kind": "commute"},
			{"name": "Berlin", "ring": "country", "kind": "orbit"}
		]}
	}`))

	if len(current.Locations.Places) != 2 {
		t.Fatalf("places = %+v", current.Locations.Places)
	}
	if current.Locations.Places[0].Ring != PlaceRings[0] {
		t.Fatalf("ring = %q, want the first known ring", current.Locations.Places[0].Ring)
	}
	if current.Locations.Places[1].Ring != "country" {
		t.Fatalf("ring = %q", current.Locations.Places[1].Ring)
	}
	if current.Locations.Places[1].Kind != LocationKinds[0] {
		t.Fatalf("kind = %q, want the first known kind", current.Locations.Places[1].Kind)
	}
}

func TestOutOfRangeNumbersAreClampedAndJunkFallsBack(t *testing.T) {
	current := Normalize(decode(t, `{
		"budget": {"maxQueriesPerCycle": 99, "cycleMinutes": 1, "dailyModelCallCap": "lots"},
		"harvest": {"requestTimeoutMs": 1}
	}`))

	if current.Budget.MaxQueriesPerCycle != 8 {
		t.Fatalf("maxQueriesPerCycle = %d", current.Budget.MaxQueriesPerCycle)
	}
	if current.Budget.CycleMinutes != 2 {
		t.Fatalf("cycleMinutes = %d", current.Budget.CycleMinutes)
	}
	if current.Budget.DailyModelCallCap != Defaults().Budget.DailyModelCallCap {
		t.Fatalf("dailyModelCallCap = %d", current.Budget.DailyModelCallCap)
	}
	if current.Harvest.RequestTimeoutMs != 2000 {
		t.Fatalf("requestTimeoutMs = %d", current.Harvest.RequestTimeoutMs)
	}
}

func TestSeniorityBoundsAreOrderedAndSnapToTheLadder(t *testing.T) {
	current := Normalize(decode(t, `{"roles": {"seniority": {"min": "staff", "max": "junior"}}}`))
	if current.Roles.Seniority.Min != "junior" || current.Roles.Seniority.Max != "staff" {
		t.Fatalf("seniority = %+v", current.Roles.Seniority)
	}

	current = Normalize(decode(t, `{"roles": {"seniority": {"min": "wizard", "max": "senior"}}}`))
	if current.Roles.Seniority.Min != "intern" || current.Roles.Seniority.Max != "senior" {
		t.Fatalf("seniority = %+v", current.Roles.Seniority)
	}
}

func TestMergeReplacesListsAndDescendsIntoGroups(t *testing.T) {
	base := decode(t, `{"locations": {"workplace": {"onsite": true, "remote": true}}, "budget": {"cycleMinutes": 10}}`)
	patch := decode(t, `{"locations": {"workplace": {"remote": false}}}`)

	merged := Merge(base, patch)
	current := Normalize(merged)
	if !current.Locations.Workplace.Onsite {
		t.Fatal("the merge dropped a sibling field")
	}
	if current.Locations.Workplace.Remote {
		t.Fatal("the merge did not apply the patch")
	}
	if current.Budget.CycleMinutes != 10 {
		t.Fatalf("cycleMinutes = %d, the merge dropped a sibling group", current.Budget.CycleMinutes)
	}
}

func TestUploadFileNameAlwaysEndsInPdf(t *testing.T) {
	current := Normalize(decode(t, `{"apply": {"uploadFileName": "../../etc/passwd"}}`))
	if current.Apply.UploadFileName != "....etcpasswd.pdf" {
		t.Fatalf("uploadFileName = %q", current.Apply.UploadFileName)
	}
}

func TestResumeLanguagesKeepOnlyTwoLetterCodes(t *testing.T) {
	current := Normalize(decode(t, `{"apply": {"resumeLanguages": ["EN", "tr", "deutsch", "tr", 7]}}`))
	if !slices.Equal(current.Apply.ResumeLanguages, []string{"en", "tr"}) {
		t.Fatalf("resumeLanguages = %v", current.Apply.ResumeLanguages)
	}
}
