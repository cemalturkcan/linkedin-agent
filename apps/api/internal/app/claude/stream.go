package claude

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"api/internal/app/trace"
)

const (
	stopReasonRefusal = "refusal"
	blockTypeToolUse  = "tool_use"
	deltaTypeInput    = "input_json_delta"
)

type streamUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

type stopDetail struct {
	Category string `json:"category"`
}

type streamFrame struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message *struct {
		Model string       `json:"model"`
		Usage *streamUsage `json:"usage"`
	} `json:"message"`
	ContentBlock *struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta *struct {
		Type        string      `json:"type"`
		PartialJSON string      `json:"partial_json"`
		StopReason  string      `json:"stop_reason"`
		StopDetails *stopDetail `json:"stop_details"`
	} `json:"delta"`
	StopDetails *stopDetail  `json:"stop_details"`
	Usage       *streamUsage `json:"usage"`
	Error       *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type streamResult struct {
	model         string
	stopReason    string
	stopCategory  string
	usage         trace.Usage
	toolInput     string
	toolCallFound bool
}

func parseStream(body io.Reader, toolName string, onDelta func(string)) (streamResult, error) {
	reader := bufio.NewReader(body)
	result := streamResult{}
	var input strings.Builder
	toolIndex := -1

	var data strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			trimmed := strings.TrimRight(line, "\r\n")
			switch {
			case trimmed == "":
				if failure := dispatch(data.String(), toolName, &result, &input, &toolIndex, onDelta); failure != nil {
					return result, failure
				}
				data.Reset()
			case strings.HasPrefix(trimmed, "data:"):
				data.WriteString(strings.TrimPrefix(strings.TrimPrefix(trimmed, "data:"), " "))
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return result, fmt.Errorf("read model stream: %w", err)
		}
	}
	if failure := dispatch(data.String(), toolName, &result, &input, &toolIndex, onDelta); failure != nil {
		return result, failure
	}

	result.toolInput = input.String()
	return result, nil
}

func dispatch(
	payload string,
	toolName string,
	result *streamResult,
	input *strings.Builder,
	toolIndex *int,
	onDelta func(string),
) error {
	if payload == "" {
		return nil
	}
	var frame streamFrame
	if err := json.Unmarshal([]byte(payload), &frame); err != nil {
		return fmt.Errorf("decode model stream frame: %w", err)
	}

	switch frame.Type {
	case "error":
		if frame.Error != nil {
			return fmt.Errorf("the model stream failed: %s", frame.Error.Type)
		}
		return errors.New("the model stream failed without saying why")
	case "message_start":
		if frame.Message != nil {
			result.model = frame.Message.Model
			applyUsage(&result.usage, frame.Message.Usage)
		}
	case "content_block_start":
		if frame.ContentBlock != nil &&
			frame.ContentBlock.Type == blockTypeToolUse &&
			frame.ContentBlock.Name == toolName {
			*toolIndex = frame.Index
			result.toolCallFound = true
		}
	case "content_block_delta":
		if frame.Delta != nil &&
			frame.Delta.Type == deltaTypeInput &&
			frame.Index == *toolIndex &&
			frame.Delta.PartialJSON != "" {
			input.WriteString(frame.Delta.PartialJSON)
			onDelta(frame.Delta.PartialJSON)
		}
	case "message_delta":
		if frame.Delta != nil {
			if frame.Delta.StopReason != "" {
				result.stopReason = frame.Delta.StopReason
			}
			if frame.Delta.StopDetails != nil {
				result.stopCategory = frame.Delta.StopDetails.Category
			}
		}
		if frame.StopDetails != nil {
			result.stopCategory = frame.StopDetails.Category
		}
		applyUsage(&result.usage, frame.Usage)
	}
	return nil
}

func applyUsage(target *trace.Usage, source *streamUsage) {
	if source == nil {
		return
	}
	if source.InputTokens > 0 {
		target.InputTokens = source.InputTokens
	}
	if source.OutputTokens > 0 {
		target.OutputTokens = source.OutputTokens
	}
	if source.CacheReadInputTokens > 0 {
		target.CacheReadTokens = source.CacheReadInputTokens
	}
	if source.CacheCreationInputTokens > 0 {
		target.CacheWriteTokens = source.CacheCreationInputTokens
	}
}
