package events

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/sse"

	appevents "api/internal/app/events"
	"api/internal/app/httpx"
)

type Config struct {
	Retry     time.Duration
	Heartbeat time.Duration
}

func NewStreamHandler(hub *appevents.Hub, config Config) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Get(fiber.HeaderOrigin) == "" {
			c.Set(fiber.HeaderAccessControlAllowOrigin, "*")
		}

		subscriber, admitted := hub.Subscribe(lastEventID(c))
		if !admitted {
			return httpx.ErrUnavailable.WithMessage(
				strconv.Itoa(appevents.MaxSubscribers) +
					" event streams are already open, close one before opening another",
			)
		}
		return sse.New(sse.Config{
			Retry:             config.Retry,
			HeartbeatInterval: config.Heartbeat,
			Handler: func(_ fiber.Ctx, stream *sse.Stream) error {
				defer hub.Unsubscribe(subscriber)
				return pump(subscriber, stream)
			},
		})(c)
	}
}

func pump(subscriber *appevents.Subscriber, stream *sse.Stream) error {
	if err := stream.Comment("open"); err != nil {
		return err
	}
	for {
		select {
		case frame := <-subscriber.State():
			if err := write(stream, frame); err != nil {
				return err
			}
			continue
		default:
		}

		select {
		case frame := <-subscriber.State():
			if err := write(stream, frame); err != nil {
				return err
			}
		case frame := <-subscriber.Delta():
			if err := write(stream, frame); err != nil {
				return err
			}
		case <-subscriber.Done():
			return nil
		case <-stream.Done():
			return nil
		}
	}
}

func write(stream *sse.Stream, frame appevents.Frame) error {
	event := sse.Event{Name: frame.Name, Data: frame.Data}
	if frame.ID > 0 {
		event.ID = strconv.FormatInt(frame.ID, 10)
	}
	return stream.Event(event)
}

func lastEventID(c fiber.Ctx) int64 {
	raw := strings.TrimSpace(c.Get(fiber.HeaderLastEventID))
	if raw == "" {
		raw = strings.TrimSpace(c.Query("lastEventId"))
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0
	}
	return value
}
