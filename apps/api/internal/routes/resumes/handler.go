package resumes

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"

	"api/internal/app/httpx"
)

type Config struct {
	OperationTimeout time.Duration
}

type Handler struct {
	resumes *Service
	config  Config
}

func NewHandler(folder *Service, config Config) *Handler {
	return &Handler{resumes: folder, config: config}
}

type listResponse struct {
	ResumeDir string   `json:"resumeDir"`
	Resumes   []Resume `json:"resumes"`
}

func (h *Handler) List(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	dir, found, err := h.resumes.List(ctx)
	if err != nil {
		return Refusal(err)
	}
	return httpx.Respond(c, listResponse{ResumeDir: dir, Resumes: found})
}

func (h *Handler) File(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	file, err := h.resumes.File(ctx, c.Params("code"), c.Params("lang"))
	if err != nil {
		return Refusal(err)
	}
	c.Set(fiber.HeaderContentType, "application/pdf")
	c.Set(fiber.HeaderContentDisposition, `inline; filename="`+file.Name+`"`)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Send(file.Bytes)
}

func Refusal(err error) error {
	var folder *FolderError
	var language *LanguageError
	switch {
	case errors.Is(err, ErrNoFolder), errors.Is(err, ErrNoPDFs):
		return httpx.ErrUnavailable.WithMessage(err.Error()).WithCause(err)
	case errors.As(err, &folder):
		return httpx.ErrUnavailable.WithMessage(folder.Error()).WithCause(err)
	case errors.As(err, &language):
		return httpx.ErrNotFound.WithMessage(language.Error()).WithCause(err)
	case errors.Is(err, ErrNoResume):
		return httpx.ErrNotFound.WithMessage(err.Error()).WithCause(err)
	}
	return httpx.ErrInternal.WithCause(err)
}
