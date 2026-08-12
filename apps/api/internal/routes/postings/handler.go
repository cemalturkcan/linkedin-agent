package postings

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"api/internal/app/httpx"
)

type HandlerConfig struct {
	OperationTimeout time.Duration
}

type Handler struct {
	service *Service
	config  HandlerConfig
}

func NewHandler(service *Service, config HandlerConfig) *Handler {
	return &Handler{service: service, config: config}
}

type harvestRequest struct {
	CycleID    *int64      `json:"cycleId"`
	QueryLabel string      `json:"queryLabel"`
	Pages      int         `json:"pages"`
	Jobs       []Harvested `json:"jobs"`
}

type openedRequest struct {
	Jobs       []Harvested `json:"jobs"`
	ResumeCode string      `json:"resumeCode"`
	ResumeLang string      `json:"resumeLang"`
}

type listResponse struct {
	Jobs []Posting `json:"jobs"`
}

type placesResponse struct {
	Places []Place `json:"places"`
	Reason string  `json:"reason,omitempty"`
}

type placesRequest struct {
	Places []Resolution `json:"places"`
}

type storedPlaces struct {
	Stored int     `json:"stored"`
	Places []Place `json:"places"`
	Known  int     `json:"known"`
}

func (h *Handler) Ingest(c fiber.Ctx) error {
	var body harvestRequest
	if appErr := httpx.DecodeObject(c, &body); appErr != nil {
		return appErr.WithMessage("a harvest carries a jobs array")
	}
	if body.Jobs == nil {
		return httpx.ErrBadRequest.WithMessage("a harvest carries a jobs array")
	}
	for position, posting := range body.Jobs {
		if strings.TrimSpace(posting.ID) == "" {
			return httpx.ErrBadRequest.WithMessage(
				"jobs[" + itoa(position) + "] has no id, and a posting without one cannot be stored",
			)
		}
	}
	if body.CycleID != nil && strings.TrimSpace(body.QueryLabel) == "" {
		return httpx.ErrBadRequest.WithMessage(
			"results tagged with a round have to name the query label that found them",
		)
	}

	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	result, err := h.service.Ingest(
		ctx,
		body.Jobs,
		body.CycleID,
		strings.TrimSpace(body.QueryLabel),
		body.Pages,
	)
	if err != nil {
		return refusal(err)
	}
	return httpx.Respond(c, result)
}

func (h *Handler) Opened(c fiber.Ctx) error {
	var body openedRequest
	if appErr := httpx.DecodeObject(c, &body); appErr != nil {
		return appErr.WithMessage("a posting you opened carries a jobs array")
	}
	if body.Jobs == nil {
		return httpx.ErrBadRequest.WithMessage("a posting you opened carries a jobs array")
	}
	for position, posting := range body.Jobs {
		if strings.TrimSpace(posting.ID) == "" {
			return httpx.ErrBadRequest.WithMessage(
				"jobs[" + itoa(position) + "] has no id, and a posting without one cannot be held",
			)
		}
	}

	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	result, err := h.service.Open(ctx, body.Jobs, Pick{
		Code: strings.ToUpper(strings.TrimSpace(body.ResumeCode)),
		Lang: strings.ToLower(strings.TrimSpace(body.ResumeLang)),
	})
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	return httpx.Respond(c, result)
}

func (h *Handler) List(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	found, err := h.service.List(ctx, c.Query("status"))
	if errors.Is(err, ErrUnknownStatus) {
		return httpx.ErrBadRequest.WithMessage(
			"status has to be one of " + strings.Join(ListStatuses, ", "),
		)
	}
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	return httpx.Respond(c, listResponse{Jobs: found})
}

func (h *Handler) Places(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	term := strings.TrimSpace(c.Query("q"))
	found, err := h.service.Places(ctx, term)
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	if len(found) > 0 {
		return httpx.Respond(c, placesResponse{Places: found})
	}
	return httpx.Respond(c, placesResponse{Places: []Place{}, Reason: noPlaces(term)})
}

func (h *Handler) SavePlaces(c fiber.Ctx) error {
	var body placesRequest
	if appErr := httpx.DecodeObject(c, &body); appErr != nil {
		return appErr.WithMessage("places must be an array of resolutions")
	}
	if body.Places == nil {
		return httpx.ErrBadRequest.WithMessage("places must be an array of resolutions")
	}
	for position, resolution := range body.Places {
		if strings.TrimSpace(resolution.Name) == "" {
			return httpx.ErrBadRequest.WithMessage(
				"places[" + itoa(position) + "] has no name",
			)
		}
		if strings.TrimSpace(resolution.LocationID) == "" {
			return httpx.ErrBadRequest.WithMessage(
				"places[" + itoa(position) +
					"] carries no linkedin id, so nothing resolved it and it cannot be offered",
			)
		}
	}

	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	stored, known, err := h.service.SavePlaces(ctx, body.Places)
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	return httpx.Respond(c, storedPlaces{Stored: len(stored), Places: stored, Known: known})
}

func noPlaces(term string) string {
	if term == "" {
		return "no place has been resolved yet, so there is nothing to offer. " +
			"the next round resolves the names the planner writes."
	}
	return "nothing resolved so far matches " + term +
		". the name still works: the next round asks linkedin about it."
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

func refusal(err error) error {
	return httpx.ErrInternal.WithCause(err)
}
