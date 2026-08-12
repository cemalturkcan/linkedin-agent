package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"api/internal/app/clock"
	"api/internal/app/events"
	"api/internal/app/store"
	settingsdb "api/internal/routes/settings/gen"
)

type LanguageSource interface {
	Languages(ctx context.Context) []string
}

type NameSource interface {
	UploadName(ctx context.Context) string
}

type changedEvent struct {
	Changed []string `json:"changed"`
}

type Dependencies struct {
	Reads     store.DBTX
	Hub       *events.Hub
	Languages LanguageSource
	Names     NameSource
	Clock     clock.Clock
}

type Service struct {
	queries   *settingsdb.Queries
	hub       *events.Hub
	languages LanguageSource
	names     NameSource
	clock     clock.Clock
}

func NewService(dependencies Dependencies) *Service {
	return &Service{
		queries:   settingsdb.New(dependencies.Reads),
		hub:       dependencies.Hub,
		languages: dependencies.Languages,
		names:     dependencies.Names,
		clock:     dependencies.Clock,
	}
}

func (s *Service) Load(ctx context.Context) (Settings, error) {
	stored, err := s.document(ctx)
	if err != nil {
		return Settings{}, err
	}
	return s.derived(ctx, Normalize(stored)), nil
}

func (s *Service) Save(ctx context.Context, patch map[string]any) (Settings, error) {
	stored, err := s.document(ctx)
	if err != nil {
		return Settings{}, err
	}
	before := Normalize(stored)
	after := Normalize(Merge(stored, patch))

	document, err := json.Marshal(after)
	if err != nil {
		return Settings{}, fmt.Errorf("encode settings: %w", err)
	}
	if err := s.queries.SaveSettings(ctx, settingsdb.SaveSettingsParams{
		Document:  string(document),
		UpdatedAt: clock.Stamp(s.clock),
	}); err != nil {
		return Settings{}, err
	}
	if changed := changedGroups(before, after); len(changed) > 0 {
		s.hub.Emit(events.TypeSettings, changedEvent{Changed: changed})
	}
	return s.derived(ctx, after), nil
}

func (s *Service) document(ctx context.Context) (map[string]any, error) {
	raw, err := s.queries.Settings(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeDocument(raw), nil
}

func decodeDocument(raw string) map[string]any {
	var stored map[string]any
	if raw == "" || json.Unmarshal([]byte(raw), &stored) != nil || stored == nil {
		return map[string]any{}
	}
	return stored
}

func (s *Service) derived(ctx context.Context, current Settings) Settings {
	return s.withPostingLanguages(ctx, s.withUploadName(ctx, s.withResumeLanguages(ctx, current)))
}

func (s *Service) withPostingLanguages(ctx context.Context, current Settings) Settings {
	if len(current.Roles.PostingLanguages) > 0 || s.languages == nil {
		return current
	}
	current.Roles.PostingLanguages = s.languages.Languages(ctx)
	return current
}

func (s *Service) withUploadName(ctx context.Context, current Settings) Settings {
	if current.Apply.UploadFileName != "" {
		return current
	}
	if s.names != nil {
		current.Apply.UploadFileName = s.names.UploadName(ctx)
	}
	if current.Apply.UploadFileName == "" {
		current.Apply.UploadFileName = FallbackUploadName
	}
	return current
}

func (s *Service) withResumeLanguages(ctx context.Context, current Settings) Settings {
	if len(current.Apply.ResumeLanguages) > 0 || s.languages == nil {
		return current
	}
	current.Apply.ResumeLanguages = s.languages.Languages(ctx)
	return current
}

func changedGroups(before, after Settings) []string {
	groups := []struct {
		name   string
		before any
		after  any
	}{
		{"locations", before.Locations, after.Locations},
		{"roles", before.Roles, after.Roles},
		{"companies", before.Companies, after.Companies},
		{"apply", before.Apply, after.Apply},
		{"budget", before.Budget, after.Budget},
		{"harvest", before.Harvest, after.Harvest},
		{"operatorNotes", before.OperatorNotes, after.OperatorNotes},
	}
	changed := make([]string, 0, len(groups))
	for _, entry := range groups {
		if !reflect.DeepEqual(entry.before, entry.after) {
			changed = append(changed, entry.name)
		}
	}
	return changed
}
