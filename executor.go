package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/router-for-me/cliproxy-plugin-mirasim/mirasim"
)

var (
	mirasimClientMu sync.Mutex
	mirasimClient   *mirasim.Client
	processMgr      *mirasim.ProcessManager
)

// EnsureMirasimClient returns an active Mirasim client based on configuration and auth.
func EnsureMirasimClient(cfg pluginConfig) (*mirasim.Client, error) {
	mirasimClientMu.Lock()
	defer mirasimClientMu.Unlock()

	// 1. Check if auth has been established via OAuth or auth.parse
	_, wsURL := GetActiveAuth()
	if wsURL != "" {
		if mirasimClient == nil {
			mirasimClient = mirasim.NewClient(wsURL)
		} else {
			mirasimClient.UpdateURL(wsURL)
		}
		return mirasimClient, nil
	}

	if mirasimClient != nil && mirasimClient.GetURL() != "" {
		return mirasimClient, nil
	}

	wsURL = cfg.WsURL

	if wsURL == "" {
		// 2. Scan for running instances
		instances, err := mirasim.FindActiveInstances()
		if err == nil && len(instances) > 0 {
			wsURL = instances[0].WsURL
			SetActiveAuth(instances[0].Token, wsURL)
			log.Printf("[Plugin:Mirasim] Attached to running instance on port %d", instances[0].Port)
		} else if cfg.AutoSpawn {
			// 3. Auto-spawn backend
			scriptPath, err := mirasim.LocateServerScript(cfg.ServerScript)
			if err != nil {
				return nil, fmt.Errorf("locate server.cjs: %w", err)
			}
			log.Printf("[Plugin:Mirasim] Spawning server.cjs on port %d...", cfg.MirasimPort)

			pm, err := mirasim.StartMirasimServer(scriptPath, cfg.MirasimPort, "")
			if err != nil {
				return nil, fmt.Errorf("start mirasim server: %w", err)
			}
			processMgr = pm
			wsURL = pm.WsURL()
			SetActiveAuth(pm.Token(), wsURL)
		} else {
			return nil, fmt.Errorf("no active Mirasim server found and auto_spawn is disabled")
		}
	}

	mirasimClient = mirasim.NewClient(wsURL)
	return mirasimClient, nil
}

// ChatMessage represents a single message in OpenAI format.
type ChatMessage struct {
	Role             string      `json:"role"`
	Content          interface{} `json:"content"`
	ReasoningContent string      `json:"reasoning_content,omitempty"`
}

// ChatCompletionRequest is the OpenAI request payload.
type ChatCompletionRequest struct {
	Model      string        `json:"model"`
	Messages   []ChatMessage `json:"messages"`
	Stream     bool          `json:"stream,omitempty"`
	SessionKey string        `json:"session_key,omitempty"`
	Effort     string        `json:"effort,omitempty"`
	Workdir    string        `json:"workdir,omitempty"`
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

// ConvertMessagesToPrompt converts OpenAI messages to Mirasim prompt.
func ConvertMessagesToPrompt(messages []ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}
	if len(messages) == 1 && (messages[0].Role == "user" || messages[0].Role == "") {
		return ExtractTextContent(messages[0].Content)
	}

	var systemPrompt string
	var conversation []ChatMessage

	for _, msg := range messages {
		if msg.Role == "system" || msg.Role == "developer" {
			txt := ExtractTextContent(msg.Content)
			if systemPrompt == "" {
				systemPrompt = txt
			} else {
				systemPrompt += "\n" + txt
			}
		} else {
			conversation = append(conversation, msg)
		}
	}

	if systemPrompt == "" && len(conversation) == 1 && conversation[0].Role == "user" {
		return ExtractTextContent(conversation[0].Content)
	}

	var sb strings.Builder
	if systemPrompt != "" {
		sb.WriteString("[System Instructions]\n")
		sb.WriteString(systemPrompt)
		sb.WriteString("\n\n")
	}

	if len(conversation) > 0 {
		lastIdx := len(conversation) - 1
		lastMsg := conversation[lastIdx]

		if lastIdx > 0 {
			sb.WriteString("[Conversation History]\n")
			for i := 0; i < lastIdx; i++ {
				m := conversation[i]
				role := strings.Title(m.Role)
				if role == "" {
					role = "User"
				}
				sb.WriteString(fmt.Sprintf("%s: %s\n", role, ExtractTextContent(m.Content)))
			}
			sb.WriteString("\n")
		}

		if lastMsg.Role == "user" {
			sb.WriteString(ExtractTextContent(lastMsg.Content))
		} else {
			sb.WriteString(fmt.Sprintf("%s: %s", strings.Title(lastMsg.Role), ExtractTextContent(lastMsg.Content)))
		}
	}

	return sb.String()
}

