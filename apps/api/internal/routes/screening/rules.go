package screening

import (
	"slices"
	"sort"
	"strings"

	"api/internal/routes/postings"
	"api/internal/routes/settings"
)

const (
	RuleBlockedCompany = "blocked-company"
	RuleCooldown       = "reapply-cooldown"
	RuleAgency         = "agency"
	RulePayFloor       = "pay-floor"
	RuleLocation       = "location"
	RuleNonEasyApply   = "non-easy-apply"
	RulePostingLang    = "posting-language"

	KindCommute     = "commute"
	KindRelocate    = "relocate"
	KindRemoteScope = "remote-scope"

	ScopeGlobal = "global"
)

type Engaged struct {
	Company string
	At      string
	ByHand  bool
}

type Rules struct {
	Settings  settings.Settings
	Accepted  []string
	Rejected  []string
	Blocked   map[string]string
	Preferred map[string]string
	Languages []string
}

type Routed struct {
	Status string
	Reason string
	Rule   string
}

func NewRules(current settings.Settings, fileLanguages []string) Rules {
	accepted, rejected := settings.SeniorityRange(current.Roles.Seniority)
	languages := current.Apply.ResumeLanguages
	if len(languages) == 0 {
		languages = append([]string{}, fileLanguages...)
		sort.Strings(languages)
	}
	if len(languages) == 0 {
		languages = []string{"en"}
	}
	return Rules{
		Settings:  current,
		Accepted:  accepted,
		Rejected:  rejected,
		Blocked:   companyKeys(current.Companies.Blocked),
		Preferred: companyKeys(current.Companies.Preferred),
		Languages: languages,
	}
}

func companyKeys(names []string) map[string]string {
	keyed := make(map[string]string, len(names))
	for _, name := range names {
		if key := postings.CompanyKey(name); key != "" {
			keyed[key] = name
		}
	}
	return keyed
}

func hardGate(posting Candidate, rules Rules, engaged map[string]Engaged) *Routed {
	key := postings.CompanyKey(posting.Company)
	if key == "" {
		return nil
	}
	if blocked, present := rules.Blocked[key]; present {
		return &Routed{
			Status: postings.StatusSkipped,
			Reason: blocked + " is on the blocked company list.",
			Rule:   RuleBlockedCompany,
		}
	}
	if recent, present := engaged[key]; present {
		what := "already applied to "
		if recent.ByHand {
			what = "you opened a posting at "
		}
		return &Routed{
			Status: postings.StatusSkipped,
			Reason: what + recent.Company + " on " + day(recent.At) +
				", inside the " + itoa(rules.Settings.Companies.ReapplyCooldownDays) +
				" day reapply cooldown.",
			Rule: RuleCooldown,
		}
	}
	return nil
}

func day(stamp string) string {
	if len(stamp) >= 10 {
		return stamp[:10]
	}
	return "an earlier round"
}

var yearly = map[string]int{"month": 12, "year": 1}

type below struct {
	Posted int
	Wanted int
}

func payBelowFloor(stated *Pay, floor settings.Compensation) *below {
	if !floor.HardFilter || floor.Amount <= 0 || floor.Currency == "" || floor.Period == "" {
		return nil
	}
	if stated == nil || stated.Amount <= 0 {
		return nil
	}
	if !strings.EqualFold(stated.Currency, floor.Currency) {
		return nil
	}
	postedFactor, postedKnown := yearly[stated.Period]
	floorFactor, floorKnown := yearly[floor.Period]
	if !postedKnown || !floorKnown {
		return nil
	}
	posted := stated.Amount * postedFactor
	wanted := floor.Amount * floorFactor
	if posted >= wanted {
		return nil
	}
	return &below{Posted: posted, Wanted: wanted}
}

func matchPlace(location string, places []settings.Place) (settings.Place, bool) {
	anchored := postings.PlaceKey(location)
	if anchored == "" {
		return settings.Place{}, false
	}
	for _, place := range places {
		key := postings.PlaceKey(place.Name)
		if key == "" {
			continue
		}
		if strings.Contains(anchored, key) || strings.Contains(key, anchored) {
			return place, true
		}
	}
	return settings.Place{}, false
}

func matchTarget(location string, targets []string) (string, bool) {
	anchored := postings.PlaceKey(location)
	if anchored == "" {
		return "", false
	}
	for _, target := range targets {
		key := postings.PlaceKey(target)
		if key != "" && (strings.Contains(anchored, key) || strings.Contains(key, anchored)) {
			return target, true
		}
	}
	return "", false
}

