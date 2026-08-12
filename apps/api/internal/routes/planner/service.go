package planner

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"api/internal/app/claude"
	"api/internal/app/clock"
	"api/internal/app/events"
	"api/internal/app/store"
	"api/internal/routes/indexer"
	plannerdb "api/internal/routes/planner/gen"
	"api/internal/routes/plugin"
	"api/internal/routes/postings"
	"api/internal/routes/settings"
)

type Config struct {
	HistoryRounds    int
	NoteCap          int
	KnownIDWindow    time.Duration
	KnownIDCap       int
	OperationTimeout time.Duration
	PlanBudget       time.Duration
}

type Digest interface {
	RecentJudgedIDs(ctx context.Context, window time.Duration, limit int) ([]string, error)
}

type Dependencies struct {
	Reads        store.DBTX
	Transactions store.TransactionRunner
	Claude       *claude.Client
	Settings     *settings.Service
	Plugin       *plugin.Service
	Profiles     *indexer.Service
	Digest       Digest
	Hub          *events.Hub
	Clock        clock.Clock
	Config       Config
}

type Service struct {
	queries      *plannerdb.Queries
	transactions store.TransactionRunner
	claude       *claude.Client
	settings     *settings.Service
	plugin       *plugin.Service
	profiles     *indexer.Service
	digest       Digest
	hub          *events.Hub
	clock        clock.Clock
	config       Config
}

func NewService(dependencies Dependencies) *Service {
	config := dependencies.Config
	if config.HistoryRounds <= 0 {
		config.HistoryRounds = HistoryRounds
	}
	if config.NoteCap <= 0 {
		config.NoteCap = NoteCap
	}
	return &Service{
		queries:      plannerdb.New(dependencies.Reads),
		transactions: dependencies.Transactions,
		claude:       dependencies.Claude,
		settings:     dependencies.Settings,
		plugin:       dependencies.Plugin,
		profiles:     dependencies.Profiles,
		digest:       dependencies.Digest,
		hub:          dependencies.Hub,
		clock:        dependencies.Clock,
		config:       config,
	}
}

type desk struct {
	settings settings.Settings
	plugin   plugin.State
	profile  indexer.State
}

func (s *Service) desk(ctx context.Context) (desk, error) {
	current, err := s.settings.Load(ctx)
	if err != nil {
		return desk{}, err
	}
	state, err := s.plugin.State(ctx)
	if err != nil {
		return desk{}, err
	}
	return desk{settings: current, plugin: state, profile: s.profiles.State(ctx)}, nil
}

func (s *Service) gate(ctx context.Context, at desk) error {
	if at.settings.Apply.Paused {
		return refuse(ReasonPaused,
			"the agent is paused. nothing is planned, screened or attached until you switch it back on.",
		)
	}
	if !at.plugin.EverSeen {
		return refuse(ReasonExecutorOffline,
			"no browser extension has ever checked in, so there is nothing to run a round. planning spends no model call until one does.",
		)
	}
	if !at.plugin.Connected {
		return refuse(ReasonExecutorOffline,
			"the browser extension last checked in "+ago(at.plugin.LastSeen, s.clock)+
				". planning stays off until it checks in again.",
		)
	}
	if at.profile.Candidate == nil {
		return refuse(ReasonNoProfile,
			"the cv folder is not indexed yet, so there is no candidate to plan a round for.",
		)
	}
	budget, err := s.budget(ctx, at.settings)
	if err != nil {
		return err
	}
	if budget.Remaining <= 0 {
		return refuse(ReasonDailyCap,
			itoa(budget.Used)+" of "+itoa(budget.Cap)+
				" model calls used today. the round is off until the count resets.",
		)
	}
	round, err := s.roundBudget(ctx, at.settings)
	if err != nil {
		return err
	}
	if round.Reached() {
		return refuse(ReasonRoundCap,
			"round "+itoa(int(round.CycleID))+" has spent all "+itoa(round.Cap)+
				" model calls it may. the next round starts once this one closes.",
		)
	}
	return nil
}

