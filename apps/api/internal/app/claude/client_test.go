package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"api/internal/app/clock"
	"api/internal/app/events"
	"api/internal/app/store"
	"api/internal/app/trace"
)

type staticToken string

func (s staticToken) AccessToken(context.Context) (string, error) {
	return string(s), nil
}

const sampleCall = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","model":"claude-opus-5","usage":{"input_tokens":120,"cache_read_input_tokens":30,"cache_creation_input_tokens":10,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"record","input":{}}}

event: ping
data: {"type":"ping"}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"verdict\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"keep\",\"score\":88}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":64}}

event: message_stop
data: {"type":"message_stop"}

`

const prose = `event: message_start
data: {"type":"message_start","message":{"model":"claude-opus-5","usage":{"input_tokens":5}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}

event: message_stop
data: {"type":"message_stop"}

`

func newRecorder(t *testing.T) (*trace.Recorder, *events.Hub) {
	t.Helper()
	opened, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	hub := events.NewHub(clock.System{})
	return trace.NewRecorder(trace.Dependencies{
		Reads:        opened.Reads(),
		Transactions: opened,
		Hub:          hub,
		Clock:        clock.System{},
		Config: trace.Config{
			FlushInterval:   time.Millisecond,
			MaxFrameChars:   8_000,
			MaxPreviewChars: 64_000,
			MaxStoredChars:  262_144,
			WriteTimeout:    2 * time.Second,
		},
	}), hub
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	recorder, _ := newRecorder(t)
	client := NewClient(
		staticToken("sk-ant-oat01-TESTTOKENTESTTOKEN"),
		recorder,
		server.Client(),
		Config{
			Model:           Model,
			MaxOutputTokens: MaxOutputTokens,
			RequestTimeout:  10 * time.Second,
			MaxAttempts:     3,
			RetryBackoff:    time.Millisecond,
		},
		clock.System{},
	)
	client.endpoint = server.URL
	return client
}

func sampleTool() Tool {
	return Tool{
		Name:        "record",
		Description: "record the verdict",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"verdict": map[string]any{"type": "string"}},
			"required":   []string{"verdict"},
		},
	}
}

func TestCallToolSendsTheClaudeCodeRequest(t *testing.T) {
	var captured map[string]any
	var header http.Header

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Clone()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sampleCall))
	})

	result, err := client.CallTool(context.Background(), Call{
		Purpose: "triage",
		System:  "the briefing",
		User:    "the posting",
		Tool:    sampleTool(),
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	assertClaudeCodeHeaders(t, header)
	assertForcedToolBody(t, captured)
	assertSystemBlocks(t, captured)

	if string(result.Input) != `{"verdict":"keep","score":88}` {
		t.Fatalf("input = %s", result.Input)
	}
	if result.Model != "claude-opus-5" {
		t.Fatalf("model = %q", result.Model)
	}
	want := trace.Usage{
		InputTokens:      120,
		OutputTokens:     64,
		CacheReadTokens:  30,
		CacheWriteTokens: 10,
	}
	if result.Usage != want {
		t.Fatalf("usage = %+v, want %+v", result.Usage, want)
	}
	if result.StopReason != "tool_use" {
		t.Fatalf("stop reason = %q", result.StopReason)
	}
	if result.TraceID == 0 {
		t.Fatal("the call was not traced")
	}
}

func assertClaudeCodeHeaders(t *testing.T, header http.Header) {
	t.Helper()
	if got := header.Get("Authorization"); got != "Bearer sk-ant-oat01-TESTTOKENTESTTOKEN" {
		t.Fatalf("Authorization = %q", got)
	}
	if header.Get("X-Api-Key") != "" {
		t.Fatal("the request carried an api key header")
	}
	if got := header.Get(HeaderBeta); got != "oauth-2025-04-20,claude-code-20250219" {
		t.Fatalf("%s = %q", HeaderBeta, got)
	}
	if got := header.Get(HeaderVersion); got != AnthropicVersion {
		t.Fatalf("%s = %q", HeaderVersion, got)
	}
	if got := header.Get(HeaderApp); got != "cli" {
		t.Fatalf("%s = %q", HeaderApp, got)
	}
	if got := header.Get("User-Agent"); !strings.HasPrefix(got, "claude-cli/") {
		t.Fatalf("User-Agent = %q", got)
	}
	if got := header.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("Accept = %q", got)
	}
}

func assertForcedToolBody(t *testing.T, captured map[string]any) {
	t.Helper()
	if captured["model"] != Model {
		t.Fatalf("model = %v", captured["model"])
	}
	if captured["max_tokens"] != float64(MaxOutputTokens) {
		t.Fatalf("max_tokens = %v", captured["max_tokens"])
	}
	if captured["stream"] != true {
		t.Fatalf("stream = %v", captured["stream"])
	}
	tools, _ := captured["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v", captured["tools"])
	}
	if tool, _ := tools[0].(map[string]any); tool["strict"] == true {
		t.Fatal("strict constrains the planner into empty answers, measured 1 of 6 against 9 of 10")
	}
	for _, forbidden := range []string{"thinking", "output_config", "effort", "temperature"} {
		if _, present := captured[forbidden]; present {
			t.Fatalf("the request carried %q", forbidden)
		}
	}
	tools, ok := captured["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %v", captured["tools"])
	}
	choice, _ := captured["tool_choice"].(map[string]any)
	if choice["type"] != "tool" || choice["name"] != "record" {
		t.Fatalf("tool_choice = %v", captured["tool_choice"])
	}
	messages, ok := captured["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %v", captured["messages"])
	}
	message, _ := messages[0].(map[string]any)
	if message["role"] != "user" || message["content"] != "the posting" {
		t.Fatalf("message = %v", message)
	}
}

func assertSystemBlocks(t *testing.T, captured map[string]any) {
	t.Helper()
	system, ok := captured["system"].([]any)
	if !ok || len(system) != 2 {
		t.Fatalf("system = %v", captured["system"])
	}
	first, _ := system[0].(map[string]any)
	if first["text"] != CLIIdentity {
		t.Fatalf("first system block = %v", first)
	}
	if _, present := first["cache_control"]; present {
		t.Fatal("the cli identity block carries cache_control")
	}
	second, _ := system[1].(map[string]any)
	if second["text"] != "the briefing" {
		t.Fatalf("second system block = %v", second)
	}
	cache, _ := second["cache_control"].(map[string]any)
	if cache["type"] != "ephemeral" {
		t.Fatalf("cache_control = %v", second["cache_control"])
	}
}

func TestCallToolStreamsTheToolInputAsDeltas(t *testing.T) {
	recorder, hub := newRecorder(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sampleCall))
	}))
	defer server.Close()

	subscriber, admitted := hub.Subscribe(0)
	if !admitted {
		t.Fatal("subscribe was refused")
	}

	client := NewClient(staticToken("token"), recorder, server.Client(), Config{
		Model:           Model,
		MaxOutputTokens: MaxOutputTokens,
		RequestTimeout:  10 * time.Second,
		MaxAttempts:     1,
	}, clock.System{})
	client.endpoint = server.URL

	if _, err := client.CallTool(context.Background(), Call{
		Purpose: "triage",
		System:  "briefing",
		User:    "posting",
		Tool:    sampleTool(),
	}); err != nil {
		t.Fatalf("call: %v", err)
	}

	var preview strings.Builder
	for {
		select {
		case frame := <-subscriber.Delta():
			preview.Write(frame.Data)
			continue
		default:
		}
		break
	}
	if !strings.Contains(preview.String(), "verdict") {
		t.Fatalf("no tool input reached the delta channel: %q", preview.String())
	}

	states := make([]string, 0, 2)
	for {
		select {
		case frame := <-subscriber.State():
			states = append(states, string(frame.Data))
			continue
		default:
		}
		break
	}
	if len(states) != 2 {
		t.Fatalf("state events = %v", states)
	}
	if !strings.Contains(states[0], `"state":"running"`) {
		t.Fatalf("first state event = %s", states[0])
	}
	if !strings.Contains(states[1], `"state":"ok"`) {
		t.Fatalf("second state event = %s", states[1])
	}
}

func TestARefusalIsAnError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`event: message_start
data: {"type":"message_start","message":{"model":"claude-opus-5","usage":{"input_tokens":5}}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"refusal","stop_details":{"category":"cyber"}}}

