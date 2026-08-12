package screening

import (
	"strings"
	"testing"

	"api/internal/routes/postings"
	"api/internal/routes/settings"
)

func baseSettings() settings.Settings {
	current := settings.Defaults()
	current.Apply.ResumeLanguages = []string{"en", "tr"}
	return current
}

func posting(company, location string, easyApply bool) Candidate {
	return Candidate{
		ID:        "1",
		Title:     "Senior Go Engineer",
		Company:   company,
		Location:  location,
		EasyApply: easyApply,
	}
}

func applying() Verdict {
	return Verdict{
		Verdict:   VerdictApply,
		Reason:    "the core work is Go services.",
		Score:     80,
		Workplace: "onsite",
	}
}

func TestABlockedCompanyNeverPasses(t *testing.T) {
	current := baseSettings()
	current.Companies.Blocked = []string{"Acme GmbH"}
	rules := NewRules(current, []string{"en"})

	gate := hardGate(posting("ACME Gmbh.", "Berlin", true), rules, map[string]Engaged{})
	if gate == nil {
		t.Fatal("the blocked company was let through")
	}
	if gate.Rule != RuleBlockedCompany || gate.Status != postings.StatusSkipped {
		t.Fatalf("gate = %+v", gate)
	}
	if !strings.Contains(gate.Reason, "blocked company list") {
		t.Fatalf("reason = %q", gate.Reason)
	}
}

func TestACompanyInsideTheCooldownNeverPasses(t *testing.T) {
	current := baseSettings()
	current.Companies.ReapplyCooldownDays = 90
	rules := NewRules(current, []string{"en"})
	engaged := map[string]Engaged{
		postings.CompanyKey("Acme GmbH"): {Company: "Acme GmbH", At: "2026-07-01T09:00:00Z"},
	}

	gate := hardGate(posting("Acme GmbH", "Berlin", true), rules, engaged)
	if gate == nil {
		t.Fatal("the cooldown did not fire")
	}
	if gate.Rule != RuleCooldown {
		t.Fatalf("gate = %+v", gate)
	}
	if !strings.Contains(gate.Reason, "2026-07-01") || !strings.Contains(gate.Reason, "90 day") {
		t.Fatalf("reason = %q", gate.Reason)
	}
}

func TestACompanyTheHandOpenedNeverPasses(t *testing.T) {
	current := baseSettings()
	current.Companies.ReapplyCooldownDays = 90
	rules := NewRules(current, []string{"en"})
	engaged := map[string]Engaged{
		postings.CompanyKey("Acme GmbH"): {
			Company: "Acme GmbH",
			At:      "2026-07-01T09:00:00Z",
			ByHand:  true,
		},
	}

	gate := hardGate(posting("Acme GmbH", "Berlin", true), rules, engaged)
	if gate == nil || gate.Rule != RuleCooldown {
		t.Fatalf("gate = %+v", gate)
	}
	if !strings.Contains(gate.Reason, "you opened a posting at Acme GmbH") {
		t.Fatalf("reason = %q", gate.Reason)
	}
}

func TestAnAgencyIsDroppedOnlyWhenAgenciesAreExcluded(t *testing.T) {
	current := baseSettings()
	current.Companies.ExcludeAgencies = true
	rules := NewRules(current, []string{"en"})

	decision := applying()
	decision.Agency = true
	routed := routeApply(posting("Talent Partners", "Berlin", true), decision, rules)
	if routed.Rule != RuleAgency || routed.Status != postings.StatusSkipped {
		t.Fatalf("routed = %+v", routed)
	}

	current.Companies.ExcludeAgencies = false
	relaxed := NewRules(current, []string{"en"})
	if routed := routeApply(posting("Talent Partners", "Berlin", true), decision, relaxed); routed.Status != postings.StatusInbox {
		t.Fatalf("routed = %+v", routed)
	}
}

