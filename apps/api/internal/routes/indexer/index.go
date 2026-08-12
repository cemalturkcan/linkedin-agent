package indexer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"api/internal/app/claude"
	"api/internal/app/clock"
	"api/internal/app/events"
	"api/internal/app/store"
	indexerdb "api/internal/routes/indexer/gen"
	"api/internal/routes/resumes"
)

const (
	StateNever   = "never"
	StateStale   = "stale"
	StateCurrent = "current"

	PhaseIdle        = "idle"
	PhaseStarting    = "starting"
	PhaseExtracting  = "extracting"
	PhaseProfiling   = "profiling"
	PhaseSynthesised = "synthesising"
	PhaseDone        = "done"
	PhaseFailed      = "failed"
)

var ErrIndexing = errors.New("indexing is already running, wait for it to finish")

type Config struct {
	Binary            string
	ExtractTimeout    time.Duration
	MaxOutputBytes    int
	MaxCharsPerResume int
	MaxCharsPerCall   int
	RunBudget         time.Duration
}

type Progress struct {
	Phase string `json:"phase"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

type ResumeState struct {
	Code          string         `json:"code"`
	Label         string         `json:"label"`
	FileLanguages []string       `json:"fileLanguages"`
	Indexed       bool           `json:"indexed"`
	Profile       *ResumeProfile `json:"profile"`
}

type State struct {
	ResumeDir  string        `json:"resumeDir"`
	Candidate  *Candidate    `json:"candidate"`
	IndexedAt  string        `json:"indexedAt"`
	Resumes    []ResumeState `json:"resumes"`
	IndexState string        `json:"indexState"`
	Indexing   bool          `json:"indexing"`
	Progress   Progress      `json:"progress"`
	Error      string        `json:"error,omitempty"`
}

type Result struct {
	ResumeDir string          `json:"resumeDir"`
	Indexed   int             `json:"indexed"`
	Cached    int             `json:"cached"`
	Calls     int             `json:"calls"`
	Model     string          `json:"model"`
	Candidate Candidate       `json:"candidate"`
	Resumes   []ResumeProfile `json:"resumes"`
}

type Dependencies struct {
	Resumes *resumes.Service
	Claude  *claude.Client
	Reads   store.DBTX
	Hub     *events.Hub
	Clock   clock.Clock
	Config  Config
}

type Service struct {
	resumes   *resumes.Service
	claude    *claude.Client
	queries   *indexerdb.Queries
	hub       *events.Hub
	extractor *Extractor
	config    Config
	clock     clock.Clock

	mu       sync.Mutex
	running  bool
	progress Progress
}

func NewService(dependencies Dependencies) *Service {
	return &Service{
		resumes: dependencies.Resumes,
		claude:  dependencies.Claude,
		queries: indexerdb.New(dependencies.Reads),
		hub:     dependencies.Hub,
		extractor: NewExtractor(
			dependencies.Config.Binary,
			dependencies.Config.ExtractTimeout,
			dependencies.Config.MaxOutputBytes,
			dependencies.Config.MaxCharsPerResume,
		),
		config:   dependencies.Config,
		clock:    dependencies.Clock,
		progress: Progress{Phase: PhaseIdle},
	}
}

type source struct {
	resume resumes.Resume
	hash   string
}

func (s *Service) sources(ctx context.Context) (string, []source, error) {
	dir, found, err := s.resumes.List(ctx)
	if err != nil {
		return dir, nil, err
	}
	shape := profileShape()
	sources := make([]source, 0, len(found))
	for _, resume := range found {
		hash, err := hashResume(shape, resume)
		if err != nil {
			return dir, nil, err
		}
		sources = append(sources, source{resume: resume, hash: hash})
	}
	return dir, sources, nil
}

func hashResume(shape string, resume resumes.Resume) (string, error) {
	digest := sha256.New()
	digest.Write([]byte(shape))
	languages := make([]string, 0, len(resume.Files))
	for language := range resume.Files {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	for _, language := range languages {
		digest.Write([]byte(language))
		file, err := os.Open(resume.Files[language])
		if err != nil {
			return "", fmt.Errorf("read %s: %w", resume.Files[language], err)
		}
		_, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return "", fmt.Errorf("read %s: %w", resume.Files[language], err)
		}
	}
	return hex.EncodeToString(digest.Sum(nil))[:32], nil
}

func sourceKey(sources []source) string {
	keys := make([]string, 0, len(sources))
	for _, entry := range sources {
		keys = append(keys, entry.resume.Code+":"+entry.hash)
	}
	sort.Strings(keys)
	digest := sha256.New()
	for _, key := range keys {
		digest.Write([]byte(key + ";"))
	}
	return hex.EncodeToString(digest.Sum(nil))[:32]
}

func (s *Service) storedProfiles(ctx context.Context) (map[string]ResumeProfile, error) {
	rows, err := s.queries.ResumeProfiles(ctx)
	if err != nil {
		return nil, err
	}
	profiles := make(map[string]ResumeProfile, len(rows))
	for _, row := range rows {
		var profile ResumeProfile
		if err := json.Unmarshal([]byte(row.Profile), &profile); err != nil {
			continue
		}
		profiles[row.Code+":"+row.Hash] = profile
	}
	return profiles, nil
}

func (s *Service) storedCandidate(ctx context.Context) (indexerdb.CandidateProfileRow, bool, error) {
	row, err := s.queries.CandidateProfile(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return indexerdb.CandidateProfileRow{}, false, nil
	}
	if err != nil {
		return indexerdb.CandidateProfileRow{}, false, err
	}
	return row, true, nil
}

func (s *Service) State(ctx context.Context) State {
	snapshot := s.snapshot()
	state := State{
		IndexState: StateNever,
		Indexing:   snapshot.Phase != PhaseIdle,
		Progress:   snapshot,
		Resumes:    []ResumeState{},
	}

	dir, sources, err := s.sources(ctx)
	state.ResumeDir = dir
	if err != nil {
		state.Error = err.Error()
		return state
	}

	stored, err := s.storedProfiles(ctx)
	if err != nil {
		state.Error = err.Error()
		return state
	}
	candidate, present, err := s.storedCandidate(ctx)
	if err != nil {
		state.Error = err.Error()
		return state
	}

	everyIndexed := true
	for _, entry := range sources {
		profile, indexed := stored[entry.resume.Code+":"+entry.hash]
		everyIndexed = everyIndexed && indexed
		record := ResumeState{
			Code:          entry.resume.Code,
			Label:         entry.resume.Label,
			FileLanguages: entry.resume.Languages,
			Indexed:       indexed,
		}
		if indexed {
			record.Profile = &profile
		}
		state.Resumes = append(state.Resumes, record)
	}

	if !present {
		return state
	}
	var derived Candidate
	if err := json.Unmarshal([]byte(candidate.Profile), &derived); err != nil {
		return state
	}
	state.Candidate = &derived
	state.IndexedAt = candidate.IndexedAt
	state.IndexState = StateStale
	if candidate.SourceKey == sourceKey(sources) && everyIndexed {
		state.IndexState = StateCurrent
	}
	return state
}

func (s *Service) Start(ctx context.Context, force bool) {
	if !s.begin() {
		return
	}
	detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.config.RunBudget)
	go func() {
		defer cancel()
		defer s.end()
		if _, err := s.guarded(detached, force); err != nil {
			slog.Error("indexing failed", "err", err)
		}
	}()
}

func (s *Service) Run(ctx context.Context, force bool) (Result, error) {
	if !s.begin() {
		return Result{}, ErrIndexing
	}
	defer s.end()
	return s.guarded(ctx, force)
}

func (s *Service) guarded(ctx context.Context, force bool) (Result, error) {
	result, err := s.index(ctx, force)
	if err != nil {
		s.setProgress(Progress{Phase: PhaseIdle})
		s.hub.Emit(events.TypeIndex, map[string]any{"phase": PhaseFailed, "message": err.Error()})
		s.hub.Emit(events.TypeError, map[string]any{"message": err.Error()})
		return Result{}, err
	}
	return result, nil
}

func (s *Service) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	s.progress = Progress{Phase: PhaseStarting}
	s.hub.Emit(events.TypeIndex, s.progress)
	return true
}

func (s *Service) end() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.progress = Progress{Phase: PhaseIdle}
}

func (s *Service) Indexing() bool {
	return s.snapshot().Phase != PhaseIdle
}

func (s *Service) snapshot() Progress {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return Progress{Phase: PhaseIdle}
	}
	return s.progress
}

func (s *Service) setProgress(next Progress) {
	s.mu.Lock()
	s.progress = next
	s.mu.Unlock()
	s.hub.Emit(events.TypeIndex, next)
}

func (s *Service) advance(done int) {
	s.mu.Lock()
	s.progress.Done = done
	next := s.progress
	s.mu.Unlock()
	s.hub.Emit(events.TypeIndex, next)
}

type pending struct {
	code      string
	label     string
	hash      string
	languages []string
	text      string
}

func (s *Service) index(ctx context.Context, force bool) (Result, error) {
	dir, sources, err := s.sources(ctx)
	if err != nil {
		return Result{}, err
	}
	if len(sources) == 0 {
		return Result{}, refuse("no resumes to index")
	}

	stored := map[string]ResumeProfile{}
	if !force {
		if stored, err = s.storedProfiles(ctx); err != nil {
			return Result{}, err
		}
	}
	key := sourceKey(sources)
	missing := make([]source, 0, len(sources))
	for _, entry := range sources {
		if _, indexed := stored[entry.resume.Code+":"+entry.hash]; !indexed {
			missing = append(missing, entry)
		}
	}

	if !force && len(missing) == 0 {
		if result, fresh := s.cached(ctx, dir, key, sources, stored); fresh {
			s.finish(result)
			return result, nil
		}
	}

	extracted, err := s.extract(ctx, missing)
	if err != nil {
		return Result{}, err
	}

	calls, err := s.profile(ctx, extracted, stored)
	if err != nil {
		return Result{}, err
	}

	profiles := make([]ResumeProfile, 0, len(sources))
	for _, entry := range sources {
		profiles = append(profiles, stored[entry.resume.Code+":"+entry.hash])
	}

	s.setProgress(Progress{Phase: PhaseSynthesised, Total: 1})
	candidate, model, err := s.synthesise(ctx, profiles)
	if err != nil {
		return Result{}, err
	}
	calls++

	if err := s.persist(ctx, key, model, candidate, sources); err != nil {
		return Result{}, err
	}

	result := Result{
		ResumeDir: dir,
		Indexed:   len(extracted),
		Cached:    len(sources) - len(extracted),
		Calls:     calls,
		Model:     model,
		Candidate: candidate,
		Resumes:   profiles,
	}
	s.finish(result)
	return result, nil
}

func (s *Service) cached(
	ctx context.Context,
	dir, key string,
	sources []source,
	stored map[string]ResumeProfile,
) (Result, bool) {
	candidate, present, err := s.storedCandidate(ctx)
	if err != nil || !present || candidate.SourceKey != key {
		return Result{}, false
	}
	var derived Candidate
	if err := json.Unmarshal([]byte(candidate.Profile), &derived); err != nil {
		return Result{}, false
	}
	profiles := make([]ResumeProfile, 0, len(sources))
	for _, entry := range sources {
		profiles = append(profiles, stored[entry.resume.Code+":"+entry.hash])
	}
	return Result{
		ResumeDir: dir,
		Cached:    len(sources),
		Model:     candidate.Model,
		Candidate: derived,
		Resumes:   profiles,
	}, true
}

func (s *Service) extract(ctx context.Context, missing []source) ([]pending, error) {
	s.setProgress(Progress{Phase: PhaseExtracting, Total: len(missing)})
	extracted := make([]pending, 0, len(missing))
	for _, entry := range missing {
		text, err := s.extractor.Text(ctx, primaryFile(entry.resume))
		if err != nil {
			return nil, err
		}
		extracted = append(extracted, pending{
			code:      entry.resume.Code,
			label:     entry.resume.Label,
			hash:      entry.hash,
			languages: entry.resume.Languages,
			text:      text,
		})
		s.advance(len(extracted))
	}
	return extracted, nil
}

func primaryFile(resume resumes.Resume) string {
	if path, present := resume.Files[resumes.DefaultLanguage]; present {
		return path
	}
	return resume.Files[resume.Languages[0]]
}

type payloadEntry struct {
	Code          string   `json:"code"`
	Label         string   `json:"label"`
	FileLanguages []string `json:"fileLanguages"`
	Text          string   `json:"text"`
}

func (s *Service) profile(
	ctx context.Context,
	extracted []pending,
	stored map[string]ResumeProfile,
) (int, error) {
	s.setProgress(Progress{Phase: PhaseProfiling, Total: len(extracted)})
	calls := 0
	done := 0

	for _, batch := range s.batches(extracted) {
		payload := make([]payloadEntry, 0, len(batch))
		for _, entry := range batch {
			payload = append(payload, payloadEntry{
				Code:          entry.code,
				Label:         entry.label,
				FileLanguages: entry.languages,
				Text:          entry.text,
			})
		}
		encoded, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return calls, fmt.Errorf("encode resume payload: %w", err)
		}
		result, err := s.claude.CallTool(ctx, claude.Call{
			Purpose: PurposeResumes,
			System:  resumeSystem,
			User: "Profile these " + strconv.Itoa(len(batch)) +
				" resume variants and call resume_profiles with one entry per code.\n\n" +
				string(encoded),
			Tool: resumeTool(),
		})
		if err != nil {
			return calls, refuse(err.Error())
		}
		calls++

		answered := decodeResumeProfiles(result.Input)
		at := clock.Stamp(s.clock)
		for _, entry := range batch {
			profile := shapeResumeProfile(
				answered[entry.code],
				entry.code,
				entry.label,
				s.clock.Now().UTC(),
			)
			document, err := json.Marshal(profile)
			if err != nil {
				return calls, fmt.Errorf("encode resume profile: %w", err)
			}
			if err := s.queries.SaveResumeProfile(ctx, indexerdb.SaveResumeProfileParams{
				Code:      entry.code,
				Hash:      entry.hash,
				Profile:   string(document),
				IndexedAt: at,
			}); err != nil {
				return calls, err
			}
			stored[entry.code+":"+entry.hash] = profile
		}
		done += len(batch)
		s.advance(done)
	}
	return calls, nil
}

func decodeResumeProfiles(input json.RawMessage) map[string]ResumeProfile {
	var answer struct {
		Profiles []ResumeProfile `json:"profiles"`
	}
	answered := map[string]ResumeProfile{}
	if err := json.Unmarshal(input, &answer); err != nil {
		return answered
	}
	for _, profile := range answer.Profiles {
		code := strings.ToUpper(strings.TrimSpace(profile.Code))
		if code == "" {
			continue
		}
		if _, present := answered[code]; !present {
			answered[code] = profile
		}
	}
	return answered
}

func (s *Service) synthesise(
	ctx context.Context,
	profiles []ResumeProfile,
) (Candidate, string, error) {
	encoded, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return Candidate{}, "", fmt.Errorf("encode variant profiles: %w", err)
	}
	result, err := s.claude.CallTool(ctx, claude.Call{
		Purpose: PurposeCandidate,
		System:  candidateSystem,
		User: "Synthesise the candidate behind these " + strconv.Itoa(len(profiles)) +
			" resume variants and call candidate_profile.\n\n" + string(encoded),
		Tool: candidateTool(),
	})
	if err != nil {
		return Candidate{}, "", refuse(err.Error())
	}
	var answered Candidate
	if err := json.Unmarshal(result.Input, &answered); err != nil {
		answered = Candidate{}
	}
	return shapeCandidate(answered, profiles, s.clock.Now().UTC()), result.Model, nil
}

func (s *Service) persist(
	ctx context.Context,
	key, model string,
	candidate Candidate,
	sources []source,
) error {
	document, err := json.Marshal(candidate)
	if err != nil {
		return fmt.Errorf("encode candidate profile: %w", err)
	}
	if err := s.queries.SaveCandidateProfile(ctx, indexerdb.SaveCandidateProfileParams{
		Profile:   string(document),
		SourceKey: key,
		Model:     model,
		IndexedAt: clock.Stamp(s.clock),
	}); err != nil {
		return err
	}
	return s.prune(ctx, sources)
}

func (s *Service) prune(ctx context.Context, sources []source) error {
	if len(sources) == 0 {
		return s.queries.DeleteResumeProfiles(ctx)
	}
	keep := make([]string, 0, len(sources))
	for _, entry := range sources {
		keep = append(keep, entry.resume.Code+":"+entry.hash)
	}
	encoded, err := json.Marshal(keep)
	if err != nil {
		return fmt.Errorf("encode kept resume keys: %w", err)
	}
	return s.queries.PruneResumeProfiles(ctx, string(encoded))
}

func (s *Service) batches(items []pending) [][]pending {
	batched := make([][]pending, 0, 1)
	current := make([]pending, 0, len(items))
	chars := 0
	for _, item := range items {
		if len(current) > 0 && chars+len(item.text) > s.config.MaxCharsPerCall {
			batched = append(batched, current)
			current = make([]pending, 0, len(items))
			chars = 0
		}
		current = append(current, item)
		chars += len(item.text)
	}
	if len(current) > 0 {
		batched = append(batched, current)
	}
	return batched
}

func (s *Service) finish(result Result) {
	s.hub.Emit(events.TypeIndex, map[string]any{
		"phase":   PhaseDone,
		"done":    len(result.Resumes),
		"total":   len(result.Resumes),
		"indexed": result.Indexed,
		"cached":  result.Cached,
		"calls":   result.Calls,
		"candidate": map[string]any{
			"headline":        result.Candidate.Headline,
			"seniorityBand":   result.Candidate.SeniorityBand,
			"yearsExperience": result.Candidate.YearsExperience,
			"coreStack":       head(result.Candidate.CoreStack, maxCandidateSet),
		},
	})
}

func head(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}
