package screening

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"api/internal/app/claude"
	"api/internal/app/clock"
	"api/internal/app/events"
	"api/internal/app/store"
	"api/internal/routes/indexer"
	"api/internal/routes/postings"
	screeningdb "api/internal/routes/screening/gen"
	"api/internal/routes/settings"
)

type Config struct {
	ScreenBudget      time.Duration
	DescriptionWindow time.Duration
	PendingLimit      int
}

type Reader interface {
	Get(ctx context.Context, id string) (postings.Posting, error)
}

type Rounds interface {
	OpenRoundCalls(ctx context.Context) (int, bool, error)
	ChargeRound(ctx context.Context, inputTokens, outputTokens int64) error
}

type Dependencies struct {
	Reads        store.DBTX
	Transactions store.TransactionRunner
	Claude       *claude.Client
	Settings     *settings.Service
	Profiles     *indexer.Service
	Postings     Reader
	Rounds       Rounds
	Hub          *events.Hub
	Clock        clock.Clock
	Config       Config
}

type Service struct {
	reflecting sync.Mutex

	queries      *screeningdb.Queries
	transactions store.TransactionRunner
	claude       *claude.Client
	settings     *settings.Service
	profiles     *indexer.Service
	postings     Reader
	rounds       Rounds
	hub          *events.Hub
	clock        clock.Clock
	config       Config
}

func NewService(dependencies Dependencies) *Service {
	config := dependencies.Config
	if config.DescriptionWindow <= 0 {
		config.DescriptionWindow = 10 * time.Minute
	}
	if config.PendingLimit <= 0 {
		config.PendingLimit = 40
	}
	return &Service{
		queries:      screeningdb.New(dependencies.Reads),
		transactions: dependencies.Transactions,
		claude:       dependencies.Claude,
		settings:     dependencies.Settings,
		profiles:     dependencies.Profiles,
		postings:     dependencies.Postings,
		rounds:       dependencies.Rounds,
		hub:          dependencies.Hub,
		clock:        dependencies.Clock,
		config:       config,
	}
}

type desk struct {
	state     indexer.State
	rules     Rules
	notes     []Note
	screenKey string
}

func (d desk) roundCap() int {
	return d.rules.Settings.Budget.MaxModelCallsPerCycle
}

func (s *Service) roundCapReached(ctx context.Context, ceiling int) (bool, error) {
	if ceiling <= 0 || s.rounds == nil {
		return false, nil
	}
	spent, open, err := s.rounds.OpenRoundCalls(ctx)
	if err != nil {
		return false, err
	}
	return open && spent >= ceiling, nil
}

func (s *Service) stopAtCeiling(ctx context.Context, at desk, result *Result) (bool, string) {
	if result.Stopped != "" {
		return true, ""
	}
	ceiling := at.roundCap()
	reached, err := s.roundCapReached(ctx, ceiling)
	if err != nil {
		return false, why(err)
	}
	if !reached {
		return false, ""
	}
	result.Stopped = "the round has spent all " + itoa(ceiling) +
		" model calls it may. the postings left are unjudged and the next round takes them."
	s.hub.Emit(events.TypeScreen, map[string]any{
		"phase":  "stopped",
		"reason": result.Stopped,
	})
	return true, ""
}

func (s *Service) desk(ctx context.Context) (desk, error) {
	state := s.profiles.State(ctx)
	if state.Error != "" {
		return desk{}, unavailable(state.Error)
	}
	if state.Candidate == nil {
		return desk{}, refuse(
			"the cv folder is not indexed yet, so there is no candidate to screen against.",
		)
	}
	if len(state.Resumes) == 0 {
		return desk{}, refuse("the cv folder holds no resume this screening could assign.")
	}
	current, err := s.settings.Load(ctx)
	if err != nil {
		return desk{}, err
	}
	fileLanguages := map[string]struct{}{}
	for _, resume := range state.Resumes {
		for _, language := range resume.FileLanguages {
			fileLanguages[language] = struct{}{}
		}
	}
	available := make([]string, 0, len(fileLanguages))
	for language := range fileLanguages {
		available = append(available, language)
	}
	sort.Strings(available)

	notes, err := s.Notes(ctx, Scope, NoteCap)
	if err != nil {
		return desk{}, err
	}
	rules := NewRules(current, available)
	return desk{
		state:     state,
		rules:     rules,
		notes:     notes,
		screenKey: fingerprint(state, current),
	}, nil
}

