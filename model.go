package main

import (
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func staticModels() []pluginapi.ModelInfo {
	created := time.Now().Unix()
	return []pluginapi.ModelInfo{
		{
			ID:          "mirasim/default",
			Object:      "model",
			Created:     created,
			OwnedBy:     "mirasim",
			Type:        "chat",
			DisplayName: "Mirasim Default Agent",
		},
		{
			ID:          "mirasim/pi",
			Object:      "model",
			Created:     created,
			OwnedBy:     "mirasim",
			Type:        "chat",
			DisplayName: "Pi Coding Agent",
		},
		{
			ID:          "mirasim/claude",
			Object:      "model",
			Created:     created,
			OwnedBy:     "anthropic",
			Type:        "chat",
			DisplayName: "Claude Code",
		},
		{
			ID:          "mirasim/claude-3-7-sonnet",
			Object:      "model",
			Created:     created,
			OwnedBy:     "anthropic",
			Type:        "chat",
			DisplayName: "Claude 3.7 Sonnet",
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
			ID:          "mirasim/codex",
			Object:      "model",
			Created:     created,
			OwnedBy:     "openai",
			Type:        "chat",
			DisplayName: "ChatGPT Codex",
		},
		{
			ID:          "mirasim/gpt-4o",
			Object:      "model",
			Created:     created,
			OwnedBy:     "openai",
			Type:        "chat",
			DisplayName: "GPT-4o",
		},
		{
			ID:          "mirasim/kimi",
			Object:      "model",
			Created:     created,
			OwnedBy:     "moonshot",
			Type:        "chat",
			DisplayName: "Kimi Code",
		},
		{
			ID:          "mirasim/qwen",
			Object:      "model",
			Created:     created,
			OwnedBy:     "alibaba",
			Type:        "chat",
			DisplayName: "Qwen Code",
		},
		{
			ID:          "mirasim/grok",
			Object:      "model",
			Created:     created,
			OwnedBy:     "xai",
			Type:        "chat",
			DisplayName: "Grok Build",
		},
		{
			ID:          "mirasim/zcode",
			Object:      "model",
			Created:     created,
			OwnedBy:     "zhipu",
			Type:        "chat",
			DisplayName: "ZCode CLI",
		},
	}
}
