package app

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"api/internal/app/httpx"
)

const (
	extensionScheme           = "chrome-extension://"
	requestPrivateNetwork     = "Access-Control-Request-Private-Network"
	allowPrivateNetworkHeader = "Access-Control-Allow-Private-Network"
)

func newServer(config Config) *fiber.App {
	server := fiber.New(fiber.Config{
		AppName:      "linkedin-agent-api",
		BodyLimit:    config.HTTPBodyLimitBytes,
		ErrorHandler: errorHandler,
		ReadTimeout:  config.HTTPReadTimeout,
		IdleTimeout:  config.HTTPIdleTimeout,
	})
	server.Use(recover.New(recover.Config{
		PanicHandler: func(_ fiber.Ctx, value any) error {
			return httpx.ErrInternal.WithCause(errors.New(fibererr(value)))
		},
	}))
	server.Use(requestLog)
	server.Use(allowPrivateNetwork)
	server.Use(cors.New(cors.Config{
		AllowOriginsFunc: func(origin string) bool {
			return strings.HasPrefix(origin, extensionScheme)
		},
		AllowMethods: []string{
			fiber.MethodGet,
			fiber.MethodPost,
			fiber.MethodPut,
			fiber.MethodDelete,
			fiber.MethodOptions,
		},
		AllowHeaders: []string{fiber.HeaderContentType, fiber.HeaderLastEventID},
		MaxAge:       600,
	}))
	return server
}

func requestLog(c fiber.Ctx) error {
	started := time.Now()
	err := c.Next()
	slog.Info(
		"request",
		"method", c.Method(),
		"path", c.Path(),
		"status", c.Response().StatusCode(),
		"ms", time.Since(started).Milliseconds(),
	)
	return err
}

func allowPrivateNetwork(c fiber.Ctx) error {
	if c.Method() == fiber.MethodOptions &&
		c.Get(requestPrivateNetwork) == "true" &&
		strings.HasPrefix(c.Get(fiber.HeaderOrigin), extensionScheme) {
		c.Set(allowPrivateNetworkHeader, "true")
	}
	return c.Next()
}

func errorHandler(c fiber.Ctx, err error) error {
	var appErr *httpx.Error
	if !errors.As(err, &appErr) {
		var fiberErr *fiber.Error
		switch {
		case errors.As(err, &fiberErr) && fiberErr.Code == fiber.StatusNotFound:
			appErr = httpx.ErrNotFound
		case errors.As(err, &fiberErr) && fiberErr.Code == fiber.StatusMethodNotAllowed:
			appErr = httpx.ErrNotFound
		case errors.As(err, &fiberErr) && fiberErr.Code < fiber.StatusInternalServerError:
			appErr = httpx.ErrBadRequest
		default:
			appErr = httpx.ErrInternal.WithCause(err)
		}
	}
	if appErr.Status >= fiber.StatusInternalServerError {
		slog.Error(
			"request failed",
			"method", c.Method(),
			"path", c.Path(),
			"err", errors.Unwrap(appErr),
		)
	}
	return httpx.RespondError(c, appErr)
}

func fibererr(value any) string {
	if message, ok := value.(string); ok {
		return "panic: " + message
	}
	if failure, ok := value.(error); ok {
		return "panic: " + failure.Error()
	}
	return "panic in a request handler"
}
