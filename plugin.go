package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const (
	pluginName       = "mirasim"
	pluginIdentifier = "mirasim"
	pluginVersion    = "1.0.0"
)

var (
	currentConfig atomic.Value
)

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type lifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}

type rpcExecutorRequest struct {
	pluginapi.ExecutorRequest
	StreamID       string `json:"stream_id,omitempty"`
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type streamResponse struct {
	Headers http.Header                     `json:"headers,omitempty"`
	Chunks  []pluginapi.ExecutorStreamChunk `json:"chunks,omitempty"`
}

type registration struct {
	SchemaVersion uint32                 `json:"schema_version"`
	Metadata      pluginapi.Metadata     `json:"metadata"`
	Capabilities  registrationCapability `json:"capabilities"`
}

type registrationCapability struct {
	ModelProvider         bool                         `json:"model_provider"`
	Executor              bool                         `json:"executor"`
	ExecutorModelScope    pluginapi.ExecutorModelScope `json:"executor_model_scope"`
	ExecutorInputFormats  []string                     `json:"executor_input_formats,omitempty"`
	ExecutorOutputFormats []string                     `json:"executor_output_formats,omitempty"`
}

func init() {
	currentConfig.Store(defaultPluginConfig())
}

func loadedConfig() pluginConfig {
	if v := currentConfig.Load(); v != nil {
		if cfg, ok := v.(pluginConfig); ok {
			return cfg
		}
	}
	return defaultPluginConfig()
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             pluginName,
			Version:          pluginVersion,
			Author:           "cpa-mirasim",
			GitHubRepository: "https://github.com/router-for-me/CLIProxyAPI",
			ConfigFields: []pluginapi.ConfigField{
				{
					Name:        "enabled",
					Type:        pluginapi.ConfigFieldTypeBoolean,
					Description: "Whether the Mirasim executor is enabled",
				},
				{
					Name:        "port",
					Type:        pluginapi.ConfigFieldTypeInteger,
					Description: "Local Mirasim server port (default 4939)",
				},
				{
					Name:        "ws_url",
					Type:        pluginapi.ConfigFieldTypeString,
					Description: "Explicit WebSocket URL for Mirasim server (ws://...)",
				},
				{
					Name:        "default_agent",
					Type:        pluginapi.ConfigFieldTypeString,
					Description: "Default agent to use (pi, claude, codex, kimi, qwen, grok)",
				},
				{
					Name:        "server_script",
					Type:        pluginapi.ConfigFieldTypeString,
					Description: "Path to server.cjs script",
				},
				{
					Name:        "auto_spawn",
					Type:        pluginapi.ConfigFieldTypeBoolean,
					Description: "Whether to auto-launch server.cjs if not running",
				},
			},
		},
		Capabilities: registrationCapability{
			ModelProvider:         true,
			Executor:              true,
			ExecutorModelScope:    pluginapi.ExecutorModelScopeBoth,
			ExecutorInputFormats:  []string{"chat-completions", "openai.chat"},
			ExecutorOutputFormats: []string{"chat-completions", "openai.chat"},
		},
	}
}

func handlePluginMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister:
		return okEnvelope(pluginRegistration())

	case pluginabi.MethodPluginReconfigure:
		var req lifecycleRequest
		if len(request) > 0 {
			_ = json.Unmarshal(request, &req)
		}
		cfg, err := decodeConfig(req.ConfigYAML)
		if err != nil {
			return errorEnvelope("config_error", err.Error()), nil
		}
		currentConfig.Store(cfg)
		return okEnvelope(pluginRegistration())

	case pluginabi.MethodPluginQuiesce:
		return okEnvelope(map[string]bool{"quiesced": true})

	case pluginabi.MethodPluginShutdown:
		mirasimClientMu.Lock()
		if processMgr != nil {
			_ = processMgr.Stop()
			processMgr = nil
		}
		mirasimClientMu.Unlock()
		return okEnvelope(map[string]bool{"shutdown": true})

	case pluginabi.MethodModelStatic:
		return okEnvelope(staticModels())

	case pluginabi.MethodExecutorIdentifier:
		return okEnvelope(map[string]string{"identifier": pluginIdentifier})

	case pluginabi.MethodExecutorCountTokens:
		return okEnvelope(pluginapi.ExecutorResponse{
			Payload: []byte(`{"input_tokens":0}`),
		})

	case pluginabi.MethodExecutorExecute:
		var req pluginapi.ExecutorRequest
		if err := json.Unmarshal(request, &req); err != nil {
			return errorEnvelope("invalid_request", err.Error()), nil
		}
		resp, err := executeNonStream(context.Background(), loadedConfig(), req)
		if err != nil {
			return errorEnvelope("executor_error", err.Error()), nil
		}
		return okEnvelope(resp)

	case pluginabi.MethodExecutorExecuteStream:
		var req rpcExecutorRequest
		if err := json.Unmarshal(request, &req); err != nil {
			return errorEnvelope("invalid_request", err.Error()), nil
		}
		resp, err := executeStreamRequest(loadedConfig(), req)
		if err != nil {
			return errorEnvelope("stream_error", err.Error()), nil
		}
		return okEnvelope(resp)

	default:
		return errorEnvelope("unknown_method", fmt.Sprintf("method not handled: %s", method)), nil
	}
}

func okEnvelope(result any) ([]byte, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

func errorEnvelope(code, message string, httpStatus ...int) []byte {
	status := 0
	if len(httpStatus) > 0 {
		status = httpStatus[0]
	}
	raw, _ := json.Marshal(envelope{
		OK: false,
		Error: &envelopeError{
			Code:       code,
			Message:    message,
			HTTPStatus: status,
		},
	})
	return raw
}
