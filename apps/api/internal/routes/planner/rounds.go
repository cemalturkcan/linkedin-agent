package planner

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	plannerdb "api/internal/routes/planner/gen"
)

var ErrNoRound = errors.New("no round with that id")

func (s *Service) round(ctx context.Context, stored plannerdb.Cycle) (Round, error) {
	rows, err := s.queries.CycleQueries(ctx, stored.ID)
	if err != nil {
		return Round{}, fmt.Errorf("read round queries: %w", err)
	}
	queries := make([]RoundQuery, 0, len(rows))
	for _, row := range rows {
		var query Query
		if err := json.Unmarshal([]byte(row.Query), &query); err != nil {
			query = Query{}
		}
		entry := RoundQuery{
			Label:      row.Label,
			Query:      query,
			Widened:    row.Widened,
			Pages:      int(row.Pages),
			Received:   int(row.Received),
			Inserted:   int(row.Inserted),
			Duplicates: int(row.Duplicates),
			Skipped:    row.Skipped,
		}
		if row.SkipReason != nil {
			entry.SkipReason = *row.SkipReason
		}
		queries = append(queries, entry)
	}
	return Round{
		ID:                stored.ID,
		StartedAt:         stored.StartedAt,
		FinishedAt:        stored.FinishedAt,
		Status:            stored.Status,
		Rationale:         stored.Rationale,
		ScreenBudget:      int(stored.ScreenBudget),
		Received:          int(stored.Received),
		Inserted:          int(stored.Inserted),
		Duplicates:        int(stored.Duplicates),
		WideningSteps:     int(stored.WideningSteps),
		ModelCalls:        int(stored.ModelCalls),
		TerminationReason: stored.TerminationReason,
		Error:             stored.Error,
		Queries:           queries,
	}, nil
}

func (s *Service) Round(ctx context.Context, cycleID int64) (Round, error) {
	stored, err := s.queries.Cycle(ctx, cycleID)
	if errors.Is(err, sql.ErrNoRows) {
		return Round{}, ErrNoRound
	}
	if err != nil {
		return Round{}, fmt.Errorf("read round: %w", err)
	}
	return s.round(ctx, stored)
}

func (s *Service) currentRound(ctx context.Context) (Round, bool, error) {
	stored, err := s.queries.CurrentCycle(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Round{}, false, nil
	}
	if err != nil {
		return Round{}, false, fmt.Errorf("read the open round: %w", err)
	}
	round, err := s.round(ctx, stored)
	return round, err == nil, err
}

func (s *Service) history(ctx context.Context, limit int) ([]Round, error) {
	stored, err := s.queries.RecentCycles(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("read recent rounds: %w", err)
	}
	rounds := make([]Round, 0, len(stored))
	for _, entry := range stored {
		round, err := s.round(ctx, entry)
		if err != nil {
			return nil, err
		}
		rounds = append(rounds, round)
	}
	return rounds, nil
}

func (s *Service) anchor(ctx context.Context) (time.Time, bool) {
	stored, err := s.queries.LastFinishedCycle(ctx)
	if err == nil && stored.FinishedAt != nil {
		if at, parseErr := time.Parse(time.RFC3339Nano, *stored.FinishedAt); parseErr == nil {
			return at, true
		}
	}
	open, err := s.queries.CurrentCycle(ctx)
	if err == nil {
		if at, parseErr := time.Parse(time.RFC3339Nano, open.StartedAt); parseErr == nil {
			return at, true
		}
	}
	return time.Time{}, false
}

func (s *Service) nextRunAt(ctx context.Context, minutes int, auto bool) *string {
	if !auto {
		return nil
	}
	at, found := s.anchor(ctx)
	if !found {
		now := s.clock.Now().UTC().Format(time.RFC3339Nano)
		return &now
	}
	due := at.Add(time.Duration(minutes) * time.Minute).UTC().Format(time.RFC3339Nano)
	return &due
}

func deadlineOf(startedAt string, deadlineMs int) string {
	at, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return startedAt
	}
	return at.Add(time.Duration(deadlineMs) * time.Millisecond).UTC().Format(time.RFC3339Nano)
}

func (s *Service) expired(startedAt string, deadlineMs int) bool {
	at, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return false
	}
	return s.clock.Now().Sub(at) >= time.Duration(deadlineMs)*time.Millisecond
}
