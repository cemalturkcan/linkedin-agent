package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"api/internal/app/credentials"
	"api/internal/testutil"
)

const cvFolderEnv = "AGENT_CV_FOLDER"

type modelTurn struct {
	Tool   string
	System string
	User   string
}

type modelScript struct {
	mu    sync.Mutex
	turns []modelTurn
	reply func(turn modelTurn, seen int) string
}

func (m *modelScript) record(turn modelTurn) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.turns = append(m.turns, turn)
	seen := 0
	for _, earlier := range m.turns {
		if earlier.Tool == turn.Tool {
			seen++
		}
	}
	return seen
}

func (m *modelScript) seen(tool string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, turn := range m.turns {
		if turn.Tool == tool {
			count++
		}
	}
	return count
}

func (m *modelScript) last(tool string) modelTurn {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, turn := range slices.Backward(m.turns) {
		if turn.Tool == tool {
			return turn
		}
	}
	return modelTurn{}
}

func toolCallStream(name string, input any) string {
	document, err := json.Marshal(input)
	if err != nil {
		document = []byte("{}")
	}
	partial, err := json.Marshal(string(document))
	if err != nil {
		partial = []byte(`"{}"`)
	}
	return strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"model":"claude-opus-5","usage":{"input_tokens":100}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"` + name + `","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":` + string(partial) + `}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":40}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
		``,
	}, "\n")
}

const proseStream = `event: message_start
data: {"type":"message_start","message":{"model":"claude-opus-5","usage":{"input_tokens":10}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}

`

type desk struct {
	runtime *Runtime
	script  *modelScript
	cvDir   string
	budget  time.Duration
}

func (d *desk) timeout() time.Duration {
	if d.budget <= 0 {
		return 10 * time.Second
	}
	return d.budget
}

func startScript(t *testing.T, script *modelScript) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			System []struct {
				Text string `json:"text"`
			} `json:"system"`
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode the model request: %v", err)
		}
		turn := modelTurn{}
		if len(body.Tools) > 0 {
			turn.Tool = body.Tools[0].Name
		}
		if len(body.System) > 1 {
			turn.System = body.System[1].Text
		}
		if len(body.Messages) > 0 {
			turn.User = body.Messages[0].Content
		}
		seen := script.record(turn)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(script.reply(turn, seen)))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func cvSource(t *testing.T) string {
	t.Helper()
	folder := strings.TrimSpace(os.Getenv(cvFolderEnv))
	if folder == "" {
		t.Skipf("set %s to a folder of real cv pdfs to run this", cvFolderEnv)
	}
	return folder
}

func firstResume(t *testing.T, role string) []byte {
	t.Helper()
	folder := cvSource(t)
	entries, err := os.ReadDir(filepath.Join(folder, role))
	if err != nil || len(entries) == 0 {
		t.Skipf("the cv folder %s holds no readable pdf: %v", role, err)
	}
	raw, err := os.ReadFile(filepath.Join(folder, role, entries[0].Name()))
	if err != nil {
		t.Fatalf("read the %s cv: %v", role, err)
	}
	return raw
}

func copyResumes(t *testing.T) string {
	t.Helper()
	target := t.TempDir()
	for _, role := range []string{"go", "go-tr", "backend"} {
		raw := firstResume(t, role)
		if err := os.MkdirAll(filepath.Join(target, role), 0o750); err != nil {
			t.Fatalf("create the %s folder: %v", role, err)
		}
		if err := os.WriteFile(
			filepath.Join(target, role, "resume.pdf"), raw, 0o600,
		); err != nil {
			t.Fatalf("write the %s cv: %v", role, err)
		}
	}
	return target
}

func indexerReply(turn modelTurn) (string, bool) {
	switch turn.Tool {
	case "resume_profiles":
		var payload []struct {
			Code  string `json:"code"`
			Label string `json:"label"`
		}
		opened := strings.Index(turn.User, "[")
		if opened >= 0 {
			_ = json.Unmarshal([]byte(turn.User[opened:]), &payload)
		}
		profiles := make([]map[string]any, 0, len(payload))
		for _, entry := range payload {
			core := []string{"Go", "PostgreSQL"}
			role := "Go Backend Engineer"
			if strings.EqualFold(entry.Code, "BA") {
				core = []string{"Java", "Spring Boot"}
				role = "Java Backend Engineer"
			}
			profiles = append(profiles, map[string]any{
				"code":             entry.Code,
				"targetRole":       role,
				"seniorityClaimed": "senior",
				"coreStack":        core,
				"secondaryStack":   []string{"Docker"},
				"domains":          []string{"payments"},
				"languages":        []string{"en", "tr"},
				"yearsClaimed":     6,
				"earliestStart":    "2019-01",
				"places":           []map[string]string{{"name": "Istanbul", "ring": "city"}},
				"summary":          "Leads with " + core[0] + " services.",
			})
		}
		return toolCallStream("resume_profiles", map[string]any{"profiles": profiles}), true
	case "candidate_profile":
		return toolCallStream("candidate_profile", map[string]any{
			"yearsExperience": 6,
			"seniorityBand":   "senior",
			"coreStack":       []string{"Go", "PostgreSQL", "Java"},
			"secondaryStack":  []string{"Docker"},
			"domains":         []string{"payments"},
			"workLanguages":   []string{"en", "tr"},
			"places":          []map[string]string{{"name": "Istanbul", "ring": "city"}},
			"headline":        "Backend engineer building Go and Java services.",
		}), true
	}
	return "", false
}

func idsIn(user string) []string {
	opened := strings.Index(user, "<postings>")
	closed := strings.Index(user, "</postings>")
	if opened < 0 || closed < 0 {
		return nil
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(user[opened+len("<postings>"):closed]), &rows); err != nil {
		return nil
	}
	found := make([]string, 0, len(rows))
	for _, row := range rows {
		found = append(found, row.ID)
	}
	return found
}

func newDesk(t *testing.T, script *modelScript) *desk {
	t.Helper()
	testutil.RequireIntegration(t)

	cvDir := copyResumes(t)

	config := testConfig(t)
	config.ModelEndpoint = startScript(t, script)
	t.Setenv(credentials.EnvVar, `{"accessToken":"sk-ant-oat01-STUBSTUBSTUBSTUB"}`)

	runtime, err := Build(context.Background(), config)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	board := &desk{runtime: runtime, script: script, cvDir: cvDir}
	folder, err := json.Marshal(map[string]string{"dir": cvDir})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	board.post(t, "/api/setup", string(folder))
	board.waitForIndex(t)
	return board
}

