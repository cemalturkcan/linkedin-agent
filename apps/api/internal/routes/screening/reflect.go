package screening

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"api/internal/app/claude"
	"api/internal/app/clock"
	"api/internal/app/events"
	"api/internal/routes/postings"
	screeningdb "api/internal/routes/screening/gen"
)

const PlanningScope = "planning"

func (s *Service) Due(ctx context.Context) (Due, error) {
	last, err := s.queries.LastReflection(ctx)
	since := "1970-01-01T00:00:00Z"
	var ran *string
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return Due{}, fmt.Errorf("read the last reflection: %w", err)
	default:
		since = last.RanAt
		stamp := last.RanAt
		ran = &stamp
	}
	fresh, err := s.queries.CountOutcomesSince(ctx, &since)
	if err != nil {
		return Due{}, fmt.Errorf("count the outcomes since the last reflection: %w", err)
	}
	recorded, err := s.outcomes(ctx)
	if err != nil {
		return Due{}, err
	}
	return Due{
		Due:            len(recorded) >= MinOutcomes && int(fresh) >= OutcomesPerReflection,
		Fresh:          int(fresh),
		Total:          len(recorded),
		LastReflection: ran,
	}, nil
}

func (s *Service) outcomes(ctx context.Context) ([]Outcome, error) {
	rows, err := s.queries.RecentOutcomes(ctx, OutcomeWindow)
	if err != nil {
		return nil, fmt.Errorf("read the recorded outcomes: %w", err)
	}
	recorded := make([]Outcome, 0, len(rows))
	for _, row := range rows {
		entry := Outcome{
			ID:        row.ID,
			Title:     row.Title,
			Company:   row.Company,
			Location:  row.Location,
			AppliedAt: row.UpdatedAt,
		}
		if row.Score != nil {
			entry.Score = int(*row.Score)
		}
		if row.VerdictReason != nil {
			entry.Reason = *row.VerdictReason
		}
		if row.ResumeCode != nil {
			entry.Resume = *row.ResumeCode
		}
		if row.ResumeFit != nil {
			entry.Fit = *row.ResumeFit
		}
		if row.Seniority != nil {
			entry.Seniority = *row.Seniority
		}
		if row.Workplace != nil {
			entry.Workplace = *row.Workplace
		}
		if row.Agency != nil {
			entry.Agency = *row.Agency
		}
		if row.Outcome != nil {
			entry.Outcome = *row.Outcome
		}
		if row.OutcomeNote != nil {
			entry.Note = *row.OutcomeNote
		}
		recorded = append(recorded, entry)
	}
	return recorded, nil
}

func (s *Service) RecordOutcome(
	ctx context.Context,
	id, outcome, note string,
) (Due, error) {
	posting, err := s.Posting(ctx, id)
	if err != nil {
		return Due{}, err
	}
	wanted := line(outcome, 40)
	if wanted != "" && !slices.Contains(Outcomes, wanted) {
		return Due{}, refuse("unknown outcome " + wanted + ", it has to be one of " +
			list(Outcomes, "") + ".")
	}
	byHand := posting.Stage == postings.StageHand
	if wanted != "" && posting.Status != postings.StatusApplied && !byHand {
		return Due{}, refuse(posting.Title +
			" is not marked applied, so it has no outcome to record.")
	}

	at := clock.Stamp(s.clock)
	status := posting.Status
	if wanted != "" {
		status = postings.StatusApplied
	}
	params := screeningdb.SetOutcomeParams{Status: status, UpdatedAt: at, ID: id}
	if wanted != "" {
		params.Outcome = &wanted
		params.OutcomeAt = &at
	}
	if cleaned := line(note, maxNote); cleaned != "" {
		params.OutcomeNote = &cleaned
	}
	if _, err := s.queries.SetOutcome(ctx, params); err != nil {
		return Due{}, fmt.Errorf("record the outcome: %w", err)
	}
	s.hub.Emit(events.TypeJobs, map[string]any{
		"updated": 1,
		"id":      id,
		"status":  status,
		"outcome": wanted,
	})
	due, err := s.Due(ctx)
	if err != nil {
		return Due{}, err
	}
	if due.Due {
		s.reflectInBackground(ctx)
	}
	return due, nil
}

func (s *Service) reflectInBackground(ctx context.Context) {
	detached, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		s.config.ScreenBudget,
	)
	go func() {
		defer cancel()
		if _, err := s.Reflect(detached, false); err != nil {
			slog.Error("the reflection failed", "err", err)
		}
	}()
}

