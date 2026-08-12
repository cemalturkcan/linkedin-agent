package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePDF(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func isolateHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func decodeBody(t *testing.T, body string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), target); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
}

type settingsBody struct {
	Settings struct {
		Locations struct {
			Places []struct {
				Name string `json:"name"`
				Ring string `json:"ring"`
				Kind string `json:"kind"`
			} `json:"places"`
			Relocation struct {
				Open bool `json:"open"`
			} `json:"relocation"`
			Workplace struct {
				Onsite bool   `json:"onsite"`
				Hybrid bool   `json:"hybrid"`
				Remote bool   `json:"remote"`
				Scope  string `json:"scope"`
			} `json:"workplace"`
		} `json:"locations"`
		Budget struct {
			CycleMinutes int `json:"cycleMinutes"`
		} `json:"budget"`
		Apply struct {
			ResumeLanguages []string `json:"resumeLanguages"`
		} `json:"apply"`
	} `json:"settings"`
	Enums struct {
		SeniorityLadder []string `json:"seniorityLadder"`
		PlaceRings      []string `json:"placeRings"`
	} `json:"enums"`
}

func TestSettingsShipWideAndCarryTheEnumerations(t *testing.T) {
	runtime := newTestRuntime(t)

	status, body := request(t, runtime, http.MethodGet, "/api/settings", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s)", status, body)
	}
	var answer settingsBody
	decodeBody(t, body, &answer)

	workplace := answer.Settings.Locations.Workplace
	if !workplace.Onsite || !workplace.Hybrid || !workplace.Remote || workplace.Scope != "global" {
		t.Fatalf("workplace = %+v", workplace)
	}
	if !answer.Settings.Locations.Relocation.Open {
		t.Fatal("relocation ships closed")
	}
	if len(answer.Settings.Locations.Places) != 0 {
		t.Fatalf("places = %+v", answer.Settings.Locations.Places)
	}
	if len(answer.Enums.SeniorityLadder) == 0 || len(answer.Enums.PlaceRings) == 0 {
		t.Fatalf("enums = %+v", answer.Enums)
	}
}

func TestSettingsDeepMergeAndRefuseAnInvalidRing(t *testing.T) {
	runtime := newTestRuntime(t)

	patch := `{"locations": {"places": [
		{"name": "Istanbul", "ring": "city", "kind": "commute"},
		{"name": "Nowhere", "ring": "galaxy", "kind": "commute"}
	]}}`
	status, body := request(t, runtime, http.MethodPut, "/api/settings", patch)
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s)", status, body)
	}

	status, body = request(t, runtime, http.MethodPut, "/api/settings", `{"budget": {"cycleMinutes": 25}}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s)", status, body)
	}

	status, body = request(t, runtime, http.MethodGet, "/api/settings", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s)", status, body)
	}
	var answer settingsBody
	decodeBody(t, body, &answer)

	places := answer.Settings.Locations.Places
	if len(places) != 2 {
		t.Fatalf("places = %+v", places)
	}
	if places[0].Name != "Istanbul" || places[0].Ring != "city" || places[0].Kind != "commute" {
		t.Fatalf("place = %+v", places[0])
	}
	if places[1].Ring != "city" {
		t.Fatalf("the invalid ring was stored: %+v", places[1])
	}
	if answer.Settings.Budget.CycleMinutes != 25 {
		t.Fatalf("cycleMinutes = %d", answer.Settings.Budget.CycleMinutes)
	}
	if !answer.Settings.Locations.Workplace.Remote {
		t.Fatal("a later patch dropped an untouched group")
	}
}

type setupBody struct {
	Configured bool     `json:"configured"`
	Ready      bool     `json:"ready"`
	Missing    []string `json:"missing"`
	IndexState string   `json:"indexState"`
	Indexing   bool     `json:"indexing"`
	ResumeDir  string   `json:"resumeDir"`
	Candidates []struct {
		Dir   string `json:"dir"`
		Count int    `json:"count"`
	} `json:"candidates"`
	Resumes []struct {
		Code      string   `json:"code"`
		Label     string   `json:"label"`
		Languages []string `json:"languages"`
	} `json:"resumes"`
}

func TestSetupNamesWhatIsMissingBeforeAFolderIsChosen(t *testing.T) {
	isolateHome(t)
	runtime := newTestRuntime(t)

	status, body := request(t, runtime, http.MethodGet, "/api/setup", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s)", status, body)
	}
	var answer setupBody
	decodeBody(t, body, &answer)

	if answer.Configured || answer.Ready {
		t.Fatalf("state = %+v", answer)
	}
	if answer.IndexState != "never" {
		t.Fatalf("indexState = %q", answer.IndexState)
	}
	if !strings.Contains(strings.Join(answer.Missing, "|"), "cv pdfs") {
		t.Fatalf("missing = %v", answer.Missing)
	}
}

func TestSetupRefusesAFolderWithNoPDFs(t *testing.T) {
	isolateHome(t)
	runtime := newTestRuntime(t)

	empty := t.TempDir()
	status, body := request(t, runtime, http.MethodPost, "/api/setup", `{"dir": "`+empty+`"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d (%s)", status, body)
	}
	if !strings.Contains(body, "no pdfs") {
		t.Fatalf("body = %s", body)
	}

	status, body = request(t, runtime, http.MethodPost, "/api/setup", `{"dir": "`+empty+`/absent"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d (%s)", status, body)
	}
	if !strings.Contains(body, "not there") {
		t.Fatalf("body = %s", body)
	}

	status, body = request(t, runtime, http.MethodPost, "/api/setup", `{"dir": "  "}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d (%s)", status, body)
	}
}