func (s *Service) budget(ctx context.Context, current settings.Settings) (Budget, error) {
	day := s.clock.Now().UTC().Format(time.DateOnly)
	usage, err := s.queries.ModelUsage(ctx, day)
	if errors.Is(err, sql.ErrNoRows) {
		usage = plannerdb.ModelUsage{Day: day}
	} else if err != nil {
		return Budget{}, fmt.Errorf("read the model call count: %w", err)
	}
	ceiling := current.Budget.DailyModelCallCap
	used := int(usage.Calls)
	if ceiling <= 0 {
		return Budget{Used: used, Cap: 0, Remaining: math.MaxInt32, Day: day}, nil
	}
	return Budget{Used: used, Cap: ceiling, Remaining: max(0, ceiling-used), Day: day}, nil
}

func (s *Service) roundBudget(
	ctx context.Context,
	current settings.Settings,
) (RoundBudget, error) {
	ceiling := current.Budget.MaxModelCallsPerCycle
	spend, err := s.queries.OpenCycleSpend(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return RoundBudget{Cap: ceiling, Remaining: math.MaxInt32}, nil
	}
	if err != nil {
		return RoundBudget{}, fmt.Errorf("read the round's model call count: %w", err)
	}
	used := int(spend.ModelCalls)
	budget := RoundBudget{CycleID: spend.ID, Open: true, Used: used, Cap: ceiling}
	budget.Remaining = math.MaxInt32
	if ceiling > 0 {
		budget.Remaining = max(0, ceiling-used)
	}
	return budget, nil
}

func (s *Service) OpenRoundCalls(ctx context.Context) (int, bool, error) {
	spend, err := s.queries.OpenCycleSpend(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read the round's model call count: %w", err)
	}
	return int(spend.ModelCalls), true, nil
}

func (s *Service) ChargeRound(ctx context.Context, inputTokens, outputTokens int64) error {
	spend, err := s.queries.OpenCycleSpend(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the round's model call count: %w", err)
	}
	if err := s.queries.BumpCycle(ctx, plannerdb.BumpCycleParams{
		ModelCalls:   1,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		ID:           spend.ID,
	}); err != nil {
		return fmt.Errorf("charge the model call to the round: %w", err)
	}
	return nil
}

func (s *Service) planContext(at desk) planContext {
	knobs := at.plugin.Capabilities.Fetch
	return planContext{
		capabilities:  *at.plugin.Capabilities,
		maxQueries:    at.settings.Budget.MaxQueriesPerCycle,
		screenBudget:  at.settings.Budget.MaxScreenPerCycle,
		easyApplyOnly: at.settings.Roles.EasyApplyOnly,
		maxWant: min(
			knobs.PageSize*knobs.MaxPagesPerQuery,
			max(at.settings.Harvest.NewJobTarget*2, 25),
		),
	}
}

func (s *Service) promptInput(ctx context.Context, at desk, screenBudget int) (PromptInput, error) {
	recent, err := s.history(ctx, s.config.HistoryRounds*3)
	if err != nil {
		return PromptInput{}, err
	}
	history := aimedRounds(recent, s.config.HistoryRounds)
	notes, err := s.notesInScope(ctx)
	if err != nil {
		return PromptInput{}, err
	}
	return PromptInput{
		Candidate:    *at.profile.Candidate,
		Resumes:      at.profile.Resumes,
		Settings:     at.settings,
		Plugin:       at.plugin,
		History:      history,
		Notes:        notes,
		ScreenBudget: screenBudget,
	}, nil
}

func aimedRounds(rounds []Round, limit int) []Round {
	kept := make([]Round, 0, limit)
	for _, round := range rounds {
		if round.TerminationReason != nil && *round.TerminationReason == ReasonPlannerFailed {
			continue
		}
		kept = append(kept, round)
		if len(kept) >= limit {
			break
		}
	}
	return kept
}

func (s *Service) Preview(ctx context.Context) (string, *RefusalError, error) {
	at, err := s.desk(ctx)
	if err != nil {
		return "", nil, err
	}
	if at.profile.Candidate == nil {
		return "", refuse(ReasonNoProfile,
			"the cv folder is not indexed yet, so there is no candidate to write a prompt about.",
		), nil
	}
	if !at.plugin.EverSeen {
		return "", refuse(ReasonExecutorOffline,
			"no browser extension has checked in, so the executor's real capabilities are unknown and the planning prompt cannot be assembled.",
		), nil
	}
	input, err := s.promptInput(ctx, at, at.settings.Budget.MaxScreenPerCycle)
	if err != nil {
		return "", nil, err
	}
	return BuildPlanningPrompt(input), nil, nil
}

