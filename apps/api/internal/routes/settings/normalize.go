package settings

import (
	"maps"
	"regexp"
	"strings"
)

const (
	maxList         = 40
	maxEntry        = 80
	maxFreeText     = 1200
	maxOperatorText = 4000
	maxCurrency     = 8
	maxLevel        = 20
	maxChoice       = 40
)

var (
	collapse     = regexp.MustCompile(`\s+`)
	blankLines   = regexp.MustCompile(`\n{3,}`)
	languageCode = regexp.MustCompile(`^[a-z]{2}$`)
)

type bounds struct {
	min      int
	max      int
	fallback int
}

func Normalize(raw map[string]any) Settings {
	return Settings{
		Locations:     locations(group(raw["locations"])),
		Roles:         roles(group(raw["roles"])),
		Companies:     companies(group(raw["companies"])),
		Apply:         apply(group(raw["apply"])),
		Budget:        budget(group(raw["budget"])),
		Harvest:       harvest(group(raw["harvest"])),
		OperatorNotes: paragraph(raw["operatorNotes"], maxOperatorText),
	}
}

func locations(raw map[string]any) Locations {
	fallback := Defaults().Locations
	relocation := group(raw["relocation"])
	workplace := group(raw["workplace"])
	return Locations{
		Places: places(raw["places"]),
		Relocation: Relocation{
			Open:    boolean(relocation["open"], fallback.Relocation.Open),
			Targets: list(relocation["targets"]),
			Notes:   paragraph(relocation["notes"], maxFreeText),
		},
		Workplace: Workplace{
			Onsite: boolean(workplace["onsite"], fallback.Workplace.Onsite),
			Hybrid: boolean(workplace["hybrid"], fallback.Workplace.Hybrid),
			Remote: boolean(workplace["remote"], fallback.Workplace.Remote),
			Scope:  pick(workplace["scope"], WorkplaceScopes, fallback.Workplace.Scope),
		},
		Authorization: paragraph(raw["authorization"], maxFreeText),
	}
}

func places(raw any) []Place {
	entries, ok := raw.([]any)
	if !ok {
		return []Place{}
	}
	seen := make(map[string]struct{}, len(entries))
	cleaned := make([]Place, 0, len(entries))
	for _, entry := range entries {
		source := group(entry)
		name := line(source["name"], maxEntry)
		key := strings.ToLower(name)
		if name == "" {
			continue
		}
		if _, present := seen[key]; present {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, Place{
			Name: name,
			Ring: pick(source["ring"], PlaceRings, PlaceRings[0]),
			Kind: pick(source["kind"], LocationKinds, LocationKinds[0]),
		})
		if len(cleaned) >= maxList {
			break
		}
	}
	return cleaned
}

func roles(raw map[string]any) Roles {
	fallback := Defaults().Roles
	return Roles{
		PostingLanguages:  languages(raw["postingLanguages"]),
		Seniority:           seniority(group(raw["seniority"])),
		ExcludeStacks:       list(raw["excludeStacks"]),
		ExcludeIndustries:   list(raw["excludeIndustries"]),
		ContractTypes:       pickList(raw["contractTypes"], ContractTypes, fallback.ContractTypes),
		MinCompensation:     compensation(group(raw["minCompensation"])),
		ApplyToNonEasyApply: boolean(raw["applyToNonEasyApply"], fallback.ApplyToNonEasyApply),
		EasyApplyOnly:       boolean(raw["easyApplyOnly"], fallback.EasyApplyOnly),
	}
}

func seniority(raw map[string]any) Seniority {
	fallback := Defaults().Roles.Seniority
	low := indexOf(SeniorityLadder, strings.ToLower(line(raw["min"], maxLevel)))
	high := indexOf(SeniorityLadder, strings.ToLower(line(raw["max"], maxLevel)))
	if low == -1 {
		low = indexOf(SeniorityLadder, fallback.Min)
	}
	if high == -1 {
		high = indexOf(SeniorityLadder, fallback.Max)
	}
	return Seniority{
		Min: SeniorityLadder[min(low, high)],
		Max: SeniorityLadder[max(low, high)],
	}
}

func compensation(raw map[string]any) Compensation {
	fallback := Defaults().Roles.MinCompensation
	amount := 0
	if numeric, ok := number(raw["amount"]); ok && numeric > 0 {
		amount = int(numeric)
	}
	return Compensation{
		Amount:     amount,
		Currency:   strings.ToUpper(line(raw["currency"], maxCurrency)),
		Period:     pick(raw["period"], PayPeriods, fallback.Period),
		HardFilter: boolean(raw["hardFilter"], fallback.HardFilter),
	}
}

func companies(raw map[string]any) Companies {
	fallback := Defaults().Companies
	return Companies{
		Blocked:         list(raw["blocked"]),
		Preferred:       list(raw["preferred"]),
		ExcludeAgencies: boolean(raw["excludeAgencies"], fallback.ExcludeAgencies),
		ReapplyCooldownDays: integer(raw["reapplyCooldownDays"], bounds{
			min: 0, max: 730, fallback: fallback.ReapplyCooldownDays,
		}),
		Dedupe: boolean(raw["dedupe"], fallback.Dedupe),
	}
}

func apply(raw map[string]any) Apply {
	fallback := Defaults().Apply
	return Apply{
		AutoAttach:         boolean(raw["autoAttach"], fallback.AutoAttach),
		UploadFileName:     fileName(raw["uploadFileName"], fallback.UploadFileName),
		UploadFileNameMode: pick(raw["uploadFileNameMode"], UploadNameModes, fallback.UploadFileNameMode),
		ResumeLanguages:    languages(raw["resumeLanguages"]),
		Paused:             boolean(raw["paused"], fallback.Paused),
	}
}

