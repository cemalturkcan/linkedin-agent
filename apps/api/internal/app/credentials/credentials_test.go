package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"api/internal/app/clock"
)

const (
	sampleToken    = "sk-ant-oat01-AAAAAAAAAAAAAAAAAAAAAAAA"
	sampleRefresh  = "sk-ant-ort01-BBBBBBBBBBBBBBBBBBBBBBBB"
	sampleAPIKey   = "sk-ant-api03-CCCCCCCCCCCCCCCCCCCCCCCC"
	fixedNowMillis = int64(1_700_000_000_000)
)

func fixedNow() time.Time {
	return time.UnixMilli(fixedNowMillis)
}

func newTestResolver(t *testing.T, env map[string]string) (*Resolver, Config) {
	t.Helper()
	dir := t.TempDir()
	config := Config{
		EnvVar:    EnvVar,
		StorePath: filepath.Join(dir, "store", StoreName),
		FilePath:  filepath.Join(dir, "home", claudeFileName),
	}
	resolver := NewResolver(config, http.DefaultClient, clock.Fixed{Time: fixedNow()})
	resolver.lookupEnv = func(name string) (string, bool) {
		value, present := env[name]
		return value, present
	}
	return resolver, config
}

const claudeFileName = ".credentials.json"

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestParseAcceptsBareOAuthToken(t *testing.T) {
	block, err := Parse(sampleToken, EnvVar)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if block.AccessToken != sampleToken {
		t.Fatalf("access token = %q", block.AccessToken)
	}
}

func TestParseRefusals(t *testing.T) {
	cases := map[string]struct {
		raw      string
		fragment string
	}{
		"empty":       {"   ", "nothing was pasted"},
		"path":        {"~/.claude/.credentials.json", "not the path to a file"},
		"api key":     {sampleAPIKey, "anthropic api key"},
		"nonsense":    {"hello", "neither the credentials json"},
		"bad json":    {`{"claudeAiOauth":`, "does not parse"},
		"no token":    {`{"claudeAiOauth":{"refreshToken":"x"}}`, "carries no accessToken"},
		"key json":    {`{"claudeAiOauth":{"accessToken":"` + sampleAPIKey + `"}}`, "carries an api key"},
		"array block": {`{"claudeAiOauth":[]}`, "has to be an object"},
		"bare array":  {`[]`, "neither the credentials json"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(testCase.raw, EnvVar)
			var refused *RefusalError
			if !errors.As(err, &refused) {
				t.Fatalf("expected refusal, got %v", err)
			}
			if !strings.Contains(refused.Message, testCase.fragment) {
				t.Fatalf("message %q does not carry %q", refused.Message, testCase.fragment)
			}
		})
	}
}

func TestParseKeepsUnknownFields(t *testing.T) {
	raw := `{"claudeAiOauth":{"accessToken":"` + sampleToken +
		`","refreshToken":"` + sampleRefresh +
		`","expiresAt":123,"subscriptionType":"max","rateLimitTier":"tier-9"}}`
	block, err := Parse(raw, EnvVar)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	encoded, err := json.Marshal(&block)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, field := range []string{"subscriptionType", "rateLimitTier"} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("re-encoded block dropped %s: %s", field, encoded)
		}
	}
}

func TestResolvePrefersEnvThenStoreThenFile(t *testing.T) {
	resolver, config := newTestResolver(t, map[string]string{EnvVar: sampleToken})
	writeFile(t, config.StorePath, `{"claudeAiOauth":{"accessToken":"`+sampleToken+`store"}}`)
	writeFile(t, config.FilePath, `{"claudeAiOauth":{"accessToken":"`+sampleToken+`file"}}`)

	resolved, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Source != SourceEnv {
		t.Fatalf("source = %q, want env", resolved.Source)
	}

	resolver, config = newTestResolver(t, map[string]string{})
	writeFile(t, config.StorePath, `{"claudeAiOauth":{"accessToken":"`+sampleToken+`store"}}`)
	writeFile(t, config.FilePath, `{"claudeAiOauth":{"accessToken":"`+sampleToken+`file"}}`)
	resolved, err = resolver.Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Source != SourceStore {
		t.Fatalf("source = %q, want store", resolved.Source)
	}

	resolver, config = newTestResolver(t, map[string]string{})
	writeFile(t, config.FilePath, `{"claudeAiOauth":{"accessToken":"`+sampleToken+`file"}}`)
	resolved, err = resolver.Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Source != SourceFile {
		t.Fatalf("source = %q, want file", resolved.Source)
	}
}