func (d *desk) waitForIndex(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		state := d.get(t, "/api/profile")
		if strings.Contains(state, `"indexState":"current"`) &&
			strings.Contains(state, `"indexing":false`) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("indexing never finished: %s", d.get(t, "/api/profile"))
}

func (d *desk) post(t *testing.T, target, body string) string {
	t.Helper()
	status, answered := requestWithin(t, d.runtime, http.MethodPost, target, body, d.timeout())
	if status != http.StatusOK {
		t.Fatalf("POST %s = %d (%s)", target, status, answered)
	}
	return answered
}

func (d *desk) get(t *testing.T, target string) string {
	t.Helper()
	status, answered := requestWithin(t, d.runtime, http.MethodGet, target, "", d.timeout())
	if status != http.StatusOK {
		t.Fatalf("GET %s = %d (%s)", target, status, answered)
	}
	return answered
}

func (d *desk) settings(t *testing.T, patch map[string]any) {
	t.Helper()
	document, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("encode settings: %v", err)
	}
	status, answered := requestWithin(
		t, d.runtime, http.MethodPut, "/api/settings", string(document), d.timeout(),
	)
	if status != http.StatusOK {
		t.Fatalf("PUT /api/settings = %d (%s)", status, answered)
	}
}

type harvest struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Company   string `json:"company"`
	Location  string `json:"location"`
	URL       string `json:"url"`
	EasyApply bool   `json:"easyApply"`
}

func (d *desk) ingest(t *testing.T, jobs ...harvest) {
	t.Helper()
	document, err := json.Marshal(map[string]any{"jobs": jobs})
	if err != nil {
		t.Fatalf("encode harvest: %v", err)
	}
	d.post(t, "/api/jobs", string(document))
}

func (d *desk) describe(t *testing.T, bodies map[string]string) {
	t.Helper()
	entries := make([]map[string]string, 0, len(bodies))
	for id, text := range bodies {
		entries = append(entries, map[string]string{"id": id, "description": text})
	}
	document, err := json.Marshal(map[string]any{"descriptions": entries})
	if err != nil {
		t.Fatalf("encode descriptions: %v", err)
	}
	d.post(t, "/api/jobs/descriptions", string(document))
}

type storedPosting struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Company  string `json:"company"`
	Location string `json:"location"`
	Status   string `json:"status"`
}

func (d *desk) postings(t *testing.T) map[string]storedPosting {
	t.Helper()
	var answered struct {
		Jobs []storedPosting `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(d.get(t, "/api/jobs")), &answered); err != nil {
		t.Fatalf("decode postings: %v", err)
	}
	found := make(map[string]storedPosting, len(answered.Jobs))
	for _, posting := range answered.Jobs {
		found[posting.ID] = posting
	}
	return found
}

type screenResult struct {
	Paused    bool   `json:"paused"`
	Triaged   int    `json:"triaged"`
	Kept      int    `json:"kept"`
	Gated     int    `json:"gated"`
	Screened  int    `json:"screened"`
	Picked    int    `json:"picked"`
	Manual    int    `json:"manual"`
	Skipped   int    `json:"skipped"`
	Flagged   int    `json:"flagged"`
	Corrected int    `json:"corrected"`
	Nudges    int    `json:"nudges"`
	ScreenKey string `json:"screenKey"`
	Error     string `json:"error"`
	Verdicts  []struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Stage    string `json:"stage"`
		Score    int    `json:"score"`
		Reason   string `json:"reason"`
		Flagged  bool   `json:"flagged"`
		Flag     string `json:"flag"`
		Rewrote  bool   `json:"rewrote"`
		Original *struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"original"`
		Verdict struct {
			Verdict    string `json:"verdict"`
			ResumeCode string `json:"resumeCode"`
			ResumeLang string `json:"resumeLang"`
			ResumeFit  string `json:"resumeFit"`
			Tailored   struct {
				Needed bool   `json:"needed"`
				Focus  string `json:"focus"`
			} `json:"tailoredResume"`
		} `json:"verdict"`
	} `json:"verdicts"`
}

func (d *desk) screen(t *testing.T) screenResult {
	t.Helper()
	var result screenResult
	if err := json.Unmarshal([]byte(d.post(t, "/api/jobs/screen", `{}`)), &result); err != nil {
		t.Fatalf("decode the screening result: %v", err)
	}
	return result
}

func (r screenResult) find(t *testing.T, id string) struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Stage    string `json:"stage"`
	Score    int    `json:"score"`
	Reason   string `json:"reason"`
	Flagged  bool   `json:"flagged"`
	Flag     string `json:"flag"`
	Rewrote  bool   `json:"rewrote"`
	Original *struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	} `json:"original"`
	Verdict struct {
		Verdict    string `json:"verdict"`
		ResumeCode string `json:"resumeCode"`
		ResumeLang string `json:"resumeLang"`
		ResumeFit  string `json:"resumeFit"`
		Tailored   struct {
			Needed bool   `json:"needed"`
			Focus  string `json:"focus"`
		} `json:"tailoredResume"`
	} `json:"verdict"`
} {
	t.Helper()
	for _, verdict := range r.Verdicts {
		if verdict.ID == id {
			return verdict
		}
	}
	t.Fatalf("no verdict for %s in %+v", id, r.Verdicts)
	return r.Verdicts[0]
}

func keepEverything(ids []string) map[string]any {
	decisions := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		decisions = append(decisions, map[string]any{
			"id":      id,
			"keep":    true,
			"promise": 70,
			"reason":  "the header reads like backend work this person does.",
		})
	}
	return map[string]any{"decisions": decisions}
}

type decision struct {
	Verdict string
	Reason  string
	Score   int
	Agency  bool
	Pay     map[string]any
	Code    string
	Lang    string
	Fit     string
	Focus   string
}

func (d decision) encode(id string) map[string]any {
	pay := map[string]any{"present": false, "amount": 0, "currency": "", "period": ""}
	if d.Pay != nil {
		pay = d.Pay
	}
	code := d.Code
	if code == "" {
		code = "GO"
	}
	language := d.Lang
	if language == "" {
		language = "en"
	}
	fit := d.Fit
	if fit == "" {
		fit = "strong"
	}
	tailored := map[string]any{"needed": false, "focus": ""}
	if d.Focus != "" {
		tailored = map[string]any{"needed": true, "focus": d.Focus}
	}
	return map[string]any{
		"id":             id,
		"verdict":        d.Verdict,
		"reason":         d.Reason,
		"score":          d.Score,
		"seniority":      "senior",
		"workplace":      "onsite",
		"contractType":   "full-time",
		"postingLang":    "en",
		"agency":         d.Agency,
		"statedPay":      pay,
		"resumeCode":     code,
		"resumeLang":     language,
		"resumeFit":      fit,
		"tailoredResume": tailored,
	}
}

