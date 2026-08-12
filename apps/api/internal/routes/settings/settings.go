package settings

const (
	SeniorityUnknown = "unknown"

	RingCity      = "city"
	RingCountry   = "country"
	RingRegion    = "region"
	RingWorldwide = "worldwide"

	UploadNameOnce       = "one-name"
	UploadNamePerVariant = "per-variant"

	FallbackUploadName = "resume.pdf"
)

var (
	SeniorityLadder = []string{"intern", "junior", "mid", "senior", "lead", "staff", "principal"}
	SeniorityValues = append(append([]string{}, SeniorityLadder...), SeniorityUnknown)
	LocationKinds   = []string{"commute", "relocate", "remote-scope"}
	PlaceRings      = []string{RingCity, RingCountry, RingRegion, RingWorldwide}
	ProfileRings    = []string{RingCity, RingCountry}
	WorkplaceScopes = []string{"local", "country", "region", "global"}
	ContractTypes   = []string{"full-time", "contract", "freelance", "internship"}
	PayPeriods      = []string{"hour", "day", "month", "year"}
	UploadNameModes = []string{UploadNameOnce, UploadNamePerVariant}
)

type Enums struct {
	SeniorityLadder []string `json:"seniorityLadder"`
	LocationKinds   []string `json:"locationKinds"`
	PlaceRings      []string `json:"placeRings"`
	WorkplaceScopes []string `json:"workplaceScopes"`
	ContractTypes   []string `json:"contractTypes"`
	PayPeriods      []string `json:"payPeriods"`
	UploadNameModes []string `json:"uploadNameModes"`
}

func Enumerations() Enums {
	return Enums{
		SeniorityLadder: SeniorityLadder,
		LocationKinds:   LocationKinds,
		PlaceRings:      PlaceRings,
		WorkplaceScopes: WorkplaceScopes,
		ContractTypes:   ContractTypes,
		PayPeriods:      PayPeriods,
		UploadNameModes: UploadNameModes,
	}
}

type Place struct {
	Name string `json:"name"`
	Ring string `json:"ring"`
	Kind string `json:"kind"`
}

type Relocation struct {
	Open    bool     `json:"open"`
	Targets []string `json:"targets"`
	Notes   string   `json:"notes"`
}

type Workplace struct {
	Onsite bool   `json:"onsite"`
	Hybrid bool   `json:"hybrid"`
	Remote bool   `json:"remote"`
	Scope  string `json:"scope"`
}

type Locations struct {
	Places        []Place    `json:"places"`
	Relocation    Relocation `json:"relocation"`
	Workplace     Workplace  `json:"workplace"`
	Authorization string     `json:"authorization"`
}

type Seniority struct {
	Min string `json:"min"`
	Max string `json:"max"`
}

type Compensation struct {
	Amount     int    `json:"amount"`
	Currency   string `json:"currency"`
	Period     string `json:"period"`
	HardFilter bool   `json:"hardFilter"`
}

type Roles struct {
	Seniority           Seniority    `json:"seniority"`
	ExcludeStacks       []string     `json:"excludeStacks"`
	ExcludeIndustries   []string     `json:"excludeIndustries"`
	ContractTypes       []string     `json:"contractTypes"`
	MinCompensation     Compensation `json:"minCompensation"`
	ApplyToNonEasyApply bool         `json:"applyToNonEasyApply"`
	EasyApplyOnly       bool         `json:"easyApplyOnly"`
}

type Companies struct {
	Blocked             []string `json:"blocked"`
	Preferred           []string `json:"preferred"`
	ExcludeAgencies     bool     `json:"excludeAgencies"`
	ReapplyCooldownDays int      `json:"reapplyCooldownDays"`
	Dedupe              bool     `json:"dedupe"`
}

type Apply struct {
	AutoAttach         bool     `json:"autoAttach"`
	UploadFileName     string   `json:"uploadFileName"`
	UploadFileNameMode string   `json:"uploadFileNameMode"`
	ResumeLanguages    []string `json:"resumeLanguages"`
	Paused             bool     `json:"paused"`
}

type Budget struct {
	MaxQueriesPerCycle    int  `json:"maxQueriesPerCycle"`
	MaxScreenPerCycle     int  `json:"maxScreenPerCycle"`
	MaxModelCallsPerCycle int  `json:"maxModelCallsPerCycle"`
	MaxPagesPerQuery      int  `json:"maxPagesPerQuery"`
	CycleMinutes          int  `json:"cycleMinutes"`
	AutoCycle             bool `json:"autoCycle"`
	DailyModelCallCap     int  `json:"dailyModelCallCap"`
	RetentionDays         int  `json:"retentionDays"`
}

type Harvest struct {
	NewJobTarget     int `json:"newJobTarget"`
	MaxWideningSteps int `json:"maxWideningSteps"`
	RoundDeadlineMs  int `json:"roundDeadlineMs"`
	RequestDelayMs   int `json:"requestDelayMs"`
	RequestTimeoutMs int `json:"requestTimeoutMs"`
}

type Settings struct {
	Locations     Locations `json:"locations"`
	Roles         Roles     `json:"roles"`
	Companies     Companies `json:"companies"`
	Apply         Apply     `json:"apply"`
	Budget        Budget    `json:"budget"`
	Harvest       Harvest   `json:"harvest"`
	OperatorNotes string    `json:"operatorNotes"`
}

func Defaults() Settings {
	return Settings{
		Locations: Locations{
			Places:     []Place{},
			Relocation: Relocation{Open: true, Targets: []string{}, Notes: ""},
			Workplace:  Workplace{Onsite: true, Hybrid: true, Remote: true, Scope: "global"},
		},
		Roles: Roles{
			Seniority:           Seniority{Min: SeniorityLadder[0], Max: last(SeniorityLadder)},
			ExcludeStacks:       []string{},
			ExcludeIndustries:   []string{},
			ContractTypes:       append([]string{}, ContractTypes...),
			MinCompensation:     Compensation{Period: "year"},
			ApplyToNonEasyApply: false,
			EasyApplyOnly:       true,
		},
		Companies: Companies{
			Blocked:             []string{},
			Preferred:           []string{},
			ReapplyCooldownDays: 90,
			Dedupe:              true,
		},
		Apply: Apply{
			AutoAttach:         true,
			UploadFileName:     "",
			UploadFileNameMode: UploadNameOnce,
			ResumeLanguages:    []string{},
		},
		Budget: Budget{
			MaxQueriesPerCycle:    8,
			MaxScreenPerCycle:     200,
			MaxModelCallsPerCycle: 0,
			MaxPagesPerQuery:      15,
			CycleMinutes:          10,
			DailyModelCallCap:     0,
			RetentionDays:         30,
			AutoCycle:             true,
		},
		Harvest: Harvest{
			NewJobTarget:     100,
			MaxWideningSteps: 2,
			RoundDeadlineMs:  3_600_000,
			RequestDelayMs:   900,
			RequestTimeoutMs: 120_000,
		},
	}
}

func SeniorityRange(bounds Seniority) (accepted, rejected []string) {
	low := indexOf(SeniorityLadder, bounds.Min)
	high := indexOf(SeniorityLadder, bounds.Max)
	for position, level := range SeniorityLadder {
		if position >= low && position <= high {
			accepted = append(accepted, level)
			continue
		}
		rejected = append(rejected, level)
	}
	return accepted, rejected
}

func indexOf(values []string, wanted string) int {
	for position, value := range values {
		if value == wanted {
			return position
		}
	}
	return -1
}

func last(values []string) string {
	return values[len(values)-1]
}
