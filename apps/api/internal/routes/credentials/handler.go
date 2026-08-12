package credentials

import (
	"errors"

	"github.com/gofiber/fiber/v3"

	appcredentials "api/internal/app/credentials"
	"api/internal/app/events"
	"api/internal/app/httpx"
)

type Handler struct {
	resolver *appcredentials.Resolver
	hub      *events.Hub
}

func NewHandler(resolver *appcredentials.Resolver, hub *events.Hub) *Handler {
	return &Handler{resolver: resolver, hub: hub}
}

type storeRequest struct {
	Value string `json:"value"`
}

type announcement struct {
	Credentials presence `json:"credentials"`
}

type presence struct {
	Loaded bool    `json:"loaded"`
	Source *string `json:"source"`
	Stored bool    `json:"stored"`
}

func (h *Handler) Get(c fiber.Ctx) error {
	return httpx.Respond(c, h.resolver.Status())
}

func (h *Handler) Put(c fiber.Ctx) error {
	var body storeRequest
	if appErr := httpx.DecodeObject(c, &body); appErr != nil {
		return appErr
	}
	status, err := h.resolver.Store(body.Value)
	if err != nil {
		return refusal(err)
	}
	h.announce(status)
	return httpx.Respond(c, status)
}

func (h *Handler) Delete(c fiber.Ctx) error {
	status, err := h.resolver.Clear()
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	h.announce(status)
	return httpx.Respond(c, status)
}

func (h *Handler) announce(status appcredentials.Status) {
	h.hub.Emit(events.TypeSetup, announcement{
		Credentials: presence{
			Loaded: status.Loaded,
			Source: status.Source,
			Stored: status.Stored,
		},
	})
}

func refusal(err error) error {
	var refused *appcredentials.RefusalError
	if errors.As(err, &refused) {
		return httpx.ErrBadRequest.WithMessage(appcredentials.Scrub(refused.Message)).WithCause(err)
	}
	return httpx.ErrInternal.WithCause(err)
}