// MapModelToAgent resolves the Mirasim agent & sub-model from the requested model name.
// Supports mirasim/{model} format (e.g. mirasim/pi, mirasim/claude-3-7-sonnet, mirasim/codex).
func MapModelToAgent(modelName string) (agent string, actualModel string) {
	m := strings.TrimSpace(modelName)
	// Strip "mirasim/" prefix if present
	if strings.HasPrefix(strings.ToLower(m), "mirasim/") {
		m = m[len("mirasim/"):]
	}

	mLower := strings.ToLower(m)

	if parts := strings.SplitN(m, ":", 2); len(parts) == 2 {
		return parts[0], parts[1]
	}
	if parts := strings.SplitN(m, "/", 2); len(parts) == 2 {
		return parts[0], parts[1]
	}

	switch {
	case mLower == "" || mLower == "default":
		return "", ""
	case mLower == "pi":
		return "pi", ""
	case mLower == "claude":
		return "claude", ""
	case strings.HasPrefix(mLower, "claude-"):
		return "claude", m
	case mLower == "codex":
		return "codex", ""
	case strings.HasPrefix(mLower, "gpt-") || strings.HasPrefix(mLower, "o1") || strings.HasPrefix(mLower, "o3"):
		return "codex", m
	case strings.HasPrefix(mLower, "kimi"):
		return "kimi", m
	case strings.HasPrefix(mLower, "qwen"):
		return "qwen", m
	case strings.HasPrefix(mLower, "grok"):
		return "grok", m
	case strings.HasPrefix(mLower, "zcode") || strings.HasPrefix(mLower, "glm"):
		return "zcode", m
	default:
		return m, ""
	}
}

