package main

import (
	"strings"

	"gopkg.in/yaml.v3"
)

type pluginConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Priority     int    `yaml:"priority"`
	MirasimPort  int    `yaml:"port"`
	ServerScript string `yaml:"server_script"`
	DefaultAgent string `yaml:"default_agent"`
	WsURL        string `yaml:"ws_url"`
	AutoSpawn    bool   `yaml:"auto_spawn"`
}

func defaultPluginConfig() pluginConfig {
	return pluginConfig{
		Enabled:      true,
		Priority:     1,
		MirasimPort:  4939,
		DefaultAgent: "pi",
		AutoSpawn:    true,
	}
}

func decodeConfig(raw []byte) (pluginConfig, error) {
	cfg := defaultPluginConfig()
	if len(raw) > 0 {
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return cfg, err
		}
	}
	cfg.DefaultAgent = strings.TrimSpace(cfg.DefaultAgent)
	if cfg.DefaultAgent == "" {
		cfg.DefaultAgent = "pi"
	}
	if cfg.MirasimPort <= 0 {
		cfg.MirasimPort = 4939
	}
	return cfg, nil
}
