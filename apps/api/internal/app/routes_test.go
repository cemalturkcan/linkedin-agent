package app

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"api/internal/app/events"
)

const testToken = "sk-ant-oat01-ROUTETESTROUTETEST"

func testConfig(t *testing.T) Config {
	t.Helper()
	config := defaults()
	config.DataDir = t.TempDir()
	config.CredentialFile = t.TempDir() + "/absent.json"
	config.StreamHeartbeat = 150 * time.Millisecond
	config.StreamRetry = 3 * time.Second
	return config
}

func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	runtime, err := Build(context.Background(), testConfig(t))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

func serve(t *testing.T, runtime *Runtime) string {
	t.Helper()
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		_ = runtime.Server.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	}()
	t.Cleanup(func() { _ = runtime.Server.ShutdownWithTimeout(time.Second) })

	base := "http://" + listener.Addr().String()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := get(t, base+"/api/health")
		if err == nil {
			_ = response.Body.Close()
			return base
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the server never answered")
	return ""
}

func request(t *testing.T, runtime *Runtime, method, target, body string) (int, string) {
	t.Helper()
	return requestWithin(t, runtime, method, target, body, 5*time.Second)
}

func requestWithin(
	t *testing.T,
	runtime *Runtime,
	method, target, body string,
	budget time.Duration,
) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, target, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != "" {
		req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}
	response, err := runtime.Server.Test(req, fiber.TestConfig{Timeout: budget})
	if err != nil {
		t.Fatalf("test request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return response.StatusCode, string(raw)
}

func TestHealthAnswersLivenessOnly(t *testing.T) {
	runtime := newTestRuntime(t)
	status, body := request(t, runtime, http.MethodGet, "/api/health", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if strings.TrimSpace(body) != `{"ok":true}` {
		t.Fatalf("body = %s", body)
	}
}

func TestAnUnknownRouteAnswersTheErrorEnvelope(t *testing.T) {
	runtime := newTestRuntime(t)
	status, body := request(t, runtime, http.MethodGet, "/api/nowhere", "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d", status)
	}
	var envelope struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if envelope.Error == "" {
		t.Fatalf("body = %s", body)
	}
}

func TestCredentialRoutesNeverCarryTheToken(t *testing.T) {
	runtime := newTestRuntime(t)

	status, body := request(t, runtime, http.MethodGet, "/api/credentials", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s)", status, body)
	}
	if !strings.Contains(body, `"loaded":false`) {
		t.Fatalf("body = %s", body)
	}

	stored, err := json.Marshal(map[string]string{"value": testToken})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	status, body = request(t, runtime, http.MethodPut, "/api/credentials", string(stored))
	if status != http.StatusOK {
		t.Fatalf("put status = %d (%s)", status, body)
	}
	if strings.Contains(body, testToken) {
		t.Fatalf("the response carried the token: %s", body)
	}
	if !strings.Contains(body, `"source":"store"`) {
		t.Fatalf("body = %s", body)
	}

	status, body = request(t, runtime, http.MethodPut, "/api/credentials", `{"value":"nope"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("refusal status = %d (%s)", status, body)
	}

	status, body = request(t, runtime, http.MethodDelete, "/api/credentials", "")
	if status != http.StatusOK {
		t.Fatalf("delete status = %d (%s)", status, body)
	}
	if !strings.Contains(body, `"loaded":false`) {
		t.Fatalf("body after delete = %s", body)
	}
}

func TestTraceRoutes(t *testing.T) {
	runtime := newTestRuntime(t)

	status, body := request(t, runtime, http.MethodGet, "/api/traces", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s)", status, body)
	}
	if !strings.Contains(body, `"running":null`) || !strings.Contains(body, `"traces":[]`) {
		t.Fatalf("body = %s", body)
	}

	status, body = request(t, runtime, http.MethodGet, "/api/traces/91", "")
	if status != http.StatusNotFound || !strings.Contains(body, "no such call") {
		t.Fatalf("status = %d, body = %s", status, body)
	}

	status, body = request(t, runtime, http.MethodGet, "/api/traces/not-a-number", "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", status, body)
	}
}

func get(t *testing.T, target string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	return http.DefaultClient.Do(req)
}

func readFrames(t *testing.T, reader *bufio.Reader, until time.Time) []string {
	t.Helper()
	lines := make([]string, 0, 16)
	for time.Now().Before(until) {
		line, err := reader.ReadString('\n')
		if line != "" {
			lines = append(lines, strings.TrimRight(line, "\r\n"))
		}
		if err != nil {
			break
		}
	}
	return lines
}

func openStream(t *testing.T, base string, lastEventID string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/api/events", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if lastEventID != "" {
		req.Header.Set(fiber.HeaderLastEventID, lastEventID)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	return response
}

func TestEventStreamOpensReplaysAndHeartbeats(t *testing.T) {
	runtime := newTestRuntime(t)
	base := serve(t, runtime)

	runtime.Hub.Emit(events.TypeTrace, map[string]any{"id": 1, "state": "running"})
	runtime.Hub.Emit(events.TypeTrace, map[string]any{"id": 1, "state": "ok"})

	response := openStream(t, base, "1")
	defer func() { _ = response.Body.Close() }()
	if got := response.Header.Get(fiber.HeaderContentType); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("content type = %q", got)
	}

	reader := bufio.NewReader(response.Body)
	lines := readFrames(t, reader, time.Now().Add(500*time.Millisecond))
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "retry: 3000") {
		t.Fatalf("stream carried no retry hint:\n%s", joined)
	}
	if !strings.Contains(joined, ": open") {
		t.Fatalf("stream carried no opening comment:\n%s", joined)
	}
	if strings.Contains(joined, "id: 1\n") {
		t.Fatalf("replay resent an event the client had seen:\n%s", joined)
	}
	if !strings.Contains(joined, "id: 2") {
		t.Fatalf("replay skipped the event after the last id:\n%s", joined)
	}
	if !strings.Contains(joined, `"state":"ok"`) {
		t.Fatalf("replayed frame lost its payload:\n%s", joined)
	}
	if strings.Count(joined, ":\n") < 1 && !strings.Contains(joined, ": ping") {
		t.Fatalf("no heartbeat arrived within the idle window:\n%s", joined)
	}
}

func TestEventStreamDeliversDeltasWithoutIds(t *testing.T) {
	runtime := newTestRuntime(t)
	base := serve(t, runtime)

	response := openStream(t, base, "")
	defer func() { _ = response.Body.Close() }()
	reader := bufio.NewReader(response.Body)
	readFrames(t, reader, time.Now().Add(100*time.Millisecond))

	runtime.Hub.EmitDelta(events.TypeTraceDelta, map[string]any{"id": 7, "text": "partial"})
	lines := readFrames(t, reader, time.Now().Add(400*time.Millisecond))
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "event: trace-delta") {
		t.Fatalf("no delta arrived:\n%s", joined)
	}
	if strings.Contains(joined, "id:") {
		t.Fatalf("a delta carried an id:\n%s", joined)
	}
}

func TestTheSubscriberCapIsAnswered(t *testing.T) {
	runtime := newTestRuntime(t)
	base := serve(t, runtime)

	for range events.MaxSubscribers {
		open := openStream(t, base, "")
		defer func() { _ = open.Body.Close() }()
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.Hub.Subscribers() < events.MaxSubscribers && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	response, err := get(t, base+"/api/events")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(body), strconv.Itoa(events.MaxSubscribers)) {
		t.Fatalf("body = %s", body)
	}
}

func TestPreflightIsAnsweredForTheExtensionOnly(t *testing.T) {
	runtime := newTestRuntime(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodOptions, "/api/credentials", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set(fiber.HeaderOrigin, "chrome-extension://abcdefghijklmnopabcdefghijklmnop")
	req.Header.Set(fiber.HeaderAccessControlRequestMethod, http.MethodPut)
	response, err := runtime.Server.Test(req, fiber.TestConfig{Timeout: time.Second})
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if got := response.Header.Get(fiber.HeaderAccessControlAllowOrigin); got == "" {
		t.Fatal("the extension origin was refused")
	}

	req, err = http.NewRequestWithContext(context.Background(), http.MethodOptions, "/api/credentials", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set(fiber.HeaderOrigin, "https://example.com")
	req.Header.Set(fiber.HeaderAccessControlRequestMethod, http.MethodPut)
	response, err = runtime.Server.Test(req, fiber.TestConfig{Timeout: time.Second})
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if got := response.Header.Get(fiber.HeaderAccessControlAllowOrigin); got != "" {
		t.Fatalf("a foreign origin was allowed: %q", got)
	}
}

func TestListenAddrStaysOnLoopback(t *testing.T) {
	config := defaults()
	config.Port = 9999
	if config.ListenAddr() != "127.0.0.1:9999" {
		t.Fatalf("listen addr = %q", config.ListenAddr())
	}
}

func TestTheLoopbackPreflightAnswersChromesPrivateNetworkCheck(t *testing.T) {
	runtime := newTestRuntime(t)
	const origin = "chrome-extension://abcdefghijklmnopabcdefghijklmnop"

	req, err := http.NewRequestWithContext(context.Background(), http.MethodOptions, "/api/health", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set(fiber.HeaderOrigin, origin)
	req.Header.Set(fiber.HeaderAccessControlRequestMethod, http.MethodGet)
	req.Header.Set(requestPrivateNetwork, "true")
	response, err := runtime.Server.Test(req, fiber.TestConfig{Timeout: time.Second})
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if got := response.Header.Get(allowPrivateNetworkHeader); got != "true" {
		t.Fatalf("%s = %q, chrome blocks the extension without it", allowPrivateNetworkHeader, got)
	}

	req, err = http.NewRequestWithContext(context.Background(), http.MethodOptions, "/api/health", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set(fiber.HeaderOrigin, origin)
	req.Header.Set(fiber.HeaderAccessControlRequestMethod, http.MethodGet)
	response, err = runtime.Server.Test(req, fiber.TestConfig{Timeout: time.Second})
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if got := response.Header.Get(allowPrivateNetworkHeader); got != "" {
		t.Fatalf("%s = %q on a request that never asked", allowPrivateNetworkHeader, got)
	}
}

func TestAStreamThatDropsGivesItsSlotBack(t *testing.T) {
	runtime := newTestRuntime(t)

	for range events.MaxSubscribers * 3 {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/events", nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		response, err := runtime.Server.Test(req, fiber.TestConfig{Timeout: 50 * time.Millisecond})
		if err != nil {
			continue
		}
		if response.StatusCode == http.StatusServiceUnavailable {
			t.Fatalf("the stream was refused after %d clean drops, the slot leaks", events.MaxSubscribers)
		}
		_ = response.Body.Close()
	}
}

func TestTheStreamAnswersARequestThatCarriesNoOrigin(t *testing.T) {
	runtime := newTestRuntime(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/events", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set(fiber.HeaderAccept, "text/event-stream")
	response, err := runtime.Server.Test(req, fiber.TestConfig{Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if got := response.Header.Get(fiber.HeaderAccessControlAllowOrigin); got == "" {
		t.Fatal("chrome sends the event stream request without an origin and drops the response without this header")
	}
}

func TestOpeningAPostingByHandIsRecordedAndListed(t *testing.T) {
	runtime := newTestRuntime(t)

	status, body := request(t, runtime, http.MethodPost, "/api/jobs/opened", "")
	if status != http.StatusBadRequest {
		t.Fatalf("an empty open answered %d (%s)", status, body)
	}

	status, body = request(t, runtime, http.MethodPost, "/api/jobs/opened",
		`{"jobs":[{"id":"","title":"nothing"}]}`)
	if status != http.StatusBadRequest || !strings.Contains(body, "has no id") {
		t.Fatalf("an idless open answered %d (%s)", status, body)
	}

	status, body = request(t, runtime, http.MethodPost, "/api/jobs/opened",
		`{"jobs":[{"id":"7788","title":"Backend Engineer","company":"Hooli",`+
			`"location":"Berlin","url":"https://www.linkedin.com/jobs/view/7788/",`+
			`"easyApply":true}],"resumeCode":"go","resumeLang":"EN"}`)
	if status != http.StatusOK {
		t.Fatalf("open status = %d (%s)", status, body)
	}
	if !strings.Contains(body, `"recorded":1`) {
		t.Fatalf("open body = %s", body)
	}

	status, body = request(t, runtime, http.MethodGet, "/api/jobs?status=opened", "")
	if status != http.StatusOK {
		t.Fatalf("list status = %d (%s)", status, body)
	}
	for _, wanted := range []string{`"id":"7788"`, `"stage":"hand"`, `"resumeCode":"GO"`, `"resumeLang":"en"`} {
		if !strings.Contains(body, wanted) {
			t.Fatalf("the opened list is missing %s: %s", wanted, body)
		}
	}
}
