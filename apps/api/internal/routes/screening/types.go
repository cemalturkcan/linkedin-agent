package screening

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"api/internal/routes/postings"
	"api/internal/routes/settings"
)

const (
	Scope = "screening"

	PurposeTriage     = "triage"
	PurposeDeep       = "deep"
	PurposeReview     = "review"
	PurposeCorrection = "correction"
	PurposeReflect    = "reflect"

	StageTriage = "triage"
	StageDeep   = "deep"

	VerdictApply = "apply"
	VerdictSkip  = "skip"

	FitStrong  = "strong"
	FitPartial = "partial"
	FitNone    = "none"

	WorkplaceUnclear = "unclear"
	ContractUnclear  = "unclear"

	TriageBatch = 30
	DeepBatch   = 8

	NoteCap            = 12
	ReflectorNoteCap   = 60
	MaxDescriptionRead = 6000
	MaxLessons         = 5

	MinOutcomes           = 3
	OutcomesPerReflection = 5
	OutcomeWindow         = 60

	maxReason   = 500
	maxFocus    = 300
	maxLesson   = 300
	maxEvidence = 160
	maxNote     = 300
)

var (
	Workplaces    = []string{"onsite", "hybrid", "remote", WorkplaceUnclear}
	ContractTypes = []string{"full-time", "contract", "freelance", "internship", ContractUnclear}
	Fits          = []string{FitStrong, FitPartial, FitNone}
	Outcomes      = []string{"no-response", "rejected", "recruiter", "interview", "offer"}
	Scopes        = []string{"planning", Scope}

	Transitions = map[string]string{
		"queue":   postings.StatusQueue,
		"applied": postings.StatusApplied,
		"skip":    postings.StatusSkipped,
	}

	whitespace = regexp.MustCompile(`\s+`)
	twoLetters = regexp.MustCompile(`^[a-z]{2}$`)
)

type (
	Pay      = postings.Pay
	Tailored = postings.Tailored
)

type Verdict struct {
	Verdict      string   `json:"verdict"`
	Reason       string   `json:"reason"`
	Score        int      `json:"score"`
	Seniority    string   `json:"seniority"`
	Workplace    string   `json:"workplace"`
	ContractType string   `json:"contractType"`
	PostingLang  string   `json:"postingLang"`
	Agency       bool     `json:"agency"`
	StatedPay    *Pay     `json:"statedPay"`
	ResumeCode   string   `json:"resumeCode"`
	ResumeLang   string   `json:"resumeLang"`
	ResumeFit    string   `json:"resumeFit"`
	Tailored     Tailored `json:"tailoredResume"`
}

type Judged struct {
	ID       string  `json:"id"`
	Status   string  `json:"status"`
	Stage    string  `json:"stage"`
	Score    int     `json:"score"`
	Reason   string  `json:"reason"`
	Verdict  Verdict `json:"verdict"`
	Flagged  bool    `json:"flagged,omitempty"`
	Flag     string  `json:"flag,omitempty"`
	Rewrote  bool    `json:"rewrote,omitempty"`
	TraceID  int64   `json:"traceId,omitempty"`
	Original *Judged `json:"original,omitempty"`
}

type Candidate struct {
	ID           string
	Title        string
	Company      string
	Location     string
	URL          string
	EasyApply    bool
	ListedAt     *int64
	Description  string
	TriageReason string
}

type Pending struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	Title   string `json:"title"`
	Company string `json:"company"`
}

type Stored struct {
	Stored  int `json:"stored"`
	Missing int `json:"missing"`
	Unknown int `json:"unknown"`
}

type Staleness struct {
	ScreenKey string `json:"screenKey"`
	Stale     int    `json:"stale"`
}

type Rescreened struct {
	Reset int `json:"reset"`
}

type Gone struct {
	Cleared int      `json:"cleared"`
	Codes   []string `json:"codes"`
}

type Result struct {
	Paused              bool     `json:"paused"`
	Triaged             int      `json:"triaged"`
	Kept                int      `json:"kept"`
	DroppedAtTriage     int      `json:"droppedAtTriage"`
	Gated               int      `json:"gated"`
	Screened            int      `json:"screened"`
	Picked              int      `json:"picked"`
	Manual              int      `json:"manual"`
	Skipped             int      `json:"skipped"`
	Flagged             int      `json:"flagged"`
	Corrected           int      `json:"corrected"`
	Nudges              int      `json:"nudges"`
	AwaitingDescription int      `json:"awaitingDescription"`
	ScreenKey           string   `json:"screenKey"`
	Pruned              int      `json:"pruned,omitempty"`
	DescriptionsGivenUp int      `json:"descriptionsGivenUp,omitempty"`
	ResumeCodesGone     *Gone    `json:"resumeCodesGone,omitempty"`
	Verdicts            []Judged `json:"verdicts"`
	Stopped             string   `json:"stopped,omitempty"`
	Error               string   `json:"error,omitempty"`
}

type Lesson struct {
	Text     string `json:"text"`
	Evidence string `json:"evidence"`
	Scope    string `json:"scope"`
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

type Due struct {
	Due            bool    `json:"due"`
	Fresh          int     `json:"fresh"`
	Total          int     `json:"total"`
	LastReflection *string `json:"lastReflection"`
}

type Reflection struct {
	Ran     bool     `json:"ran"`
	Reason  string   `json:"reason,omitempty"`
	Read    int      `json:"read"`
	Written int      `json:"written"`
	Retired int      `json:"retired"`
	Model   string   `json:"model,omitempty"`
	Lessons []Lesson `json:"lessons"`
}

type Outcome struct {
	ID        string
	Title     string
	Company   string
	Location  string
	Score     int
	Reason    string
	Resume    string
	Fit       string
	Seniority string
	Workplace string
	Agency    bool
	AppliedAt string
	Outcome   string
	Note      string
}

type RefusalError struct {
	Message     string
	Unavailable bool
}

func (e *RefusalError) Error() string {
	return e.Message
}

func refuse(message string) *RefusalError {
	return &RefusalError{Message: message}
}

func unavailable(message string) *RefusalError {
	return &RefusalError{Message: message, Unavailable: true}
}

func line(value string, limit int) string {
	collapsed := strings.TrimSpace(whitespace.ReplaceAllString(strings.TrimSpace(value), " "))
	runes := []rune(collapsed)
	if len(runes) <= limit {
		return collapsed
	}
	return string(runes[:limit])
}

func reasonOf(value string) string {
	if cleaned := line(value, maxReason); cleaned != "" {
		return cleaned
	}
	return "the model returned no reason"
}

func scoreOf(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return min(100, max(0, value))
}

func choice(value string, allowed []string, fallback string) string {
	wanted := strings.ToLower(strings.TrimSpace(value))
	if slices.Contains(allowed, wanted) {
		return wanted
	}
	return fallback
}

func languageCode(value string) *string {
	code := strings.ToLower(strings.TrimSpace(value))
	if !twoLetters.MatchString(code) {
		return nil
	}
	return &code
}

func payOf(raw *Pay) *Pay {
	if raw == nil || raw.Amount <= 0 {
		return nil
	}
	currency := strings.ToUpper(line(raw.Currency, 3))
	period := choice(raw.Period, settings.PayPeriods, "")
	if currency == "" || period == "" {
		return nil
	}
	return &Pay{Amount: raw.Amount, Currency: currency, Period: period}
}

func tailoredOf(raw Tailored) Tailored {
	focus := line(raw.Focus, maxFocus)
	if !raw.Needed || focus == "" {
		return Tailored{}
	}
	return Tailored{Needed: true, Focus: focus}
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