func screenReply(ids []string, answers map[string]decision) string {
	decisions := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		answer, present := answers[id]
		if !present {
			answer = decision{
				Verdict: "apply",
				Reason:  "the core work is Go services this person builds.",
				Score:   80,
			}
		}
		decisions = append(decisions, answer.encode(id))
	}
	return toolCallStream("screen_jobs", map[string]any{"decisions": decisions})
}

func noFlags() string {
	return toolCallStream("flag_verdicts", map[string]any{"flags": []any{}})
}

func ruleAnswers() map[string]decision {
	return map[string]decision{
		"kill": {
			Verdict: "skip",
			Reason:  "the description names Rust as the core language, not Go.",
			Score:   20,
		},
		"upgrade": {
			Verdict: "apply",
			Reason:  "the body names Go, Postgres and Kafka on a payments platform.",
			Score:   88,
		},
		"agency": {
			Verdict: "apply",
			Reason:  "the stack fits, the hiring party is a staffing firm.",
			Score:   70,
			Agency:  true,
		},
		"lowpay": {
			Verdict: "apply",
			Reason:  "the stack fits and the posting names its salary.",
			Score:   65,
			Pay: map[string]any{
				"present": true, "amount": 2000, "currency": "EUR", "period": "month",
			},
		},
		"faraway": {
			Verdict: "apply",
			Reason:  "the stack fits and the role is onsite in Warsaw.",
			Score:   62,
		},
		"manual": {
			Verdict: "apply",
			Reason:  "the stack fits and the posting is not auto-fillable.",
			Score:   72,
		},
		"tailor": {
			Verdict: "apply",
			Reason:  "the stack overlaps but no variant leads with event sourcing.",
			Score:   66,
			Fit:     "partial",
			Focus:   "lead with the event sourced payments ledger built in Go.",
		},
	}
}

func screeningScript(answers map[string]decision) *modelScript {
	script := &modelScript{}
	script.reply = func(turn modelTurn, _ int) string {
		if stream, handled := indexerReply(turn); handled {
			return stream
		}
		switch turn.Tool {
		case "triage_jobs":
			return toolCallStream("triage_jobs", keepEverything(idsIn(turn.User)))
		case "screen_jobs":
			return screenReply(idsIn(turn.User), answers)
		case "flag_verdicts":
			return noFlags()
		}
		return proseStream
	}
	return script
}

func (d *desk) rulesSettings(t *testing.T) {
	t.Helper()
	d.settings(t, map[string]any{
		"companies": map[string]any{
			"blocked":             []string{"Acme GmbH"},
			"excludeAgencies":     true,
			"reapplyCooldownDays": 90,
		},
		"roles": map[string]any{
			"applyToNonEasyApply": true,
			"minCompensation": map[string]any{
				"amount": 60000, "currency": "EUR", "period": "year", "hardFilter": true,
			},
		},
		"locations": map[string]any{
			"places": []map[string]string{
				{"name": "Istanbul", "ring": "city", "kind": "commute"},
				{"name": "Berlin", "ring": "city", "kind": "relocate"},
			},
			"workplace":     map[string]any{"scope": "country"},
			"relocation":    map[string]any{"open": true, "targets": []string{"Amsterdam"}},
			"authorization": "Turkish citizen, no EU work permit yet",
		},
	})
}

func (d *desk) ingestTheRuleBatch(t *testing.T) {
	t.Helper()
	d.ingest(t,
		harvest{ID: "kill", Title: "Backend Engineer", Company: "Kill Co",
			Location: "Istanbul", URL: "https://example.invalid/kill", EasyApply: true},
		harvest{ID: "upgrade", Title: "Platform Engineer", Company: "Upgrade Co",
			Location: "Istanbul", URL: "https://example.invalid/upgrade", EasyApply: true},
		harvest{ID: "blocked", Title: "Go Engineer", Company: "ACME Gmbh",
			Location: "Istanbul", URL: "https://example.invalid/blocked", EasyApply: true},
		harvest{ID: "cooldown", Title: "Senior Go Engineer", Company: "Repeat Bank",
			Location: "Istanbul", URL: "https://example.invalid/cooldown", EasyApply: true},
		harvest{ID: "agency", Title: "Go Developer", Company: "Talent Partners",
			Location: "Istanbul", URL: "https://example.invalid/agency", EasyApply: true},
		harvest{ID: "lowpay", Title: "Go Engineer", Company: "Thrift Co",
			Location: "Berlin", URL: "https://example.invalid/lowpay", EasyApply: true},
		harvest{ID: "faraway", Title: "Go Engineer", Company: "Vistula Co",
			Location: "Warsaw, Poland", URL: "https://example.invalid/faraway", EasyApply: true},
		harvest{ID: "manual", Title: "Go Engineer", Company: "Manual Co",
			Location: "Istanbul", URL: "https://example.invalid/manual", EasyApply: false},
		harvest{ID: "tailor", Title: "Go Engineer", Company: "Ledger Co",
			Location: "Amsterdam, Netherlands", URL: "https://example.invalid/tailor",
			EasyApply: true},
		harvest{ID: "dupe", Title: "Senior Go Engineer", Company: "Repeat Bank",
			Location: "Ankara", URL: "https://example.invalid/dupe", EasyApply: true},
	)
}

func (d *desk) pendingIDs(t *testing.T) []string {
	t.Helper()
	var pending struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(
		[]byte(d.get(t, "/api/jobs/pending-descriptions")), &pending,
	); err != nil {
		t.Fatalf("decode pending: %v", err)
	}
	found := make([]string, 0, len(pending.Jobs))
	for _, entry := range pending.Jobs {
		found = append(found, entry.ID)
	}
	return found
}

