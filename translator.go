package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sdktr "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

// StreamState keeps track of streaming context across SSE events.
type StreamState struct {
	ID        string
	Created   int64
	Model     string
	MessageID string
	Finished  bool
}

func NewStreamState(model string) *StreamState {
	return &StreamState{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Created: time.Now().Unix(),
		Model:   model,
	}
}

// TranslateOpenAIToClaude translates an OpenAI chat completions payload into Anthropic Messages API format.
// Reuses the official CLIProxyAPI translator if registered, otherwise applies full native transformation.
func TranslateOpenAIToClaude(openAIPayload []byte, modelID string) ([]byte, error) {
	// 1. Check if host has registered OpenAI -> Claude translator
	if sdktr.HasRequestTransformerByFormatName(sdktr.FormatOpenAI, sdktr.FormatClaude) {
		translated := sdktr.TranslateRequestByFormatName(sdktr.FormatOpenAI, sdktr.FormatClaude, modelID, openAIPayload, false)
		if len(translated) > 0 {
			return translated, nil
		}
	}

	// 2. Direct robust translation
	var openAIReq struct {
		Model       string          `json:"model"`
		Messages    []ChatMessage   `json:"messages"`
		MaxTokens   *int            `json:"max_tokens,omitempty"`
		Temperature *float64        `json:"temperature,omitempty"`
		TopP        *float64        `json:"top_p,omitempty"`
		Stream      bool            `json:"stream,omitempty"`
		Tools       json.RawMessage `json:"tools,omitempty"`
	}

	if err := json.Unmarshal(openAIPayload, &openAIReq); err != nil {
		return nil, fmt.Errorf("unmarshal openai request: %w", err)
	}

	var systemMessages []string
	var claudeMessages []map[string]any

	for _, msg := range openAIReq.Messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		text := ExtractTextContent(msg.Content)

		switch role {
		case "system", "developer":
			if text != "" {
				systemMessages = append(systemMessages, text)
			}
		case "assistant":
			claudeMessages = append(claudeMessages, map[string]any{
				"role":    "assistant",
				"content": text,
			})
		default: // "user", etc.
			claudeMessages = append(claudeMessages, map[string]any{
				"role":    "user",
				"content": text,
			})
		}
	}

	maxTokens := 4096
	if openAIReq.MaxTokens != nil && *openAIReq.MaxTokens > 0 {
		maxTokens = *openAIReq.MaxTokens
	}

	claudeReq := map[string]any{
		"model":      modelID,
		"messages":   claudeMessages,
		"max_tokens": maxTokens,
		"stream":     openAIReq.Stream,
	}

	if len(systemMessages) > 0 {
		claudeReq["system"] = strings.Join(systemMessages, "\n\n")
	}
	if openAIReq.Temperature != nil {
		claudeReq["temperature"] = *openAIReq.Temperature
	}
	if openAIReq.TopP != nil {
		claudeReq["top_p"] = *openAIReq.TopP
	}

	// Translate tools if present
	if len(openAIReq.Tools) > 0 {
		var openAITools []struct {
			Type     string `json:"type"`
			Function struct {
				Name        string          `json:"name"`
				Description string          `json:"description,omitempty"`
				Parameters  json.RawMessage `json:"parameters,omitempty"`
			} `json:"function"`
		}
		if err := json.Unmarshal(openAIReq.Tools, &openAITools); err == nil && len(openAITools) > 0 {
			var claudeTools []map[string]any
			for _, t := range openAITools {
				tool := map[string]any{
					"name": t.Function.Name,
				}
				if t.Function.Description != "" {
					tool["description"] = t.Function.Description
				}
				if len(t.Function.Parameters) > 0 {
					var schema any
					if err := json.Unmarshal(t.Function.Parameters, &schema); err == nil {
						tool["input_schema"] = schema
					}
				}
				if tool["input_schema"] == nil {
					tool["input_schema"] = map[string]any{
						"type": "object",
					}
				}
				claudeTools = append(claudeTools, tool)
			}
			claudeReq["tools"] = claudeTools
		}
	}

	return json.Marshal(claudeReq)
}

