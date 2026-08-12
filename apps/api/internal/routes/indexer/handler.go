package indexer

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"

	"api/internal/app/httpx"
	"api/internal/routes/resumes"
)

type HandlerConfig struct {
	OperationTimeout time.Duration
	IndexBudget      time.Duration
}

type Handler struct {
	indexer *Service
	config  HandlerConfig
}

func NewHandler(service *Service, config HandlerConfig) *Handler {
	return &Handler{indexer: service, config: config}
}

type indexRequest struct {
	Force bool `json:"force"`
}

func (h *Handler) Profile(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	return httpx.Respond(c, h.indexer.State(ctx))
}

func (h *Handler) Index(c fiber.Ctx) error {
	var body indexRequest
	if len(c.Body()) > 0 {
		if appErr := httpx.DecodeObject(c, &body); appErr != nil {
			return appErr
		}
	}
	ctx, cancel := httpx.BoundedRequestContext(
		c,
		context.WithoutCancel(c.Context()),
		h.config.IndexBudget,
	)
	defer cancel()

	result, err := h.indexer.Run(ctx, body.Force)
	if err != nil {
		return refusal(err)
	}
	return httpx.Respond(c, result)
}

func refusal(err error) error {
	var refused *RefusalError
	switch {
	case errors.Is(err, ErrIndexing):
		return httpx.ErrConflict.WithMessage(err.Error()).WithCause(err)
	case errors.As(err, &refused):
		return httpx.ErrUnavailable.WithMessage(refused.Error()).WithCause(err)
	}
	return resumes.Refusal(err)
}