func assertTriageGates(t *testing.T, first screenResult, board *desk) {
	t.Helper()
	if first.Gated != 2 {
		t.Fatalf("the pre-model gate fired %d times, want 2", first.Gated)
	}
	blocked := first.find(t, "blocked")
	if blocked.Status != "skipped" || !strings.Contains(blocked.Reason, "blocked company list") {
		t.Fatalf("blocked = %+v", blocked)
	}
	t.Logf("RULE blocked-company -> %s: %s", blocked.Status, blocked.Reason)

	cooldown := first.find(t, "cooldown")
	if cooldown.Status != "skipped" || !strings.Contains(cooldown.Reason, "reapply cooldown") {
		t.Fatalf("cooldown = %+v", cooldown)
	}
	t.Logf("RULE reapply-cooldown -> %s: %s", cooldown.Status, cooldown.Reason)

	if stored := board.postings(t)["dupe"]; stored.Status != "duplicate" {
		t.Fatalf("the reposted role did not collapse: %+v", stored)
	}
	t.Log("RULE duplicate -> the reposted role collapsed onto the first record")
}

func assertDeepRules(t *testing.T, second screenResult) {
	t.Helper()
	killed := second.find(t, "kill")
	if killed.Status != "skipped" || killed.Stage != "deep" {
		t.Fatalf("the description did not kill the posting: %+v", killed)
	}
	t.Logf("DESCRIPTION KILLED %s (%d): %s", killed.Status, killed.Score, killed.Reason)

	upgraded := second.find(t, "upgrade")
	if upgraded.Status != "inbox" || upgraded.Score < 80 {
		t.Fatalf("the description did not upgrade the posting: %+v", upgraded)
	}
	t.Logf("DESCRIPTION UPGRADED %s (%d): %s", upgraded.Status, upgraded.Score, upgraded.Reason)

	for name, want := range map[string]string{
		"agency":  "recruiting agency posting",
		"lowpay":  "under the EUR 60000 per year floor",
		"faraway": "outside every place on the list",
		"manual":  "manual send",
		"tailor":  "relocating to Amsterdam",
	} {
		found := second.find(t, name)
		if !strings.Contains(found.Reason, want) {
			t.Fatalf("%s = %+v, want a reason naming %q", name, found, want)
		}
		t.Logf("RULE %s -> %s: %s", name, found.Status, found.Reason)
	}
	if second.find(t, "manual").Status != "manual" {
		t.Fatalf("a non easy apply posting was not routed to manual: %+v", second.find(t, "manual"))
	}
	if second.find(t, "faraway").Status != "skipped" {
		t.Fatalf("a posting outside every place survived: %+v", second.find(t, "faraway"))
	}

	tailored := second.find(t, "tailor")
	if !tailored.Verdict.Tailored.Needed || tailored.Verdict.Tailored.Focus == "" {
		t.Fatalf("tailoredResume = %+v", tailored.Verdict.Tailored)
	}
	if tailored.Verdict.ResumeFit != "partial" {
		t.Fatalf("resumeFit = %q", tailored.Verdict.ResumeFit)
	}
	t.Logf(
		"RESUME FIT %s, tailored focus: %s",
		tailored.Verdict.ResumeFit,
		tailored.Verdict.Tailored.Focus,
	)
}

func TestScreeningJudgesInTwoStagesAndEveryRuleFires(t *testing.T) {
	board := newDesk(t, screeningScript(ruleAnswers()))
	board.rulesSettings(t)

	board.ingest(t, harvest{
		ID: "seed", Title: "Go Engineer", Company: "Repeat Bank",
		Location: "Istanbul", URL: "https://example.invalid/seed", EasyApply: true,
	})
	board.post(t, "/api/jobs/seed/applied", "")
	board.ingestTheRuleBatch(t)

	first := board.screen(t)
	t.Logf("TRIAGE kept %d, gated %d, dropped %d", first.Kept, first.Gated, first.Triaged-first.Kept-first.Gated)
	assertTriageGates(t, first, board)

	pending := board.pendingIDs(t)
	if len(pending) != first.Kept {
		t.Fatalf("pending = %d, kept = %d", len(pending), first.Kept)
	}
	bodies := map[string]string{}
	for _, id := range pending {
		bodies[id] = "We build payment services. Stack: Go, PostgreSQL, Kafka. Onsite."
	}
	bodies["kill"] = "This team writes Rust exclusively and owns the trading core in Rust."
	bodies["upgrade"] = "Behind the vague title sits a Go and PostgreSQL payments platform."
	board.describe(t, bodies)

	second := board.screen(t)
	t.Logf("DEEP screened %d, picked %d, manual %d, skipped %d",
		second.Screened, second.Picked, second.Manual, second.Skipped)
	assertDeepRules(t, second)
}

func TestANonEasyApplyPostingIsSkippedWhenTheSettingIsOff(t *testing.T) {
	board := newDesk(t, screeningScript(nil))
	board.settings(t, map[string]any{
		"roles": map[string]any{"applyToNonEasyApply": false},
	})
	board.ingest(t, harvest{
		ID: "slow", Title: "Go Engineer", Company: "Slow Co",
		Location: "Istanbul", URL: "https://example.invalid/slow", EasyApply: false,
	})
	board.screen(t)
	board.describe(t, map[string]string{"slow": "Go and PostgreSQL services, onsite."})
	result := board.screen(t)

	found := result.find(t, "slow")
	if found.Status != "skipped" {
		t.Fatalf("the setting being off did not skip the posting: %+v", found)
	}
	if !strings.Contains(found.Reason, "Not Easy Apply, and those are turned off") {
		t.Fatalf("reason = %q", found.Reason)
	}
	t.Logf("RULE non-easy-apply, setting off -> %s: %s", found.Status, found.Reason)
}

func TestATurnWithNoToolCallIsNudgedInsideAScreeningRun(t *testing.T) {
	script := &modelScript{}
	script.reply = func(turn modelTurn, seen int) string {
		if stream, handled := indexerReply(turn); handled {
			return stream
		}
		if turn.Tool == "triage_jobs" && seen == 1 {
			return proseStream
		}
		switch turn.Tool {
		case "triage_jobs":
			return toolCallStream("triage_jobs", keepEverything(idsIn(turn.User)))
		case "screen_jobs":
			return screenReply(idsIn(turn.User), nil)
		case "flag_verdicts":
			return noFlags()
		}
		return proseStream
	}

	board := newDesk(t, script)
	board.ingest(t, harvest{
		ID: "nudge", Title: "Go Engineer", Company: "Nudge Co",
		Location: "Istanbul", URL: "https://example.invalid/nudge", EasyApply: true,
	})

	result := board.screen(t)
	if result.Nudges != 1 {
		t.Fatalf("nudges = %d, want 1", result.Nudges)
	}
	if result.Kept != 1 || result.Error != "" {
		t.Fatalf("the batch did not recover: %+v", result)
	}
	if board.script.seen("triage_jobs") != 2 {
		t.Fatalf("triage turns = %d, want 2", board.script.seen("triage_jobs"))
	}
	nudged := board.script.last("triage_jobs")
	opened := strings.Index(nudged.User, "(system:")
	if opened < 0 || !strings.Contains(nudged.User, "your turn ended without a call to triage_jobs") {
		t.Fatalf("the second turn carried no nudge: %q", nudged.User)
	}
	t.Logf("NUDGE the second turn carried: %s", nudged.User[opened:])
}