// TranslateClaudeToOpenAIResponse translates a non-streaming Anthropic response into OpenAI format.
func TranslateClaudeToOpenAIResponse(claudePayload []byte, modelID string) ([]byte, error) {
	var claudeResp struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text,omitempty"`
			Thinking string `json:"thinking,omitempty"`
		} `json:"content"`
		StopReason *string `json:"stop_reason,omitempty"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(claudePayload, &claudeResp); err != nil {
		return nil, fmt.Errorf("unmarshal claude response: %w", err)
	}

	var fullText strings.Builder
	var fullThinking strings.Builder

	for _, block := range claudeResp.Content {
		switch block.Type {
		case "text":
			fullText.WriteString(block.Text)
		case "thinking":
			fullThinking.WriteString(block.Thinking)
		}
	}

	finishReason := "stop"
	if claudeResp.StopReason != nil {
		switch *claudeResp.StopReason {
		case "max_tokens":
			finishReason = "length"
		case "tool_use":
			finishReason = "tool_calls"
		default:
			finishReason = "stop"
		}
	}

	openAIResp := ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   modelID,
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:             "assistant",
					Content:          fullText.String(),
					ReasoningContent: fullThinking.String(),
				},
				FinishReason: finishReason,
			},
		},
		Usage: &UsageInfo{
			PromptTokens:     claudeResp.Usage.InputTokens,
			CompletionTokens: claudeResp.Usage.OutputTokens,
			TotalTokens:      claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens,
		},
	}

	return json.Marshal(openAIResp)
}

// TranslateClaudeEventToOpenAIChunks parses an Anthropic SSE event and converts it to OpenAI SSE lines.
func TranslateClaudeEventToOpenAIChunks(eventType string, eventData []byte, state *StreamState) [][]byte {
	var chunks [][]byte

	var raw map[string]any
	if err := json.Unmarshal(eventData, &raw); err != nil {
		return nil
	}

	switch eventType {
	case "message_start":
		if msg, ok := raw["message"].(map[string]any); ok {
			if id, ok := msg["id"].(string); ok && id != "" {
				state.MessageID = id
			}
		}
		// Emit initial role chunk
		initChunk := ChatCompletionChunk{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []ChatChunkChoice{
				{
					Index: 0,
					Delta: ChatMessageDelta{
						Role: "assistant",
					},
					FinishReason: nil,
				},
			},
		}
		b, _ := json.Marshal(initChunk)
		chunks = append(chunks, []byte(fmt.Sprintf("data: %s\n\n", b)))

	case "content_block_delta":
		if delta, ok := raw["delta"].(map[string]any); ok {
			deltaType, _ := delta["type"].(string)
			switch deltaType {
			case "text_delta":
				if text, ok := delta["text"].(string); ok && text != "" {
					chunk := ChatCompletionChunk{
						ID:      state.ID,
						Object:  "chat.completion.chunk",
						Created: state.Created,
						Model:   state.Model,
						Choices: []ChatChunkChoice{
							{
								Index: 0,
								Delta: ChatMessageDelta{
									Content: text,
								},
								FinishReason: nil,
							},
						},
					}
					b, _ := json.Marshal(chunk)
					chunks = append(chunks, []byte(fmt.Sprintf("data: %s\n\n", b)))
				}

			case "thinking_delta":
				if thinking, ok := delta["thinking"].(string); ok && thinking != "" {
					chunk := ChatCompletionChunk{
						ID:      state.ID,
						Object:  "chat.completion.chunk",
						Created: state.Created,
						Model:   state.Model,
						Choices: []ChatChunkChoice{
							{
								Index: 0,
								Delta: ChatMessageDelta{
									ReasoningContent: thinking,
								},
								FinishReason: nil,
							},
						},
					}
					b, _ := json.Marshal(chunk)
					chunks = append(chunks, []byte(fmt.Sprintf("data: %s\n\n", b)))
				}
			}
		}

	case "message_delta":
		var finishReason *string
		if delta, ok := raw["delta"].(map[string]any); ok {
			if stopReason, ok := delta["stop_reason"].(string); ok && stopReason != "" {
				fr := "stop"
				switch stopReason {
				case "max_tokens":
					fr = "length"
				case "tool_use":
					fr = "tool_calls"
				}
				finishReason = &fr
			}
		}

		var usage *UsageInfo
		if u, ok := raw["usage"].(map[string]any); ok {
			outputTokens, _ := u["output_tokens"].(float64)
			usage = &UsageInfo{
				CompletionTokens: int(outputTokens),
			}
		}

		if finishReason != nil || usage != nil {
			state.Finished = true
			finalChunk := ChatCompletionChunk{
				ID:      state.ID,
				Object:  "chat.completion.chunk",
				Created: state.Created,
				Model:   state.Model,
				Choices: []ChatChunkChoice{
					{
						Index:        0,
						Delta:        ChatMessageDelta{},
						FinishReason: finishReason,
					},
				},
				Usage: usage,
			}
			b, _ := json.Marshal(finalChunk)
			chunks = append(chunks, []byte(fmt.Sprintf("data: %s\n\n", b)))
		}

	case "message_stop":
		if !state.Finished {
			state.Finished = true
			fr := "stop"
			finalChunk := ChatCompletionChunk{
				ID:      state.ID,
				Object:  "chat.completion.chunk",
				Created: state.Created,
				Model:   state.Model,
				Choices: []ChatChunkChoice{
					{
						Index:        0,
						Delta:        ChatMessageDelta{},
						FinishReason: &fr,
					},
				},
			}
			b, _ := json.Marshal(finalChunk)
			chunks = append(chunks, []byte(fmt.Sprintf("data: %s\n\n", b)))
		}
	}

	return chunks
}