func workplaceAccepted(mode string, accepted settings.Workplace) bool {
	switch mode {
	case "onsite":
		return accepted.Onsite
	case "hybrid":
		return accepted.Hybrid
	case "remote":
		return accepted.Remote
	}
	return true
}

func routeLocation(posting Candidate, decision Verdict, rules Rules) (string, *Routed) {
	locations := rules.Settings.Locations
	if !workplaceAccepted(decision.Workplace, locations.Workplace) {
		return "", &Routed{
			Status: postings.StatusSkipped,
			Reason: decision.Workplace + " work is not one of the workplaces this search accepts.",
			Rule:   RuleLocation,
		}
	}

	where := line(posting.Location, 120)
	if where == "" {
		where = "an unnamed place"
	}
	if len(locations.Places) == 0 {
		return "", nil
	}

	if place, found := matchPlace(posting.Location, locations.Places); found {
		switch {
		case place.Kind == KindRemoteScope && decision.Workplace != "remote":
			return "", &Routed{
				Status: postings.StatusSkipped,
				Reason: place.Name + " is on the list as remote work only, and this posting is " +
					decision.Workplace + ".",
				Rule: RuleLocation,
			}
		case place.Kind == KindRelocate:
			return "In scope only by moving to " + place.Name + ".", nil
		}
		return "", nil
	}

	if locations.Relocation.Open {
		if target, found := matchTarget(posting.Location, locations.Relocation.Targets); found {
			return "In scope only by relocating to " + target + ".", nil
		}
	}
	if decision.Workplace == "remote" &&
		locations.Workplace.Remote &&
		locations.Workplace.Scope == ScopeGlobal {
		return "In scope as remote work rather than by place.", nil
	}

	return "", &Routed{
		Status: postings.StatusSkipped,
		Reason: where + " is outside every place on the list, and it is not remote work inside the " +
			locations.Workplace.Scope + " scope." + authorizationClause(locations.Authorization),
		Rule: RuleLocation,
	}
}

func authorizationClause(authorization string) string {
	if strings.TrimSpace(authorization) == "" {
		return ""
	}
	return " Work authorization stands at: " + strings.TrimSuffix(line(authorization, 200), ".") + "."
}

func refuseLanguage(decision Verdict, rules Rules) *Routed {
	accepted := rules.Settings.Roles.PostingLanguages
	if len(accepted) == 0 {
		return nil
	}
	written := strings.ToLower(strings.TrimSpace(decision.PostingLang))
	if written == "" || written == "unclear" || slices.Contains(accepted, written) {
		return nil
	}
	return &Routed{
		Status: postings.StatusSkipped,
		Reason: "written in " + written + ", and this person works in " +
			strings.Join(accepted, ", ") + ".",
		Rule: RulePostingLang,
	}
}

func routeApply(posting Candidate, decision Verdict, rules Rules) Routed {
	reason := reasonOf(decision.Reason)

	if rules.Settings.Companies.ExcludeAgencies && decision.Agency {
		return Routed{
			Status: postings.StatusSkipped,
			Reason: "recruiting agency posting, excluded by the company rules. " + reason,
			Rule:   RuleAgency,
		}
	}

	if refused := refuseLanguage(decision, rules); refused != nil {
		refused.Reason += " " + reason
		return *refused
	}

	floor := rules.Settings.Roles.MinCompensation
	if under := payBelowFloor(decision.StatedPay, floor); under != nil {
		return Routed{
			Status: postings.StatusSkipped,
			Reason: "stated pay is under the " + floor.Currency + " " + itoa(floor.Amount) +
				" per " + floor.Period + " floor. " + reason,
			Rule: RulePayFloor,
		}
	}

	note, refused := routeLocation(posting, decision, rules)
	if refused != nil {
		refused.Reason += " " + reason
		return *refused
	}
	if note != "" {
		reason += " " + note
	}

	if !posting.EasyApply {
		if !rules.Settings.Roles.ApplyToNonEasyApply {
			return Routed{
				Status: postings.StatusSkipped,
				Reason: reason + " Not Easy Apply, and those are turned off.",
				Rule:   RuleNonEasyApply,
			}
		}
		return Routed{
			Status: postings.StatusManual,
			Reason: reason + " Not Easy Apply, so it needs a manual send.",
			Rule:   RuleNonEasyApply,
		}
	}

	return Routed{Status: postings.StatusInbox, Reason: reason}
}