func TestTheCorrectionPassCommitsOnlyTheSecondAnswer(t *testing.T) {
	script := &modelScript{}
	script.reply = func(turn modelTurn, seen int) string {
		if stream, handled := indexerReply(turn); handled {
			return stream
		}
		switch turn.Tool {
		case "triage_jobs":
			return toolCallStream("triage_jobs", keepEverything(idsIn(turn.User)))
		case "screen_jobs":
			ids := idsIn(turn.User)
			if strings.Contains(turn.User, "<reviewer-correction>") {
				return screenReply(ids, map[string]decision{
					"flagged": {
						Verdict: "skip",
						Reason: "read again, the body wants Rust for the core service and " +
							"the Go mention is a legacy footnote.",
						Score: 25,
						Fit:   "none",
					},
				})
			}
			return screenReply(ids, map[string]decision{
				"flagged": {
					Verdict: "apply",
					Reason:  "the posting names Go, so the core work fits.",
					Score:   84,
				},
			})
		case "flag_verdicts":
			if seen > 1 {
				return noFlags()
			}
			return toolCallStream("flag_verdicts", map[string]any{
				"flags": []map[string]string{{
					"id": "flagged",
					"instruction": "the description names Rust as the core language and Go only " +
						"as legacy, read the stack again.",
				}},
			})
		}
		return proseStream
	}

	board := newDesk(t, script)
	board.ingest(t, harvest{
		ID: "flagged", Title: "Backend Engineer", Company: "Rusty Co",
		Location: "Istanbul", URL: "https://example.invalid/flagged", EasyApply: true,
	})
	board.screen(t)
	board.describe(t, map[string]string{
		"flagged": "Core service in Rust. A legacy Go component is being retired.",
	})

	result := board.screen(t)
	if result.Flagged != 1 || result.Corrected != 1 {
		t.Fatalf("flagged = %d, corrected = %d", result.Flagged, result.Corrected)
	}
	found := result.find(t, "flagged")
	if !found.Flagged || found.Flag == "" || !found.Rewrote {
		t.Fatalf("the flag was not recorded on the verdict: %+v", found)
	}
	if found.Original == nil || found.Original.Status != "inbox" {
		t.Fatalf("the first answer was not carried: %+v", found)
	}
	if found.Status != "skipped" {
		t.Fatalf("the corrected verdict is not the one that landed: %+v", found)
	}
	t.Logf("FLAG %s", found.Flag)
	t.Logf("FIRST ANSWER %s: %s", found.Original.Status, found.Original.Reason)
	t.Logf("SECOND ANSWER %s: %s", found.Status, found.Reason)

	stored := board.postings(t)
	if stored["flagged"].Status != "skipped" {
		t.Fatalf("the store kept the first answer: %+v", stored["flagged"])
	}
	t.Logf("COMMITTED %s, so only the second answer was written", stored["flagged"].Status)

	traces := board.get(t, "/api/traces")
	for _, purpose := range []string{`"purpose":"review"`, `"purpose":"correction"`} {
		if !strings.Contains(traces, purpose) {
			t.Fatalf("the trace list is missing %s: %s", purpose, traces)
		}
	}
	t.Logf("TRACES carry both the review call and the correction call")
}

func TestSettingsChangeMakesVerdictsStaleAndARescreenClearsThem(t *testing.T) {
	board := newDesk(t, screeningScript(nil))
	board.ingest(t, harvest{
		ID: "stale", Title: "Go Engineer", Company: "Stale Co",
		Location: "Istanbul", URL: "https://example.invalid/stale", EasyApply: true,
	})
	board.screen(t)
	board.describe(t, map[string]string{"stale": "Go and PostgreSQL, onsite in Istanbul."})
	board.screen(t)

	before := board.get(t, "/api/jobs/stale")
	if !strings.Contains(before, `"stale":0`) {
		t.Fatalf("a fresh verdict already reads stale: %s", before)
	}
	t.Logf("STALE BEFORE %s", strings.TrimSpace(before))

	board.settings(t, map[string]any{
		"roles": map[string]any{"excludeStacks": []string{"php"}},
	})
	after := board.get(t, "/api/jobs/stale")
	if !strings.Contains(after, `"stale":1`) {
		t.Fatalf("the settings change left the verdict fresh: %s", after)
	}
	t.Logf("STALE AFTER SETTINGS CHANGE %s", strings.TrimSpace(after))

	reset := board.post(t, "/api/jobs/rescreen", "")
	if !strings.Contains(reset, `"reset":1`) {
		t.Fatalf("the rescreen reset nothing: %s", reset)
	}
	cleared := board.get(t, "/api/jobs/stale")
	if !strings.Contains(cleared, `"stale":0`) {
		t.Fatalf("the stale count survived the rescreen: %s", cleared)
	}
	t.Logf("RESCREEN %s, STALE NOW %s", strings.TrimSpace(reset), strings.TrimSpace(cleared))

	if board.postings(t)["stale"].Status != "new" {
		t.Fatalf("the rescreened posting is not unjudged: %+v", board.postings(t)["stale"])
	}
}

