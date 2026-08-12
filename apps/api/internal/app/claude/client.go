package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"api/internal/app/clock"
	"api/internal/app/credentials"
	"api/internal/app/trace"
)

const DefaultMaxNudges = 2

type Config struct {
	Model           string
	MaxOutputTokens int
	RequestTimeout  time.Duration
	MaxAttempts     int
	RetryBackoff    time.Duration
	MaxNudges       int
	Endpoint        string
}

type Result struct {
	Input      json.RawMessage
	Usage      trace.Usage
	Model      string
	StopReason string
	DurationMs int64
	TraceID    int64
	Nudges     int
}

type MissingToolCallError struct {
	Tool       string
	StopReason string
}

func (e *MissingToolCallError) Error() string {
	return "the model did not call " + e.Tool + " (stop reason: " + fallback(e.StopReason, "none") + ")"
}

type TokenSource interface {
	AccessToken(ctx context.Context) (string, error)
}

type Client struct {
	tokens   TokenSource
	recorder *trace.Recorder
	http     *http.Client
	config   Config
	endpoint string
	clock    clock.Clock
}

func NewClient(
	tokens TokenSource,
	recorder *trace.Recorder,
	httpClient *http.Client,
	config Config,
	source clock.Clock,
) *Client {
	if config.Model == "" {
		config.Model = Model
	}
	if config.MaxOutputTokens <= 0 {
		config.MaxOutputTokens = MaxOutputTokens
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 1
	}
	if config.MaxNudges < 0 {
		config.MaxNudges = 0
	}
	if config.Endpoint == "" {
		config.Endpoint = MessagesURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		tokens:   tokens,
		recorder: recorder,
		http:     httpClient,
		config:   config,
		endpoint: config.Endpoint,
		clock:    source,
	}
}

func (c *Client) CallTool(ctx context.Context, call Call) (Result, error) {
	if call.Tool.Name == "" {
		return Result{}, errors.New("a model call needs exactly one tool")
	}
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return Result{}, err
	}

	if c.config.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.config.RequestTimeout)
		defer cancel()
	}

	user := call.User
	for nudges := 0; ; nudges++ {
		result, err := c.turn(ctx, token, call, user)
		if err == nil {
			result.Nudges = nudges
			return result, nil
		}
		var missing *MissingToolCallError
		if !errors.As(err, &missing) || nudges >= c.config.MaxNudges {
			return Result{}, err
		}
		slog.Warn(
			"model turn ended with no tool call, nudging",
			"purpose", call.Purpose,
			"tool", call.Tool.Name,
			"stopReason", missing.StopReason,
			"nudge", nudges+1,
		)
		user = nudged(call.User, call.Tool.Name)
	}
}

func (c *Client) turn(ctx context.Context, token string, call Call, user string) (Result, error) {
	body, err := encodeRequest(c.config.Model, c.config.MaxOutputTokens, call, user)
	if err != nil {
		return Result{}, err
	}

	recorded, err := c.recorder.Begin(ctx, trace.Begin{
		Purpose:  call.Purpose,
		Model:    c.config.Model,
		ToolName: call.Tool.Name,
		System:   call.System,
		User:     user,
	})
	if err != nil {
		return Result{}, err
	}

	startedAt := c.clock.Now()
	result, err := c.run(ctx, token, body, call.Tool.Name, recorded)
	if err != nil {
		recorded.Fail(ctx, err)
		return Result{}, err
	}
	result.DurationMs = c.clock.Now().Sub(startedAt).Milliseconds()
	result.TraceID = recorded.ID()

	recorded.Settle(ctx, trace.Settle{
		Model:  result.Model,
		Usage:  result.Usage,
		Output: indent(result.Input),
	})
	return result, nil
}

func (c *Client) run(
	ctx context.Context,
	token string,
	body []byte,
	toolName string,
	recorded *trace.Call,
) (Result, error) {
	response, err := c.send(ctx, token, body)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = response.Body.Close() }()

	stream, err := parseStream(response.Body, toolName, recorded.Push)
	if err != nil {
		return Result{}, err
	}
	if stream.stopReason == stopReasonRefusal {
		category := stream.stopCategory
		if category == "" {
			category = "unknown"
		}
		return Result{}, errors.New("the model declined the request (" + category + ")")
	}
	if !stream.toolCallFound {
		return Result{}, &MissingToolCallError{Tool: toolName, StopReason: stream.stopReason}
	}
	input := json.RawMessage(stream.toolInput)
	if len(bytes.TrimSpace(input)) == 0 {
		input = json.RawMessage("{}")
	}
	if !json.Valid(input) {
		return Result{}, fmt.Errorf("the model sent %s input that is not valid json", toolName)
	}
	model := stream.model
	if model == "" {
		model = c.config.Model
	}
	return Result{
		Input:      input,
		Usage:      stream.usage,
		Model:      model,
		StopReason: stream.stopReason,
	}, nil
}

func (c *Client) send(ctx context.Context, token string, body []byte) (*http.Response, error) {
	var lastErr error
	for attempt := 1; attempt <= c.config.MaxAttempts; attempt++ {
		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			c.endpoint,
			bytes.NewReader(body),
		)
		if err != nil {
			return nil, fmt.Errorf("build model request: %w", err)
		}
		applyHeaders(request, token)

		response, err := c.http.Do(request)
		switch {
		case err != nil:
			lastErr = errors.New("the model call could not reach api.anthropic.com")
		case response.StatusCode == http.StatusOK:
			return response, nil
		default:
			lastErr = statusError(response)
			_ = response.Body.Close()
			if !retryable(response.StatusCode) {
				return nil, lastErr
			}
		}
		if attempt == c.config.MaxAttempts {
			break
		}
		if err := wait(ctx, c.config.RetryBackoff*time.Duration(attempt)); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func applyHeaders(request *http.Request, token string) {
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set(HeaderVersion, AnthropicVersion)
	request.Header.Set(HeaderBeta, BetaHeader())
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set(HeaderApp, AppHeader)
	request.Header.Set("User-Agent", UserAgent)
}

func statusError(response *http.Response) error {
	detail := struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}{}
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	_ = json.Unmarshal(raw, &detail)
	kind := detail.Error.Type
	if kind == "" {
		kind = http.StatusText(response.StatusCode)
	}
	return errors.New(
		"the model call was rejected with status " +
			strconv.Itoa(response.StatusCode) + " (" + credentials.Scrub(kind) + ")",
	)
}

func retryable(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests:
		return true
	}
	return status >= http.StatusInternalServerError
}

func wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func fallback(value, empty string) string {
	if value == "" {
		return empty
	}
	return value
}

func indent(input json.RawMessage) string {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, input, "", "  "); err != nil {
		return string(input)
	}
	return pretty.String()
}