func budget(raw map[string]any) Budget {
	fallback := Defaults().Budget
	return Budget{
		MaxQueriesPerCycle: integer(raw["maxQueriesPerCycle"], bounds{
			min: 1, max: 8, fallback: fallback.MaxQueriesPerCycle,
		}),
		MaxScreenPerCycle: integer(raw["maxScreenPerCycle"], bounds{
			min: 1, max: 200, fallback: fallback.MaxScreenPerCycle,
		}),
		MaxModelCallsPerCycle: integer(raw["maxModelCallsPerCycle"], bounds{
			min: 0, max: 100_000, fallback: fallback.MaxModelCallsPerCycle,
		}),
		MaxPagesPerQuery: integer(raw["maxPagesPerQuery"], bounds{
			min: 1, max: 20, fallback: fallback.MaxPagesPerQuery,
		}),
		CycleMinutes: integer(raw["cycleMinutes"], bounds{
			min: 2, max: 720, fallback: fallback.CycleMinutes,
		}),
		AutoCycle: boolean(raw["autoCycle"], fallback.AutoCycle),
		DailyModelCallCap: integer(raw["dailyModelCallCap"], bounds{
			min: 0, max: 100_000, fallback: fallback.DailyModelCallCap,
		}),
		RetentionDays: integer(raw["retentionDays"], bounds{
			min: 1, max: 365, fallback: fallback.RetentionDays,
		}),
	}
}

func harvest(raw map[string]any) Harvest {
	fallback := Defaults().Harvest
	return Harvest{
		NewJobTarget: integer(raw["newJobTarget"], bounds{
			min: 1, max: 200, fallback: fallback.NewJobTarget,
		}),
		MaxWideningSteps: integer(raw["maxWideningSteps"], bounds{
			min: 0, max: 5, fallback: fallback.MaxWideningSteps,
		}),
		RoundDeadlineMs: integer(raw["roundDeadlineMs"], bounds{
			min: 30_000, max: 24 * 3_600_000, fallback: fallback.RoundDeadlineMs,
		}),
		RequestDelayMs: integer(raw["requestDelayMs"], bounds{
			min: 0, max: 10_000, fallback: fallback.RequestDelayMs,
		}),
		RequestTimeoutMs: integer(raw["requestTimeoutMs"], bounds{
			min: 2_000, max: 600_000, fallback: fallback.RequestTimeoutMs,
		}),
	}
}

func fileName(raw any, fallback string) string {
	name := strings.NewReplacer("/", "", `\`, "").Replace(line(raw, maxEntry))
	if name == "" {
		return fallback
	}
	if strings.HasSuffix(strings.ToLower(name), ".pdf") {
		return name
	}
	return name + ".pdf"
}

func languages(raw any) []string {
	cleaned := make([]string, 0, 8)
	seen := make(map[string]struct{}, 8)
	for _, entry := range list(raw) {
		code := strings.ToLower(entry)
		if !languageCode.MatchString(code) {
			continue
		}
		if _, present := seen[code]; present {
			continue
		}
		seen[code] = struct{}{}
		cleaned = append(cleaned, code)
	}
	return cleaned
}

func group(raw any) map[string]any {
	if fields, ok := raw.(map[string]any); ok {
		return fields
	}
	return map[string]any{}
}

func boolean(raw any, fallback bool) bool {
	if value, ok := raw.(bool); ok {
		return value
	}
	return fallback
}

func number(raw any) (float64, bool) {
	value, ok := raw.(float64)
	return value, ok
}

func integer(raw any, limit bounds) int {
	value, ok := number(raw)
	if !ok {
		return limit.fallback
	}
	return min(limit.max, max(limit.min, int(value)))
}

func line(raw any, limit int) string {
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return clip(collapse.ReplaceAllString(strings.TrimSpace(value), " "), limit)
}

func paragraph(raw any, limit int) string {
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	normalized := strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	return clip(strings.TrimSpace(blankLines.ReplaceAllString(normalized, "\n\n")), limit)
}

func clip(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	cut := limit
	for cut > 0 && !isRuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
}

func isRuneStart(character byte) bool {
	return character&0xC0 != 0x80
}

func list(raw any) []string {
	entries, ok := raw.([]any)
	if !ok {
		return []string{}
	}
	cleaned := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		value := line(entry, maxEntry)
		key := strings.ToLower(value)
		if value == "" {
			continue
		}
		if _, present := seen[key]; present {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, value)
		if len(cleaned) >= maxList {
			break
		}
	}
	return cleaned
}

func pick(raw any, allowed []string, fallback string) string {
	wanted := strings.ToLower(line(raw, maxChoice))
	if indexOf(allowed, wanted) >= 0 {
		return wanted
	}
	return fallback
}

func pickList(raw any, allowed, fallback []string) []string {
	entries, ok := raw.([]any)
	if !ok {
		return append([]string{}, fallback...)
	}
	wanted := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		wanted[strings.ToLower(line(entry, maxChoice))] = struct{}{}
	}
	cleaned := make([]string, 0, len(allowed))
	for _, value := range allowed {
		if _, present := wanted[value]; present {
			cleaned = append(cleaned, value)
		}
	}
	if len(cleaned) == 0 {
		return append([]string{}, fallback...)
	}
	return cleaned
}

func Merge(base, patch map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(patch))
	maps.Copy(merged, base)
	for key, value := range patch {
		nested, patchIsGroup := value.(map[string]any)
		current, baseIsGroup := merged[key].(map[string]any)
		if patchIsGroup && baseIsGroup {
			merged[key] = Merge(current, nested)
			continue
		}
		merged[key] = value
	}
	return merged
}
