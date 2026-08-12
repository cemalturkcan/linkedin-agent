package traces

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"api/internal/app/store"
	"api/internal/app/trace"
	tracesdb "api/internal/routes/traces/gen"
)

type Dependencies struct {
	Reads store.DBTX
}

type Service struct {
	queries *tracesdb.Queries
}

func NewService(dependencies Dependencies) *Service {
	return &Service{queries: tracesdb.New(dependencies.Reads)}
}

func (s *Service) Recent(ctx context.Context, limit int) ([]Trace, error) {
	rows, err := s.queries.RecentTraces(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("list traces: %w", err)
	}
	recent := make([]Trace, 0, len(rows))
	for _, row := range rows {
		recent = append(recent, fromRecentRow(row))
	}
	return recent, nil
}

func (s *Service) Running(ctx context.Context) (Trace, bool, error) {
	row, err := s.queries.RunningTrace(ctx, trace.StateRunning)
	if errors.Is(err, sql.ErrNoRows) {
		return Trace{}, false, nil
	}
	if err != nil {
		return Trace{}, false, fmt.Errorf("read the running trace: %w", err)
	}
	return fromRunningRow(row), true, nil
}

func (s *Service) Trace(ctx context.Context, id int64) (Trace, error) {
	row, err := s.queries.Trace(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Trace{}, ErrNoTrace
	}
	if err != nil {
		return Trace{}, fmt.Errorf("read trace %d: %w", id, err)
	}
	return fromTraceRow(row), nil
}

func fromRecentRow(row tracesdb.RecentTracesRow) Trace {
	return Trace{
		ID:               row.ID,
		Purpose:          row.Purpose,
		Model:            row.Model,
		ToolName:         row.ToolName,
		State:            row.State,
		StartedAt:        row.StartedAt,
		FinishedAt:       row.FinishedAt,
		DurationMs:       row.DurationMs,
		InputTokens:      row.InputTokens,
		OutputTokens:     row.OutputTokens,
		CacheReadTokens:  row.CacheReadTokens,
		CacheWriteTokens: row.CacheWriteTokens,
		Error:            row.Error,
	}
}

func fromRunningRow(row tracesdb.RunningTraceRow) Trace {
	return Trace{
		ID:               row.ID,
		Purpose:          row.Purpose,
		Model:            row.Model,
		ToolName:         row.ToolName,
		State:            row.State,
		StartedAt:        row.StartedAt,
		FinishedAt:       row.FinishedAt,
		DurationMs:       row.DurationMs,
		InputTokens:      row.InputTokens,
		OutputTokens:     row.OutputTokens,
		CacheReadTokens:  row.CacheReadTokens,
		CacheWriteTokens: row.CacheWriteTokens,
		Error:            row.Error,
	}
}

func fromTraceRow(row tracesdb.TraceRow) Trace {
	return Trace{
		ID:               row.ID,
		Purpose:          row.Purpose,
		Model:            row.Model,
		ToolName:         row.ToolName,
		State:            row.State,
		StartedAt:        row.StartedAt,
		FinishedAt:       row.FinishedAt,
		DurationMs:       row.DurationMs,
		InputTokens:      row.InputTokens,
		OutputTokens:     row.OutputTokens,
		CacheReadTokens:  row.CacheReadTokens,
		CacheWriteTokens: row.CacheWriteTokens,
		Error:            row.Error,
		System:           &row.SystemPrompt,
		User:             &row.UserPrompt,
		Output:           row.Output,
	}
}