// executeNonStream processes a non-streaming completion.
func executeNonStream(ctx context.Context, cfg pluginConfig, req pluginapi.ExecutorRequest) (pluginapi.ExecutorResponse, error) {
	body := req.Payload
	if len(body) == 0 {
		body = req.OriginalRequest
	}

	var chatReq ChatCompletionRequest
	if err := json.Unmarshal(body, &chatReq); err != nil {
		return pluginapi.ExecutorResponse{}, fmt.Errorf("unmarshal chat request: %w", err)
	}

	prompt := ConvertMessagesToPrompt(chatReq.Messages)
	if prompt == "" {
		return pluginapi.ExecutorResponse{}, fmt.Errorf("empty prompt")
	}

	modelName := chatReq.Model
	if modelName == "" {
		modelName = req.Model
	}

	agent, model := MapModelToAgent(modelName)

	client, err := EnsureMirasimClient(cfg)
	if err != nil {
		return pluginapi.ExecutorResponse{}, fmt.Errorf("ensure mirasim client: %w", err)
	}

	events, err := client.ExecuteTurn(ctx, mirasim.TurnOptions{
		Prompt:     prompt,
		SessionKey: chatReq.SessionKey,
		Agent:      agent,
		Model:      model,
		Effort:     chatReq.Effort,
		Workdir:    chatReq.Workdir,
	})
	if err != nil {
		return pluginapi.ExecutorResponse{}, fmt.Errorf("execute mirasim turn: %w", err)
	}

	var fullText strings.Builder
	var fullReasoning strings.Builder
	var lastUsage *UsageInfo
	var turnErr error

	for evt := range events {
		if evt.Error != nil {
			turnErr = evt.Error
			break
		}
		if evt.AppendText != "" {
			fullText.WriteString(evt.AppendText)
		}
		if evt.AppendReasoning != "" {
			fullReasoning.WriteString(evt.AppendReasoning)
		}
		if evt.Usage != nil {
			lastUsage = &UsageInfo{
				PromptTokens:     evt.Usage.InputTokens,
				CompletionTokens: evt.Usage.OutputTokens,
				TotalTokens:      evt.Usage.InputTokens + evt.Usage.OutputTokens,
			}
		}
		if evt.Done {
			break
		}
	}

	if turnErr != nil {
		return pluginapi.ExecutorResponse{}, turnErr
	}

	resp := ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   modelName,
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:             "assistant",
					Content:          fullText.String(),
					ReasoningContent: fullReasoning.String(),
				},
				FinishReason: "stop",
			},
		},
		Usage: lastUsage,
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		return pluginapi.ExecutorResponse{}, err
	}

	return pluginapi.ExecutorResponse{
		Payload: respBytes,
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

// executeStreamRequest handles streaming execution via host callback or synchronous chunks.
func executeStreamRequest(cfg pluginConfig, req rpcExecutorRequest) (streamResponse, error) {
	body := req.Payload
	if len(body) == 0 {
		body = req.OriginalRequest
	}

	var chatReq ChatCompletionRequest
	if err := json.Unmarshal(body, &chatReq); err != nil {
		return streamResponse{}, fmt.Errorf("unmarshal chat request: %w", err)
	}

	prompt := ConvertMessagesToPrompt(chatReq.Messages)
	if prompt == "" {
		return streamResponse{}, fmt.Errorf("empty prompt")
	}

	modelName := chatReq.Model
	if modelName == "" {
		modelName = req.Model
	}

	agent, model := MapModelToAgent(modelName)

	client, err := EnsureMirasimClient(cfg)
	if err != nil {
		return streamResponse{}, fmt.Errorf("ensure mirasim client: %w", err)
	}

	ctx := context.Background()
	events, err := client.ExecuteTurn(ctx, mirasim.TurnOptions{
		Prompt:     prompt,
		SessionKey: chatReq.SessionKey,
		Agent:      agent,
		Model:      model,
		Effort:     chatReq.Effort,
		Workdir:    chatReq.Workdir,
	})
	if err != nil {
		return streamResponse{}, fmt.Errorf("execute mirasim turn: %w", err)
	}

	reqID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()
	streamHeaders := http.Header{"Content-Type": []string{"text/event-stream"}}

	// If Host supports asynchronous stream emitting via StreamID
	if req.StreamID != "" {
		go func() {
			// Initial chunk
			initChunk := ChatCompletionChunk{
				ID:      reqID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   modelName,
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
			initBytes, _ := json.Marshal(initChunk)
			_, _ = callHost("host.stream.emit", rpcStreamEmitRequest{
				StreamID: req.StreamID,
				Payload:  []byte(fmt.Sprintf("data: %s\n\n", initBytes)),
			})

			var lastUsage *UsageInfo

			for evt := range events {
				if evt.Error != nil {
					errChunk := ChatCompletionChunk{
						ID:      reqID,
						Object:  "chat.completion.chunk",
						Created: created,
						Model:   modelName,
						Choices: []ChatChunkChoice{
							{
								Index: 0,
								Delta: ChatMessageDelta{
									Content: fmt.Sprintf("\n[Error: %v]", evt.Error),
								},
								FinishReason: nil,
							},
						},
					}
					eb, _ := json.Marshal(errChunk)
					_, _ = callHost("host.stream.emit", rpcStreamEmitRequest{
						StreamID: req.StreamID,
						Payload:  []byte(fmt.Sprintf("data: %s\n\n", eb)),
					})
					break
				}

				if evt.Usage != nil {
					lastUsage = &UsageInfo{
						PromptTokens:     evt.Usage.InputTokens,
						CompletionTokens: evt.Usage.OutputTokens,
						TotalTokens:      evt.Usage.InputTokens + evt.Usage.OutputTokens,
					}
				}

				if evt.AppendReasoning != "" {
					chunk := ChatCompletionChunk{
						ID:      reqID,
						Object:  "chat.completion.chunk",
						Created: created,
						Model:   modelName,
						Choices: []ChatChunkChoice{
							{
								Index: 0,
								Delta: ChatMessageDelta{
									ReasoningContent: evt.AppendReasoning,
								},
								FinishReason: nil,
							},
						},
					}
					cb, _ := json.Marshal(chunk)
					_, _ = callHost("host.stream.emit", rpcStreamEmitRequest{
						StreamID: req.StreamID,
						Payload:  []byte(fmt.Sprintf("data: %s\n\n", cb)),
					})
				}

				if evt.AppendText != "" {
					chunk := ChatCompletionChunk{
						ID:      reqID,
						Object:  "chat.completion.chunk",
						Created: created,
						Model:   modelName,
						Choices: []ChatChunkChoice{
							{
								Index: 0,
								Delta: ChatMessageDelta{
									Content: evt.AppendText,
								},
								FinishReason: nil,
							},
						},
					}
					cb, _ := json.Marshal(chunk)
					_, _ = callHost("host.stream.emit", rpcStreamEmitRequest{
						StreamID: req.StreamID,
						Payload:  []byte(fmt.Sprintf("data: %s\n\n", cb)),
					})
				}

				if evt.Done {
					finishReason := "stop"
					finalChunk := ChatCompletionChunk{
						ID:      reqID,
						Object:  "chat.completion.chunk",
						Created: created,
						Model:   modelName,
						Choices: []ChatChunkChoice{
							{
								Index:        0,
								Delta:        ChatMessageDelta{},
								FinishReason: &finishReason,
							},
						},
						Usage: lastUsage,
					}
					fb, _ := json.Marshal(finalChunk)
					_, _ = callHost("host.stream.emit", rpcStreamEmitRequest{
						StreamID: req.StreamID,
						Payload:  []byte(fmt.Sprintf("data: %s\n\n", fb)),
					})
					_, _ = callHost("host.stream.emit", rpcStreamEmitRequest{
						StreamID: req.StreamID,
						Payload:  []byte("data: [DONE]\n\n"),
					})
					break
				}
			}

			_, _ = callHost("host.stream.close", rpcStreamCloseRequest{StreamID: req.StreamID})
		}()

		return streamResponse{Headers: streamHeaders}, nil
	}

	// Fallback to synchronous chunks array
	var chunks []pluginapi.ExecutorStreamChunk

	initChunk := ChatCompletionChunk{
		ID:      reqID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   modelName,
		Choices: []ChatChunkChoice{{Index: 0, Delta: ChatMessageDelta{Role: "assistant"}}},
	}
	initBytes, _ := json.Marshal(initChunk)
	chunks = append(chunks, pluginapi.ExecutorStreamChunk{Payload: []byte(fmt.Sprintf("data: %s\n\n", initBytes))})

	var lastUsage *UsageInfo
	for evt := range events {
		if evt.Usage != nil {
			lastUsage = &UsageInfo{
				PromptTokens:     evt.Usage.InputTokens,
				CompletionTokens: evt.Usage.OutputTokens,
				TotalTokens:      evt.Usage.InputTokens + evt.Usage.OutputTokens,
			}
		}
		if evt.AppendReasoning != "" {
			c := ChatCompletionChunk{
				ID:      reqID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   modelName,
				Choices: []ChatChunkChoice{{Index: 0, Delta: ChatMessageDelta{ReasoningContent: evt.AppendReasoning}}},
			}
			cb, _ := json.Marshal(c)
			chunks = append(chunks, pluginapi.ExecutorStreamChunk{Payload: []byte(fmt.Sprintf("data: %s\n\n", cb))})
		}
		if evt.AppendText != "" {
			c := ChatCompletionChunk{
				ID:      reqID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   modelName,
				Choices: []ChatChunkChoice{{Index: 0, Delta: ChatMessageDelta{Content: evt.AppendText}}},
			}
			cb, _ := json.Marshal(c)
			chunks = append(chunks, pluginapi.ExecutorStreamChunk{Payload: []byte(fmt.Sprintf("data: %s\n\n", cb))})
		}
		if evt.Done {
			finishReason := "stop"
			fc := ChatCompletionChunk{
				ID:      reqID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   modelName,
				Choices: []ChatChunkChoice{{Index: 0, Delta: ChatMessageDelta{}, FinishReason: &finishReason}},
				Usage:   lastUsage,
			}
			fb, _ := json.Marshal(fc)
			chunks = append(chunks, pluginapi.ExecutorStreamChunk{Payload: []byte(fmt.Sprintf("data: %s\n\n", fb))})
			chunks = append(chunks, pluginapi.ExecutorStreamChunk{Payload: []byte("data: [DONE]\n\n")})
			break
		}
	}

	return streamResponse{Headers: streamHeaders, Chunks: chunks}, nil
}
