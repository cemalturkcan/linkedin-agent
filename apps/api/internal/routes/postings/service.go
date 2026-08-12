package postings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"api/internal/app/clock"
	"api/internal/app/events"
	"api/internal/app/store"
	postingsdb "api/internal/routes/postings/gen"
	"api/internal/routes/settings"
)

var (
	ErrUnknownStatus = errors.New("unknown status")
	ErrNoPosting     = errors.New("no posting with that id")
)

type Dependencies struct {
	Reads        store.DBTX
	Transactions store.TransactionRunner
	Settings     *settings.Service
	Rounds       Rounds
	ScreenKeys   ScreenKeys
	Hub          *events.Hub
	Clock        clock.Clock
}

type Service struct {
	queries      *postingsdb.Queries
	transactions store.TransactionRunner
	settings     *settings.Service
	rounds       Rounds
	screenKeys   ScreenKeys
	hub          *events.Hub
	clock        clock.Clock
}

func NewService(dependencies Dependencies) *Service {
	return &Service{
		queries:      postingsdb.New(dependencies.Reads),
		transactions: dependencies.Transactions,
		settings:     dependencies.Settings,
		rounds:       dependencies.Rounds,
		screenKeys:   dependencies.ScreenKeys,
		hub:          dependencies.Hub,
		clock:        dependencies.Clock,
	}
}

func (s *Service) UseRounds(rounds Rounds) {
	s.rounds = rounds
}

func (s *Service) UseScreenKeys(keys ScreenKeys) {
	s.screenKeys = keys
}

type jobsEvent struct {
	Inserted   int    `json:"inserted"`
	Duplicates int    `json:"duplicates"`
	Judged     int    `json:"judged"`
	CycleID    *int64 `json:"cycleId"`
}

func (s *Service) Ingest(
	ctx context.Context,
	harvested []Harvested,
	cycleID *int64,
	queryLabel string,
	pages int,
) (Ingest, error) {
	current, err := s.settings.Load(ctx)
	if err != nil {
		return Ingest{}, err
	}

	handled, err := s.handled(ctx, ids(harvested))
	if err != nil {
		return Ingest{}, err
	}
	fresh := make([]Harvested, 0, len(harvested))
	for _, posting := range harvested {
		if _, carried := handled[posting.ID]; carried {
			continue
		}
		fresh = append(fresh, posting)
	}

	result := Ingest{
		Received: len(harvested),
		Judged:   len(handled),
		NewIDs:   []string{},
	}
	if err := s.write(ctx, fresh, cycleID, queryLabel, current.Companies.Dedupe, &result); err != nil {
		return Ingest{}, err
	}

	result.NewJobTarget = current.Harvest.NewJobTarget
	if cycleID != nil && s.rounds != nil {
		state, err := s.rounds.RecordYield(ctx, Yield{
			CycleID:    *cycleID,
			QueryLabel: queryLabel,
			Pages:      pages,
			Received:   result.Received,
			Inserted:   result.Inserted,
			Duplicates: result.Duplicates + result.Collapsed + result.Judged,
		})
		if err != nil {
			return Ingest{}, err
		}
		result.RoundNew = state.RoundNew
		result.NewJobTarget = state.NewJobTarget
		result.Stop = state.Stop
		result.Reason = state.Reason
	}

	s.hub.Emit(events.TypeJobs, jobsEvent{
		Inserted:   result.Inserted,
		Duplicates: result.Duplicates + result.Collapsed,
		Judged:     result.Judged,
		CycleID:    cycleID,
	})
	return result, nil
}

func ids(harvested []Harvested) []string {
	found := make([]string, 0, len(harvested))
	for _, posting := range harvested {
		found = append(found, posting.ID)
	}
	return found
}

func (s *Service) handled(ctx context.Context, wanted []string) (map[string]struct{}, error) {
	carried := make(map[string]struct{}, len(wanted))
	if len(wanted) == 0 {
		return carried, nil
	}
	asked, err := json.Marshal(wanted)
	if err != nil {
		return nil, fmt.Errorf("read handled postings: %w", err)
	}
	statuses, err := json.Marshal(HandledStatuses)
	if err != nil {
		return nil, fmt.Errorf("read handled postings: %w", err)
	}
	found, err := s.queries.HandledIDs(ctx, postingsdb.HandledIDsParams{
		JsonEach:   string(asked),
		JsonEach_2: string(statuses),
	})
	if err != nil {
		return nil, fmt.Errorf("read handled postings: %w", err)
	}
	for _, id := range found {
		carried[id] = struct{}{}
	}
	return carried, nil
}