func TestTheResumeListAndTheFileComeFromTheFolderAtRequestTime(t *testing.T) {
	isolateHome(t)
	runtime := newTestRuntime(t)

	status, body := request(t, runtime, http.MethodGet, "/api/resumes", "")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d (%s)", status, body)
	}

	folder := t.TempDir()
	writePDF(t, filepath.Join(folder, "backend", "one.pdf"), "%PDF-first")
	writePDF(t, filepath.Join(folder, "backend-tr", "one.pdf"), "%PDF-turkish")
	writePDF(t, filepath.Join(folder, "go.pdf"), "%PDF-go")

	status, body = request(t, runtime, http.MethodPost, "/api/setup", `{"dir": "`+folder+`"}`)
	if status != http.StatusOK {
		t.Fatalf("setup status = %d (%s)", status, body)
	}
	var state setupBody
	decodeBody(t, body, &state)
	if len(state.Resumes) != 2 {
		t.Fatalf("resumes = %+v", state.Resumes)
	}
	if state.Resumes[0].Code != "BA" || state.Resumes[1].Code != "GO" {
		t.Fatalf("codes = %+v", state.Resumes)
	}
	if strings.Join(state.Resumes[0].Languages, ",") != "en,tr" {
		t.Fatalf("languages = %v", state.Resumes[0].Languages)
	}

	status, body = request(t, runtime, http.MethodGet, "/api/resumes/BA/en/file", "")
	if status != http.StatusOK || body != "%PDF-first" {
		t.Fatalf("status = %d body = %q", status, body)
	}

	writePDF(t, filepath.Join(folder, "backend", "one.pdf"), "%PDF-replaced")
	status, body = request(t, runtime, http.MethodGet, "/api/resumes/BA/en/file", "")
	if status != http.StatusOK || body != "%PDF-replaced" {
		t.Fatalf("a replaced pdf still served the old bytes: status = %d body = %q", status, body)
	}

	status, body = request(t, runtime, http.MethodGet, "/api/resumes/BA/de/file", "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d (%s)", status, body)
	}
	status, body = request(t, runtime, http.MethodGet, "/api/resumes/ZZ/en/file", "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d (%s)", status, body)
	}
}

func TestProfileReportsNeverIndexedWithoutCallingItAnError(t *testing.T) {
	runtime := newTestRuntime(t)

	folder := t.TempDir()
	writePDF(t, filepath.Join(folder, "backend.pdf"), "%PDF-first")
	if _, _, err := runtime.Resumes.SetFolder(t.Context(), folder); err != nil {
		t.Fatalf("set folder: %v", err)
	}

	status, body := request(t, runtime, http.MethodGet, "/api/profile", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s)", status, body)
	}
	var answer struct {
		IndexState string `json:"indexState"`
		Indexing   bool   `json:"indexing"`
		Candidate  any    `json:"candidate"`
		Resumes    []struct {
			Code    string `json:"code"`
			Indexed bool   `json:"indexed"`
		} `json:"resumes"`
	}
	decodeBody(t, body, &answer)

	if answer.IndexState != "never" {
		t.Fatalf("indexState = %q", answer.IndexState)
	}
	if answer.Indexing {
		t.Fatal("nothing is running, indexing should be false")
	}
	if answer.Candidate != nil {
		t.Fatalf("candidate = %v", answer.Candidate)
	}
	if len(answer.Resumes) != 1 || answer.Resumes[0].Indexed {
		t.Fatalf("resumes = %+v", answer.Resumes)
	}
}
