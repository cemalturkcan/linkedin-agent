package settings

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"api/internal/app/httpx"
)

type Config struct {
	OperationTimeout time.Duration
}

type Handler struct {
	service *Service
	config  Config
}

func NewHandler(service *Service, config Config) *Handler {
	return &Handler{service: service, config: config}
}

type document struct {
	Settings Settings `json:"settings"`
	Enums    Enums    `json:"enums"`
}

func (h *Handler) Get(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	current, err := h.service.Load(ctx)
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	return httpx.Respond(c, document{Settings: current, Enums: Enumerations()})
}

func (h *Handler) Put(c fiber.Ctx) error {
	patch := map[string]any{}
	if appErr := httpx.DecodeObject(c, &patch); appErr != nil {
		return appErr.WithMessage("settings must be a json object holding the groups you are changing")
	}
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	saved, err := h.service.Save(ctx, patch)
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	return httpx.Respond(c, document{Settings: saved, Enums: Enumerations()})
}