func (s *Service) write(
	ctx context.Context,
	fresh []Harvested,
	cycleID *int64,
	queryLabel string,
	dedupe bool,
	result *Ingest,
) error {
	if len(fresh) == 0 {
		return nil
	}
	at := clock.Stamp(s.clock)
	var label *string
	if queryLabel != "" {
		label = &queryLabel
	}

	return s.transactions.WithTx(ctx, func(tx *sql.Tx) error {
		queries := s.queries.WithTx(tx)
		for _, posting := range fresh {
			canonical, collapsed, err := canonicalFor(ctx, queries, posting, dedupe)
			if err != nil {
				return err
			}
			mark := found{status: StatusNew, stage: StageNone, cycleID: cycleID, queryLabel: label}
			var dupeOf *string
			if collapsed {
				dupeOf = &canonical
				mark.status = StatusDuplicate
			}
			changed, err := queries.InsertJob(ctx, insertParams(posting, dupeOf, mark, at))
			if err != nil {
				return fmt.Errorf("store posting: %w", err)
			}
			switch {
			case changed == 0:
				result.Duplicates++
			case collapsed:
				result.Collapsed++
			default:
				result.Inserted++
				result.NewIDs = append(result.NewIDs, posting.ID)
			}
		}
		return nil
	})
}

type Pick struct {
	Code string `json:"resumeCode"`
	Lang string `json:"resumeLang"`
}

