package httpx

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
)

func BoundedRequestContext(
	c fiber.Ctx,
	parent context.Context,
	budget time.Duration,
) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = c.Context()
	}
	if budget <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, budget)
}

func DecodeObject(c fiber.Ctx, target any) *Error {
	if err := json.Unmarshal(c.Body(), target); err != nil {
		return ErrBadRequest.WithCause(err)
	}
	return nil
}

func PositiveInt(raw string) (int64, bool) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

func BoundedQueryInt(c fiber.Ctx, name string, fallback, limit int) int {
	value, err := strconv.Atoi(c.Query(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return min(value, limit)
}