func (s *Service) Start(ctx context.Context) (Plan, error) {
	at, err := s.desk(ctx)
	if err != nil {
		return Plan{}, err
	}
	if err := s.gate(ctx, at); err != nil {
		return Plan{}, err
	}

	if open, found, err := s.currentRound(ctx); err != nil {
		return Plan{}, err
	} else if found {
		if !s.expired(open.StartedAt, at.settings.Harvest.RoundDeadlineMs) {
			plan, err := s.executorPlan(ctx, open, at)
			plan.Resumed = true
			return plan, err
		}
		if err := s.close(ctx, open.ID, StatusDone,
			"deadline passed, the executor never reported back", nil); err != nil {
			return Plan{}, err
		}
	}

	context := s.planContext(at)
	input, err := s.promptInput(ctx, at, context.screenBudget)
	if err != nil {
		return Plan{}, err
	}
	known, err := s.knownIDs(ctx)
	if err != nil {
		return Plan{}, err
	}

	result, err := s.claude.CallTool(ctx, claude.Call{
		Purpose: PurposePlan,
		System:  BuildPlanningPrompt(input),
		User:    buildPlanningUser(at.settings, len(known)),
		Tool:    planTool(context),
	})
	if err != nil {
		return Plan{}, s.plannerFailed("the planner call failed: " + why(err))
	}
	if err := s.recordModelCall(ctx, result); err != nil {
		return Plan{}, err
	}

	var answered modelPlan
	if err := json.Unmarshal(result.Input, &answered); err != nil {
		return Plan{}, s.plannerFailed("the planner answered with input this round cannot read")
	}
	queries, rejected := compileQueries(answered.Queries, context)
	if len(queries) == 0 {
		message := "the planner answered with no query at all, so this round had nothing to run"
		if len(rejected) > 0 {
			message = "every query the planner wrote was rejected: " +
				strings.Join(reasonsOf(rejected), "; ")
		}
		if err := s.recordEmptyRound(ctx, answered, result, message); err != nil {
			return Plan{}, err
		}
		return Plan{}, s.plannerFailed(message)
	}

	round, err := s.open(ctx, answered, queries, rejected, context, result)
	if err != nil {
		return Plan{}, err
	}
	curated, err := s.applyNotes(ctx, answered.Notes, round.ID)
	if err != nil {
		return Plan{}, err
	}

	labels := make([]string, 0, len(queries))
	for _, query := range queries {
		labels = append(labels, query.Label)
	}
	s.hub.Emit(events.TypeCycle, map[string]any{
		"phase":        "planned",
		"cycleId":      round.ID,
		"queries":      labels,
		"screenBudget": round.ScreenBudget,
		"rationale":    round.Rationale,
	})

	plan, err := s.executorPlan(ctx, round, at)
	plan.Rejected = reasonsOf(rejected)
	plan.Notes = &curated
	return plan, err
}

func (s *Service) recordEmptyRound(
	ctx context.Context,
	answered modelPlan,
	result claude.Result,
	message string,
) error {
	round, err := s.open(ctx, answered, nil, nil, planContext{screenBudget: 1}, result)
	if err != nil {
		return err
	}
	failure := message
	return s.close(ctx, round.ID, StatusFailed, ReasonPlannerFailed, &failure)
}

