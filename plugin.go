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
	pluginName       = "mirasim-proxy"
	pluginIdentifier = "mirasim-proxy"
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

type rpcAuthLoginStartRequest struct {
	pluginapi.AuthLoginStartRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type rpcAuthLoginPollRequest struct {
	pluginapi.AuthLoginPollRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type rpcAuthRefreshRequest struct {
	pluginapi.AuthRefreshRequest
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
	AuthProvider          bool                         `json:"auth_provider"`
	ManagementAPI         bool                         `json:"management_api,omitempty"`
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
			Author:           "jellyfish-p",
			GitHubRepository: "https://github.com/jellyfish-p/cpa-plugin-mirasim-proxy",
			ConfigFields: []pluginapi.ConfigField{
				{
					Name:        "enabled",
					Type:        pluginapi.ConfigFieldTypeBoolean,
					Description: "Whether the Mirasim executor is enabled",
				},
				{
					Name:        "relay_url",
					Type:        pluginapi.ConfigFieldTypeString,
					Description: "Mirasim Relay / Gateway URL (default: https://relay.mirasim.ai)",
				},
				{
					Name:        "token",
					Type:        pluginapi.ConfigFieldTypeString,
					Description: "Manual authentication token (or configure via OAuth)",
				},
				{
					Name:        "oauth_provider",
					Type:        pluginapi.ConfigFieldTypeString,
					Description: "OAuth provider option: 'all' (default web selection page with GitHub/Google/Session), 'github', or 'google'",
				},
			},
		},
		Capabilities: registrationCapability{
			ModelProvider:         true,
			Executor:              true,
			AuthProvider:          true,
			ManagementAPI:         true,
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
		return okEnvelope(map[string]bool{"shutdown": true})

	case pluginabi.MethodModelStatic:
		return okEnvelope(GetModelsCatalog())

	case pluginabi.MethodModelForAuth:
		token, relayURL := GetActiveAuth()
		if token == "" {
			token = resolveActiveToken(loadedConfig())
		}
		if relayURL == "" {
			relayURL = resolveRelayBaseURL(loadedConfig())
		}
		_, _ = RefreshModelsFromUpstream(context.Background(), token, relayURL)
		return okEnvelope(pluginapi.ModelResponse{
			Provider: pluginIdentifier,
			Models:   GetModelsCatalog(),
		})

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

	case pluginabi.MethodAuthIdentifier:
		return okEnvelope(handleAuthIdentifier())

	case pluginabi.MethodAuthParse:
		var req pluginapi.AuthParseRequest
		if err := json.Unmarshal(request, &req); err != nil {
			return errorEnvelope("invalid_request", err.Error()), nil
		}
		resp, err := handleAuthParse(req)
		if err != nil {
			return errorEnvelope("auth_parse_error", err.Error()), nil
		}
		return okEnvelope(resp)

	case pluginabi.MethodAuthLoginStart:
		var req rpcAuthLoginStartRequest
		if err := json.Unmarshal(request, &req); err != nil {
			return errorEnvelope("invalid_request", err.Error()), nil
		}
		resp, err := handleAuthLoginStart(req.AuthLoginStartRequest)
		if err != nil {
			return errorEnvelope("auth_login_start_error", err.Error()), nil
		}
		return okEnvelope(resp)

	case pluginabi.MethodAuthLoginPoll:
		var req rpcAuthLoginPollRequest
		if err := json.Unmarshal(request, &req); err != nil {
			return errorEnvelope("invalid_request", err.Error()), nil
		}
		resp, err := handleAuthLoginPoll(context.Background(), req.AuthLoginPollRequest)
		if err != nil {
			return errorEnvelope("auth_login_poll_error", err.Error()), nil
		}
		return okEnvelope(resp)

	case pluginabi.MethodAuthRefresh:
		var req rpcAuthRefreshRequest
		if err := json.Unmarshal(request, &req); err != nil {
			return errorEnvelope("invalid_request", err.Error()), nil
		}
		resp, err := handleAuthRefresh(req.AuthRefreshRequest)
		if err != nil {
			return errorEnvelope("auth_refresh_error", err.Error()), nil
		}
		return okEnvelope(resp)

	case pluginabi.MethodManagementRegister:
		return okEnvelope(pluginapi.ManagementRegistrationResponse{
			Resources: []pluginapi.ResourceRoute{{
				Path:        "/auth",
				Menu:        "Mirasim Auth",
				Description: "Mirasim OAuth and Session login selection page",
			}},
		})

	case pluginabi.MethodManagementHandle:
		var req pluginapi.ManagementRequest
		if err := json.Unmarshal(request, &req); err != nil {
			return errorEnvelope("invalid_request", err.Error()), nil
		}
		return okEnvelope(handleManagementRequest(req))

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
