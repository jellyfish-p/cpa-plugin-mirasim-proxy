package main

import (
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// staticModels registers pure, real model IDs under the mirasim/ namespace.
// Harness execution runners (pi, codex, claude, etc.) are strictly internal.
func staticModels() []pluginapi.ModelInfo {
	created := time.Now().Unix()
	return []pluginapi.ModelInfo{
		{
			ID:          "mirasim/claude-3-7-sonnet",
			Object:      "model",
			Created:     created,
			OwnedBy:     "anthropic",
			Type:        "chat",
			DisplayName: "Claude 3.7 Sonnet (via Mirasim)",
		},
		{
			ID:          "mirasim/claude-3-5-sonnet",
			Object:      "model",
			Created:     created,
			OwnedBy:     "anthropic",
			Type:        "chat",
			DisplayName: "Claude 3.5 Sonnet (via Mirasim)",
		},
		{
			ID:          "mirasim/claude-3-5-haiku",
			Object:      "model",
			Created:     created,
			OwnedBy:     "anthropic",
			Type:        "chat",
			DisplayName: "Claude 3.5 Haiku (via Mirasim)",
		},
		{
			ID:          "mirasim/gpt-4o",
			Object:      "model",
			Created:     created,
			OwnedBy:     "openai",
			Type:        "chat",
			DisplayName: "GPT-4o (via Mirasim)",
		},
		{
			ID:          "mirasim/gpt-4o-mini",
			Object:      "model",
			Created:     created,
			OwnedBy:     "openai",
			Type:        "chat",
			DisplayName: "GPT-4o Mini (via Mirasim)",
		},
		{
			ID:          "mirasim/o1",
			Object:      "model",
			Created:     created,
			OwnedBy:     "openai",
			Type:        "chat",
			DisplayName: "o1 (via Mirasim)",
		},
		{
			ID:          "mirasim/o3-mini",
			Object:      "model",
			Created:     created,
			OwnedBy:     "openai",
			Type:        "chat",
			DisplayName: "o3-mini (via Mirasim)",
		},
		{
			ID:          "mirasim/gemini-2.5-pro",
			Object:      "model",
			Created:     created,
			OwnedBy:     "google",
			Type:        "chat",
			DisplayName: "Gemini 2.5 Pro (via Mirasim)",
		},
		{
			ID:          "mirasim/gemini-2.0-flash",
			Object:      "model",
			Created:     created,
			OwnedBy:     "google",
			Type:        "chat",
			DisplayName: "Gemini 2.0 Flash (via Mirasim)",
		},
		{
			ID:          "mirasim/kimi-k1.5",
			Object:      "model",
			Created:     created,
			OwnedBy:     "moonshot",
			Type:        "chat",
			DisplayName: "Kimi k1.5 (via Mirasim)",
		},
		{
			ID:          "mirasim/qwen-2.5-coder-32b",
			Object:      "model",
			Created:     created,
			OwnedBy:     "alibaba",
			Type:        "chat",
			DisplayName: "Qwen 2.5 Coder 32B (via Mirasim)",
		},
		{
			ID:          "mirasim/grok-2",
			Object:      "model",
			Created:     created,
			OwnedBy:     "xai",
			Type:        "chat",
			DisplayName: "Grok 2 (via Mirasim)",
		},
		{
			ID:          "mirasim/glm-4",
			Object:      "model",
			Created:     created,
			OwnedBy:     "zhipu",
			Type:        "chat",
			DisplayName: "GLM-4 (via Mirasim)",
		},
	}
}