func TestARenamedCVFolderLeavesTheQueuedPostingResolvable(t *testing.T) {
	board := newDesk(t, screeningScript(nil))
	board.ingest(t, harvest{
		ID: "queued", Title: "Go Engineer", Company: "Rename Co",
		Location: "Istanbul", URL: "https://example.invalid/queued", EasyApply: true,
	})
	board.screen(t)
	board.describe(t, map[string]string{"queued": "Go and PostgreSQL services, onsite."})
	first := board.screen(t)
	if code := first.find(t, "queued").Verdict.ResumeCode; code != "GO" {
		t.Fatalf("resume code = %q", code)
	}
	board.post(t, "/api/jobs/queued/queue", "")

	if err := os.Rename(
		filepath.Join(board.cvDir, "go"),
		filepath.Join(board.cvDir, "platform"),
	); err != nil {
		t.Fatalf("rename the cv folder: %v", err)
	}
	if err := os.Rename(
		filepath.Join(board.cvDir, "go-tr"),
		filepath.Join(board.cvDir, "platform-tr"),
	); err != nil {
		t.Fatalf("rename the turkish cv folder: %v", err)
	}
	board.post(t, "/api/index", `{"force":true}`)
	board.waitForIndex(t)

	status, body := request(t, board.runtime, http.MethodGet, "/api/jobs?status=queue", "")
	if status != http.StatusOK {
		t.Fatalf("GET /api/jobs?status=queue = %d (%s)", status, body)
	}
	t.Logf("QUEUE AFTER RENAME %s", strings.TrimSpace(body))

	result := board.screen(t)
	if result.ScreenKey == "" {
		t.Fatal("the screen key is empty")
	}
	stored := board.postings(t)["queued"]
	if stored.Status == "queue" {
		t.Fatalf("the posting kept a resume code that is gone: %+v", stored)
	}
	rejudged := result.find(t, "queued")
	if rejudged.Verdict.ResumeCode == "GO" {
		t.Fatalf("a resume code that is gone was carried forward: %+v", rejudged.Verdict)
	}
	if rejudged.Verdict.ResumeCode != "BA" && rejudged.Verdict.ResumeCode != "PL" {
		t.Fatalf("the row was not judged against the current cv set: %+v", rejudged.Verdict)
	}
	t.Logf(
		"RENAMED the stored code GO is gone, the row was unscreened and judged again as %s with %s",
		stored.Status,
		rejudged.Verdict.ResumeCode,
	)

	status, body = request(t, board.runtime, http.MethodGet, "/api/resumes/GO/en/file", "")
	if status != http.StatusNotFound {
		t.Fatalf("the old code still resolves: %d (%s)", status, body)
	}
	t.Logf("OLD CODE GO now answers %d: %s", status, strings.TrimSpace(body))
}

