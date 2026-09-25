package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

var httpClient = &http.Client{
	Timeout: 5 * time.Minute,
}

// ChatMessage represents a single message in OpenAI format.
type ChatMessage struct {
	Role             string      `json:"role"`
	Content          interface{} `json:"content"`
	ReasoningContent string      `json:"reasoning_content,omitempty"`
}

// ChatCompletionRequest is the OpenAI request payload.
type ChatCompletionRequest struct {
	Model       string          `json:"model"`
	Messages    []ChatMessage   `json:"messages"`
	Stream      bool            `json:"stream,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	Tools       json.RawMessage `json:"tools,omitempty"`
}

// ChatCompletionResponse is the OpenAI response payload.
type ChatCompletionResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   *UsageInfo   `json:"usage,omitempty"`
}

type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatCompletionChunk struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Created int64             `json:"created"`
	Model   string            `json:"model"`
	Choices []ChatChunkChoice `json:"choices"`
	Usage   *UsageInfo        `json:"usage,omitempty"`
}

type ChatChunkChoice struct {
	Index        int              `json:"index"`
	Delta        ChatMessageDelta `json:"delta"`
	FinishReason *string          `json:"finish_reason"`
}

type ChatMessageDelta struct {
	Role             string `json:"role,omitempty"`
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

// ExtractTextContent extracts text from string or multi-part content.
func ExtractTextContent(content interface{}) string {
	if content == nil {
		return ""
	}
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var sb strings.Builder
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["type"].(string); ok && t == "text" {
					if txt, ok := m["text"].(string); ok {
						sb.WriteString(txt)
					}
				}
			}
		}
		return sb.String()
	default:
		b, _ := json.Marshal(content)
		return string(b)
	}
}

// resolveActiveToken retrieves the active token from memory, config, or discovery.
func resolveActiveToken(cfg pluginConfig) string {
	token, _ := GetActiveAuth()
	if token != "" {
		return token
	}
	if cfg.Token != "" {
		return cfg.Token
	}
	// Try discovery
	if tok := discoverLocalToken(); tok != "" {
		return tok
	}
	return ""
}

// resolveRelayBaseURL returns the configured or default relay gateway URL.
func resolveRelayBaseURL(cfg pluginConfig) string {
	url := strings.TrimSpace(cfg.RelayURL)
	if url == "" {
		url = "https://relay.mirasim.ai"
	}
	return strings.TrimRight(url, "/")
}

// executeNonStream processes a direct pass-through non-streaming completion.
func executeNonStream(ctx context.Context, cfg pluginConfig, req pluginapi.ExecutorRequest) (pluginapi.ExecutorResponse, error) {
	body := req.Payload
	if len(body) == 0 {
		body = req.OriginalRequest
	}

	modelName := req.Model
	var chatReq ChatCompletionRequest
	if err := json.Unmarshal(body, &chatReq); err == nil && chatReq.Model != "" {
		modelName = chatReq.Model
	}

	modelID := strings.TrimPrefix(strings.TrimSpace(modelName), "mirasim-proxy/")
	modelID = strings.TrimPrefix(modelID, "mirasim/")
	token := resolveActiveToken(cfg)
	if token == "" {
		return pluginapi.ExecutorResponse{}, fmt.Errorf("no Mirasim authentication token found; please log in via OAuth panel or set token in config")
	}

	relayURL := resolveRelayBaseURL(cfg)
	isClaude := strings.HasPrefix(strings.ToLower(modelID), "claude")

	var targetURL string
	var reqBody []byte
	var err error

	if isClaude {
		targetURL = relayURL + "/v1/messages"
		reqBody, err = TranslateOpenAIToClaude(body, modelID)
		if err != nil {
			return pluginapi.ExecutorResponse{}, fmt.Errorf("translate request to anthropic: %w", err)
		}
	} else {
		targetURL = relayURL + "/v1/chat/completions"
		reqBody = body
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(reqBody))
	if err != nil {
		return pluginapi.ExecutorResponse{}, fmt.Errorf("create upstream request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("x-api-key", token)
	if isClaude {
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return pluginapi.ExecutorResponse{}, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return pluginapi.ExecutorResponse{}, fmt.Errorf("read upstream response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return pluginapi.ExecutorResponse{}, fmt.Errorf("upstream error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var finalRespBytes []byte
	if isClaude {
		finalRespBytes, err = TranslateClaudeToOpenAIResponse(respBytes, modelID)
		if err != nil {
			return pluginapi.ExecutorResponse{}, fmt.Errorf("translate response to openai: %w", err)
		}
	} else {
		finalRespBytes = respBytes
	}

	return pluginapi.ExecutorResponse{
		Payload: finalRespBytes,
		Headers: http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

type rpcStreamEmitRequest struct {
	StreamID string `json:"stream_id"`
	Payload  []byte `json:"payload,omitempty"`
	Error    string `json:"error,omitempty"`
}

type rpcStreamCloseRequest struct {
	StreamID string `json:"stream_id"`
	Error    string `json:"error,omitempty"`
}

// executeStreamRequest directly forwards and translates streaming completions over HTTP.
func executeStreamRequest(cfg pluginConfig, req rpcExecutorRequest) (streamResponse, error) {
	body := req.Payload
	if len(body) == 0 {
		body = req.OriginalRequest
	}

	modelName := req.Model
	var chatReq ChatCompletionRequest
	if err := json.Unmarshal(body, &chatReq); err == nil && chatReq.Model != "" {
		modelName = chatReq.Model
	}

	modelID := strings.TrimPrefix(strings.TrimSpace(modelName), "mirasim-proxy/")
	modelID = strings.TrimPrefix(modelID, "mirasim/")
	token := resolveActiveToken(cfg)
	if token == "" {
		return streamResponse{}, fmt.Errorf("no Mirasim authentication token found; please log in via OAuth panel or set token in config")
	}

	relayURL := resolveRelayBaseURL(cfg)
	isClaude := strings.HasPrefix(strings.ToLower(modelID), "claude")

	var targetURL string
	var reqBody []byte
	var err error

	if isClaude {
		targetURL = relayURL + "/v1/messages"
		reqBody, err = TranslateOpenAIToClaude(body, modelID)
		if err != nil {
			return streamResponse{}, fmt.Errorf("translate request to anthropic: %w", err)
		}
	} else {
		targetURL = relayURL + "/v1/chat/completions"
		reqBody = body
	}

	ctx := context.Background()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(reqBody))
	if err != nil {
		return streamResponse{}, fmt.Errorf("create upstream request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("x-api-key", token)
	if isClaude {
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return streamResponse{}, fmt.Errorf("upstream request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		errBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return streamResponse{}, fmt.Errorf("upstream error (status %d): %s", resp.StatusCode, string(errBytes))
	}

	streamHeaders := http.Header{"Content-Type": []string{"text/event-stream"}}

	// If Host supports asynchronous stream emitting via StreamID
	if req.StreamID != "" {
		go func() {
			defer resp.Body.Close()
			defer func() {
				_, _ = callHost("host.stream.close", rpcStreamCloseRequest{StreamID: req.StreamID})
			}()

			reader := bufio.NewReader(resp.Body)
			state := NewStreamState(modelID)

			if isClaude {
				var currentEvent string
				for {
					line, errRead := reader.ReadString('\n')
					if errRead != nil {
						break
					}
					line = strings.TrimRight(line, "\r\n")

					if strings.HasPrefix(line, "event:") {
						currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
						continue
					}
					if strings.HasPrefix(line, "data:") {
						dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
						if dataStr == "" || dataStr == "[DONE]" {
							continue
						}
						chunks := TranslateClaudeEventToOpenAIChunks(currentEvent, []byte(dataStr), state)
						for _, chunk := range chunks {
							_, _ = callHost("host.stream.emit", rpcStreamEmitRequest{
								StreamID: req.StreamID,
								Payload:  chunk,
							})
						}
					}
				}
				// Final DONE
				_, _ = callHost("host.stream.emit", rpcStreamEmitRequest{
					StreamID: req.StreamID,
					Payload:  []byte("data: [DONE]\n\n"),
				})
			} else {
				// Pure OpenAI stream pass-through
				for {
					line, errRead := reader.ReadBytes('\n')
					if errRead != nil {
						break
					}
					if len(line) > 0 {
						_, _ = callHost("host.stream.emit", rpcStreamEmitRequest{
							StreamID: req.StreamID,
							Payload:  line,
						})
					}
				}
			}
		}()

		return streamResponse{Headers: streamHeaders}, nil
	}

	// Fallback to synchronous chunks collection
	defer resp.Body.Close()
	var chunks []pluginapi.ExecutorStreamChunk
	reader := bufio.NewReader(resp.Body)
	state := NewStreamState(modelID)

	if isClaude {
		var currentEvent string
		for {
			line, errRead := reader.ReadString('\n')
			if errRead != nil {
				break
			}
			line = strings.TrimRight(line, "\r\n")

			if strings.HasPrefix(line, "event:") {
				currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if dataStr == "" || dataStr == "[DONE]" {
					continue
				}
				translatedChunks := TranslateClaudeEventToOpenAIChunks(currentEvent, []byte(dataStr), state)
				for _, tc := range translatedChunks {
					chunks = append(chunks, pluginapi.ExecutorStreamChunk{Payload: tc})
				}
			}
		}
		chunks = append(chunks, pluginapi.ExecutorStreamChunk{Payload: []byte("data: [DONE]\n\n")})
	} else {
		for {
			line, errRead := reader.ReadBytes('\n')
			if errRead != nil {
				break
			}
			if len(line) > 0 {
				chunks = append(chunks, pluginapi.ExecutorStreamChunk{Payload: line})
			}
		}
	}

	return streamResponse{Headers: streamHeaders, Chunks: chunks}, nil
}