func (s *Service) open(
	ctx context.Context,
	answered modelPlan,
	queries []Query,
	rejected []Rejection,
	context planContext,
	result claude.Result,
) (Round, error) {
	startedAt := clock.Stamp(s.clock)
	budget := clamp(answered.ScreenBudget, 1, context.screenBudget, context.screenBudget)

	var cycleID int64
	err := s.transactions.WithTx(ctx, func(tx *sql.Tx) error {
		queried := s.queries.WithTx(tx)
		outcome, err := queried.OpenCycle(ctx, plannerdb.OpenCycleParams{
			StartedAt:    startedAt,
			Rationale:    sentence(answered.Rationale, maxRationale, "no rationale given"),
			ScreenBudget: int64(budget),
		})
		if err != nil {
			return fmt.Errorf("open round: %w", err)
		}
		if cycleID, err = outcome.LastInsertId(); err != nil {
			return fmt.Errorf("read the round id: %w", err)
		}
		for position, query := range queries {
			if err := saveQuery(ctx, queried, cycleID, position, query, false); err != nil {
				return err
			}
		}
		for offset, entry := range rejected {
			position := len(queries) + offset
			if err := saveQuery(ctx, queried, cycleID, position, entry.Record, false); err != nil {
				return err
			}
			reason := RejectedBeforeRun + entry.Reason
			if _, err := queried.SkipCycleQuery(ctx, plannerdb.SkipCycleQueryParams{
				SkipReason: &reason,
				CycleID:    cycleID,
				Label:      entry.Record.Label,
			}); err != nil {
				return fmt.Errorf("record the rejected query: %w", err)
			}
		}
		return queried.BumpCycle(ctx, plannerdb.BumpCycleParams{
			ModelCalls:   1,
			InputTokens:  result.Usage.InputTokens,
			OutputTokens: result.Usage.OutputTokens,
			ID:           cycleID,
		})
	})
	if err != nil {
		return Round{}, err
	}
	return s.Round(ctx, cycleID)
}

func saveQuery(
	ctx context.Context,
	queries *plannerdb.Queries,
	cycleID int64,
	position int,
	query Query,
	widened bool,
) error {
	document, err := json.Marshal(query)
	if err != nil {
		return fmt.Errorf("encode round query: %w", err)
	}
	if err := queries.SaveCycleQuery(ctx, plannerdb.SaveCycleQueryParams{
		CycleID:  cycleID,
		Position: int64(position),
		Label:    query.Label,
		Query:    string(document),
		Widened:  widened,
	}); err != nil {
		return fmt.Errorf("store round query: %w", err)
	}
	return nil
}

func (s *Service) executorPlan(ctx context.Context, round Round, at desk) (Plan, error) {
	known, err := s.knownIDs(ctx)
	if err != nil {
		return Plan{}, err
	}
	queries := make([]Query, 0, len(round.Queries))
	for _, entry := range round.Queries {
		queries = append(queries, entry.Query)
	}
	return Plan{
		OK:           true,
		CycleID:      round.ID,
		StartedAt:    round.StartedAt,
		DeadlineAt:   deadlineOf(round.StartedAt, at.settings.Harvest.RoundDeadlineMs),
		NextRunAt:    s.nextRunAt(ctx, at.settings.Budget.CycleMinutes, at.settings.Budget.AutoCycle),
		Rationale:    round.Rationale,
		ScreenBudget: round.ScreenBudget,
		Queries:      queries,
		Pacing: Pacing{
			NewJobTarget:     at.settings.Harvest.NewJobTarget,
			MaxWideningSteps: at.settings.Harvest.MaxWideningSteps,
			RoundDeadlineMs:  at.settings.Harvest.RoundDeadlineMs,
			RequestDelayMs:   at.settings.Harvest.RequestDelayMs,
			RequestTimeoutMs: at.settings.Harvest.RequestTimeoutMs,
			CycleMinutes:     at.settings.Budget.CycleMinutes,
			MaxPagesPerQuery: min(
				at.settings.Budget.MaxPagesPerQuery,
				at.plugin.Capabilities.Fetch.MaxPagesPerQuery,
			),
		},
		Apply: Attach{
			AutoAttach:         at.settings.Apply.AutoAttach,
			UploadFileName:     at.settings.Apply.UploadFileName,
			UploadFileNameMode: at.settings.Apply.UploadFileNameMode,
		},
		KnownIDs: known,
	}, nil
}

func (s *Service) knownIDs(ctx context.Context) ([]string, error) {
	if s.digest == nil {
		return []string{}, nil
	}
	return s.digest.RecentJudgedIDs(ctx, s.config.KnownIDWindow, s.config.KnownIDCap)
}