func fingerprint(state indexer.State, current settings.Settings) string {
	variants := make([][]string, 0, len(state.Resumes))
	found := make([]string, 0, len(state.Resumes))
	for _, resume := range state.Resumes {
		found = append(found, resume.Code)
		if resume.Profile == nil {
			variants = append(variants, []string{})
			continue
		}
		variants = append(variants, resume.Profile.CoreStack)
	}
	sort.Strings(found)

	document, err := json.Marshal(map[string]any{
		"codes":    found,
		"variants": variants,
		"candidate": append(
			[]string{state.Candidate.Headline, state.Candidate.SeniorityBand},
			state.Candidate.CoreStack...,
		),
		"screening": map[string]any{
			"locations":     current.Locations,
			"roles":         current.Roles,
			"companies":     current.Companies,
			"languages":     current.Apply.ResumeLanguages,
			"operatorNotes": current.OperatorNotes,
		},
	})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(document)
	return hex.EncodeToString(digest[:])[:16]
}

func (s *Service) ScreenKey(ctx context.Context) string {
	state := s.profiles.State(ctx)
	if state.Error != "" || state.Candidate == nil {
		return ""
	}
	current, err := s.settings.Load(ctx)
	if err != nil {
		return ""
	}
	return fingerprint(state, current)
}

func (s *Service) PromptInput(ctx context.Context) (PromptInput, error) {
	at, err := s.desk(ctx)
	if err != nil {
		return PromptInput{}, err
	}
	return PromptInput{
		Candidate: *at.state.Candidate,
		Resumes:   at.state.Resumes,
		Rules:     at.rules,
		Notes:     at.notes,
	}, nil
}

