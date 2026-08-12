package screening

import (
	"context"
	"fmt"
	"strings"
	"time"

	"api/internal/app/claude"
	"api/internal/app/clock"
	"api/internal/app/events"
	"api/internal/routes/postings"
	screeningdb "api/internal/routes/screening/gen"
)

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (s *Service) call(
	ctx context.Context,
	request claude.Call,
	result *Result,
) (claude.Result, error) {
	answered, err := s.claude.CallTool(ctx, request)
	if err != nil {
		return claude.Result{}, err
	}
	if result != nil {
		result.Nudges += answered.Nudges
	}
	if err := s.queries.BumpModelUsage(ctx, screeningdb.BumpModelUsageParams{
		Day:          s.clock.Now().UTC().Format(time.DateOnly),
		Calls:        1,
		InputTokens:  answered.Usage.InputTokens,
		OutputTokens: answered.Usage.OutputTokens,
	}); err != nil {
		return claude.Result{}, fmt.Errorf("record the model call: %w", err)
	}
	if s.rounds != nil {
		if err := s.rounds.ChargeRound(
			ctx,
			answered.Usage.InputTokens,
			answered.Usage.OutputTokens,
		); err != nil {
			return claude.Result{}, err
		}
	}
	return answered, nil
}

func (s *Service) untriaged(ctx context.Context, limit int) ([]Candidate, error) {
	rows, err := s.queries.Untriaged(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("read the unjudged postings: %w", err)
	}
	found := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		found = append(found, candidateOf(
			row.ID, row.Title, row.Company, row.Location, row.URL,
			row.EasyApply, row.ListedAt, row.Description, row.TriageReason,
		))
	}
	return found, nil
}

func (s *Service) readyForDeep(ctx context.Context, limit int) ([]Candidate, error) {
	rows, err := s.queries.ReadyForDeepScreen(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("read the postings ready for screening: %w", err)
	}
	found := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		found = append(found, candidateOf(
			row.ID, row.Title, row.Company, row.Location, row.URL,
			row.EasyApply, row.ListedAt, row.Description, row.TriageReason,
		))
	}
	return found, nil
}

func (s *Service) awaitingDescription(ctx context.Context, limit int) ([]Candidate, error) {
	rows, err := s.queries.AwaitingDescription(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("read the postings waiting for a description: %w", err)
	}
	found := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		found = append(found, candidateOf(
			row.ID, row.Title, row.Company, row.Location, row.URL,
			row.EasyApply, row.ListedAt, row.Description, row.TriageReason,
		))
	}
	return found, nil
}

func candidateOf(
	id, title, company, location, url string,
	easyApply bool,
	listedAt *int64,
	description, triageReason *string,
) Candidate {
	posting := Candidate{
		ID:        id,
		Title:     title,
		Company:   company,
		Location:  location,
		URL:       url,
		EasyApply: easyApply,
		ListedAt:  listedAt,
	}
	if description != nil {
		posting.Description = *description
	}
	if triageReason != nil {
		posting.TriageReason = *triageReason
	}
	return posting
}

func (s *Service) engagedCompanies(
	ctx context.Context,
	cooldownDays int,
) (map[string]Engaged, error) {
	since := s.clock.Now().Add(-time.Duration(cooldownDays) * 24 * time.Hour).
		UTC().Format(time.RFC3339Nano)
	rows, err := s.queries.EngagedCompaniesSince(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("read the companies already worked on: %w", err)
	}
	engaged := make(map[string]Engaged, len(rows))
	for _, row := range rows {
		engaged[row.CompanyKey] = Engaged{
			Company: row.Company,
			At:      row.EngagedAt,
			ByHand:  row.Status == postings.StatusOpened,
		}
	}
	return engaged, nil
}

func (s *Service) emitVerdict(judged Judged) {
	s.hub.Emit(events.TypeScreen, map[string]any{
		"phase":  "verdict",
		"id":     judged.ID,
		"status": judged.Status,
		"stage":  judged.Stage,
		"score":  judged.Score,
		"reason": judged.Reason,
	})
}

