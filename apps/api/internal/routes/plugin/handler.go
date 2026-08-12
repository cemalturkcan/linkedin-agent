package plugin

import (
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

type document struct {
	Plugin State `json:"plugin"`
}

func (h *Handler) Get(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	state, err := h.service.State(ctx)
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	return httpx.Respond(c, document{Plugin: state})
}

func (h *Handler) Hello(c fiber.Ctx) error {
	var body Hello
	if appErr := httpx.DecodeObject(c, &body); appErr != nil {
		return appErr.WithMessage(
			"a hello carries the extension id, its version and its capability manifest",
		)
	}
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	state, err := h.service.Hello(ctx, body)
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	return httpx.Respond(c, document{Plugin: state})
}
