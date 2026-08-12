package indexer

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"api/internal/routes/settings"
)

const (
	maxEntry         = 60
	maxRole          = 80
	maxSummary       = 240
	maxHeadline      = 300
	maxStack         = 12
	maxDomains       = 6
	maxLanguages     = 8
	maxPlaces        = 6
	maxCandidateList = 14
	maxCandidateSet  = 8
	maxYears         = 60
	earliestYear     = 1950
)

var (
	monthPattern = regexp.MustCompile(`^(\d{4})-(0[1-9]|1[0-2])$`)
	languageCode = regexp.MustCompile(`^[a-z]{2}$`)
	whitespace   = regexp.MustCompile(`\s+`)
)

type Place struct {
	Name string `json:"name"`
	Ring string `json:"ring"`
}

type ResumeProfile struct {
	Code             string   `json:"code"`
	TargetRole       string   `json:"targetRole"`
	SeniorityClaimed string   `json:"seniorityClaimed"`
	CoreStack        []string `json:"coreStack"`
	SecondaryStack   []string `json:"secondaryStack"`
	Domains          []string `json:"domains"`
	Languages        []string `json:"languages"`
	YearsClaimed     int      `json:"yearsClaimed"`
	EarliestStart    string   `json:"earliestStart"`
	Places           []Place  `json:"places"`
	Summary          string   `json:"summary"`
}

type Candidate struct {
	YearsExperience int      `json:"yearsExperience"`
	SeniorityBand   string   `json:"seniorityBand"`
	CoreStack       []string `json:"coreStack"`
	SecondaryStack  []string `json:"secondaryStack"`
	Domains         []string `json:"domains"`
	WorkLanguages   []string `json:"workLanguages"`
	Places          []Place  `json:"places"`
	Headline        string   `json:"headline"`
}

func shapeResumeProfile(raw ResumeProfile, code, label string, now time.Time) ResumeProfile {
	return ResumeProfile{
		Code:             code,
		TargetRole:       fallbackLine(raw.TargetRole, maxRole, label),
		SeniorityClaimed: level(raw.SeniorityClaimed),
		CoreStack:        entries(raw.CoreStack, maxStack),
		SecondaryStack:   entries(raw.SecondaryStack, maxStack),
		Domains:          entries(raw.Domains, maxDomains),
		Languages:        codes(raw.Languages),
		YearsClaimed:     years(raw.YearsClaimed),
		EarliestStart:    month(raw.EarliestStart, now),
		Places:           placeList(raw.Places, maxPlaces),
		Summary:          line(raw.Summary, maxSummary),
	}
}

func shapeCandidate(raw Candidate, profiles []ResumeProfile, now time.Time) Candidate {
	return Candidate{
		YearsExperience: EvidencedYears(profiles, now, years(raw.YearsExperience)),
		SeniorityBand:   EvidencedBand(profiles, level(raw.SeniorityBand)),
		CoreStack:       entries(raw.CoreStack, maxCandidateList),
		SecondaryStack:  entries(raw.SecondaryStack, maxCandidateList),
		Domains:         entries(raw.Domains, maxCandidateSet),
		WorkLanguages:   codes(raw.WorkLanguages),
		Places:          placeList(raw.Places, maxCandidateSet),
		Headline:        line(raw.Headline, maxHeadline),
	}
}

func EvidencedYears(profiles []ResumeProfile, now time.Time, claimed int) int {
	fullest := 0
	earliest := ""
	for _, profile := range profiles {
		fullest = max(fullest, years(profile.YearsClaimed))
		start := month(profile.EarliestStart, now)
		if start != "" && (earliest == "" || start < earliest) {
			earliest = start
		}
	}
	if fullest > 0 {
		return fullest
	}
	if span := yearsSince(earliest, now); span > 0 {
		return span
	}
	return claimed
}

func EvidencedBand(profiles []ResumeProfile, claimed string) string {
	highest := -1
	for _, profile := range profiles {
		highest = max(highest, ladderIndex(profile.SeniorityClaimed))
	}
	if highest >= 0 {
		return settings.SeniorityLadder[highest]
	}
	return claimed
}

func ladderIndex(value string) int {
	return slices.Index(settings.SeniorityLadder, value)
}

func level(value string) string {
	wanted := strings.ToLower(line(value, 40))
	if slices.Contains(settings.SeniorityValues, wanted) {
		return wanted
	}
	return settings.SeniorityUnknown
}

func years(value int) int {
	if value <= 0 {
		return 0
	}
	return min(value, maxYears)
}

func month(value string, now time.Time) string {
	raw := line(value, 7)
	match := monthPattern.FindStringSubmatch(raw)
	if match == nil {
		return ""
	}
	year, err := strconv.Atoi(match[1])
	if err != nil || year < earliestYear || year > now.Year() {
		return ""
	}
	return raw
}

func yearsSince(start string, now time.Time) int {
	match := monthPattern.FindStringSubmatch(start)
	if match == nil {
		return 0
	}
	from, err := time.Parse("2006-01", start)
	if err != nil {
		return 0
	}
	months := (now.Year()-from.Year())*12 + int(now.Month()) - int(from.Month())
	if months <= 0 {
		return 0
	}
	return min(months/12, maxYears)
}

func line(value string, limit int) string {
	collapsed := whitespace.ReplaceAllString(strings.TrimSpace(value), " ")
	runes := []rune(collapsed)
	if len(runes) <= limit {
		return collapsed
	}
	return string(runes[:limit])
}

func fallbackLine(value string, limit int, fallback string) string {
	if cleaned := line(value, limit); cleaned != "" {
		return cleaned
	}
	return fallback
}

func entries(values []string, limit int) []string {
	cleaned := make([]string, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, value := range values {
		entry := line(value, maxEntry)
		key := strings.ToLower(entry)
		if entry == "" {
			continue
		}
		if _, present := seen[key]; present {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, entry)
		if len(cleaned) >= limit {
			break
		}
	}
	return cleaned
}

func codes(values []string) []string {
	cleaned := make([]string, 0, maxLanguages)
	seen := make(map[string]struct{}, maxLanguages)
	for _, value := range values {
		code := strings.ToLower(line(value, 8))
		if !languageCode.MatchString(code) {
			continue
		}
		if _, present := seen[code]; present {
			continue
		}
		seen[code] = struct{}{}
		cleaned = append(cleaned, code)
		if len(cleaned) >= maxLanguages {
			break
		}
	}
	return cleaned
}

func placeList(values []Place, limit int) []Place {
	cleaned := make([]Place, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, value := range values {
		name := line(value.Name, maxEntry)
		ring := strings.ToLower(line(value.Ring, 20))
		key := ring + ":" + strings.ToLower(name)
		if name == "" || !slices.Contains(settings.ProfileRings, ring) {
			continue
		}
		if _, present := seen[key]; present {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, Place{Name: name, Ring: ring})
		if len(cleaned) >= limit {
			break
		}
	}
	return cleaned
}
