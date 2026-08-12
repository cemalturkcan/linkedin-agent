package planner

import (
	"errors"
	"slices"
	"strings"
)

type modelQuery struct {
	Label      string `json:"label"`
	Ring       string `json:"ring"`
	Place      string `json:"place"`
	Keyword    string `json:"keyword"`
	Range      string `json:"range"`
	EasyApply  bool   `json:"easyApply"`
	RemoteOnly bool   `json:"remoteOnly"`
	Want       int    `json:"want"`
	Reason     string `json:"reason"`
}

type modelNote struct {
	ID       int64  `json:"id"`
	Text     string `json:"text"`
	Priority string `json:"priority"`
}

type modelNotes struct {
	Add    []modelNote `json:"add"`
	Revise []modelNote `json:"revise"`
	Drop   []int64     `json:"drop"`
}

type modelPlan struct {
	Queries      []modelQuery `json:"queries"`
	ScreenBudget int          `json:"screenBudget"`
	Notes        modelNotes   `json:"notes"`
	Rationale    string       `json:"rationale"`
}

type modelWidening struct {
	Query   modelQuery `json:"query"`
	Relaxed string     `json:"relaxed"`
	Reason  string     `json:"reason"`
}

const RejectedBeforeRun = "rejected before it ran, so this is about the query you wrote: "

type Rejection struct {
	Reason string
	Record Query
}

func compileQuery(
	raw modelQuery,
	label string,
	context planContext,
	taken map[string]struct{},
) (Query, error) {
	if _, present := taken[label]; present {
		return Query{}, errors.New(`duplicate query label "` + label + `"`)
	}

	rings := context.capabilities.Fetch.Rings
	ring := strings.ToLower(strings.TrimSpace(raw.Ring))
	if !slices.Contains(rings, ring) {
		return Query{}, errors.New(`query "` + label + `" works the ` +
			sentence(raw.Ring, 20, "unnamed") + " ring, and the executor only works " +
			strings.Join(rings, ", "))
	}

	place := sentence(raw.Place, maxPlaceName, "")
	if ring == worldwide() && place != "" {
		return Query{}, errors.New(`query "` + label + `" is ` + worldwide() +
			" and still names " + place + ", and " + worldwide() + " carries no place")
	}
	if ring != worldwide() && place == "" {
		return Query{}, errors.New(`query "` + label + `" works the ` + ring +
			" ring and names no place")
	}

	query := Query{
		Label:  label,
		Ring:   ring,
		Place:  place,
		Range:  pickRange(raw.Range, context.capabilities.Fetch.Ranges),
		Want:   clamp(raw.Want, 1, context.maxWant, min(25, context.maxWant)),
		Reason: sentence(raw.Reason, maxReason, "no reason given"),
	}
	if context.capabilities.Fetch.Keyword {
		query.Keyword = sentence(raw.Keyword, maxKeyword, "")
	}
	if context.capabilities.Fetch.EasyApply {
		query.EasyApply = raw.EasyApply || context.easyApplyOnly
	}
	if context.capabilities.Fetch.RemoteOnly {
		query.RemoteOnly = raw.RemoteOnly
	}
	return query, nil
}

func pickRange(reported string, allowed []string) string {
	wanted := strings.TrimSpace(reported)
	if slices.Contains(allowed, wanted) {
		return wanted
	}
	if len(allowed) == 0 {
		return ""
	}
	return allowed[len(allowed)-1]
}

func rejectedRecord(raw modelQuery, label string, context planContext) Query {
	record := Query{
		Label:  label,
		Ring:   strings.ToLower(strings.TrimSpace(raw.Ring)),
		Place:  sentence(raw.Place, maxPlaceName, ""),
		Range:  pickRange(raw.Range, context.capabilities.Fetch.Ranges),
		Want:   clamp(raw.Want, 1, context.maxWant, min(25, context.maxWant)),
		Reason: sentence(raw.Reason, maxReason, "no reason given"),
	}
	if context.capabilities.Fetch.Keyword {
		record.Keyword = sentence(raw.Keyword, maxKeyword, "")
	}
	return record
}

func freeLabel(label string, position int, taken map[string]struct{}) string {
	if _, present := taken[label]; !present {
		return label
	}
	return label + "-" + itoa(position+1)
}

func compileQueries(raw []modelQuery, context planContext) ([]Query, []Rejection) {
	if len(raw) > context.maxQueries {
		raw = raw[:context.maxQueries]
	}
	queries := make([]Query, 0, len(raw))
	rejected := make([]Rejection, 0, len(raw))
	taken := make(map[string]struct{}, len(raw))

	for position, entry := range raw {
		label := slug(entry.Label, "query-"+itoa(position+1))
		query, err := compileQuery(entry, label, context, taken)
		if err != nil {
			record := rejectedRecord(entry, freeLabel(label, position, taken), context)
			taken[record.Label] = struct{}{}
			rejected = append(rejected, Rejection{Reason: err.Error(), Record: record})
			continue
		}
		taken[query.Label] = struct{}{}
		queries = append(queries, query)
	}
	return queries, rejected
}

func reasonsOf(rejected []Rejection) []string {
	reasons := make([]string, 0, len(rejected))
	for _, entry := range rejected {
		reasons = append(reasons, entry.Reason)
	}
	return reasons
}