func (s *Service) recordModelCall(ctx context.Context, result claude.Result) error {
	if err := s.queries.BumpModelUsage(ctx, plannerdb.BumpModelUsageParams{
		Day:          s.clock.Now().UTC().Format(time.DateOnly),
		Calls:        1,
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
	}); err != nil {
		return fmt.Errorf("record the model call: %w", err)
	}
	return nil
}

func (s *Service) plannerFailed(message string) error {
	s.hub.Emit(events.TypeError, map[string]any{"message": message})
	return refuse(ReasonPlannerFailed, message)
}

func why(err error) string {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "the model call failed with no message"
	}
	return clip(strings.SplitN(message, "\n", 2)[0], 200)
}

func ago(lastSeen *string, source clock.Clock) string {
	if lastSeen == nil {
		return "never"
	}
	at, err := time.Parse(time.RFC3339Nano, *lastSeen)
	if err != nil {
		return "never"
	}
	minutes := int(source.Now().Sub(at).Minutes())
	switch {
	case minutes < 1:
		return "under a minute ago"
	case minutes < 60:
		return itoa(minutes) + " minutes ago"
	case minutes < 1440:
		return itoa(minutes/60) + " hours ago"
	}
	return itoa(minutes/1440) + " days ago"
}

func (s *Service) notesInScope(ctx context.Context) ([]Note, error) {
	rows, err := s.queries.NotesInScope(ctx, plannerdb.NotesInScopeParams{
		Scope: Scope,
		Limit: int64(s.config.NoteCap),
	})
	if err != nil {
		return nil, fmt.Errorf("read standing notes: %w", err)
	}
	return notes(rows), nil
}

func (s *Service) RecentNotes(ctx context.Context, limit int) ([]Note, error) {
	rows, err := s.queries.RecentNotes(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("read standing notes: %w", err)
	}
	return notes(rows), nil
}

func notes(rows []plannerdb.Note) []Note {
	kept := make([]Note, 0, len(rows))
	for _, row := range rows {
		kept = append(kept, Note{
			ID:       row.ID,
			Scope:    row.Scope,
			Text:     row.Body,
			Evidence: row.Evidence,
			Priority: row.Priority,
			Created:  row.CreatedAt,
			Updated:  row.UpdatedAt,
		})
	}
	return kept
}

func (s *Service) DeleteNote(ctx context.Context, id int64) (bool, error) {
	removed, err := s.queries.DeleteNote(ctx, id)
	if err != nil {
		return false, fmt.Errorf("delete the standing note: %w", err)
	}
	return removed > 0, nil
}

func (s *Service) applyNotes(
	ctx context.Context,
	operations modelNotes,
	cycleID int64,
) (Curated, error) {
	curated := Curated{}
	at := clock.Stamp(s.clock)

	err := s.transactions.WithTx(ctx, func(tx *sql.Tx) error {
		queries := s.queries.WithTx(tx)
		for _, id := range operations.Drop {
			removed, err := queries.DeleteNote(ctx, id)
			if err != nil {
				return fmt.Errorf("delete the standing note: %w", err)
			}
			curated.Dropped += int(removed)
		}
		for _, entry := range operations.Revise {
			text := sentence(entry.Text, maxReason, "")
			if entry.ID == 0 || text == "" {
				continue
			}
			revised, err := queries.UpdateNote(ctx, plannerdb.UpdateNoteParams{
				Body:         text,
				Priority:     priority(entry.Priority),
				UpdatedCycle: &cycleID,
				UpdatedAt:    at,
				ID:           entry.ID,
			})
			if err != nil {
				return fmt.Errorf("revise the standing note: %w", err)
			}
			curated.Revised += int(revised)
		}
		for _, entry := range operations.Add {
			text := sentence(entry.Text, maxReason, "")
			if text == "" {
				continue
			}
			if err := queries.InsertNote(ctx, plannerdb.InsertNoteParams{
				Scope:        Scope,
				Body:         text,
				Priority:     priority(entry.Priority),
				CreatedCycle: &cycleID,
				UpdatedCycle: &cycleID,
				CreatedAt:    at,
				UpdatedAt:    at,
			}); err != nil {
				return fmt.Errorf("store the standing note: %w", err)
			}
			curated.Added++
		}
		if curated.Added == 0 {
			return nil
		}
		return queries.PruneNotes(ctx, plannerdb.PruneNotesParams{
			Scope:   Scope,
			Scope_2: Scope,
			Limit:   int64(s.config.NoteCap),
		})
	})
	if err != nil {
		return Curated{}, err
	}
	return curated, nil
}

