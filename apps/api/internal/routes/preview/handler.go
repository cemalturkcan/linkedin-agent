package preview

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"

	"api/internal/app/httpx"
	"api/internal/routes/planner"
	"api/internal/routes/screening"
)

type Config struct {
	OperationTimeout time.Duration
}

type Planning interface {
	Preview(ctx context.Context) (string, *planner.RefusalError, error)
}

type Screening interface {
	PromptInput(ctx context.Context) (screening.PromptInput, error)
}

type Handler struct {
	planning  Planning
	screening Screening
	config    Config
}

func NewHandler(planning Planning, screening Screening, config Config) *Handler {
	return &Handler{planning: planning, screening: screening, config: config}
}

type Prompt struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	System      string  `json:"system"`
	Unavailable *string `json:"unavailable"`
}

type Response struct {
	Ready   bool     `json:"ready"`
	Missing string   `json:"missing,omitempty"`
	Prompts []Prompt `json:"prompts"`
}

func (h *Handler) Get(c fiber.Ctx) error {
	ctx, cancel := httpx.BoundedRequestContext(c, c.Context(), h.config.OperationTimeout)
	defer cancel()

	input, err := h.screening.PromptInput(ctx)
	if err != nil {
		var refused *screening.RefusalError
		if !errors.As(err, &refused) {
			return httpx.ErrInternal.WithCause(err)
		}
		return httpx.Respond(c, Response{Missing: refused.Error(), Prompts: []Prompt{}})
	}

	planning, refusal, err := h.planning.Preview(ctx)
	if err != nil {
		return httpx.ErrInternal.WithCause(err)
	}
	entry := Prompt{ID: "planning", Label: "planning a round", System: planning}
	if refusal != nil {
		message := refusal.Message
		entry.Unavailable = &message
	}

	return httpx.Respond(c, Response{
		Ready: true,
		Prompts: []Prompt{
			entry,
			{
				ID:     "triage",
				Label:  "triage from the title",
				System: screening.BuildTriagePrompt(input),
			},
			{
				ID:     "deep",
				Label:  "screening the full posting",
				System: screening.BuildDeepPrompt(input),
			},
			{
				ID:     "review",
				Label:  "reviewing the batch of verdicts",
				System: screening.BuildReviewPrompt(input),
			},
			{
				ID:     "reflector",
				Label:  "learning from outcomes",
				System: screening.BuildReflectorPrompt(input),
			},
		},
	})
}