func (s *Service) commitGate(ctx context.Context, judged Judged, screenKey string) error {
	score := int64(judged.Score)
	if _, err := s.queries.SetVerdict(ctx, screeningdb.SetVerdictParams{
		Status:        judged.Status,
		Stage:         judged.Stage,
		Score:         &score,
		VerdictReason: optional(judged.Reason),
		ScreenKey:     optional(screenKey),
		UpdatedAt:     clock.Stamp(s.clock),
		ID:            judged.ID,
	}); err != nil {
		return fmt.Errorf("store the verdict: %w", err)
	}
	s.emitVerdict(judged)
	return nil
}

func (s *Service) commitTriage(
	ctx context.Context,
	id string,
	promise int,
	reason, screenKey string,
) error {
	score := int64(promise)
	if _, err := s.queries.SetTriage(ctx, screeningdb.SetTriageParams{
		Score:        &score,
		TriageReason: optional(reason),
		ScreenKey:    optional(screenKey),
		UpdatedAt:    clock.Stamp(s.clock),
		ID:           id,
	}); err != nil {
		return fmt.Errorf("store the triage decision: %w", err)
	}
	s.hub.Emit(events.TypeScreen, map[string]any{
		"phase":  "reading",
		"id":     id,
		"score":  promise,
		"reason": reason,
	})
	return nil
}

func (s *Service) commitVerdict(ctx context.Context, judged Judged, screenKey string) error {
	verdict := judged.Verdict
	score := int64(judged.Score)
	params := screeningdb.SetVerdictParams{
		Status:         judged.Status,
		Stage:          judged.Stage,
		Score:          &score,
		VerdictReason:  optional(judged.Reason),
		Seniority:      optional(verdict.Seniority),
		Workplace:      optional(verdict.Workplace),
		ContractType:   optional(verdict.ContractType),
		PostingLang:    optional(verdict.PostingLang),
		Agency:         &verdict.Agency,
		ResumeCode:     optional(verdict.ResumeCode),
		ResumeLang:     optional(verdict.ResumeLang),
		ResumeFit:      optional(verdict.ResumeFit),
		TailoredNeeded: &verdict.Tailored.Needed,
		TailoredFocus:  optional(verdict.Tailored.Focus),
		ScreenKey:      optional(screenKey),
		UpdatedAt:      clock.Stamp(s.clock),
		ID:             judged.ID,
	}
	if verdict.StatedPay != nil {
		amount := int64(verdict.StatedPay.Amount)
		params.PayAmount = &amount
		params.PayCurrency = optional(verdict.StatedPay.Currency)
		params.PayPeriod = optional(verdict.StatedPay.Period)
	}
	if _, err := s.queries.SetVerdict(ctx, params); err != nil {
		return fmt.Errorf("store the verdict: %w", err)
	}
	s.emitVerdict(judged)
	return nil
}

func (s *Service) invalidateMissingResumes(ctx context.Context, available []string) (Gone, error) {
	live, err := s.queries.LiveResumeCodes(ctx)
	if err != nil {
		return Gone{}, fmt.Errorf("read the resume codes in use: %w", err)
	}
	known := make(map[string]struct{}, len(available))
	for _, code := range available {
		known[strings.ToUpper(code)] = struct{}{}
	}
	gone := Gone{Codes: []string{}}
	at := clock.Stamp(s.clock)
	for _, stored := range live {
		if stored == nil || *stored == "" {
			continue
		}
		if _, present := known[strings.ToUpper(*stored)]; present {
			continue
		}
		cleared, err := s.queries.ClearVerdictForResumeCode(
			ctx,
			screeningdb.ClearVerdictForResumeCodeParams{UpdatedAt: at, ResumeCode: stored},
		)
		if err != nil {
			return Gone{}, fmt.Errorf("unscreen the postings holding a missing resume: %w", err)
		}
		gone.Codes = append(gone.Codes, *stored)
		gone.Cleared += int(cleared)
	}
	if gone.Cleared > 0 {
		s.hub.Emit(events.TypeJobs, map[string]any{
			"updated": gone.Cleared,
			"reason":  "resume codes " + strings.Join(gone.Codes, ", ") + " are no longer on disk",
		})
	}
	return gone, nil
}

func (s *Service) prune(ctx context.Context, retentionDays int) (int, error) {
	cutoff := s.clock.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).
		UTC().Format(time.RFC3339Nano)
	pruned, err := s.queries.PruneOldJobs(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune the postings past the retention window: %w", err)
	}
	return int(pruned), nil
}