func TestPayUnderTheFloorIsDropped(t *testing.T) {
	current := baseSettings()
	current.Roles.MinCompensation = settings.Compensation{
		Amount:     60000,
		Currency:   "EUR",
		Period:     "year",
		HardFilter: true,
	}
	rules := NewRules(current, []string{"en"})

	decision := applying()
	decision.StatedPay = &Pay{Amount: 3000, Currency: "EUR", Period: "month"}
	routed := routeApply(posting("Acme", "Berlin", true), decision, rules)
	if routed.Rule != RulePayFloor || routed.Status != postings.StatusSkipped {
		t.Fatalf("routed = %+v", routed)
	}
	if !strings.Contains(routed.Reason, "EUR 60000 per year floor") {
		t.Fatalf("reason = %q", routed.Reason)
	}

	decision.StatedPay = &Pay{Amount: 6000, Currency: "EUR", Period: "month"}
	if routed := routeApply(posting("Acme", "Berlin", true), decision, rules); routed.Status != postings.StatusInbox {
		t.Fatalf("a salary above the floor was dropped: %+v", routed)
	}
}

func TestAPostingNamingNoPayIsNeverDroppedOnPay(t *testing.T) {
	current := baseSettings()
	current.Roles.MinCompensation = settings.Compensation{
		Amount: 60000, Currency: "EUR", Period: "year", HardFilter: true,
	}
	rules := NewRules(current, []string{"en"})

	if routed := routeApply(posting("Acme", "Berlin", true), applying(), rules); routed.Status != postings.StatusInbox {
		t.Fatalf("routed = %+v", routed)
	}
}

func placedSettings() settings.Settings {
	current := baseSettings()
	current.Locations.Places = []settings.Place{
		{Name: "Istanbul", Ring: settings.RingCity, Kind: KindCommute},
		{Name: "Berlin", Ring: settings.RingCity, Kind: KindRelocate},
	}
	current.Locations.Relocation = settings.Relocation{Open: true, Targets: []string{"Amsterdam"}}
	current.Locations.Workplace.Scope = "country"
	current.Locations.Authorization = "Turkish citizen, no EU work permit"
	return current
}

func TestAPostingOutsideEveryPlaceIsDropped(t *testing.T) {
	rules := NewRules(placedSettings(), []string{"en"})

	routed := routeApply(posting("Acme", "Warsaw, Poland", true), applying(), rules)
	if routed.Rule != RuleLocation || routed.Status != postings.StatusSkipped {
		t.Fatalf("routed = %+v", routed)
	}
	if !strings.Contains(routed.Reason, "outside every place on the list") {
		t.Fatalf("reason = %q", routed.Reason)
	}
	if !strings.Contains(routed.Reason, "no EU work permit") {
		t.Fatalf("the reason never named the work authorization: %q", routed.Reason)
	}
}

func TestAConfiguredPlaceAndARelocationTargetBothSurviveAndSayWhy(t *testing.T) {
	rules := NewRules(placedSettings(), []string{"en"})

	inside := routeApply(posting("Acme", "Istanbul, Türkiye", true), applying(), rules)
	if inside.Status != postings.StatusInbox {
		t.Fatalf("routed = %+v", inside)
	}

	moving := routeApply(posting("Acme", "Berlin, Germany", true), applying(), rules)
	if moving.Status != postings.StatusInbox {
		t.Fatalf("routed = %+v", moving)
	}
	if !strings.Contains(moving.Reason, "moving to Berlin") {
		t.Fatalf("the reason never said it depends on moving: %q", moving.Reason)
	}

	target := routeApply(posting("Acme", "Amsterdam, Netherlands", true), applying(), rules)
	if target.Status != postings.StatusInbox {
		t.Fatalf("routed = %+v", target)
	}
	if !strings.Contains(target.Reason, "relocating to Amsterdam") {
		t.Fatalf("the reason never said it depends on relocating: %q", target.Reason)
	}
}

func TestRemoteSurvivesOutsideEveryPlaceOnlyWithGlobalScope(t *testing.T) {
	narrow := NewRules(placedSettings(), []string{"en"})
	remote := applying()
	remote.Workplace = "remote"

	if routed := routeApply(posting("Acme", "Warsaw, Poland", true), remote, narrow); routed.Status != postings.StatusSkipped {
		t.Fatalf("a country scope accepted a foreign remote posting: %+v", routed)
	}

	current := placedSettings()
	current.Locations.Workplace.Scope = ScopeGlobal
	wide := NewRules(current, []string{"en"})
	routed := routeApply(posting("Acme", "Warsaw, Poland", true), remote, wide)
	if routed.Status != postings.StatusInbox {
		t.Fatalf("routed = %+v", routed)
	}
	if !strings.Contains(routed.Reason, "remote work rather than by place") {
		t.Fatalf("reason = %q", routed.Reason)
	}
}

