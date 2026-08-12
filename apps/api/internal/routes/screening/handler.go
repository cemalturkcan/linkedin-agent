package screening

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"api/internal/app/httpx"
	"api/internal/routes/postings"
)

type HandlerConfig struct {
	OperationTimeout time.Duration
	ScreenBudget     time.Duration
	PendingLimit     int
}

type Handler struct {
	service *Service
	config  HandlerConfig
}

func NewHandler(service *Service, config HandlerConfig) *Handler {
	return &Handler{service: service, config: config}
}

type screenRequest struct {
	Limit int `json:"limit"`
}

type descriptionsRequest struct {
	Descriptions []Description `json:"descriptions"`
}

type outcomeRequest struct {
	Outcome string `json:"outcome"`
	Note    string `json:"note"`
}

type reflectRequest struct {
	Force bool `json:"force"`
}

type pendingResponse struct {
	Jobs []Pending `json:"jobs"`
}

type postingResponse struct {
	Job        postings.Posting `json:"job"`
	Reflection *Due             `json:"reflection,omitempty"`
}

func (h *Handler) Screen(c fiber.Ctx) error {
	var body screenRequest
	if len(c.Body()) > 0 {
		if appErr := httpx.DecodeObject(c, &body); appErr != nil {
			return appErr
		}
	}
	if body.Limit < 0 {
		return httpx.ErrBadRequest.WithMessage("limit has to be a positive number")
	}
	ctx, cancel := httpx.BoundedRequestContext(
		c,
		context.WithoutCancel(c.Context()),
		h.config.ScreenBudget,
	)
	defer cancel()

	result, err := h.service.Screen(ctx, body.Limit)
	if err != nil {
		return refusal(err)
	}
	if result.Error != "" && result.Screened == 0 && result.Triaged == 0 {
		return httpx.ErrUnavailable.WithMessage(result.Error)
	}
	return httpx.Respond(c, result)
}

func (h *Handler) Pending(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	limit := httpx.BoundedQueryInt(c, "limit", h.config.PendingLimit, 200)
	found, err := h.service.Pending(ctx, limit)
	if err != nil {
		return refusal(err)
	}
	return httpx.Respond(c, pendingResponse{Jobs: found})
}

func (h *Handler) Descriptions(c fiber.Ctx) error {
	var body descriptionsRequest
	if appErr := httpx.DecodeObject(c, &body); appErr != nil {
		return appErr.WithMessage("descriptions must be an array of bodies")
	}
	if body.Descriptions == nil {
		return httpx.ErrBadRequest.WithMessage("descriptions must be an array of bodies")
	}
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	stored, err := h.service.RecordDescriptions(ctx, body.Descriptions)
	if err != nil {
		return refusal(err)
	}
	return httpx.Respond(c, stored)
}

func (h *Handler) Stale(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	report, err := h.service.Staleness(ctx)
	if err != nil {
		return refusal(err)
	}
	return httpx.Respond(c, report)
}

func (h *Handler) Rescreen(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	reset, err := h.service.Rescreen(ctx)
	if err != nil {
		return refusal(err)
	}
	return httpx.Respond(c, reset)
}

func (h *Handler) Move(c fiber.Ctx) error {
	id := strings.TrimSpace(c.Params("id"))
	status, known := Transitions[strings.TrimSpace(c.Params("transition"))]
	if id == "" || !known {
		return httpx.ErrNotFound
	}
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	moved, err := h.service.Move(ctx, id, status)
	if err != nil {
		return refusal(err)
	}
	return httpx.Respond(c, postingResponse{Job: moved})
}

func (h *Handler) Outcome(c fiber.Ctx) error {
	var body outcomeRequest
	if len(c.Body()) > 0 {
		if appErr := httpx.DecodeObject(c, &body); appErr != nil {
			return appErr
		}
	}
	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		return httpx.ErrBadRequest.WithMessage("an outcome names the posting it belongs to")
	}
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	due, err := h.service.RecordOutcome(ctx, id, body.Outcome, body.Note)
	if err != nil {
		return refusal(err)
	}
	posting, err := h.service.Posting(ctx, id)
	if err != nil {
		return refusal(err)
	}
	return httpx.Respond(c, postingResponse{Job: posting, Reflection: &due})
}

func (h *Handler) Reflect(c fiber.Ctx) error {
	var body reflectRequest
	if len(c.Body()) > 0 {
		if appErr := httpx.DecodeObject(c, &body); appErr != nil {
			return appErr
		}
	}
	ctx, cancel := httpx.BoundedRequestContext(
		c,
		context.WithoutCancel(c.Context()),
		h.config.ScreenBudget,
	)
	defer cancel()

	reflection, err := h.service.Reflect(ctx, body.Force)
	if err != nil {
		return refusal(err)
	}
	return httpx.Respond(c, reflection)
}

func refusal(err error) error {
	var refused *RefusalError
	switch {
	case errors.Is(err, postings.ErrNoPosting):
		return httpx.ErrNotFound.WithMessage("no posting with that id").WithCause(err)
	case errors.As(err, &refused) && refused.Unavailable:
		return httpx.ErrUnavailable.WithMessage(refused.Error()).WithCause(err)
	case errors.As(err, &refused):
		return httpx.ErrConflict.WithMessage(refused.Error()).WithCause(err)
	}
	return httpx.ErrInternal.WithCause(err)
}