func (s *Service) RecordYield(
	ctx context.Context,
	yield postings.Yield,
) (postings.RoundState, error) {
	current, err := s.settings.Load(ctx)
	if err != nil {
		return postings.RoundState{}, err
	}
	round, err := s.Round(ctx, yield.CycleID)
	if err != nil {
		return postings.RoundState{}, err
	}
	if !hasQuery(round, yield.QueryLabel) {
		return postings.RoundState{}, fmt.Errorf(
			"round %d has no query labelled %q", round.ID, yield.QueryLabel)
	}

	if err := s.transactions.WithTx(ctx, func(tx *sql.Tx) error {
		queries := s.queries.WithTx(tx)
		if err := queries.BumpCycle(ctx, plannerdb.BumpCycleParams{
			Received:   int64(yield.Received),
			Inserted:   int64(yield.Inserted),
			Duplicates: int64(yield.Duplicates),
			ID:         round.ID,
		}); err != nil {
			return fmt.Errorf("update the round: %w", err)
		}
		return queries.BumpCycleQuery(ctx, plannerdb.BumpCycleQueryParams{
			Pages:      int64(max(yield.Pages, 1)),
			Received:   int64(yield.Received),
			Inserted:   int64(yield.Inserted),
			Duplicates: int64(yield.Duplicates),
			CycleID:    round.ID,
			Label:      yield.QueryLabel,
		})
	}); err != nil {
		return postings.RoundState{}, err
	}

	updated, err := s.Round(ctx, round.ID)
	if err != nil {
		return postings.RoundState{}, err
	}
	reason, err := s.terminationCheck(ctx, updated, current)
	if err != nil {
		return postings.RoundState{}, err
	}

	s.hub.Emit(events.TypeCycle, map[string]any{
		"phase":        "yield",
		"cycleId":      updated.ID,
		"queryLabel":   yield.QueryLabel,
		"received":     yield.Received,
		"inserted":     yield.Inserted,
		"roundNew":     updated.Inserted,
		"newJobTarget": current.Harvest.NewJobTarget,
	})

	return postings.RoundState{
		RoundNew:     updated.Inserted,
		NewJobTarget: current.Harvest.NewJobTarget,
		Stop:         reason != "",
		Reason:       reason,
	}, nil
}

func hasQuery(round Round, label string) bool {
	for _, query := range round.Queries {
		if query.Label == label {
			return true
		}
	}
	return false
}

func (s *Service) terminationCheck(
	ctx context.Context,
	round Round,
	current settings.Settings,
) (string, error) {
	if round.Status != StatusOpen {
		if round.TerminationReason != nil {
			return *round.TerminationReason, nil
		}
		return "closed", nil
	}
	if current.Apply.Paused {
		return ReasonPaused, nil
	}
	if round.Inserted >= current.Harvest.NewJobTarget {
		return "target-reached", nil
	}
	if s.expired(round.StartedAt, current.Harvest.RoundDeadlineMs) {
		return "deadline", nil
	}
	budget, err := s.budget(ctx, current)
	if err != nil {
		return "", err
	}
	if budget.Remaining <= 0 {
		return ReasonDailyCap, nil
	}
	ceiling := current.Budget.MaxModelCallsPerCycle
	if ceiling > 0 && round.ModelCalls >= ceiling {
		return ReasonRoundCap, nil
	}
	return "", nil
}