func (s *Service) Reflect(ctx context.Context, force bool) (Reflection, error) {
	s.reflecting.Lock()
	defer s.reflecting.Unlock()

	recorded, err := s.outcomes(ctx)
	if err != nil {
		return Reflection{}, err
	}
	if len(recorded) < MinOutcomes {
		return Reflection{
			Lessons: []Lesson{},
			Reason: itoa(len(recorded)) + " recorded outcomes, the reflector needs " +
				itoa(MinOutcomes) + " before a lesson means anything.",
		}, nil
	}
	if !force {
		due, err := s.Due(ctx)
		if err != nil {
			return Reflection{}, err
		}
		if !due.Due {
			return Reflection{
				Lessons: []Lesson{},
				Reason: "fewer than " + itoa(OutcomesPerReflection) +
					" new outcomes since the last reflection.",
			}, nil
		}
	}

	at, err := s.desk(ctx)
	if err != nil {
		return Reflection{}, err
	}
	standing, err := s.standingNotes(ctx)
	if err != nil {
		return Reflection{}, err
	}

	answered, err := s.call(ctx, claude.Call{
		Purpose: PurposeReflect,
		System: BuildReflectorPrompt(PromptInput{
			Candidate: *at.state.Candidate,
			Resumes:   at.state.Resumes,
			Rules:     at.rules,
			Notes:     standing,
		}),
		User: buildReflectorUser(recorded),
		Tool: lessonTool(),
	}, nil)
	if err != nil {
		return Reflection{}, refuse("the reflector call failed: " + why(err))
	}

	var decoded struct {
		Lessons []Lesson `json:"lessons"`
		Retire  []int64  `json:"retire"`
	}
	if err := json.Unmarshal(answered.Input, &decoded); err != nil {
		return Reflection{}, refuse("the reflector answered with input this run cannot read")
	}

	lessons := shapeLessons(decoded.Lessons)
	retired, err := s.commitLessons(ctx, lessons, decoded.Retire, standing, len(recorded), answered.Model)
	if err != nil {
		return Reflection{}, err
	}

	s.hub.Emit(events.TypeScreen, map[string]any{
		"phase":   "reflected",
		"read":    len(recorded),
		"written": len(lessons),
		"retired": retired,
	})
	return Reflection{
		Ran:     true,
		Read:    len(recorded),
		Written: len(lessons),
		Retired: retired,
		Model:   answered.Model,
		Lessons: lessons,
	}, nil
}

func (s *Service) commitLessons(
	ctx context.Context,
	lessons []Lesson,
	retire []int64,
	standing []Note,
	read int,
	model string,
) (int, error) {
	stamp := clock.Stamp(s.clock)
	known := make(map[int64]struct{}, len(standing))
	for _, note := range standing {
		known[note.ID] = struct{}{}
	}
	retired := 0

	err := s.transactions.WithTx(ctx, func(tx *sql.Tx) error {
		queries := s.queries.WithTx(tx)
		for _, id := range retire {
			if _, present := known[id]; !present {
				continue
			}
			removed, err := queries.RetireNote(ctx, id)
			if err != nil {
				return fmt.Errorf("retire the standing note: %w", err)
			}
			retired += int(removed)
		}

		touched := map[string]struct{}{}
		for _, lesson := range lessons {
			if err := queries.InsertLesson(ctx, screeningdb.InsertLessonParams{
				Scope:     lesson.Scope,
				Body:      lesson.Text,
				Evidence:  lesson.Evidence,
				CreatedAt: stamp,
				UpdatedAt: stamp,
			}); err != nil {
				return fmt.Errorf("store the lesson: %w", err)
			}
			touched[lesson.Scope] = struct{}{}
		}
		for scope := range touched {
			if err := queries.PruneLessons(ctx, screeningdb.PruneLessonsParams{
				Scope:   scope,
				Scope_2: scope,
				Limit:   ReflectorNoteCap,
			}); err != nil {
				return fmt.Errorf("prune the standing notes: %w", err)
			}
		}

		return queries.SaveReflection(ctx, screeningdb.SaveReflectionParams{
			RanAt:    stamp,
			Outcomes: int64(read),
			Written:  int64(len(lessons)),
			Retired:  int64(retired),
			Model:    model,
		})
	})
	return retired, err
}

func (s *Service) standingNotes(ctx context.Context) ([]Note, error) {
	planning, err := s.Notes(ctx, PlanningScope, ReflectorNoteCap)
	if err != nil {
		return nil, err
	}
	screening, err := s.Notes(ctx, Scope, ReflectorNoteCap)
	if err != nil {
		return nil, err
	}
	return append(planning, screening...), nil
}

func shapeLessons(raw []Lesson) []Lesson {
	lessons := make([]Lesson, 0, MaxLessons)
	for _, entry := range raw {
		text := line(entry.Text, maxLesson)
		if text == "" {
			continue
		}
		scope := Scope
		if entry.Scope == PlanningScope {
			scope = PlanningScope
		}
		lessons = append(lessons, Lesson{
			Text:     text,
			Evidence: line(entry.Evidence, maxEvidence),
			Scope:    scope,
		})
		if len(lessons) >= MaxLessons {
			break
		}
	}
	return lessons
}