func (s *Service) Open(ctx context.Context, cards []Harvested, chosen Pick) (Opened, error) {
	current, err := s.settings.Load(ctx)
	if err != nil {
		return Opened{}, err
	}
	already, err := s.handled(ctx, ids(cards))
	if err != nil {
		return Opened{}, err
	}

	result := Opened{Received: len(cards), IDs: []string{}}
	at := clock.Stamp(s.clock)
	code := optional(line(chosen.Code, 8))
	lang := optional(line(chosen.Lang, 8))

	err = s.transactions.WithTx(ctx, func(tx *sql.Tx) error {
		queries := s.queries.WithTx(tx)
		for _, card := range cards {
			if _, carried := already[card.ID]; carried {
				result.Held++
				continue
			}
			canonical, collapsed, err := canonicalFor(ctx, queries, card, current.Companies.Dedupe)
			if err != nil {
				return err
			}
			mark := found{status: StatusOpened, stage: StageHand, resumeCode: code, resumeLang: lang}
			var dupeOf *string
			if collapsed {
				dupeOf = &canonical
				mark = found{status: StatusDuplicate, stage: StageNone}
			}
			if _, err := queries.InsertJob(ctx, insertParams(card, dupeOf, mark, at)); err != nil {
				return fmt.Errorf("store the posting you opened: %w", err)
			}
			if collapsed {
				result.Collapsed++
				continue
			}
			if _, err := queries.MarkOpened(ctx, postingsdb.MarkOpenedParams{
				ResumeCode: code,
				ResumeLang: lang,
				UpdatedAt:  at,
				ID:         card.ID,
			}); err != nil {
				return fmt.Errorf("record the posting you opened: %w", err)
			}
			result.Recorded++
			result.IDs = append(result.IDs, card.ID)
		}
		return nil
	})
	if err != nil {
		return Opened{}, err
	}

	if result.Recorded > 0 || result.Collapsed > 0 {
		s.hub.Emit(events.TypeJobs, map[string]any{
			"updated": result.Recorded + result.Collapsed,
			"reason":  "postings you opened yourself",
		})
	}
	return result, nil
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func canonicalFor(
	ctx context.Context,
	queries *postingsdb.Queries,
	posting Harvested,
	dedupe bool,
) (string, bool, error) {
	key := DupeKey(posting.Company, posting.Title)
	if !dedupe || key == "" {
		return "", false, nil
	}
	canonical, err := queries.CanonicalFor(ctx, postingsdb.CanonicalForParams{
		DupeKey: key,
		Status:  StatusDuplicate,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read the canonical posting: %w", err)
	}
	if canonical == posting.ID {
		return "", false, nil
	}
	return canonical, true, nil
}

type found struct {
	status     string
	stage      string
	resumeCode *string
	resumeLang *string
	cycleID    *int64
	queryLabel *string
}

func insertParams(
	posting Harvested,
	canonical *string,
	mark found,
	at string,
) postingsdb.InsertJobParams {
	state := describedPending
	var description, describedAt *string
	if posting.Description != "" {
		state = describedOK
		description = &posting.Description
		describedAt = &at
	}
	return postingsdb.InsertJobParams{
		ID:               posting.ID,
		Title:            line(posting.Title, maxTitle),
		Company:          line(posting.Company, maxTitle),
		Location:         line(posting.Location, maxTitle),
		URL:              posting.URL,
		CompanyKey:       CompanyKey(posting.Company),
		DupeKey:          DupeKey(posting.Company, posting.Title),
		DupeOf:           canonical,
		Description:      description,
		DescriptionState: state,
		DescribedAt:      describedAt,
		ListedAt:         posting.ListedAt,
		EasyApply:        posting.EasyApply,
		Status:           mark.status,
		Stage:            mark.stage,
		ResumeCode:       mark.resumeCode,
		ResumeLang:       mark.resumeLang,
		CycleID:          mark.cycleID,
		QueryLabel:       mark.queryLabel,
		CreatedAt:        at,
		UpdatedAt:        at,
	}
}

func (s *Service) screenKey(ctx context.Context) string {
	if s.screenKeys == nil {
		return ""
	}
	return s.screenKeys.ScreenKey(ctx)
}

func (s *Service) List(ctx context.Context, status string) ([]Posting, error) {
	if status == "" {
		status = ListAll
	}
	if !slices.Contains(ListStatuses, status) {
		return nil, ErrUnknownStatus
	}
	key := s.screenKey(ctx)

	if status == ListAll {
		rows, err := s.queries.Jobs(ctx, &key)
		if err != nil {
			return nil, fmt.Errorf("list postings: %w", err)
		}
		postings := make([]Posting, 0, len(rows))
		for _, row := range rows {
			postings = append(postings, posting(postingsdb.JobsByStatusRow(row)))
		}
		return postings, nil
	}
	rows, err := s.queries.JobsByStatus(ctx, postingsdb.JobsByStatusParams{
		ScreenKey: &key,
		Status:    status,
	})
	if err != nil {
		return nil, fmt.Errorf("list postings: %w", err)
	}
	postings := make([]Posting, 0, len(rows))
	for _, row := range rows {
		postings = append(postings, posting(row))
	}
	return postings, nil
}

func (s *Service) Get(ctx context.Context, id string) (Posting, error) {
	key := s.screenKey(ctx)
	row, err := s.queries.Job(ctx, postingsdb.JobParams{ScreenKey: &key, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return Posting{}, ErrNoPosting
	}
	if err != nil {
		return Posting{}, fmt.Errorf("read the posting: %w", err)
	}
	return posting(postingsdb.JobsByStatusRow(row)), nil
}

func posting(row postingsdb.JobsByStatusRow) Posting {
	described := ""
	if row.Description != nil {
		described = *row.Description
	}
	found := Posting{
		ID:               row.ID,
		Title:            row.Title,
		Company:          row.Company,
		Location:         row.Location,
		URL:              row.URL,
		DupeOf:           row.DupeOf,
		Description:      described,
		DescriptionState: row.DescriptionState,
		ListedAt:         row.ListedAt,
		EasyApply:        row.EasyApply,
		Status:           row.Status,
		Stage:            row.Stage,
		TriageReason:     row.TriageReason,
		VerdictReason:    row.VerdictReason,
		Seniority:        row.Seniority,
		Workplace:        row.Workplace,
		ContractType:     row.ContractType,
		PostingLang:      row.PostingLang,
		Agency:           row.Agency,
		StatedPay:        pay(row.PayAmount, row.PayCurrency, row.PayPeriod),
		ResumeCode:       row.ResumeCode,
		ResumeLang:       row.ResumeLang,
		ResumeFit:        row.ResumeFit,
		TailoredResume:   tailored(row.TailoredNeeded, row.TailoredFocus),
		Stale:            row.Stale,
		Outcome:          row.Outcome,
		OutcomeNote:      row.OutcomeNote,
		CycleID:          row.CycleID,
		QueryLabel:       row.QueryLabel,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
	if row.Score != nil {
		score := int(*row.Score)
		found.Score = &score
	}
	return found
}

func pay(amount *int64, currency, period *string) *Pay {
	if amount == nil || *amount <= 0 || currency == nil || period == nil {
		return nil
	}
	return &Pay{Amount: int(*amount), Currency: *currency, Period: *period}
}

func tailored(needed *bool, focus *string) *Tailored {
	if needed == nil {
		return nil
	}
	written := ""
	if focus != nil {
		written = *focus
	}
	return &Tailored{Needed: *needed, Focus: written}
}

func (s *Service) Counts(ctx context.Context) (map[string]int, error) {
	rows, err := s.queries.JobCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("count postings: %w", err)
	}
	counts := make(map[string]int, len(Statuses))
	for _, status := range Statuses {
		counts[status] = 0
	}
	for _, row := range rows {
		counts[row.Status] = int(row.Total)
	}
	return counts, nil
}

func (s *Service) RecentJudgedIDs(
	ctx context.Context,
	window time.Duration,
	limit int,
) ([]string, error) {
	since := s.clock.Now().Add(-window).UTC().Format(time.RFC3339Nano)
	ids, err := s.queries.RecentJudgedIDs(ctx, postingsdb.RecentJudgedIDsParams{
		Status:    StatusNew,
		CreatedAt: since,
		Limit:     int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("read recently judged postings: %w", err)
	}
	if ids == nil {
		return []string{}, nil
	}
	return ids, nil
}

func (s *Service) Places(ctx context.Context, term string) ([]Place, error) {
	rows, err := s.queries.Places(ctx)
	if err != nil {
		return nil, fmt.Errorf("list places: %w", err)
	}
	wanted := PlaceKey(term)
	near := make([]Place, 0, maxPlaceHits)
	far := make([]Place, 0, maxPlaceHits)
	for _, row := range rows {
		asked := PlaceKey(row.Asked)
		place := Place{Name: row.Name, Ring: row.Ring}
		switch {
		case wanted == "" ||
			strings.HasPrefix(row.PlaceKey, wanted) ||
			strings.HasPrefix(asked, wanted):
			near = append(near, place)
		case strings.Contains(row.PlaceKey, wanted) || strings.Contains(asked, wanted):
			far = append(far, place)
		}
	}
	ranked := make([]Place, 0, len(near)+len(far))
	ranked = append(ranked, near...)
	ranked = append(ranked, far...)
	if len(ranked) > maxPlaceHits {
		ranked = ranked[:maxPlaceHits]
	}
	return ranked, nil
}

func (s *Service) SavePlaces(
	ctx context.Context,
	resolutions []Resolution,
) ([]Place, int, error) {
	at := clock.Stamp(s.clock)
	stored := make([]Place, 0, len(resolutions))

	err := s.transactions.WithTx(ctx, func(tx *sql.Tx) error {
		queries := s.queries.WithTx(tx)
		for _, resolution := range resolutions {
			name := line(resolution.Name, maxPlaceName)
			key := PlaceKey(name)
			if key == "" || strings.TrimSpace(resolution.LocationID) == "" {
				continue
			}
			ring := strings.ToLower(strings.TrimSpace(resolution.Ring))
			if !slices.Contains(settings.PlaceRings, ring) {
				ring = ""
			}
			if err := queries.SavePlace(ctx, postingsdb.SavePlaceParams{
				PlaceKey:    key,
				Name:        name,
				Asked:       line(resolution.Asked, maxPlaceName),
				Ring:        ring,
				LocationID:  strings.TrimSpace(resolution.LocationID),
				Source:      line(resolution.Source, 20),
				FirstSeenAt: at,
				LastSeenAt:  at,
			}); err != nil {
				return fmt.Errorf("save place: %w", err)
			}
			saved, err := queries.Place(ctx, key)
			if err != nil {
				return fmt.Errorf("read place: %w", err)
			}
			stored = append(stored, Place{Name: saved.Name, Ring: saved.Ring})
		}
		return queries.PrunePlaces(ctx, maxPlaces)
	})
	if err != nil {
		return nil, 0, err
	}

	known, err := s.queries.CountPlaces(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count places: %w", err)
	}
	return stored, int(known), nil
}

func line(value string, limit int) string {
	collapsed := strings.TrimSpace(manySpaces.ReplaceAllString(value, " "))
	runes := []rune(collapsed)
	if len(runes) <= limit {
		return collapsed
	}
	return string(runes[:limit])
}