func (s *Service) Notes(ctx context.Context, scope string, limit int) ([]Note, error) {
	rows, err := s.queries.ScreeningNotes(ctx, screeningdb.ScreeningNotesParams{
		Scope: scope,
		Limit: int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("read standing notes: %w", err)
	}
	notes := make([]Note, 0, len(rows))
	for _, row := range rows {
		notes = append(notes, Note{
			ID:       row.ID,
			Scope:    row.Scope,
			Text:     row.Body,
			Evidence: row.Evidence,
			Priority: row.Priority,
			Created:  row.CreatedAt,
			Updated:  row.UpdatedAt,
		})
	}
	return notes, nil
}

func (s *Service) Screen(ctx context.Context, limit int) (Result, error) {
	at, err := s.desk(ctx)
	if err != nil {
		return Result{}, err
	}
	result := Result{ScreenKey: at.screenKey, Verdicts: []Judged{}}
	if at.rules.Settings.Apply.Paused {
		result.Paused = true
		return result, nil
	}

	budget := at.rules.Settings.Budget.MaxScreenPerCycle
	if limit > 0 {
		budget = limit
	}

	gone, err := s.invalidateMissingResumes(ctx, codes(at.state.Resumes))
	if err != nil {
		return Result{}, err
	}
	if gone.Cleared > 0 {
		result.ResumeCodesGone = &gone
	}
	if result.Pruned, err = s.prune(ctx, at.rules.Settings.Budget.RetentionDays); err != nil {
		return Result{}, err
	}
	if result.DescriptionsGivenUp, err = s.expireDescriptions(ctx); err != nil {
		return Result{}, err
	}

	engaged, err := s.engagedCompanies(ctx, at.rules.Settings.Companies.ReapplyCooldownDays)
	if err != nil {
		return Result{}, err
	}

	pending, err := s.untriaged(ctx, budget)
	if err != nil {
		return Result{}, err
	}
	failure := ""
	if len(pending) > 0 {
		failure = s.runTriage(ctx, at, pending, engaged, &result)
	}

	ready, err := s.readyForDeep(ctx, budget)
	if err != nil {
		return Result{}, err
	}
	if len(ready) > 0 && failure == "" {
		failure = s.runDeep(ctx, at, ready, &result)
	}

	awaiting, err := s.awaitingDescription(ctx, budget)
	if err != nil {
		return Result{}, err
	}
	result.AwaitingDescription = len(awaiting)
	if failure != "" {
		result.Error = failure
		s.hub.Emit(events.TypeError, map[string]any{"message": failure})
	}

	s.hub.Emit(events.TypeScreen, map[string]any{
		"phase":               "done",
		"triaged":             result.Triaged,
		"screened":            result.Screened,
		"picked":              result.Picked,
		"manual":              result.Manual,
		"skipped":             result.Skipped,
		"flagged":             result.Flagged,
		"corrected":           result.Corrected,
		"awaitingDescription": result.AwaitingDescription,
		"stopped":             result.Stopped,
	})
	return result, nil
}

func (s *Service) gate(
	ctx context.Context,
	at desk,
	pending []Candidate,
	engaged map[string]Engaged,
	result *Result,
) ([]Candidate, string) {
	batch := make([]Candidate, 0, len(pending))
	for _, posting := range pending {
		gated := hardGate(posting, at.rules, engaged)
		if gated == nil {
			batch = append(batch, posting)
			continue
		}
		judged := Judged{
			ID:     posting.ID,
			Status: gated.Status,
			Stage:  StageTriage,
			Reason: gated.Reason,
		}
		if err := s.commitGate(ctx, judged, at.screenKey); err != nil {
			return nil, err.Error()
		}
		result.Verdicts = append(result.Verdicts, judged)
		result.Gated++
	}
	return batch, ""
}

func (s *Service) runTriage(
	ctx context.Context,
	at desk,
	pending []Candidate,
	engaged map[string]Engaged,
	result *Result,
) string {
	batch, failure := s.gate(ctx, at, pending, engaged, result)
	if failure != "" {
		return failure
	}

	s.hub.Emit(events.TypeScreen, map[string]any{
		"phase": "triage", "done": 0, "total": len(batch),
	})
	system := BuildTriagePrompt(PromptInput{
		Candidate: *at.state.Candidate,
		Resumes:   at.state.Resumes,
		Rules:     at.rules,
		Notes:     at.notes,
	})

	for offset := 0; offset < len(batch); offset += TriageBatch {
		stopped, failure := s.stopAtCeiling(ctx, at, result)
		if failure != "" {
			return failure
		}
		if stopped {
			break
		}
		slice := batch[offset:min(offset+TriageBatch, len(batch))]
		answered, err := s.call(ctx, claude.Call{
			Purpose: PurposeTriage,
			System:  system,
			User:    buildTriageUser(slice),
			Tool:    triageTool(),
		}, result)
		if err != nil {
			return why(err)
		}

		var decoded struct {
			Decisions []triaged `json:"decisions"`
		}
		if err := json.Unmarshal(answered.Input, &decoded); err != nil {
			return "the triage answer could not be read"
		}
		if failure := s.commitTriaged(ctx, at, slice, decoded.Decisions, result); failure != "" {
			return failure
		}
		s.hub.Emit(events.TypeScreen, map[string]any{
			"phase": "triage",
			"done":  min(offset+len(slice), len(batch)),
			"total": len(batch),
		})
	}
	result.Triaged = result.Kept + result.DroppedAtTriage + result.Gated
	return ""
}

type triaged struct {
	ID      string `json:"id"`
	Keep    bool   `json:"keep"`
	Promise int    `json:"promise"`
	Reason  string `json:"reason"`
}

func (s *Service) commitTriaged(
	ctx context.Context,
	at desk,
	slice []Candidate,
	decisions []triaged,
	result *Result,
) string {
	known := make(map[string]struct{}, len(slice))
	for _, posting := range slice {
		known[posting.ID] = struct{}{}
	}
	handled := make(map[string]struct{}, len(slice))
	for _, decision := range decisions {
		if _, present := known[decision.ID]; !present {
			continue
		}
		if _, done := handled[decision.ID]; done {
			continue
		}
		handled[decision.ID] = struct{}{}
		if failure := s.commitOne(ctx, at, decision, result); failure != "" {
			return failure
		}
	}
	return ""
}

func (s *Service) commitOne(
	ctx context.Context,
	at desk,
	decision triaged,
	result *Result,
) string {
	reason := reasonOf(decision.Reason)
	fallback := 10
	if decision.Keep {
		fallback = 50
	}
	promise := scoreOf(decision.Promise, fallback)

	if decision.Keep {
		if err := s.commitTriage(ctx, decision.ID, promise, reason, at.screenKey); err != nil {
			return err.Error()
		}
		result.Kept++
		return ""
	}
	judged := Judged{
		ID:     decision.ID,
		Status: postings.StatusSkipped,
		Stage:  StageTriage,
		Score:  min(promise, 40),
		Reason: reason,
	}
	if err := s.commitGate(ctx, judged, at.screenKey); err != nil {
		return err.Error()
	}
	result.Verdicts = append(result.Verdicts, judged)
	result.DroppedAtTriage++
	return ""
}

func (s *Service) runDeep(ctx context.Context, at desk, ready []Candidate, result *Result) string {
	available := codes(at.state.Resumes)
	byCode := make(map[string]indexer.ResumeState, len(at.state.Resumes))
	for _, resume := range at.state.Resumes {
		byCode[strings.ToUpper(resume.Code)] = resume
	}
	input := PromptInput{
		Candidate: *at.state.Candidate,
		Resumes:   at.state.Resumes,
		Rules:     at.rules,
		Notes:     at.notes,
	}
	system := BuildDeepPrompt(input)
	tool := deepTool(available, at.rules.Languages)

	s.hub.Emit(events.TypeScreen, map[string]any{
		"phase": "deep", "done": 0, "total": len(ready),
	})

	for offset := 0; offset < len(ready); offset += DeepBatch {
		stopped, failure := s.stopAtCeiling(ctx, at, result)
		if failure != "" {
			return failure
		}
		if stopped {
			break
		}
		slice := ready[offset:min(offset+DeepBatch, len(ready))]
		answered, err := s.call(ctx, claude.Call{
			Purpose: PurposeDeep,
			System:  system,
			User:    buildDeepUser(slice),
			Tool:    tool,
		}, result)
		if err != nil {
			return why(err)
		}
		decisions, err := decodeDecisions(answered.Input)
		if err != nil {
			return err.Error()
		}

		judged := make(map[string]Judged, len(slice))
		for _, posting := range slice {
			decision, present := decisions[posting.ID]
			if !present {
				continue
			}
			verdict := shape(decision, byCode, at.rules, at.state.Resumes)
			routed := route(posting, verdict, at.rules)
			entry := Judged{
				ID:      posting.ID,
				Status:  routed.Status,
				Stage:   StageDeep,
				Score:   scoreOf(verdict.Score, applyFallback(verdict)),
				Reason:  routed.Reason,
				Verdict: verdict,
				TraceID: answered.TraceID,
			}
			if err := s.commitVerdict(ctx, entry, at.screenKey); err != nil {
				return err.Error()
			}
			judged[posting.ID] = entry
		}

		corrected := s.correct(ctx, at, input, slice, judged, tool, result)
		for _, posting := range slice {
			entry, present := corrected[posting.ID]
			if !present {
				continue
			}
			result.Verdicts = append(result.Verdicts, entry)
			result.Screened++
			switch entry.Status {
			case postings.StatusInbox:
				result.Picked++
			case postings.StatusManual:
				result.Manual++
			default:
				result.Skipped++
			}
		}
		s.hub.Emit(events.TypeScreen, map[string]any{
			"phase": "deep",
			"done":  min(offset+len(slice), len(ready)),
			"total": len(ready),
		})
	}
	return ""
}

func applyFallback(verdict Verdict) int {
	if verdict.Verdict == VerdictApply {
		return 50
	}
	return 0
}

func decodeDecisions(input json.RawMessage) (map[string]Verdict, error) {
	var decoded struct {
		Decisions []struct {
			ID string `json:"id"`
			Verdict
			StatedPay struct {
				Present  bool   `json:"present"`
				Amount   int    `json:"amount"`
				Currency string `json:"currency"`
				Period   string `json:"period"`
			} `json:"statedPay"`
		} `json:"decisions"`
	}
	if err := json.Unmarshal(input, &decoded); err != nil {
		return nil, refuse("the screening answer could not be read")
	}
	decisions := make(map[string]Verdict, len(decoded.Decisions))
	for _, decision := range decoded.Decisions {
		if decision.ID == "" {
			continue
		}
		if _, present := decisions[decision.ID]; present {
			continue
		}
		verdict := decision.Verdict
		verdict.StatedPay = nil
		if decision.StatedPay.Present {
			verdict.StatedPay = &Pay{
				Amount:   decision.StatedPay.Amount,
				Currency: decision.StatedPay.Currency,
				Period:   decision.StatedPay.Period,
			}
		}
		decisions[decision.ID] = verdict
	}
	return decisions, nil
}

func shape(
	raw Verdict,
	byCode map[string]indexer.ResumeState,
	rules Rules,
	resumes []indexer.ResumeState,
) Verdict {
	resume, present := byCode[strings.ToUpper(strings.TrimSpace(raw.ResumeCode))]
	if !present {
		resume = resumes[0]
	}
	shaped := Verdict{
		Verdict:      VerdictSkip,
		Reason:       reasonOf(raw.Reason),
		Score:        scoreOf(raw.Score, applyFallback(raw)),
		Seniority:    choice(raw.Seniority, settings.SeniorityValues, settings.SeniorityUnknown),
		Workplace:    choice(raw.Workplace, Workplaces, WorkplaceUnclear),
		ContractType: choice(raw.ContractType, ContractTypes, ContractUnclear),
		Agency:       raw.Agency,
		StatedPay:    payOf(raw.StatedPay),
		ResumeCode:   resume.Code,
		ResumeLang:   resumeLanguage(raw.ResumeLang, resume, rules.Languages),
		ResumeFit:    choice(raw.ResumeFit, Fits, FitPartial),
		Tailored:     tailoredOf(raw.Tailored),
	}
	if raw.Verdict == VerdictApply {
		shaped.Verdict = VerdictApply
	}
	if code := languageCode(raw.PostingLang); code != nil {
		shaped.PostingLang = *code
	}
	return shaped
}

func resumeLanguage(requested string, resume indexer.ResumeState, allowed []string) string {
	wanted := strings.ToLower(strings.TrimSpace(requested))
	if slices.Contains(allowed, wanted) && slices.Contains(resume.FileLanguages, wanted) {
		return wanted
	}
	for _, language := range allowed {
		if slices.Contains(resume.FileLanguages, language) {
			return language
		}
	}
	if len(resume.FileLanguages) > 0 {
		return resume.FileLanguages[0]
	}
	return ""
}

func route(posting Candidate, verdict Verdict, rules Rules) Routed {
	if verdict.Verdict != VerdictApply {
		return Routed{Status: postings.StatusSkipped, Reason: verdict.Reason}
	}
	return routeApply(posting, verdict, rules)
}

func why(err error) string {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "the model call failed with no message"
	}
	return line(strings.SplitN(message, "\n", 2)[0], 200)
}
