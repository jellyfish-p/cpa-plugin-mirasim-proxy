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

	if reg.Metadata.Name != "mirasim-proxy" {
		t.Errorf("expected metadata.Name = mirasim-proxy, got %s", reg.Metadata.Name)
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

func TestModelStaticNoHarnessExposed(t *testing.T) {
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

	// Harness execution runners that must NEVER be exposed as model IDs
	harnessNames := []string{
		"mirasim-proxy/pi",
		"mirasim-proxy/claude",
		"mirasim-proxy/codex",
		"mirasim-proxy/dsh",
		"mirasim-proxy/antigravity",
		"mirasim-proxy/qwen",
		"mirasim-proxy/zcode",
		"mirasim-proxy/grok",
		"mirasim-proxy/echo",
		"mirasim-proxy/default",
	}

	for _, m := range models {
		if !strings.HasPrefix(m.ID, "mirasim-proxy/") {
			t.Errorf("model %q does not have 'mirasim-proxy/' prefix", m.ID)
		}

		for _, hn := range harnessNames {
			if m.ID == hn {
				t.Errorf("harness runner %q is incorrectly exposed as a model ID", m.ID)
			}
		}
	}

	// Test model.for_auth as well
	rawAuth, err := handlePluginMethod(pluginabi.MethodModelForAuth, nil)
	if err != nil {
		t.Fatalf("handlePluginMethod(model.for_auth) error: %v", err)
	}
	var envAuth envelope
	_ = json.Unmarshal(rawAuth, &envAuth)
	if !envAuth.OK {
		t.Fatalf("model.for_auth failed: %+v", envAuth.Error)
	}
	var respAuth pluginapi.ModelResponse
	_ = json.Unmarshal(envAuth.Result, &respAuth)
	if len(respAuth.Models) == 0 {
		t.Fatalf("expected non-empty models from model.for_auth")
	}
}

func TestCatalogValidationAndDynamicUpdate(t *testing.T) {
	initialRevision := GetCatalogRevision()
	if initialRevision == 0 {
		t.Fatalf("expected non-zero initial revision")
	}

	customCatalog := []byte(`{
		"models": [
			{
				"slug": "custom-experimental-model",
				"display_name": "Custom Experimental Model",
				"context_window": 128000,
				"owned_by": "custom"
			},
			{
				"slug": "claude-3-7-sonnet",
				"display_name": "Claude 3.7 Sonnet (Thinking)",
				"context_window": 200000,
				"owned_by": "anthropic"
			}
		]
	}`)

	models, err := ValidateCatalogJSON(customCatalog)
	if err != nil {
		t.Fatalf("ValidateCatalogJSON failed: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	err = loadCatalogFromBytes(customCatalog, "test")
	if err != nil {
		t.Fatalf("loadCatalogFromBytes failed: %v", err)
	}

	newRevision := GetCatalogRevision()
	if newRevision <= initialRevision {
		t.Errorf("expected revision to increase from %d, got %d", initialRevision, newRevision)
	}

	current := GetModelsCatalog()
	foundCustom := false
	for _, m := range current {
		if m.ID == "mirasim-proxy/custom-experimental-model" {
			foundCustom = true
			if m.DisplayName != "Custom Experimental Model" {
				t.Errorf("expected display name 'Custom Experimental Model', got %s", m.DisplayName)
			}
		}
	}
	if !foundCustom {
		t.Errorf("custom-experimental-model not found in updated catalog")
	}

	// Restore embedded models
	_ = loadCatalogFromBytes(embeddedModelsJSON, "restore")
}

func TestTranslateOpenAIToClaude(t *testing.T) {
	openaiPayload := []byte(`{
		"model": "mirasim/claude-3-7-sonnet",
		"messages": [
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "Hello world!"}
		],
		"max_tokens": 1024,
		"stream": true
	}`)

	claudeBody, err := TranslateOpenAIToClaude(openaiPayload, "claude-3-7-sonnet")
	if err != nil {
		t.Fatalf("TranslateOpenAIToClaude error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(claudeBody, &parsed); err != nil {
		t.Fatalf("unmarshal translated body: %v", err)
	}

	if parsed["model"] != "claude-3-7-sonnet" {
		t.Errorf("expected model=claude-3-7-sonnet, got %v", parsed["model"])
	}
	if parsed["system"] != "You are a helpful assistant." {
		t.Errorf("expected system='You are a helpful assistant.', got %v", parsed["system"])
	}

	msgs, ok := parsed["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("expected 1 user message, got %v", parsed["messages"])
	}
	userMsg := msgs[0].(map[string]any)
	if userMsg["role"] != "user" || userMsg["content"] != "Hello world!" {
		t.Errorf("unexpected user message: %+v", userMsg)
	}
}

func TestTranslateClaudeToOpenAIResponse(t *testing.T) {
	claudeResp := []byte(`{
		"id": "msg_01XyZ",
		"type": "message",
		"role": "assistant",
		"content": [
			{"type": "thinking", "thinking": "Thinking step..."},
			{"type": "text", "text": "Final answer here."}
		],
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 25,
			"output_tokens": 15
		}
	}`)

	openAIBytes, err := TranslateClaudeToOpenAIResponse(claudeResp, "claude-3-7-sonnet")
	if err != nil {
		t.Fatalf("TranslateClaudeToOpenAIResponse error: %v", err)
	}

	var resp ChatCompletionResponse
	if err := json.Unmarshal(openAIBytes, &resp); err != nil {
		t.Fatalf("unmarshal openai response: %v", err)
	}

	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	c := resp.Choices[0]
	if c.Message.Content != "Final answer here." {
		t.Errorf("content mismatch: %s", c.Message.Content)
	}
	if c.Message.ReasoningContent != "Thinking step..." {
		t.Errorf("reasoning content mismatch: %s", c.Message.ReasoningContent)
	}
	if resp.Usage.PromptTokens != 25 || resp.Usage.CompletionTokens != 15 {
		t.Errorf("usage mismatch: %+v", resp.Usage)
	}
}

func TestTranslateClaudeStream(t *testing.T) {
	state := NewStreamState("claude-3-7-sonnet")

	// 1. message_start
	c1 := TranslateClaudeEventToOpenAIChunks("message_start", []byte(`{"type":"message_start","message":{"id":"msg_123"}}`), state)
	if len(c1) == 0 {
		t.Fatalf("expected chunk for message_start")
	}

	// 2. thinking delta
	c2 := TranslateClaudeEventToOpenAIChunks("content_block_delta", []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"let me think"}}`), state)
	if len(c2) == 0 {
		t.Fatalf("expected chunk for thinking_delta")
	}
	if !strings.Contains(string(c2[0]), "reasoning_content") || !strings.Contains(string(c2[0]), "let me think") {
		t.Errorf("expected reasoning_content chunk, got %s", string(c2[0]))
	}

	// 3. text delta
	c3 := TranslateClaudeEventToOpenAIChunks("content_block_delta", []byte(`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"hello user"}}`), state)
	if len(c3) == 0 {
		t.Fatalf("expected chunk for text_delta")
	}
	if !strings.Contains(string(c3[0]), "hello user") {
		t.Errorf("expected text chunk, got %s", string(c3[0]))
	}

	// 4. message_delta
	c4 := TranslateClaudeEventToOpenAIChunks("message_delta", []byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":42}}`), state)
	if len(c4) == 0 {
		t.Fatalf("expected chunk for message_delta")
	}
	if !strings.Contains(string(c4[0]), `"finish_reason":"stop"`) {
		t.Errorf("expected finish_reason=stop, got %s", string(c4[0]))
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

	// 3. Simulate CLIProxyAPI panel receiving pasted redirect URL and writing .oauth-mirasim-<state>.oauth
	tmpDir := t.TempDir()
	cbPayload := oauthCallbackFilePayload{
		Code:  "http://localhost:8080/v0/management/oauth-callback?code=my-secret-token&state=" + startResp.State,
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
		{"http://localhost:8080/callback?code=code-123&state=s", "code-123"},
		{"http://localhost:8080/callback?token=tok-456&state=s", "tok-456"},
		{"http://localhost:4939/#token=hash-tok-789&state=s", "hash-tok-789"},
		{"code=code-999&state=s", "code-999"},
	}

	for _, tc := range cases {
		got := extractTokenFromCode(tc.input)
		if got != tc.want {
			t.Errorf("extractTokenFromCode(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