func (s *Service) State(ctx context.Context) (State, error) {
	at, err := s.desk(ctx)
	if err != nil {
		return State{}, err
	}
	var blocked *RefusalError
	if err := s.gate(ctx, at); err != nil && !errors.As(err, &blocked) {
		return State{}, err
	}
	budget, err := s.budget(ctx, at.settings)
	if err != nil {
		return State{}, err
	}
	round, err := s.roundBudget(ctx, at.settings)
	if err != nil {
		return State{}, err
	}
	history, err := s.history(ctx, s.config.HistoryRounds)
	if err != nil {
		return State{}, err
	}
	standing, err := s.notesInScope(ctx)
	if err != nil {
		return State{}, err
	}
	known, err := s.knownIDs(ctx)
	if err != nil {
		return State{}, err
	}

	state := State{
		Plugin:       at.plugin,
		Paused:       at.settings.Apply.Paused,
		AutoCycle:    at.settings.Budget.AutoCycle,
		CycleMinutes: at.settings.Budget.CycleMinutes,
		NextRunAt:    s.nextRunAt(ctx, at.settings.Budget.CycleMinutes, at.settings.Budget.AutoCycle),
		Blocked:      blocked,
		Budget:       budget,
		RoundBudget:  round,
		History:      history,
		Notes:        standing,
		KnownIDCount: len(known),
	}
	if open, found, err := s.currentRound(ctx); err != nil {
		return State{}, err
	} else if found {
		state.Cycle = &open
	}
	return state, nil
}

func (s *Service) close(
	ctx context.Context,
	cycleID int64,
	status, reason string,
	failure *string,
) error {
	finishedAt := clock.Stamp(s.clock)
	terminationReason := reason
	closed, err := s.queries.CloseCycle(ctx, plannerdb.CloseCycleParams{
		Status:            status,
		FinishedAt:        &finishedAt,
		TerminationReason: &terminationReason,
		Error:             failure,
		ID:                cycleID,
	})
	if err != nil {
		return fmt.Errorf("close the round: %w", err)
	}
	if closed == 0 {
		return nil
	}
	round, err := s.Round(ctx, cycleID)
	if err != nil {
		return err
	}
	s.hub.Emit(events.TypeCycle, map[string]any{
		"phase":    "finished",
		"cycleId":  round.ID,
		"status":   round.Status,
		"reason":   reason,
		"inserted": round.Inserted,
		"received": round.Received,
	})
	if failure != nil {
		s.hub.Emit(events.TypeError, map[string]any{"message": *failure})
	}
	return nil
}

type FinishInput struct {
	CycleID int64
	Reason  string
	Error   string
}

func (s *Service) Finish(ctx context.Context, input FinishInput) (Round, error) {
	round, err := s.Round(ctx, input.CycleID)
	if err != nil {
		return Round{}, err
	}
	status := StatusDone
	var failure *string
	if strings.TrimSpace(input.Error) != "" {
		status = StatusFailed
		message := sentence(input.Error, maxRationale, "")
		failure = &message
	}
	if err := s.close(ctx, round.ID, status,
		sentence(input.Reason, 80, "no reason recorded"), failure); err != nil {
		return Round{}, err
	}
	return s.Round(ctx, round.ID)
}

type SkipInput struct {
	CycleID    int64
	QueryLabel string
	Place      string
	Ring       string
	Reason     string
}

type SkipResult struct {
	CycleID    int64  `json:"cycleId"`
	QueryLabel string `json:"queryLabel"`
	Reason     string `json:"reason"`
	Skipped    int    `json:"skipped"`
}

func (s *Service) Skip(ctx context.Context, input SkipInput) (SkipResult, error) {
	round, err := s.Round(ctx, input.CycleID)
	if err != nil {
		return SkipResult{}, err
	}
	if !hasQuery(round, input.QueryLabel) {
		return SkipResult{}, fmt.Errorf(
			"round %d has no query labelled %q", round.ID, input.QueryLabel)
	}

	named := sentence(input.Place, maxPlaceName, "")
	cause := sentence(input.Reason, 200, "the place could not be resolved")
	reason := cause
	if named != "" {
		reason = named + " (" + sentence(input.Ring, 20, "no ring") + "): " + cause
	}
	if _, err := s.queries.SkipCycleQuery(ctx, plannerdb.SkipCycleQueryParams{
		SkipReason: &reason,
		CycleID:    round.ID,
		Label:      input.QueryLabel,
	}); err != nil {
		return SkipResult{}, fmt.Errorf("record the skipped query: %w", err)
	}

	updated, err := s.Round(ctx, round.ID)
	if err != nil {
		return SkipResult{}, err
	}
	skipped := 0
	for _, query := range updated.Queries {
		if query.Skipped {
			skipped++
		}
	}
	s.hub.Emit(events.TypeCycle, map[string]any{
		"phase":      "skipped",
		"cycleId":    updated.ID,
		"queryLabel": input.QueryLabel,
		"reason":     reason,
	})
	return SkipResult{
		CycleID:    updated.ID,
		QueryLabel: input.QueryLabel,
		Reason:     reason,
		Skipped:    skipped,
	}, nil
}

