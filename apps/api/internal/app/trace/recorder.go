package trace

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"api/internal/app/clock"
	"api/internal/app/credentials"
	"api/internal/app/events"
	"api/internal/app/store"
	tracedb "api/internal/app/trace/gen"
)

const (
	StateRunning     = "running"
	StateOK          = "ok"
	StateError       = "error"
	StateInterrupted = "interrupted"

	Retention   = 200
	ListDefault = 40
)

type Config struct {
	FlushInterval   time.Duration
	MaxFrameChars   int
	MaxPreviewChars int
	MaxStoredChars  int
	WriteTimeout    time.Duration
}

type Dependencies struct {
	Reads        store.DBTX
	Transactions store.TransactionRunner
	Hub          *events.Hub
	Clock        clock.Clock
	Config       Config
}

type Usage struct {
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`
}

type Begin struct {
	Purpose  string
	Model    string
	ToolName string
	System   string
	User     string
}

type Settle struct {
	Model  string
	Usage  Usage
	Output string
}

type Recorder struct {
	queries      *tracedb.Queries
	transactions store.TransactionRunner
	hub          *events.Hub
	clock        clock.Clock
	config       Config
}

func NewRecorder(dependencies Dependencies) *Recorder {
	return &Recorder{
		queries:      tracedb.New(dependencies.Reads),
		transactions: dependencies.Transactions,
		hub:          dependencies.Hub,
		clock:        dependencies.Clock,
		config:       dependencies.Config,
	}
}

type stateEvent struct {
	ID         int64   `json:"id"`
	Purpose    string  `json:"purpose"`
	Model      string  `json:"model"`
	ToolName   string  `json:"toolName"`
	State      string  `json:"state"`
	StartedAt  string  `json:"startedAt"`
	FinishedAt *string `json:"finishedAt,omitempty"`
	DurationMs *int64  `json:"durationMs,omitempty"`
	Error      *string `json:"error,omitempty"`
}

type deltaEvent struct {
	ID      int64  `json:"id"`
	Purpose string `json:"purpose"`
	Text    string `json:"text,omitempty"`
	Capped  bool   `json:"capped,omitempty"`
}

type Call struct {
	recorder  *Recorder
	id        int64
	purpose   string
	model     string
	toolName  string
	startedAt time.Time

	mu     sync.Mutex
	buffer buffer
	timer  *time.Timer
	closed bool
}

func (r *Recorder) Begin(ctx context.Context, input Begin) (*Call, error) {
	startedAt := r.clock.Now().UTC()
	id, err := r.open(ctx, input, startedAt.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}

	r.hub.Emit(events.TypeTrace, stateEvent{
		ID:        id,
		Purpose:   input.Purpose,
		Model:     input.Model,
		ToolName:  input.ToolName,
		State:     StateRunning,
		StartedAt: startedAt.Format(time.RFC3339Nano),
	})

	return &Call{
		recorder:  r,
		id:        id,
		purpose:   input.Purpose,
		model:     input.Model,
		toolName:  input.ToolName,
		startedAt: startedAt,
		buffer: buffer{
			maxFrame:   r.config.MaxFrameChars,
			maxPreview: r.config.MaxPreviewChars,
		},
	}, nil
}

func (r *Recorder) open(ctx context.Context, input Begin, startedAt string) (int64, error) {
	var id int64
	err := r.transactions.WithTx(ctx, func(transaction *sql.Tx) error {
		queries := r.queries.WithTx(transaction)
		opened, err := queries.OpenTrace(ctx, tracedb.OpenTraceParams{
			Purpose:      input.Purpose,
			Model:        input.Model,
			ToolName:     input.ToolName,
			State:        StateRunning,
			StartedAt:    startedAt,
			SystemPrompt: r.clip(input.System),
			UserPrompt:   r.clip(input.User),
		})
		if err != nil {
			return fmt.Errorf("open trace: %w", err)
		}
		id, err = opened.LastInsertId()
		if err != nil {
			return fmt.Errorf("read the new trace id: %w", err)
		}
		if err := queries.PruneTraces(ctx, Retention); err != nil {
			return fmt.Errorf("prune traces: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *Recorder) ResolveInterrupted(ctx context.Context) error {
	if err := r.queries.ResolveInterrupted(ctx, tracedb.ResolveInterruptedParams{
		State:   StateInterrupted,
		State_2: StateRunning,
	}); err != nil {
		return fmt.Errorf("resolve interrupted traces: %w", err)
	}
	return nil
}

func (r *Recorder) clip(value string) string {
	text := credentials.Scrub(value)
	if len(text) <= r.config.MaxStoredChars {
		return text
	}
	cut := runeBoundary(text, r.config.MaxStoredChars)
	return text[:cut] + "\n[trace truncated at " + strconv.Itoa(r.config.MaxStoredChars) + " characters]"
}

func (c *Call) ID() int64 {
	return c.id
}

func (c *Call) Push(fragment string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.buffer.add(fragment)
	c.scheduleLocked()
}

func (c *Call) scheduleLocked() {
	if c.timer != nil || c.closed || c.buffer.pendingLen() == 0 {
		return
	}
	c.timer = time.AfterFunc(c.recorder.config.FlushInterval, c.flush)
}

func (c *Call) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timer = nil
	c.flushLocked()
	c.scheduleLocked()
}

func (c *Call) flushLocked() {
	for {
		chunk, capped, more := c.buffer.take()
		if chunk != "" {
			c.recorder.hub.EmitDelta(events.TypeTraceDelta, deltaEvent{
				ID:      c.id,
				Purpose: c.purpose,
				Text:    credentials.Scrub(chunk),
			})
		}
		if capped {
			c.recorder.hub.EmitDelta(events.TypeTraceDelta, deltaEvent{
				ID:      c.id,
				Purpose: c.purpose,
				Capped:  true,
			})
			return
		}
		if !more {
			return
		}
	}
}

func (c *Call) Settle(ctx context.Context, input Settle) {
	output := c.recorder.clip(input.Output)
	model := input.Model
	if model == "" {
		model = c.model
	}
	c.finish(ctx, StateOK, model, input.Usage, &output, nil)
}

func (c *Call) Fail(ctx context.Context, err error) {
	message := c.recorder.clip(err.Error())
	c.finish(ctx, StateError, c.model, Usage{}, nil, &message)
}

func (c *Call) finish(
	ctx context.Context,
	state, model string,
	usage Usage,
	output, failure *string,
) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.flushLocked()
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.closed = true
	c.mu.Unlock()

	finishedAt := c.recorder.clock.Now().UTC()
	duration := finishedAt.Sub(c.startedAt).Milliseconds()
	finished := finishedAt.Format(time.RFC3339Nano)

	writeCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		c.recorder.config.WriteTimeout,
	)
	defer cancel()
	if err := c.recorder.queries.CloseTrace(writeCtx, tracedb.CloseTraceParams{
		State:            state,
		FinishedAt:       &finished,
		DurationMs:       &duration,
		Model:            model,
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
		Output:           output,
		Error:            failure,
		ID:               c.id,
	}); err != nil {
		slog.Error("trace not closed", "id", c.id, "err", err)
	}

	c.recorder.hub.Emit(events.TypeTrace, stateEvent{
		ID:         c.id,
		Purpose:    c.purpose,
		Model:      model,
		ToolName:   c.toolName,
		State:      state,
		StartedAt:  c.startedAt.Format(time.RFC3339Nano),
		FinishedAt: &finished,
		DurationMs: &duration,
		Error:      failure,
	})
}
