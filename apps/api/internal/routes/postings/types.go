package postings

import "context"

const (
	StatusNew       = "new"
	StatusReading   = "reading"
	StatusInbox     = "inbox"
	StatusManual    = "manual"
	StatusQueue     = "queue"
	StatusApplied   = "applied"
	StatusOpened    = "opened"
	StatusSkipped   = "skipped"
	StatusDuplicate = "duplicate"

	StageNone = "none"
	StageHand = "hand"

	ListAll = "all"

	describedPending = "pending"
	describedOK      = "ok"

	maxPlaces    = 300
	maxPlaceName = 80
	maxPlaceHits = 8
	maxTitle     = 200
)

var (
	Statuses = []string{
		StatusNew,
		StatusReading,
		StatusInbox,
		StatusManual,
		StatusQueue,
		StatusApplied,
		StatusOpened,
		StatusSkipped,
		StatusDuplicate,
	}
	HandledStatuses = []string{
		StatusReading,
		StatusInbox,
		StatusManual,
		StatusQueue,
		StatusApplied,
		StatusOpened,
		StatusSkipped,
		StatusDuplicate,
	}
	ListStatuses = append([]string{ListAll}, Statuses...)
)

type Pay struct {
	Amount   int    `json:"amount"`
	Currency string `json:"currency"`
	Period   string `json:"period"`
}

type Tailored struct {
	Needed bool   `json:"needed"`
	Focus  string `json:"focus"`
}

type Posting struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Company          string    `json:"company"`
	Location         string    `json:"location"`
	URL              string    `json:"url"`
	DupeOf           *string   `json:"dupeOf"`
	Description      string    `json:"description"`
	DescriptionState string    `json:"descriptionState"`
	ListedAt         *int64    `json:"listedAt"`
	EasyApply        bool      `json:"easyApply"`
	Status           string    `json:"status"`
	Stage            string    `json:"stage"`
	Score            *int      `json:"score"`
	TriageReason     *string   `json:"triageReason"`
	VerdictReason    *string   `json:"verdictReason"`
	Seniority        *string   `json:"seniority"`
	Workplace        *string   `json:"workplace"`
	ContractType     *string   `json:"contractType"`
	PostingLang      *string   `json:"postingLang"`
	Agency           *bool     `json:"agency"`
	StatedPay        *Pay      `json:"statedPay"`
	ResumeCode       *string   `json:"resumeCode"`
	ResumeLang       *string   `json:"resumeLang"`
	ResumeFit        *string   `json:"resumeFit"`
	TailoredResume   *Tailored `json:"tailoredResume"`
	Stale            bool      `json:"stale"`
	Outcome          *string   `json:"outcome"`
	OutcomeNote      *string   `json:"outcomeNote"`
	CycleID          *int64    `json:"cycleId"`
	QueryLabel       *string   `json:"queryLabel"`
	CreatedAt        string    `json:"createdAt"`
	UpdatedAt        string    `json:"updatedAt"`
}

type Harvested struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Company     string `json:"company"`
	Location    string `json:"location"`
	URL         string `json:"url"`
	Description string `json:"description"`
	ListedAt    *int64 `json:"listedAt"`
	EasyApply   bool   `json:"easyApply"`
}

type Ingest struct {
	Received     int      `json:"received"`
	Inserted     int      `json:"inserted"`
	Duplicates   int      `json:"duplicates"`
	Collapsed    int      `json:"collapsed"`
	Judged       int      `json:"judged"`
	NewIDs       []string `json:"newIds"`
	RoundNew     int      `json:"roundNew"`
	NewJobTarget int      `json:"newJobTarget"`
	Stop         bool     `json:"stop"`
	Reason       string   `json:"reason,omitempty"`
}

type Opened struct {
	Received  int      `json:"received"`
	Recorded  int      `json:"recorded"`
	Collapsed int      `json:"collapsed"`
	Held      int      `json:"held"`
	IDs       []string `json:"ids"`
}

type Place struct {
	Name string `json:"name"`
	Ring string `json:"ring"`
}

type Resolution struct {
	Name       string `json:"name"`
	Asked      string `json:"asked"`
	Ring       string `json:"ring"`
	LocationID string `json:"locationId"`
	Source     string `json:"source"`
}

type Yield struct {
	CycleID    int64
	QueryLabel string
	Pages      int
	Received   int
	Inserted   int
	Duplicates int
}

type RoundState struct {
	RoundNew     int
	NewJobTarget int
	Stop         bool
	Reason       string
}

type Rounds interface {
	RecordYield(ctx context.Context, yield Yield) (RoundState, error)
}

type ScreenKeys interface {
	ScreenKey(ctx context.Context) string
}