func TestTheReflectorWritesLessonsToBothPromptsAndNeverTouchesAVerdict(t *testing.T) {
	script := &modelScript{}
	script.reply = func(turn modelTurn, _ int) string {
		if stream, handled := indexerReply(turn); handled {
			return stream
		}
		switch turn.Tool {
		case "triage_jobs":
			return toolCallStream("triage_jobs", keepEverything(idsIn(turn.User)))
		case "screen_jobs":
			return screenReply(idsIn(turn.User), nil)
		case "flag_verdicts":
			return noFlags()
		case "write_lessons":
			return toolCallStream("write_lessons", map[string]any{
				"lessons": []map[string]string{
					{
						"text": "Istanbul onsite payments roles answered every send, " +
							"so keep planning that city ring first.",
						"evidence": "3 of 3 Istanbul sends drew a reply",
						"scope":    "planning",
					},
					{
						"text": "Agency postings went silent, so weigh an agency down " +
							"even when the stack reads perfectly.",
						"evidence": "2 of 2 agency sends, no reply",
						"scope":    "screening",
					},
				},
				"retire": []int64{},
			})
		}
		return proseStream
	}

	board := newDesk(t, script)
	board.ingest(t,
		harvest{ID: "o1", Title: "Go Engineer", Company: "One Co",
			Location: "Istanbul", URL: "https://example.invalid/o1", EasyApply: true},
		harvest{ID: "o2", Title: "Go Engineer", Company: "Two Co",
			Location: "Istanbul", URL: "https://example.invalid/o2", EasyApply: true},
		harvest{ID: "o3", Title: "Go Engineer", Company: "Three Co",
			Location: "Istanbul", URL: "https://example.invalid/o3", EasyApply: true},
	)
	board.screen(t)
	board.describe(t, map[string]string{
		"o1": "Go and PostgreSQL payments platform, onsite in Istanbul.",
		"o2": "Go services for a bank, onsite in Istanbul.",
		"o3": "Go and Kafka, onsite in Istanbul, hired through a staffing partner.",
	})
	board.screen(t)

	for _, id := range []string{"o1", "o2", "o3"} {
		board.post(t, "/api/jobs/"+id+"/applied", "")
	}
	for id, outcome := range map[string]string{
		"o1": "interview", "o2": "recruiter", "o3": "no-response",
	} {
		document, err := json.Marshal(map[string]string{
			"outcome": outcome,
			"note":    "recorded by hand",
		})
		if err != nil {
			t.Fatalf("encode outcome: %v", err)
		}
		board.post(t, "/api/jobs/"+id+"/outcome", string(document))
	}

	before := board.postings(t)
	reflection := board.post(t, "/api/reflect", `{"force":true}`)
	t.Logf("REFLECTION %s", strings.TrimSpace(reflection))

	var answered struct {
		Ran     bool `json:"ran"`
		Read    int  `json:"read"`
		Written int  `json:"written"`
		Lessons []struct {
			Text     string `json:"text"`
			Evidence string `json:"evidence"`
			Scope    string `json:"scope"`
		} `json:"lessons"`
	}
	if err := json.Unmarshal([]byte(reflection), &answered); err != nil {
		t.Fatalf("decode the reflection: %v", err)
	}
	if !answered.Ran || answered.Written != 2 || answered.Read != 3 {
		t.Fatalf("reflection = %+v", answered)
	}
	scopes := map[string]string{}
	for _, lesson := range answered.Lessons {
		scopes[lesson.Scope] = lesson.Text
		t.Logf("LESSON [%s] %s (%s)", lesson.Scope, lesson.Text, lesson.Evidence)
	}
	if scopes["planning"] == "" || scopes["screening"] == "" {
		t.Fatalf("the lessons were not routed to both readers: %+v", answered.Lessons)
	}

	after := board.postings(t)
	for id, posting := range before {
		if after[id].Status != posting.Status {
			t.Fatalf("the reflector moved %s from %s to %s", id, posting.Status, after[id].Status)
		}
	}
	t.Log("VERDICTS UNCHANGED, the reflector only wrote notes")

	notes := board.get(t, "/api/notes")
	if !strings.Contains(notes, scopes["planning"]) {
		t.Fatalf("the planning lesson is not in the notes: %s", notes)
	}

	preview := board.get(t, "/api/prompt/preview")
	var prompts struct {
		Prompts []struct {
			ID          string  `json:"id"`
			System      string  `json:"system"`
			Unavailable *string `json:"unavailable"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal([]byte(preview), &prompts); err != nil {
		t.Fatalf("decode the preview: %v", err)
	}
	assembled := map[string]string{}
	for _, prompt := range prompts.Prompts {
		assembled[prompt.ID] = prompt.System
	}
	for _, wanted := range []string{"planning", "triage", "deep", "review", "reflector"} {
		if _, present := assembled[wanted]; !present {
			t.Fatalf("the preview never carries the %s prompt", wanted)
		}
	}
	if !strings.Contains(assembled["deep"], scopes["screening"]) {
		t.Fatalf("the screening lesson never reached the screening prompt")
	}
	t.Logf("SCREENING LESSON reached the deep prompt")

	board.plannerCheckIn(t)
	planning := board.previewPrompt(t, "planning")
	if !strings.Contains(planning, scopes["planning"]) {
		t.Fatalf("the planning lesson never reached the planning prompt:\n%s", planning)
	}
	t.Logf("PLANNING LESSON reached the planning prompt")
}

func (d *desk) plannerCheckIn(t *testing.T) {
	t.Helper()
	hello, err := json.Marshal(map[string]any{
		"id":      "stub-extension",
		"version": "0.0.1-test",
		"capabilities": map[string]any{
			"fetch": map[string]any{
				"place":            true,
				"rings":            []string{"city", "country", "region", "worldwide"},
				"ranges":           []string{"r86400", "r604800"},
				"easyApply":        true,
				"remoteOnly":       true,
				"sorts":            []string{"DD"},
				"keyword":          false,
				"pageSize":         25,
				"maxPagesPerQuery": 8,
			},
			"seenSet":    true,
			"autoAttach": true,
		},
	})
	if err != nil {
		t.Fatalf("encode hello: %v", err)
	}
	d.post(t, "/api/plugin/hello", string(hello))
}

func (d *desk) previewPrompt(t *testing.T, id string) string {
	t.Helper()
	var prompts struct {
		Prompts []struct {
			ID     string `json:"id"`
			System string `json:"system"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal([]byte(d.get(t, "/api/prompt/preview")), &prompts); err != nil {
		t.Fatalf("decode the preview: %v", err)
	}
	for _, prompt := range prompts.Prompts {
		if prompt.ID == id {
			return prompt.System
		}
	}
	t.Fatalf("the preview carries no %s prompt", id)
	return ""
}

func TestScreeningEventsArriveInOrderWithTheVerdict(t *testing.T) {
	board := newDesk(t, screeningScript(nil))
	base := serve(t, board.runtime)

	stream := startCurlStream(t, board.runtime, base)
	board.ingest(t, harvest{
		ID: "watched", Title: "Go Engineer", Company: "Watch Co",
		Location: "Istanbul", URL: "https://example.invalid/watched", EasyApply: true,
	})
	board.screen(t)
	board.describe(t, map[string]string{"watched": "Go and PostgreSQL, onsite in Istanbul."})
	board.screen(t)
	time.Sleep(300 * time.Millisecond)

	ordered := make([]string, 0, 32)
	for _, entry := range stream.snapshot() {
		if !strings.HasPrefix(entry.line, "data:") {
			continue
		}
		var frame struct {
			Type   string `json:"type"`
			Phase  string `json:"phase"`
			Status string `json:"status"`
			ID     string `json:"id"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(entry.line, "data:")), &frame); err != nil {
			continue
		}
		if frame.Type != "screen" {
			continue
		}
		label := frame.Phase
		if frame.ID != "" {
			label = fmt.Sprintf("%s %s -> %s", frame.Phase, frame.ID, frame.Status)
		}
		ordered = append(ordered, label)
		t.Logf("EVENT screen %s", label)
	}

	wanted := []string{"triage", "reading", "done", "deep", "verdict watched -> inbox", "done"}
	at := 0
	for _, phase := range wanted {
		found := -1
		for position := at; position < len(ordered); position++ {
			if strings.HasPrefix(ordered[position], phase) {
				found = position
				break
			}
		}
		if found < 0 {
			t.Fatalf("the stream never carried %q in order, it carried %v", phase, ordered)
		}
		at = found + 1
	}
}

func lessonScript() *modelScript {
	script := &modelScript{}
	script.reply = func(turn modelTurn, _ int) string {
		if stream, handled := indexerReply(turn); handled {
			return stream
		}
		if turn.Tool == "write_lessons" {
			return toolCallStream("write_lessons", map[string]any{
				"lessons": []map[string]string{{
					"text":     "Agency sends went silent, weigh an agency down at screening time.",
					"evidence": "3 of 5 agency sends, no reply",
					"scope":    "screening",
				}},
				"retire": []int64{},
			})
		}
		return proseStream
	}
	return script
}

func TestTheReflectorRunsItselfWhenEnoughOutcomesAccumulate(t *testing.T) {
	board := newDesk(t, lessonScript())

	for position := range 5 {
		id := "auto" + itoa(position)
		board.ingest(t, harvest{
			ID: id, Title: "Go Engineer", Company: "Auto Co " + itoa(position),
			Location: "Istanbul", URL: "https://example.invalid/" + id, EasyApply: true,
		})
		board.post(t, "/api/jobs/"+id+"/applied", "")
	}

	var last string
	for position := range 5 {
		id := "auto" + itoa(position)
		document, err := json.Marshal(map[string]string{"outcome": "no-response"})
		if err != nil {
			t.Fatalf("encode outcome: %v", err)
		}
		last = board.post(t, "/api/jobs/"+id+"/outcome", string(document))
	}
	t.Logf("OUTCOME RESPONSE %s", last)
	if !strings.Contains(last, `"due":true`) {
		t.Fatalf("the fifth outcome did not read as due: %s", last)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if board.script.seen("write_lessons") > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if board.script.seen("write_lessons") != 1 {
		t.Fatalf("the reflector never ran on its own: %d calls",
			board.script.seen("write_lessons"))
	}

	notes := ""
	for time.Now().Before(deadline) {
		notes = board.get(t, "/api/notes")
		if strings.Contains(notes, "weigh an agency down") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(notes, "weigh an agency down") {
		t.Fatalf("the lesson never landed: %s", notes)
	}
	t.Logf("AUTOMATIC REFLECTION wrote %s", strings.TrimSpace(notes))
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

func rowKeys(t *testing.T, row map[string]any) []string {
	t.Helper()
	keys := make([]string, 0, len(row))
	for key := range row {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func listedRows(t *testing.T, board *desk, status string) map[string]map[string]any {
	t.Helper()
	var answered struct {
		Jobs []map[string]any `json:"jobs"`
	}
	target := "/api/jobs"
	if status != "" {
		target += "?status=" + status
	}
	if err := json.Unmarshal([]byte(board.get(t, target)), &answered); err != nil {
		t.Fatalf("decode the list: %v", err)
	}
	rows := make(map[string]map[string]any, len(answered.Jobs))
	for _, row := range answered.Jobs {
		id, _ := row["id"].(string)
		rows[id] = row
	}
	return rows
}

func quoteRow(t *testing.T, label string, row map[string]any) {
	t.Helper()
	document, err := json.MarshalIndent(row, "", "  ")
	if err != nil {
		t.Fatalf("encode %s: %v", label, err)
	}
	t.Logf("%s\n%s", label, document)
}

var listedKeys = []string{
	"agency", "company", "contractType", "createdAt", "cycleId", "description",
	"descriptionState", "dupeOf", "easyApply", "id", "listedAt", "location", "outcome",
	"outcomeNote", "postingLang", "queryLabel", "resumeCode", "resumeFit", "resumeLang",
	"score", "seniority", "stage", "stale", "statedPay", "status", "tailoredResume", "title",
	"triageReason", "updatedAt", "url", "verdictReason", "workplace",
}

func TestTheJobListCarriesEveryVerdictFieldTheContractNames(t *testing.T) {
	board := newDesk(t, screeningScript(map[string]decision{
		"judged": {
			Verdict: "apply",
			Reason:  "the core work is Go services on PostgreSQL.",
			Score:   84,
			Fit:     "partial",
			Focus:   "lead with the event sourced payments ledger built in Go.",
			Pay: map[string]any{
				"present": true, "amount": 90000, "currency": "EUR", "period": "year",
			},
		},
		"handsend": {
			Verdict: "apply",
			Reason:  "the stack fits and the posting cannot be auto-filled.",
			Score:   71,
		},
	}))

	if _, err := board.runtime.Settings.Save(context.Background(), map[string]any{
		"roles": map[string]any{"applyToNonEasyApply": true},
	}); err != nil {
		t.Fatalf("keep non easy apply postings: %v", err)
	}

	board.ingest(t,
		harvest{ID: "judged", Title: "Go Engineer", Company: "Judged Co",
			Location: "Istanbul", URL: "https://example.invalid/judged", EasyApply: true},
		harvest{ID: "handsend", Title: "Go Engineer", Company: "Handsend Co",
			Location: "Istanbul", URL: "https://example.invalid/handsend", EasyApply: false},
	)
	board.screen(t)
	board.describe(t, map[string]string{
		"judged":   "Go, PostgreSQL and Kafka on a payments platform, onsite in Istanbul.",
		"handsend": "Go and PostgreSQL services, onsite in Istanbul.",
	})
	board.screen(t)

	board.ingest(t, harvest{
		ID: "unjudged", Title: "Go Engineer", Company: "Unjudged Co",
		Location: "Istanbul", URL: "https://example.invalid/unjudged", EasyApply: true,
	})

	rows := listedRows(t, board, "all")
	for _, id := range []string{"judged", "handsend", "unjudged"} {
		if _, present := rows[id]; !present {
			t.Fatalf("the list is missing %s", id)
		}
		if keys := rowKeys(t, rows[id]); !slices.Equal(keys, listedKeys) {
			t.Fatalf("%s keys = %v, want %v", id, keys, listedKeys)
		}
	}
	quoteRow(t, "JUDGED ROW", rows["judged"])
	quoteRow(t, "UNJUDGED ROW", rows["unjudged"])
	quoteRow(t, "MANUAL ROW", rows["handsend"])

	judged := rows["judged"]
	if judged["status"] != "inbox" || judged["stage"] != "deep" {
		t.Fatalf("judged = %+v", judged)
	}
	if judged["score"] != float64(84) {
		t.Fatalf("score = %v", judged["score"])
	}
	if judged["resumeCode"] != "GO" || judged["resumeFit"] != "partial" {
		t.Fatalf("resume = %v/%v", judged["resumeCode"], judged["resumeFit"])
	}
	tailoredResume, ok := judged["tailoredResume"].(map[string]any)
	if !ok || tailoredResume["needed"] != true || tailoredResume["focus"] == "" {
		t.Fatalf("tailoredResume = %v", judged["tailoredResume"])
	}
	statedPay, ok := judged["statedPay"].(map[string]any)
	if !ok || statedPay["amount"] != float64(90000) || statedPay["currency"] != "EUR" {
		t.Fatalf("statedPay = %v", judged["statedPay"])
	}
	if judged["stale"] != false {
		t.Fatalf("a fresh verdict reads stale: %v", judged["stale"])
	}

	unjudged := rows["unjudged"]
	if unjudged["status"] != "new" || unjudged["stage"] != "none" {
		t.Fatalf("unjudged = %+v", unjudged)
	}
	for _, field := range []string{
		"score", "triageReason", "verdictReason", "seniority", "workplace", "contractType",
		"postingLang", "agency", "statedPay", "resumeCode", "resumeLang", "resumeFit",
		"tailoredResume", "outcome", "outcomeNote",
	} {
		if unjudged[field] != nil {
			t.Fatalf("an unjudged row invented %s = %v", field, unjudged[field])
		}
	}

	manual := rows["handsend"]
	if manual["status"] != "manual" {
		t.Fatalf("manual = %+v", manual)
	}
	if manual["statedPay"] != nil {
		t.Fatalf("a posting that named no pay carries one: %v", manual["statedPay"])
	}

	board.settings(t, map[string]any{
		"roles": map[string]any{"excludeStacks": []string{"php"}},
	})
	stale := listedRows(t, board, "all")
	quoteRow(t, "STALE ROW", stale["judged"])
	if stale["judged"]["stale"] != true {
		t.Fatalf("the settings change left the row fresh: %v", stale["judged"]["stale"])
	}
	if stale["unjudged"]["stale"] != false {
		t.Fatalf("an unjudged row reads stale: %v", stale["unjudged"]["stale"])
	}

	board.post(t, "/api/jobs/judged/applied", "")
	outcome, err := json.Marshal(map[string]string{"outcome": "interview", "note": "phone screen"})
	if err != nil {
		t.Fatalf("encode outcome: %v", err)
	}
	board.post(t, "/api/jobs/judged/outcome", string(outcome))

	recorded := listedRows(t, board, "applied")["judged"]
	if keys := rowKeys(t, recorded); !slices.Equal(keys, listedKeys) {
		t.Fatalf("the filtered list has a different shape: %v", keys)
	}
	if recorded["outcome"] != "interview" || recorded["outcomeNote"] != "phone screen" {
		t.Fatalf("outcome = %v / %v", recorded["outcome"], recorded["outcomeNote"])
	}
	quoteRow(t, "APPLIED ROW WITH AN OUTCOME", recorded)
}
