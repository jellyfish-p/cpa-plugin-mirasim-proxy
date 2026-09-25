package main

import (
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func staticModels() []pluginapi.ModelInfo {
	created := time.Now().Unix()
	return []pluginapi.ModelInfo{
		{
			ID:          "mirasim",
			Object:      "model",
			Created:     created,
			OwnedBy:     "mirasim",
			Type:        "chat",
			DisplayName: "Mirasim Default Agent",
		},
		{
			ID:          "pi",
			Object:      "model",
			Created:     created,
			OwnedBy:     "mirasim",
			Type:        "chat",
			DisplayName: "Pi Coding Agent",
		},
		{
			ID:          "claude",
			Object:      "model",
			Created:     created,
			OwnedBy:     "anthropic",
			Type:        "chat",
			DisplayName: "Claude Code (via Mirasim)",
		},
		{
			ID:          "claude-3-7-sonnet",
			Object:      "model",
			Created:     created,
			OwnedBy:     "anthropic",
			Type:        "chat",
			DisplayName: "Claude 3.7 Sonnet (via Mirasim)",
		},
		{
			ID:          "claude-3-5-sonnet",
			Object:      "model",
			Created:     created,
			OwnedBy:     "anthropic",
			Type:        "chat",
			DisplayName: "Claude 3.5 Sonnet (via Mirasim)",
		},
		{
			ID:          "codex",
			Object:      "model",
			Created:     created,
			OwnedBy:     "openai",
			Type:        "chat",
			DisplayName: "ChatGPT Codex (via Mirasim)",
		},
		{
			ID:          "gpt-4o",
			Object:      "model",
			Created:     created,
			OwnedBy:     "openai",
			Type:        "chat",
			DisplayName: "GPT-4o (via Mirasim)",
		},
		{
			ID:          "kimi",
			Object:      "model",
			Created:     created,
			OwnedBy:     "moonshot",
			Type:        "chat",
			DisplayName: "Kimi Code (via Mirasim)",
		},
		{
			ID:          "qwen",
			Object:      "model",
			Created:     created,
			OwnedBy:     "alibaba",
			Type:        "chat",
			DisplayName: "Qwen Code (via Mirasim)",
		},
		{
			ID:          "grok",
			Object:      "model",
			Created:     created,
			OwnedBy:     "xai",
			Type:        "chat",
			DisplayName: "Grok Build (via Mirasim)",
		},
		{
			ID:          "zcode",
			Object:      "model",
			Created:     created,
			OwnedBy:     "zhipu",
			Type:        "chat",
			DisplayName: "ZCode CLI (via Mirasim)",
		},
	}
}
