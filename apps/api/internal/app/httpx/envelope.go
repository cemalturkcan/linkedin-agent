package httpx

import (
	"github.com/gofiber/fiber/v3"
)

const PathPrefix = "/api"

type errorEnvelope struct {
	Error string `json:"error"`
}

func Respond(c fiber.Ctx, resource any) error {
	return c.JSON(resource)
}

func RespondError(c fiber.Ctx, appErr *Error) error {
	return c.Status(appErr.Status).JSON(errorEnvelope{Error: appErr.Message})
}