func (s *Service) expireDescriptions(ctx context.Context) (int, error) {
	cutoff := s.clock.Now().Add(-s.config.DescriptionWindow).UTC().Format(time.RFC3339Nano)
	expired, err := s.queries.ExpireDescriptions(ctx, screeningdb.ExpireDescriptionsParams{
		UpdatedAt:   clock.Stamp(s.clock),
		UpdatedAt_2: cutoff,
	})
	if err != nil {
		return 0, fmt.Errorf("give up on the descriptions nobody sent: %w", err)
	}
	return int(expired), nil
}

func (s *Service) Pending(ctx context.Context, limit int) ([]Pending, error) {
	if limit <= 0 {
		limit = s.config.PendingLimit
	}
	found, err := s.awaitingDescription(ctx, limit)
	if err != nil {
		return nil, err
	}
	wanted := make([]Pending, 0, len(found))
	for _, posting := range found {
		wanted = append(wanted, Pending{
			ID:      posting.ID,
			URL:     posting.URL,
			Title:   posting.Title,
			Company: posting.Company,
		})
	}
	return wanted, nil
}

type Description struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

func (s *Service) RecordDescriptions(
	ctx context.Context,
	entries []Description,
) (Stored, error) {
	stored := Stored{}
	at := clock.Stamp(s.clock)
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == "" {
			continue
		}
		text := strings.TrimSpace(entry.Description)
		params := screeningdb.PutDescriptionParams{
			DescriptionState: "missing",
			UpdatedAt:        at,
			ID:               entry.ID,
		}
		if text != "" {
			params.Description = &text
			params.DescriptionState = "ok"
			params.DescribedAt = &at
		}
		changed, err := s.queries.PutDescription(ctx, params)
		if err != nil {
			return Stored{}, fmt.Errorf("store the posting body: %w", err)
		}
		switch {
		case changed == 0:
			stored.Unknown++
		case text != "":
			stored.Stored++
		default:
			stored.Missing++
		}
	}
	if stored.Stored+stored.Missing > 0 {
		s.hub.Emit(events.TypeJobs, map[string]any{
			"updated": stored.Stored + stored.Missing,
			"reason":  "posting bodies arrived",
		})
	}
	return stored, nil
}

func (s *Service) Staleness(ctx context.Context) (Staleness, error) {
	at, err := s.desk(ctx)
	if err != nil {
		return Staleness{}, err
	}
	stale, err := s.queries.CountStale(ctx, optional(at.screenKey))
	if err != nil {
		return Staleness{}, fmt.Errorf("count the stale postings: %w", err)
	}
	return Staleness{ScreenKey: at.screenKey, Stale: int(stale)}, nil
}

func (s *Service) Rescreen(ctx context.Context) (Rescreened, error) {
	at, err := s.desk(ctx)
	if err != nil {
		return Rescreened{}, err
	}
	reset, err := s.queries.ResetStale(ctx, screeningdb.ResetStaleParams{
		UpdatedAt: clock.Stamp(s.clock),
		ScreenKey: optional(at.screenKey),
	})
	if err != nil {
		return Rescreened{}, fmt.Errorf("unscreen the stale postings: %w", err)
	}
	if reset > 0 {
		s.hub.Emit(events.TypeJobs, map[string]any{
			"updated": int(reset),
			"reason":  "stale verdicts were sent back for re-screening",
		})
	}
	return Rescreened{Reset: int(reset)}, nil
}

func (s *Service) Move(ctx context.Context, id, status string) (postings.Posting, error) {
	changed, err := s.queries.SetStatus(ctx, screeningdb.SetStatusParams{
		Status:    status,
		UpdatedAt: clock.Stamp(s.clock),
		ID:        id,
	})
	if err != nil {
		return postings.Posting{}, fmt.Errorf("move the posting: %w", err)
	}
	if changed == 0 {
		return postings.Posting{}, postings.ErrNoPosting
	}
	moved, err := s.Posting(ctx, id)
	if err != nil {
		return postings.Posting{}, err
	}
	s.hub.Emit(events.TypeJobs, map[string]any{
		"updated": 1,
		"id":      moved.ID,
		"status":  moved.Status,
	})
	return moved, nil
}

func (s *Service) Posting(ctx context.Context, id string) (postings.Posting, error) {
	return s.postings.Get(ctx, id)
}
