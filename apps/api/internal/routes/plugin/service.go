package plugin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"api/internal/app/clock"
	"api/internal/app/events"
	"api/internal/app/store"
	plugindb "api/internal/routes/plugin/gen"
)

var (
	extensionIDShape = regexp.MustCompile(`^[a-p]{32}$`)
	versionShape     = regexp.MustCompile(`^[\w.+-]{1,32}$`)
)

type Config struct {
	ConnectedWindow time.Duration
}

type Dependencies struct {
	Reads  store.DBTX
	Hub    *events.Hub
	Clock  clock.Clock
	Config Config
}

type presence struct {
	Connected bool    `json:"connected"`
	LastSeen  *string `json:"lastSeen"`
	Version   *string `json:"version"`
}

type Service struct {
	queries *plugindb.Queries
	hub     *events.Hub
	clock   clock.Clock
	window  time.Duration

	mu    sync.Mutex
	quiet *time.Timer
}

func NewService(dependencies Dependencies) *Service {
	window := dependencies.Config.ConnectedWindow
	if window <= 0 {
		window = ConnectedWindow
	}
	return &Service{
		queries: plugindb.New(dependencies.Reads),
		hub:     dependencies.Hub,
		clock:   dependencies.Clock,
		window:  window,
	}
}

func (s *Service) Hello(ctx context.Context, hello Hello) (State, error) {
	stored, present, err := s.stored(ctx)
	if err != nil {
		return State{}, err
	}

	now := clock.Stamp(s.clock)
	capabilities, err := json.Marshal(Normalize(hello.Capabilities))
	if err != nil {
		return State{}, fmt.Errorf("encode the capability manifest: %w", err)
	}

	next := plugindb.SavePluginParams{
		ExtensionID:  identifier(hello.ID, stored.ExtensionID),
		Version:      version(hello.Version),
		Capabilities: string(capabilities),
		FirstSeen:    stored.FirstSeen,
		LastSeen:     now,
		Hellos:       stored.Hellos + 1,
	}
	if !present || next.FirstSeen == "" {
		next.FirstSeen = now
	}
	if err := s.queries.SavePlugin(ctx, next); err != nil {
		return State{}, fmt.Errorf("save the extension record: %w", err)
	}

	returning := !present || s.quietFor(stored.LastSeen)
	state := s.state(next.ExtensionID, next.Version, next.Capabilities,
		next.FirstSeen, next.LastSeen, int(next.Hellos))
	if returning {
		s.hub.Emit(events.TypePlugin, presence{
			Connected: true,
			LastSeen:  state.LastSeen,
			Version:   state.Version,
		})
	}
	s.watchForSilence(state.LastSeen)
	return state, nil
}

func (s *Service) State(ctx context.Context) (State, error) {
	stored, present, err := s.stored(ctx)
	if err != nil {
		return State{}, err
	}
	if !present {
		return State{}, nil
	}
	return s.state(stored.ExtensionID, stored.Version, stored.Capabilities,
		stored.FirstSeen, stored.LastSeen, int(stored.Hellos)), nil
}

func (s *Service) stored(ctx context.Context) (plugindb.PluginRow, bool, error) {
	stored, err := s.queries.Plugin(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return plugindb.PluginRow{}, false, nil
	}
	if err != nil {
		return plugindb.PluginRow{}, false, fmt.Errorf("read the extension record: %w", err)
	}
	return stored, true, nil
}

func (s *Service) state(
	id, reported, capabilities, firstSeen, lastSeen string,
	hellos int,
) State {
	seenAt, err := time.Parse(time.RFC3339Nano, lastSeen)
	if err != nil {
		return State{}
	}
	manifest := Normalize(decodeCapabilities(capabilities))
	state := State{
		EverSeen:     true,
		Connected:    s.clock.Now().Sub(seenAt) < s.window,
		ID:           id,
		Version:      &reported,
		FirstSeen:    &firstSeen,
		LastSeen:     &lastSeen,
		Hellos:       hellos,
		Capabilities: &manifest,
	}
	return state
}

func (s *Service) quietFor(lastSeen string) bool {
	seenAt, err := time.Parse(time.RFC3339Nano, lastSeen)
	if err != nil {
		return true
	}
	return s.clock.Now().Sub(seenAt) >= s.window
}

func (s *Service) watchForSilence(lastSeen *string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.quiet != nil {
		s.quiet.Stop()
	}
	s.quiet = time.AfterFunc(s.window, func() {
		s.hub.Emit(events.TypePlugin, presence{Connected: false, LastSeen: lastSeen})
	})
}

func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.quiet != nil {
		s.quiet.Stop()
		s.quiet = nil
	}
}

func decodeCapabilities(document string) RawCapabilities {
	var raw RawCapabilities
	if json.Unmarshal([]byte(document), &raw) != nil {
		return RawCapabilities{}
	}
	return raw
}

func Normalize(raw RawCapabilities) Capabilities {
	return Capabilities{
		Fetch: Fetch{
			Place:            boolean(raw.Fetch.Place, true),
			Rings:            subset(raw.Fetch.Rings, KnownRings),
			Ranges:           subset(raw.Fetch.Ranges, KnownRanges),
			EasyApply:        boolean(raw.Fetch.EasyApply, false),
			RemoteOnly:       boolean(raw.Fetch.RemoteOnly, false),
			Sorts:            subset(raw.Fetch.Sorts, KnownSorts),
			Keyword:          boolean(raw.Fetch.Keyword, false),
			PageSize:         bounded(raw.Fetch.PageSize, 1, maxPageSize, defaultPageSize),
			MaxPagesPerQuery: bounded(raw.Fetch.MaxPagesPerQuery, 1, maxPagesPerQuery, defaultMaxPages),
		},
		SeenSet:    boolean(raw.SeenSet, false),
		AutoAttach: boolean(raw.AutoAttach, false),
	}
}

func subset(reported, allowed []string) []string {
	if reported == nil {
		return append([]string{}, allowed...)
	}
	kept := make([]string, 0, len(allowed))
	for _, candidate := range allowed {
		for _, value := range reported {
			if strings.TrimSpace(value) == candidate {
				kept = append(kept, candidate)
				break
			}
		}
	}
	return kept
}

func boolean(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func bounded(value *int, low, high, fallback int) int {
	if value == nil {
		return fallback
	}
	return min(high, max(low, *value))
}

func identifier(reported, stored string) string {
	trimmed := strings.TrimSpace(reported)
	if extensionIDShape.MatchString(trimmed) {
		return trimmed
	}
	return stored
}

func version(reported string) string {
	trimmed := strings.TrimSpace(reported)
	if versionShape.MatchString(trimmed) {
		return trimmed
	}
	return unknownVersion
}