event: message_stop
data: {"type":"message_stop"}

`))
	})

	_, err := client.CallTool(context.Background(), Call{
		Purpose: "triage",
		System:  "briefing",
		User:    "posting",
		Tool:    sampleTool(),
	})
	if err == nil {
		t.Fatal("a refusal was accepted")
	}
	if !strings.Contains(err.Error(), "declined") || !strings.Contains(err.Error(), "cyber") {
		t.Fatalf("err = %v", err)
	}
}

func TestATurnWithoutTheToolIsAnError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(prose))
	})

	_, err := client.CallTool(context.Background(), Call{
		Purpose: "triage",
		System:  "briefing",
		User:    "posting",
		Tool:    sampleTool(),
	})
	if err == nil {
		t.Fatal("a turn without a tool call was accepted")
	}
	if !strings.Contains(err.Error(), "did not call record") {
		t.Fatalf("err = %v", err)
	}
}

func TestAStreamErrorFrameFailsTheCall(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`event: error
data: {"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}

`))
	})

	_, err := client.CallTool(context.Background(), Call{
		Purpose: "triage",
		System:  "briefing",
		User:    "posting",
		Tool:    sampleTool(),
	})
	if err == nil || !strings.Contains(err.Error(), "overloaded_error") {
		t.Fatalf("err = %v", err)
	}
}

func TestARetryableStatusIsRetriedAndTheErrorCarriesNoToken(t *testing.T) {
	var attempts int
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sampleCall))
	})

	if _, err := client.CallTool(context.Background(), Call{
		Purpose: "triage",
		System:  "briefing",
		User:    "posting",
		Tool:    sampleTool(),
	}); err != nil {
		t.Fatalf("call: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d", attempts)
	}
}

func TestANonRetryableStatusFailsAtOnce(t *testing.T) {
	var attempts int
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error"}}`))
	})

	_, err := client.CallTool(context.Background(), Call{
		Purpose: "triage",
		System:  "briefing",
		User:    "posting",
		Tool:    sampleTool(),
	})
	if err == nil {
		t.Fatal("a 401 was accepted")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

func TestACallNeedsATool(t *testing.T) {
	client := newTestClient(t, func(http.ResponseWriter, *http.Request) {})
	if _, err := client.CallTool(context.Background(), Call{Purpose: "triage"}); err == nil {
		t.Fatal("a call without a tool was accepted")
	}
}

func TestATurnWithoutTheToolIsNudgedAndRerun(t *testing.T) {
	var users []string
	var attempts int
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		users = append(users, body.Messages[0].Content)
		attempts++
		w.Header().Set("Content-Type", "text/event-stream")
		if attempts == 1 {
			_, _ = w.Write([]byte(prose))
			return
		}
		_, _ = w.Write([]byte(sampleCall))
	})
	client.config.MaxNudges = DefaultMaxNudges

	result, err := client.CallTool(context.Background(), Call{
		Purpose: "triage",
		System:  "briefing",
		User:    "the posting",
		Tool:    sampleTool(),
	})
	if err != nil {
		t.Fatalf("the nudge did not recover the call: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("turns = %d, want 2", attempts)
	}
	if result.Nudges != 1 {
		t.Fatalf("nudges = %d, want 1", result.Nudges)
	}
	if string(result.Input) != `{"verdict":"keep","score":88}` {
		t.Fatalf("input = %s", result.Input)
	}
	if users[0] != "the posting" {
		t.Fatalf("the first turn carried %q", users[0])
	}
	if !strings.HasPrefix(users[1], "the posting") {
		t.Fatalf("the nudged turn dropped the request: %q", users[1])
	}
	if !strings.Contains(users[1], "your turn ended without a call to record") {
		t.Fatalf("the nudged turn carried no nudge: %q", users[1])
	}
}

func TestTheNudgeIsBounded(t *testing.T) {
	var attempts int
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(prose))
	})
	client.config.MaxNudges = DefaultMaxNudges

	_, err := client.CallTool(context.Background(), Call{
		Purpose: "triage",
		System:  "briefing",
		User:    "posting",
		Tool:    sampleTool(),
	})
	if err == nil {
		t.Fatal("a turn that never called the tool was accepted")
	}
	if attempts != DefaultMaxNudges+1 {
		t.Fatalf("turns = %d, want %d", attempts, DefaultMaxNudges+1)
	}
	if !strings.Contains(err.Error(), "did not call record") {
		t.Fatalf("err = %v", err)
	}
}
