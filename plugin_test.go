package main

import (
	"encoding/json"
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
}

func TestModelStatic(t *testing.T) {
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

	foundMirasim := false
	for _, m := range models {
		if m.ID == "mirasim" {
			foundMirasim = true
			break
		}
	}
	if !foundMirasim {
		t.Errorf("expected to find 'mirasim' in models")
	}
}

func TestExecutorIdentifier(t *testing.T) {
	raw, err := handlePluginMethod(pluginabi.MethodExecutorIdentifier, nil)
	if err != nil {
		t.Fatalf("handlePluginMethod(executor.identifier) error: %v", err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok=true, got error: %+v", env.Error)
	}

	var res map[string]string
	if err := json.Unmarshal(env.Result, &res); err != nil {
		t.Fatalf("unmarshal identifier: %v", err)
	}

	if res["identifier"] != "mirasim" {
		t.Errorf("expected identifier = mirasim, got %s", res["identifier"])
	}
}

func TestConvertMessages(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "system", Content: "Instruction"},
		{Role: "user", Content: "Question"},
	}
	res := ConvertMessagesToPrompt(msgs)
	if res == "" {
		t.Errorf("expected non-empty prompt")
	}
}