func TestAWorkplaceThisSearchRefusesIsDropped(t *testing.T) {
	current := baseSettings()
	current.Locations.Workplace.Onsite = false
	rules := NewRules(current, []string{"en"})

	routed := routeApply(posting("Acme", "Berlin", true), applying(), rules)
	if routed.Rule != RuleLocation || routed.Status != postings.StatusSkipped {
		t.Fatalf("routed = %+v", routed)
	}
}

func TestNonEasyApplyGoesToManualWhenTheSettingIsOnAndSkippedWhenItIsOff(t *testing.T) {
	current := baseSettings()
	current.Roles.ApplyToNonEasyApply = true
	on := NewRules(current, []string{"en"})

	manual := routeApply(posting("Acme", "Berlin", false), applying(), on)
	if manual.Status != postings.StatusManual || manual.Rule != RuleNonEasyApply {
		t.Fatalf("routed = %+v", manual)
	}
	if !strings.Contains(manual.Reason, "manual send") {
		t.Fatalf("reason = %q", manual.Reason)
	}

	current.Roles.ApplyToNonEasyApply = false
	off := NewRules(current, []string{"en"})
	skipped := routeApply(posting("Acme", "Berlin", false), applying(), off)
	if skipped.Status != postings.StatusSkipped || skipped.Rule != RuleNonEasyApply {
		t.Fatalf("routed = %+v", skipped)
	}
	if !strings.Contains(skipped.Reason, "turned off") {
		t.Fatalf("reason = %q", skipped.Reason)
	}
}

func TestASkipVerdictNeverReachesTheApplyRules(t *testing.T) {
	rules := NewRules(baseSettings(), []string{"en"})
	decision := applying()
	decision.Verdict = VerdictSkip
	decision.Reason = "the stack is a different profession."

	routed := route(posting("Acme", "Berlin", false), decision, rules)
	if routed.Status != postings.StatusSkipped {
		t.Fatalf("routed = %+v", routed)
	}
	if strings.Contains(routed.Reason, "manual send") {
		t.Fatalf("a skip was routed through the easy apply rule: %q", routed.Reason)
	}
}

func TestTheResumeLanguageFallsBackToWhatTheVariantHasOnDisk(t *testing.T) {
	rules := NewRules(baseSettings(), []string{"en", "tr"})
	if len(rules.Languages) != 2 {
		t.Fatalf("languages = %v", rules.Languages)
	}
	empty := baseSettings()
	empty.Apply.ResumeLanguages = nil
	derived := NewRules(empty, []string{"tr", "en"})
	if strings.Join(derived.Languages, ",") != "en,tr" {
		t.Fatalf("languages = %v", derived.Languages)
	}
}

func TestAPostingWrittenInALanguageThePersonDoesNotWorkInIsSkipped(t *testing.T) {
	current := baseSettings()
	current.Roles.PostingLanguages = []string{"en", "tr"}
	rules := NewRules(current, []string{"en"})

	written := applying()
	written.PostingLang = "es"
	spanish := routeApply(posting("Acme", "Madrid, Spain", true), written, rules)
	if spanish.Status != postings.StatusSkipped || spanish.Rule != RulePostingLang {
		t.Fatalf("status = %q rule = %q, want a language skip", spanish.Status, spanish.Rule)
	}
	if !strings.Contains(spanish.Reason, "written in es") ||
		!strings.Contains(spanish.Reason, "en, tr") {
		t.Fatalf("reason = %q, it has to name both languages", spanish.Reason)
	}

	for _, language := range []string{"en", "TR", "", "unclear"} {
		decision := applying()
		decision.PostingLang = language
		kept := routeApply(posting("Acme", "Madrid, Spain", true), decision, rules)
		if kept.Status != postings.StatusInbox {
			t.Fatalf("postingLang %q routed to %q, want the inbox", language, kept.Status)
		}
	}
}

func TestNoLanguageConfiguredDropsNothingOnLanguage(t *testing.T) {
	current := baseSettings()
	current.Roles.PostingLanguages = nil
	rules := NewRules(current, []string{"en"})

	decision := applying()
	decision.PostingLang = "es"
	kept := routeApply(posting("Acme", "Madrid, Spain", true), decision, rules)
	if kept.Status != postings.StatusInbox {
		t.Fatalf("status = %q, want the inbox", kept.Status)
	}
}
