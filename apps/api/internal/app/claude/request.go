package claude

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	MessagesURL      = "https://api.anthropic.com/v1/messages"
	AnthropicVersion = "2023-06-01"

	HeaderVersion = "Anthropic-Version"
	HeaderBeta    = "Anthropic-Beta"
	HeaderApp     = "X-App"

	Model           = "claude-opus-5"
	MaxOutputTokens = 128000

	CLIIdentity = "You are Claude Code, Anthropic's official CLI for Claude."
	UserAgent   = "claude-cli/2.1.226 (external, sdk-cli)"
	AppHeader   = "cli"
)

var Betas = []string{"oauth-2025-04-20", "claude-code-20250219"}

func BetaHeader() string {
	return strings.Join(Betas, ",")
}

type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
	Strict      bool   `json:"strict"`
}

type Call struct {
	Effort  string
	Purpose string
	System  string
	User    string
	Tool    Tool
}

const Nudge = "(system: your turn ended without a call to %s, so nothing was recorded and the " +
	"batch is still sitting unjudged. Plain prose never reaches the system: the tool call is the " +
	"only way an answer arrives. Answer the same request again now by calling %s.)"

type cacheControl struct {
	Type string `json:"type"`
}

type systemBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type toolChoice struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type outputConfig struct {
	Effort string `json:"effort"`
}

type request struct {
	Model        string        `json:"model"`
	MaxTokens    int           `json:"max_tokens"`
	System       []systemBlock `json:"system"`
	Tools        []Tool        `json:"tools"`
	ToolChoice   toolChoice    `json:"tool_choice"`
	Messages     []message     `json:"messages"`
	Stream       bool          `json:"stream"`
	OutputConfig *outputConfig `json:"output_config,omitempty"`
}

func newRequest(model string, maxTokens int, call Call, user string) request {
	built := request{
		Model:     model,
		MaxTokens: maxTokens,
		System: []systemBlock{
			{Type: "text", Text: CLIIdentity},
			{Type: "text", Text: call.System, CacheControl: &cacheControl{Type: "ephemeral"}},
		},
		Tools:      []Tool{call.Tool},
		ToolChoice: toolChoice{Type: "tool", Name: call.Tool.Name},
		Messages:   []message{{Role: "user", Content: user}},
		Stream:     true,
	}
	if call.Effort != "" {
		built.OutputConfig = &outputConfig{Effort: call.Effort}
	}
	return built
}

func encodeRequest(model string, maxTokens int, call Call, user string) ([]byte, error) {
	body, err := json.Marshal(newRequest(model, maxTokens, call, user))
	if err != nil {
		return nil, fmt.Errorf("encode model request: %w", err)
	}
	return body, nil
}

func nudged(user, toolName string) string {
	return user + "\n\n" + fmt.Sprintf(Nudge, toolName, toolName)
}
