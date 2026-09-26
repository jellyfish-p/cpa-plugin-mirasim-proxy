package main

import (
	"gopkg.in/yaml.v3"
)

type pluginConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Priority      int    `yaml:"priority"`
	RelayURL      string `yaml:"relay_url"`
	Token         string `yaml:"token"`
	OAuthProvider string `yaml:"oauth_provider"` // "all" (default web page with github/google/session), "github", "google"
}

func defaultPluginConfig() pluginConfig {
	return pluginConfig{
		Enabled:       true,
		Priority:      1,
		RelayURL:      "https://relay.mirasim.ai",
		OAuthProvider: "all",
	}
}

func decodeConfig(raw []byte) (pluginConfig, error) {
	cfg := defaultPluginConfig()
	if len(raw) > 0 {
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return cfg, err
		}
	}
	if cfg.RelayURL == "" {
		cfg.RelayURL = "https://relay.mirasim.ai"
	}
	if cfg.OAuthProvider == "" {
		cfg.OAuthProvider = "all"
	}
	return cfg, nil
}