type Widening struct {
	OK        bool   `json:"ok"`
	CycleID   int64  `json:"cycleId"`
	Query     Query  `json:"query"`
	Relaxed   string `json:"relaxed"`
	Reason    string `json:"reason"`
	StepsLeft int    `json:"stepsLeft"`
}

func (s *Service) Widen(ctx context.Context, cycleID int64) (Widening, error) {
	at, err := s.desk(ctx)
	if err != nil {
		return Widening{}, err
	}
	if err := s.gate(ctx, at); err != nil {
		return Widening{}, err
	}

	round, err := s.Round(ctx, cycleID)
	if err != nil {
		return Widening{}, err
	}
	if round.Status != StatusOpen {
		reason := "no reason recorded"
		if round.TerminationReason != nil {
			reason = *round.TerminationReason
		}
		return Widening{}, refuse(ReasonRoundClosed,
			"round "+itoa(int(round.ID))+" already ended: "+reason+".")
	}
	if round.WideningSteps >= at.settings.Harvest.MaxWideningSteps {
		return Widening{}, refuse(ReasonWideningSpent,
			"round "+itoa(int(round.ID))+" has used all "+
				itoa(at.settings.Harvest.MaxWideningSteps)+" widening steps.")
	}

	context := s.planContext(at)
	input, err := s.promptInput(ctx, at, context.screenBudget)
	if err != nil {
		return Widening{}, err
	}
	result, err := s.claude.CallTool(ctx, claude.Call{
		Purpose: PurposeWiden,
		System:  BuildPlanningPrompt(input),
		User:    buildWideningUser(round, at.settings),
		Tool:    widenTool(context),
	})
	if err != nil {
		return Widening{}, s.plannerFailed("the widening call failed: " + why(err))
	}
	if err := s.recordModelCall(ctx, result); err != nil {
		return Widening{}, err
	}

	var answered modelWidening
	if err := json.Unmarshal(result.Input, &answered); err != nil {
		return Widening{}, s.plannerFailed("the widening answer could not be read")
	}
	taken := make(map[string]struct{}, len(round.Queries))
	for _, query := range round.Queries {
		taken[query.Label] = struct{}{}
	}
	widenLabel := slug(answered.Query.Label, "query-"+itoa(len(round.Queries)+1))
	query, err := compileQuery(answered.Query, widenLabel, context, taken)
	if err != nil {
		return Widening{}, s.plannerFailed(err.Error())
	}
	query.Relaxed = "titles"
	if slices.Contains(Relaxations, answered.Relaxed) {
		query.Relaxed = answered.Relaxed
	}
	query.Reason = sentence(answered.Reason, maxReason, query.Reason)

	if err := s.transactions.WithTx(ctx, func(tx *sql.Tx) error {
		queries := s.queries.WithTx(tx)
		if err := saveQuery(ctx, queries, round.ID, len(round.Queries), query, true); err != nil {
			return err
		}
		return queries.BumpCycle(ctx, plannerdb.BumpCycleParams{
			WideningSteps: 1,
			ModelCalls:    1,
			InputTokens:   result.Usage.InputTokens,
			OutputTokens:  result.Usage.OutputTokens,
			ID:            round.ID,
		})
	}); err != nil {
		return Widening{}, err
	}

	left := at.settings.Harvest.MaxWideningSteps - (round.WideningSteps + 1)
	s.hub.Emit(events.TypeCycle, map[string]any{
		"phase":      "widened",
		"cycleId":    round.ID,
		"queryLabel": query.Label,
		"relaxed":    query.Relaxed,
		"reason":     query.Reason,
		"stepsLeft":  left,
	})
	return Widening{
		OK:        true,
		CycleID:   round.ID,
		Query:     query,
		Relaxed:   query.Relaxed,
		Reason:    query.Reason,
		StepsLeft: left,
	}, nil
}
