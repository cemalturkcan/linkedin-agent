package planner

import (
	"context"
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"api/internal/app/httpx"
)

type Handler struct {
	service *Service
	config  Config
}

func NewHandler(service *Service, config Config) *Handler {
	return &Handler{service: service, config: config}
}

type widenRequest struct {
	CycleID int64 `json:"cycleId"`
}

type skipRequest struct {
	CycleID    int64  `json:"cycleId"`
	QueryLabel string `json:"queryLabel"`
	Place      string `json:"place"`
	Ring       string `json:"ring"`
	Reason     string `json:"reason"`
}

type finishRequest struct {
	CycleID int64  `json:"cycleId"`
	Reason  string `json:"reason"`
	Error   string `json:"error"`
}

type roundResponse struct {
	Cycle Round `json:"cycle"`
}

type notesResponse struct {
	Notes []Note `json:"notes"`
}

func (h *Handler) Start(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(
		c,
		context.WithoutCancel(c.Context()),
		h.config.PlanBudget,
	)
	defer cancel()

	plan, err := h.service.Start(ctx)
	if err != nil {
		return respondRefusal(c, err)
	}
	return httpx.Respond(c, plan)
}

func (h *Handler) Widen(c fiber.Ctx) error {
	var body widenRequest
	if appErr := httpx.DecodeObject(c, &body); appErr != nil {
		return appErr.WithMessage("a widening step names the round it belongs to")
	}
	ctx, cancel := httpx.BoundedRequestContext(
		c,
		context.WithoutCancel(c.Context()),
		h.config.PlanBudget,
	)
	defer cancel()

	widening, err := h.service.Widen(ctx, body.CycleID)
	if err != nil {
		return respondRefusal(c, err)
	}
	return httpx.Respond(c, widening)
}

func (h *Handler) Skip(c fiber.Ctx) error {
	var body skipRequest
	if appErr := httpx.DecodeObject(c, &body); appErr != nil {
		return appErr
	}
	if body.QueryLabel == "" {
		return httpx.ErrBadRequest.WithMessage(
			"a skipped query has to name the query label it was planned under",
		)
	}
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	result, err := h.service.Skip(ctx, SkipInput(body))
	if err != nil {
		return roundRefusal(err, body.CycleID)
	}
	return httpx.Respond(c, result)
}

func (h *Handler) Finish(c fiber.Ctx) error {
	var body finishRequest
	if appErr := httpx.DecodeObject(c, &body); appErr != nil {
		return appErr
	}
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	round, err := h.service.Finish(ctx, FinishInput(body))
	if err != nil {
		return roundRefusal(err, body.CycleID)
	}
	return httpx.Respond(c, roundResponse{Cycle: round})
}

func (h *Handler) Plan(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	state, err := h.service.State(ctx)
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	return httpx.Respond(c, state)
}

func (h *Handler) Notes(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	notes, err := h.service.RecentNotes(ctx, 40)
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	return httpx.Respond(c, notesResponse{Notes: notes})
}

func (h *Handler) DeleteNote(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return httpx.ErrBadRequest.WithMessage("a note id is a whole number")
	}
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	removed, err := h.service.DeleteNote(ctx, id)
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	if !removed {
		return httpx.ErrNotFound.WithMessage("no note with id " + strconv.FormatInt(id, 10))
	}
	notes, err := h.service.RecentNotes(ctx, 40)
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	return httpx.Respond(c, notesResponse{Notes: notes})
}

func respondRefusal(c fiber.Ctx, err error) error {
	var refusal *RefusalError
	if errors.As(err, &refusal) {
		return c.Status(fiber.StatusConflict).JSON(refusal)
	}
	return httpx.ErrInternal.WithCause(err)
}

func roundRefusal(err error, cycleID int64) error {
	if errors.Is(err, ErrNoRound) {
		return httpx.ErrNotFound.WithMessage(
			"no round with id " + strconv.FormatInt(cycleID, 10),
		)
	}
	var refusal *RefusalError
	if errors.As(err, &refusal) {
		return httpx.ErrConflict.WithMessage(refusal.Message)
	}
	return httpx.ErrBadRequest.WithMessage(err.Error())
}