func TestResolveWithoutAnySourceReportsMissing(t *testing.T) {
	resolver, _ := newTestResolver(t, map[string]string{})
	if _, err := resolver.Resolve(); !errors.Is(err, ErrMissing) {
		t.Fatalf("err = %v, want ErrMissing", err)
	}
}

func TestMalformedSourceStopsTheChain(t *testing.T) {
	resolver, _ := newTestResolver(t, map[string]string{EnvVar: sampleAPIKey})
	_, err := resolver.Resolve()
	var refused *RefusalError
	if !errors.As(err, &refused) {
		t.Fatalf("expected refusal, got %v", err)
	}
	if !strings.Contains(refused.Message, EnvVar) {
		t.Fatalf("refusal does not name the source: %q", refused.Message)
	}
}

func TestStoreWritesRestrictedFileAndStatusHidesToken(t *testing.T) {
	resolver, config := newTestResolver(t, map[string]string{})
	raw := `{"claudeAiOauth":{"accessToken":"` + sampleToken +
		`","refreshToken":"` + sampleRefresh +
		`","expiresAt":` + itoa(fixedNowMillis+3_600_000) + `}}`

	status, err := resolver.Store(raw)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	info, err := os.Stat(config.StorePath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
	if !status.Loaded || status.Source == nil || *status.Source != SourceStore {
		t.Fatalf("status = %+v", status)
	}
	if status.ExpiresInMinutes == nil || *status.ExpiresInMinutes != 60 {
		t.Fatalf("expiresInMinutes = %v", status.ExpiresInMinutes)
	}
	if !status.Refreshable {
		t.Fatal("status should report the token as refreshable")
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	for _, secret := range []string{sampleToken, sampleRefresh} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("status leaks a token: %s", encoded)
		}
	}
	for _, hidden := range []string{"subscriptionType", "rateLimitTier", "plan", "tier"} {
		if strings.Contains(string(encoded), hidden) {
			t.Fatalf("status carries %s: %s", hidden, encoded)
		}
	}
}

func TestClearForgetsTheStoredCredential(t *testing.T) {
	resolver, config := newTestResolver(t, map[string]string{})
	if _, err := resolver.Store(sampleToken); err != nil {
		t.Fatalf("store: %v", err)
	}
	status, err := resolver.Clear()
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if status.Stored {
		t.Fatal("status still reports a stored credential")
	}
	if _, err := os.Stat(config.StorePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("store file still present: %v", err)
	}
	if _, err := resolver.Resolve(); !errors.Is(err, ErrMissing) {
		t.Fatalf("resolve after clear = %v", err)
	}
}

func TestAccessTokenRefreshesIntoTheAnsweringSource(t *testing.T) {
	var requested int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested++
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.PostForm.Get("client_id") != oauthClientID {
			t.Errorf("client_id = %q", r.PostForm.Get("client_id"))
		}
		if r.PostForm.Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", r.PostForm.Get("grant_type"))
		}
		if r.PostForm.Get("refresh_token") != sampleRefresh {
			t.Errorf("refresh_token = %q", r.PostForm.Get("refresh_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(
			`{"access_token":"` + sampleToken + `-fresh","refresh_token":"` +
				sampleRefresh + `-fresh","expires_in":28800}`,
		))
	}))
	defer server.Close()

	resolver, config := newTestResolver(t, map[string]string{})
	writeFile(t, config.FilePath, `{"claudeAiOauth":{"accessToken":"`+sampleToken+
		`","refreshToken":"`+sampleRefresh+
		`","expiresAt":`+itoa(fixedNowMillis+1000)+`,"rateLimitTier":"keep-me"}}`)
	resolver.http = server.Client()
	resolver.tokenURL = server.URL

	token, err := resolver.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("access token: %v", err)
	}
	if token != sampleToken+"-fresh" {
		t.Fatalf("token = %q", token)
	}
	if requested != 1 {
		t.Fatalf("refresh requested %d times", requested)
	}

	written, err := os.ReadFile(config.FilePath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(written), sampleToken+"-fresh") {
		t.Fatalf("refreshed token not persisted: %s", written)
	}
	if !strings.Contains(string(written), "keep-me") {
		t.Fatalf("persisting dropped carried fields: %s", written)
	}
}

func TestScrubRemovesTokenShapedText(t *testing.T) {
	scrubbed := Scrub("failed with " + sampleToken + " attached")
	if strings.Contains(scrubbed, sampleToken) {
		t.Fatalf("scrub kept the token: %q", scrubbed)
	}
	if !strings.Contains(scrubbed, "[redacted]") {
		t.Fatalf("scrub produced %q", scrubbed)
	}
}

func itoa(value int64) string {
	return strconv.FormatInt(value, 10)
}
