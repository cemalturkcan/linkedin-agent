package httpx

import (
	"github.com/gofiber/fiber/v3"
)

type Error struct {
	Status  int
	Message string
	cause   error
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.cause
}

func NewError(status int, message string) *Error {
	return &Error{Status: status, Message: message}
}

func (e *Error) WithMessage(message string) *Error {
	clone := *e
	clone.Message = message
	return &clone
}

func (e *Error) WithCause(cause error) *Error {
	clone := *e
	clone.cause = cause
	return &clone
}

var (
	ErrBadRequest  = NewError(fiber.StatusBadRequest, "the request body is not the shape this route accepts")
	ErrNotFound    = NewError(fiber.StatusNotFound, "there is nothing at this address")
	ErrConflict    = NewError(fiber.StatusConflict, "the current state forbids this action")
	ErrUnavailable = NewError(fiber.StatusServiceUnavailable, "something this route depends on is not ready")
	ErrInternal    = NewError(fiber.StatusInternalServerError, "the service failed on its own side")
)
