package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// staticModels returns the comprehensive list of pure model IDs supported by Mirasim.
// Zero harness names are exposed; all models strictly use the mirasim/{model_id} namespace.
func staticModels() []pluginapi.ModelInfo {
	created := time.Now().Unix()
	return []pluginapi.ModelInfo{
		// --- Claude (Anthropic) ---
		{
			ID:          "mirasim/claude-3-7-sonnet",
			Object:      "model",
			Created:     created,
			OwnedBy:     "anthropic",
			Type:        "chat",
			DisplayName: "Claude 3.7 Sonnet (Thinking)",
		},
		{
			ID:          "mirasim/claude-3-5-sonnet",
			Object:      "model",
			Created:     created,
			OwnedBy:     "anthropic",
			Type:        "chat",
			DisplayName: "Claude 3.5 Sonnet",
		},
		{
			ID:          "mirasim/claude-3-5-haiku",
			Object:      "model",
			Created:     created,
			OwnedBy:     "anthropic",
			Type:        "chat",
			DisplayName: "Claude 3.5 Haiku",
		},
		{
			ID:          "mirasim/claude-opus-5-5",
			Object:      "model",
			Created:     created,
			OwnedBy:     "anthropic",
			Type:        "chat",
			DisplayName: "Claude Opus 5.5",
		},
		{
			ID:          "mirasim/claude-sonnet-5",
			Object:      "model",
			Created:     created,
			OwnedBy:     "anthropic",
			Type:        "chat",
			DisplayName: "Claude Sonnet 5",
		},
		{
			ID:          "mirasim/claude-haiku-4-5",
			Object:      "model",
			Created:     created,
			OwnedBy:     "anthropic",
			Type:        "chat",
			DisplayName: "Claude Haiku 4.5",
		},

		// --- OpenAI / Codex ---
		{
			ID:          "mirasim/gpt-4o",
			Object:      "model",
			Created:     created,
			OwnedBy:     "openai",
			Type:        "chat",
			DisplayName: "GPT-4o",
		},
		{
			ID:          "mirasim/gpt-4o-mini",
			Object:      "model",
			Created:     created,
			OwnedBy:     "openai",
			Type:        "chat",
			DisplayName: "GPT-4o Mini",
		},
		{
			ID:          "mirasim/o1",
			Object:      "model",
			Created:     created,
			OwnedBy:     "openai",
			Type:        "chat",
			DisplayName: "o1",
		},
		{
			ID:          "mirasim/o3-mini",
			Object:      "model",
			Created:     created,
			OwnedBy:     "openai",
			Type:        "chat",
			DisplayName: "o3-mini",
		},
		{
			ID:          "mirasim/gpt-6-astra",
			Object:      "model",
			Created:     created,
			OwnedBy:     "openai",
			Type:        "chat",
			DisplayName: "GPT-6 Astra",
		},
		{
			ID:          "mirasim/gpt-5.6-sol",
			Object:      "model",
			Created:     created,
			OwnedBy:     "openai",
			Type:        "chat",
			DisplayName: "GPT-5.6 Sol",
		},

		// --- Google (Gemini) ---
		{
			ID:          "mirasim/gemini-2.5-pro",
			Object:      "model",
			Created:     created,
			OwnedBy:     "google",
			Type:        "chat",
			DisplayName: "Gemini 2.5 Pro",
		},
		{
			ID:          "mirasim/gemini-2.0-flash",
			Object:      "model",
			Created:     created,
			OwnedBy:     "google",
			Type:        "chat",
			DisplayName: "Gemini 2.0 Flash",
		},

		// --- DeepSeek ---
		{
			ID:          "mirasim/deepseek-chat",
			Object:      "model",
			Created:     created,
			OwnedBy:     "deepseek",
			Type:        "chat",
			DisplayName: "DeepSeek V3",
		},
		{
			ID:          "mirasim/deepseek-reasoner",
			Object:      "model",
			Created:     created,
			OwnedBy:     "deepseek",
			Type:        "chat",
			DisplayName: "DeepSeek R1",
		},
		{
			ID:          "mirasim/deepseek-flash",
			Object:      "model",
			Created:     created,
			OwnedBy:     "deepseek",
			Type:        "chat",
			DisplayName: "DeepSeek V4.1 Flash",
		},

		// --- Kimi (Moonshot) ---
		{
			ID:          "mirasim/kimi-k1.5",
			Object:      "model",
			Created:     created,
			OwnedBy:     "moonshot",
			Type:        "chat",
			DisplayName: "Kimi k1.5",
		},
		{
			ID:          "mirasim/kimi-k3",
			Object:      "model",
			Created:     created,
			OwnedBy:     "moonshot",
			Type:        "chat",
			DisplayName: "Kimi k3",
		},

		// --- Qwen (Alibaba) ---
		{
			ID:          "mirasim/qwen-2.5-coder-32b",
			Object:      "model",
			Created:     created,
			OwnedBy:     "alibaba",
			Type:        "chat",
			DisplayName: "Qwen 2.5 Coder 32B",
		},
		{
			ID:          "mirasim/qwen-2.5-72b",
			Object:      "model",
			Created:     created,
			OwnedBy:     "alibaba",
			Type:        "chat",
			DisplayName: "Qwen 2.5 72B",
		},

		// --- xAI ---
		{
			ID:          "mirasim/grok-2",
			Object:      "model",
			Created:     created,
			OwnedBy:     "xai",
			Type:        "chat",
			DisplayName: "Grok 2",
		},

		// --- Zhipu (GLM) ---
		{
			ID:          "mirasim/glm-4",
			Object:      "model",
			Created:     created,
			OwnedBy:     "zhipu",
			Type:        "chat",
			DisplayName: "GLM-4",
		},
		{
			ID:          "mirasim/glm-5.3-flash",
			Object:      "model",
			Created:     created,
			OwnedBy:     "zhipu",
			Type:        "chat",
			DisplayName: "GLM-5.3 Flash",
		},
	}
}

// fetchUpstreamModels queries https://relay.mirasim.ai/v1/models if a token is available.
func fetchUpstreamModels(token, relayURL string) []pluginapi.ModelInfo {
	if token == "" {
		return staticModels()
	}

	if relayURL == "" {
		relayURL = "https://relay.mirasim.ai"
	}
	url := strings.TrimRight(relayURL, "/") + "/v1/models"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return staticModels()
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("x-api-key", token)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return staticModels()
	}
	defer resp.Body.Close()

	var payload struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil || len(payload.Data) == 0 {
		return staticModels()
	}

	created := time.Now().Unix()
	var dynamic []pluginapi.ModelInfo
	for _, m := range payload.Data {
		cleanID := strings.TrimPrefix(m.ID, "mirasim/")
		dynamic = append(dynamic, pluginapi.ModelInfo{
			ID:          "mirasim/" + cleanID,
			Object:      "model",
			Created:     created,
			OwnedBy:     m.OwnedBy,
			Type:        "chat",
			DisplayName: cleanID + " (via Mirasim)",
		})
	}

	return dynamic
}
