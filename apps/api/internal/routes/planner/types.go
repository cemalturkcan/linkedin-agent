package planner

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"api/internal/routes/plugin"
	"api/internal/routes/settings"
)

const (
	Scope        = "planning"
	PurposePlan  = "plan"
	PurposeWiden = "widen"

	NoteCap       = 12
	HistoryRounds = 6
	MinedOutAfter = 3

	maxLabel     = 32
	maxPlaceName = 80
	maxKeyword   = 60
	maxReason    = 400
	maxRationale = 600

	StatusOpen   = "open"
	StatusDone   = "done"
	StatusFailed = "failed"

	ReasonPaused          = "paused"
	ReasonExecutorOffline = "executor-offline"
	ReasonNoProfile       = "no-profile"
	ReasonDailyCap        = "daily-cap"
	ReasonRoundCap        = "round-cap"
	ReasonPlannerFailed   = "planner-failed"
	ReasonNoRound         = "no-round"
	ReasonRoundClosed     = "closed"
	ReasonWideningSpent   = "widening-spent"
)

var (
	Relaxations    = []string{"time-window", "easy-apply", "remote-only", "ring"}
	NotePriorities = []string{"high", "normal", "low"}

	slugNoise  = regexp.MustCompile(`[^a-z0-9]+`)
	slugEdges  = regexp.MustCompile(`^-+|-+$`)
	whitespace = regexp.MustCompile(`\s+`)
)

type Query struct {
	Label      string `json:"label"`
	Ring       string `json:"ring"`
	Place      string `json:"place"`
	Keyword    string `json:"keyword"`
	Range      string `json:"range"`
	EasyApply  bool   `json:"easyApply,omitempty"`
	RemoteOnly bool   `json:"remoteOnly,omitempty"`
	Want       int    `json:"want"`
	Reason     string `json:"reason"`
	Relaxed    string `json:"relaxed,omitempty"`
}

type Pacing struct {
	NewJobTarget     int `json:"newJobTarget"`
	MaxWideningSteps int `json:"maxWideningSteps"`
	RoundDeadlineMs  int `json:"roundDeadlineMs"`
	RequestDelayMs   int `json:"requestDelayMs"`
	RequestTimeoutMs int `json:"requestTimeoutMs"`
	CycleMinutes     int `json:"cycleMinutes"`
	MaxPagesPerQuery int `json:"maxPagesPerQuery"`
}

type Attach struct {
	AutoAttach         bool   `json:"autoAttach"`
	UploadFileName     string `json:"uploadFileName"`
	UploadFileNameMode string `json:"uploadFileNameMode"`
}

type Plan struct {
	OK           bool     `json:"ok"`
	CycleID      int64    `json:"cycleId"`
	StartedAt    string   `json:"startedAt"`
	DeadlineAt   string   `json:"deadlineAt"`
	NextRunAt    *string  `json:"nextRunAt"`
	Rationale    string   `json:"rationale"`
	ScreenBudget int      `json:"screenBudget"`
	Queries      []Query  `json:"queries"`
	Pacing       Pacing   `json:"pacing"`
	Apply        Attach   `json:"apply"`
	KnownIDs     []string `json:"knownIds"`
	Resumed      bool     `json:"resumed,omitempty"`
	Rejected     []string `json:"rejected,omitempty"`
	Notes        *Curated `json:"notes,omitempty"`
}

type Curated struct {
	Added   int `json:"added"`
	Revised int `json:"revised"`
	Dropped int `json:"dropped"`
}

type RefusalError struct {
	OK      bool   `json:"ok"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func (r *RefusalError) Error() string {
	return r.Message
}

func refuse(reason, message string) *RefusalError {
	return &RefusalError{Reason: reason, Message: message}
}

type Note struct {
	ID       int64  `json:"id"`
	Scope    string `json:"scope"`
	Text     string `json:"text"`
	Evidence string `json:"evidence"`
	Priority string `json:"priority"`
	Created  string `json:"createdAt"`
	Updated  string `json:"updatedAt"`
}

type RoundQuery struct {
	Label      string `json:"label"`
	Query      Query  `json:"query"`
	Widened    bool   `json:"widened"`
	Pages      int    `json:"pages"`
	Received   int    `json:"received"`
	Inserted   int    `json:"inserted"`
	Duplicates int    `json:"duplicates"`
	Skipped    bool   `json:"skipped"`
	SkipReason string `json:"skipReason,omitempty"`
}

type Round struct {
	ID                int64        `json:"id"`
	StartedAt         string       `json:"startedAt"`
	FinishedAt        *string      `json:"finishedAt"`
	Status            string       `json:"status"`
	Rationale         string       `json:"rationale"`
	ScreenBudget      int          `json:"screenBudget"`
	Received          int          `json:"received"`
	Inserted          int          `json:"inserted"`
	Duplicates        int          `json:"duplicates"`
	WideningSteps     int          `json:"wideningSteps"`
	ModelCalls        int          `json:"modelCalls"`
	TerminationReason *string      `json:"terminationReason"`
	Error             *string      `json:"error"`
	Queries           []RoundQuery `json:"queries"`
}

type Budget struct {
	Used      int    `json:"used"`
	Cap       int    `json:"cap"`
	Remaining int    `json:"remaining"`
	Day       string `json:"day"`
}

type RoundBudget struct {
	CycleID   int64 `json:"cycleId"`
	Open      bool  `json:"open"`
	Used      int   `json:"used"`
	Cap       int   `json:"cap"`
	Remaining int   `json:"remaining"`
}

func (b RoundBudget) Reached() bool {
	return b.Open && b.Cap > 0 && b.Remaining <= 0
}

type State struct {
	Plugin       plugin.State  `json:"plugin"`
	Paused       bool          `json:"paused"`
	AutoCycle    bool          `json:"autoCycle"`
	CycleMinutes int           `json:"cycleMinutes"`
	NextRunAt    *string       `json:"nextRunAt"`
	Blocked      *RefusalError `json:"blocked"`
	Budget       Budget        `json:"budget"`
	RoundBudget  RoundBudget   `json:"roundBudget"`
	Cycle        *Round        `json:"cycle"`
	History      []Round       `json:"history"`
	Notes        []Note        `json:"notes"`
	KnownIDCount int           `json:"knownIdCount"`
}

type planContext struct {
	capabilities  plugin.Capabilities
	maxQueries    int
	screenBudget  int
	maxWant       int
	easyApplyOnly bool
}

func slug(value string, fallback string) string {
	cleaned := slugEdges.ReplaceAllString(
		slugNoise.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-"),
		"",
	)
	if cleaned == "" {
		return fallback
	}
	return clip(cleaned, maxLabel)
}

func sentence(value string, limit int, fallback string) string {
	collapsed := strings.TrimSpace(whitespace.ReplaceAllString(strings.TrimSpace(value), " "))
	if collapsed == "" {
		return fallback
	}
	return clipWords(collapsed, limit)
}

func clip(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func clipWords(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	cut := string(runes[:limit])
	if space := strings.LastIndexByte(cut, ' '); space > limit/2 {
		cut = cut[:space]
	}
	return strings.TrimRight(cut, " ,;:-") + "..."
}

func clamp(value, low, high, fallback int) int {
	if value == 0 {
		return fallback
	}
	return min(high, max(low, value))
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

func priority(value string) string {
	wanted := strings.ToLower(strings.TrimSpace(value))
	if slices.Contains(NotePriorities, wanted) {
		return wanted
	}
	return "normal"
}

func worldwide() string {
	return settings.RingWorldwide
}
