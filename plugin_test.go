package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestPluginRegister(t *testing.T) {
	raw, err := handlePluginMethod(pluginabi.MethodPluginRegister, nil)
	if err != nil {
		t.Fatalf("handlePluginMethod(plugin.register) error: %v", err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok=true, got error: %+v", env.Error)
	}

	var reg registration
	if err := json.Unmarshal(env.Result, &reg); err != nil {
		t.Fatalf("unmarshal registration: %v", err)
	}

	if reg.Metadata.Name != "mirasim" {
		t.Errorf("expected metadata.Name = mirasim, got %s", reg.Metadata.Name)
	}
	if !reg.Capabilities.Executor {
		t.Errorf("expected Capabilities.Executor = true")
	}
	if !reg.Capabilities.ModelProvider {
		t.Errorf("expected Capabilities.ModelProvider = true")
	}
	if !reg.Capabilities.AuthProvider {
		t.Errorf("expected Capabilities.AuthProvider = true")
	}
}

func TestModelStaticPrefix(t *testing.T) {
	raw, err := handlePluginMethod(pluginabi.MethodModelStatic, nil)
	if err != nil {
		t.Fatalf("handlePluginMethod(model.static) error: %v", err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok=true, got error: %+v", env.Error)
	}

	var models []pluginapi.ModelInfo
	if err := json.Unmarshal(env.Result, &models); err != nil {
		t.Fatalf("unmarshal models: %v", err)
	}

	if len(models) == 0 {
		t.Fatalf("expected at least 1 static model, got 0")
	}

	for _, m := range models {
		if !strings.HasPrefix(m.ID, "mirasim/") {
			t.Errorf("model %q does not have 'mirasim/' prefix", m.ID)
		}
	}
}

func TestMapModelToAgent(t *testing.T) {
	cases := []struct {
		input       string
		wantAgent   string
		wantSub     string
	}{
		{"mirasim/pi", "pi", ""},
		{"mirasim/claude", "claude", ""},
		{"mirasim/claude-3-7-sonnet", "claude", "claude-3-7-sonnet"},
		{"mirasim/codex", "codex", ""},
		{"mirasim/gpt-4o", "codex", "gpt-4o"},
		{"mirasim/kimi", "kimi", "kimi"},
		{"mirasim/qwen", "qwen", "qwen"},
		{"mirasim/grok", "grok", "grok"},
		{"mirasim/zcode", "zcode", "zcode"},
		{"mirasim/custom-agent:submodel", "custom-agent", "submodel"},
	}

	for _, tc := range cases {
		agent, sub := MapModelToAgent(tc.input)
		if agent != tc.wantAgent || sub != tc.wantSub {
			t.Errorf("MapModelToAgent(%q) = (%q, %q), want (%q, %q)", tc.input, agent, sub, tc.wantAgent, tc.wantSub)
		}
	}
}

func TestAuthLifecycle(t *testing.T) {
	// 1. Auth Identifier
	rawId, err := handlePluginMethod(pluginabi.MethodAuthIdentifier, nil)
	if err != nil {
		t.Fatalf("handleAuthIdentifier error: %v", err)
	}
	var envId envelope
	_ = json.Unmarshal(rawId, &envId)
	if !envId.OK {
		t.Fatalf("auth.identifier failed: %+v", envId.Error)
	}

	// 2. Auth Login Start
	startReq := rpcAuthLoginStartRequest{
		AuthLoginStartRequest: pluginapi.AuthLoginStartRequest{
			Provider: "mirasim",
			BaseURL:  "http://127.0.0.1:8080/v0/management/oauth-callback",
		},
	}
	startBytes, _ := json.Marshal(startReq)
	rawStart, err := handlePluginMethod(pluginabi.MethodAuthLoginStart, startBytes)
	if err != nil {
		t.Fatalf("auth.login.start error: %v", err)
	}
	var envStart envelope
	_ = json.Unmarshal(rawStart, &envStart)
	if !envStart.OK {
		t.Fatalf("auth.login.start failed: %+v", envStart.Error)
	}

	var startResp pluginapi.AuthLoginStartResponse
	_ = json.Unmarshal(envStart.Result, &startResp)
	if startResp.State == "" {
		t.Fatalf("expected non-empty state from startLogin")
	}

	// 3. Simulate CLIProxyAPI panel writing callback file .oauth-mirasim-<state>.oauth
	tmpDir := t.TempDir()
	cbPayload := oauthCallbackFilePayload{
		Code:  "http://localhost:8080/v0/management/oauth-callback?token=my-secret-token&state=" + startResp.State,
		State: startResp.State,
	}
	cbBytes, _ := json.Marshal(cbPayload)
	waitFile := filepath.Join(tmpDir, fmt.Sprintf(".oauth-mirasim-%s.oauth", startResp.State))
	if err := os.WriteFile(waitFile, cbBytes, 0600); err != nil {
		t.Fatalf("failed to write callback file: %v", err)
	}

	// 4. Auth Login Poll
	pollReq := rpcAuthLoginPollRequest{
		AuthLoginPollRequest: pluginapi.AuthLoginPollRequest{
			Provider: "mirasim",
			State:    startResp.State,
			Host: pluginapi.HostConfigSummary{
				AuthDir: tmpDir,
			},
		},
	}
	pollBytes, _ := json.Marshal(pollReq)
	rawPoll, err := handlePluginMethod(pluginabi.MethodAuthLoginPoll, pollBytes)
	if err != nil {
		t.Fatalf("auth.login.poll error: %v", err)
	}

	var envPoll envelope
	_ = json.Unmarshal(rawPoll, &envPoll)
	if !envPoll.OK {
		t.Fatalf("auth.login.poll failed: %+v", envPoll.Error)
	}

	var pollResp pluginapi.AuthLoginPollResponse
	_ = json.Unmarshal(envPoll.Result, &pollResp)
	if pollResp.Status != pluginapi.AuthLoginStatusSuccess {
		t.Fatalf("expected status=success, got %s", pollResp.Status)
	}
	if tok := pollResp.Auth.Metadata["token"]; tok != "my-secret-token" {
		t.Fatalf("expected token='my-secret-token', got %v", tok)
	}

	// 5. Auth Parse
	parseReq := pluginapi.AuthParseRequest{
		Provider: "mirasim",
		FileName: "mirasim.json",
		RawJSON:  pollResp.Auth.StorageJSON,
	}
	parseBytes, _ := json.Marshal(parseReq)
	rawParse, err := handlePluginMethod(pluginabi.MethodAuthParse, parseBytes)
	if err != nil {
		t.Fatalf("auth.parse error: %v", err)
	}
	var envParse envelope
	_ = json.Unmarshal(rawParse, &envParse)
	if !envParse.OK {
		t.Fatalf("auth.parse failed: %+v", envParse.Error)
	}
	var parseResp pluginapi.AuthParseResponse
	_ = json.Unmarshal(envParse.Result, &parseResp)
	if !parseResp.Handled {
		t.Fatalf("expected Handled=true")
	}
	if parseResp.Auth.Metadata["token"] != "my-secret-token" {
		t.Fatalf("expected parsed token='my-secret-token', got %v", parseResp.Auth.Metadata["token"])
	}
}

func TestExtractTokenFromCode(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"my-token", "my-token"},
		{"http://localhost:8080/callback?token=tok-123&state=s", "tok-123"},
		{"http://localhost:8080/callback?code=code-456&state=s", "code-456"},
		{"token=tok-789&state=s", "tok-789"},
	}

	for _, tc := range cases {
		got := extractTokenFromCode(tc.input)
		if got != tc.want {
			t.Errorf("extractTokenFromCode(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
