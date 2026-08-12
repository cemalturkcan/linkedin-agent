package traces

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"

	"api/internal/app/httpx"
)

type Config struct {
	OperationTimeout time.Duration
	PageLimitDefault int
	PageLimitMax     int
}

type Handler struct {
	service *Service
	config  Config
}

func NewHandler(service *Service, config Config) *Handler {
	return &Handler{service: service, config: config}
}

type listResponse struct {
	Running *Trace  `json:"running"`
	Traces  []Trace `json:"traces"`
}

func (h *Handler) List(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	limit := httpx.BoundedQueryInt(c, "limit", h.config.PageLimitDefault, h.config.PageLimitMax)
	running, open, err := h.service.Running(ctx)
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	recent, err := h.service.Recent(ctx, limit)
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	response := listResponse{Traces: recent}
	if open {
		response.Running = &running
	}
	return httpx.Respond(c, response)
}

func (h *Handler) Get(c fiber.Ctx) error {
	id, ok := httpx.PositiveInt(c.Params("id"))
	if !ok {
		return httpx.ErrNotFound.WithMessage("no such call")
	}
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	found, err := h.service.Trace(ctx, id)
	if errors.Is(err, ErrNoTrace) {
		return httpx.ErrNotFound.WithMessage("no such call")
	}
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	return httpx.Respond(c, found)
}
