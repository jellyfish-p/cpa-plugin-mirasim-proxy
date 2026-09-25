package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

var (
	currentAuthTokenMu sync.RWMutex
	currentAuthToken   string
	currentRelayURL    string
)

func SetActiveAuth(token, relayURL string) {
	currentAuthTokenMu.Lock()
	defer currentAuthTokenMu.Unlock()
	currentAuthToken = token
	currentRelayURL = relayURL
}

func GetActiveAuth() (token, relayURL string) {
	currentAuthTokenMu.RLock()
	defer currentAuthTokenMu.RUnlock()
	return currentAuthToken, currentRelayURL
}

func generateRandomState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func extractTokenFromCode(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// 1. If it's a URL with hash fragments (e.g. #token=... or #access_token=...)
	if strings.Contains(raw, "#") {
		parts := strings.SplitN(raw, "#", 2)
		if len(parts) == 2 {
			if q, err := url.ParseQuery(parts[1]); err == nil {
				for _, k := range []string{"token", "code", "access_token", "key", "auth"} {
					if v := q.Get(k); v != "" {
						return strings.TrimSpace(v)
					}
				}
			}
		}
	}

	// 2. If it's a URL or contains query parameters
	if strings.Contains(raw, "?") || strings.Contains(raw, "&") || strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		if u, err := url.Parse(raw); err == nil {
			q := u.Query()
			for _, k := range []string{"code", "token", "access_token", "key", "auth"} {
				if v := q.Get(k); v != "" {
					return strings.TrimSpace(v)
				}
			}
		}
		if q, err := url.ParseQuery(raw); err == nil {
			for _, k := range []string{"code", "token", "access_token", "key", "auth"} {
				if v := q.Get(k); v != "" {
					return strings.TrimSpace(v)
				}
			}
		}
	}

	// 3. Raw token string
	return raw
}

// discoverLocalToken attempts to read an active token from ~/.mirasim/ setting or run files without spawning processes.
func discoverLocalToken() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// 1. Try ~/.mirasim/setting.json
	settingPath := filepath.Join(home, ".mirasim", "setting.json")
	if data, err := os.ReadFile(settingPath); err == nil {
		var setting struct {
			MirachannelToken string `json:"mirachannelToken"`
		}
		if err := json.Unmarshal(data, &setting); err == nil && setting.MirachannelToken != "" {
			return setting.MirachannelToken
		}
	}

	// 2. Try ~/.mirasim/run/local-*.token
	runDir := filepath.Join(home, ".mirasim", "run")
	if entries, err := os.ReadDir(runDir); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "local-") && strings.HasSuffix(e.Name(), ".token") {
				if tokBytes, err := os.ReadFile(filepath.Join(runDir, e.Name())); err == nil {
					tok := strings.TrimSpace(string(tokBytes))
					if tok != "" {
						return tok
					}
				}
			}
		}
	}

	return ""
}

func handleAuthIdentifier() any {
	return map[string]string{"identifier": pluginIdentifier}
}

func handleAuthParse(req pluginapi.AuthParseRequest) (pluginapi.AuthParseResponse, error) {
	prov := strings.ToLower(strings.TrimSpace(req.Provider))
	if prov != "" && prov != pluginIdentifier {
		return pluginapi.AuthParseResponse{Handled: false}, nil
	}

	var data struct {
		Type     string `json:"type"`
		Token    string `json:"token"`
		RelayURL string `json:"relay_url"`
		APIKey   string `json:"api_key"`
	}

	if len(req.RawJSON) > 0 {
		if err := json.Unmarshal(req.RawJSON, &data); err != nil {
			return pluginapi.AuthParseResponse{Handled: false}, nil
		}
	}

	// Verify if this auth file belongs to Mirasim
	if prov != pluginIdentifier && data.Type != pluginIdentifier && !strings.Contains(strings.ToLower(req.FileName), "mirasim") {
		return pluginapi.AuthParseResponse{Handled: false}, nil
	}

	token := data.Token
	if token == "" {
		token = data.APIKey
	}

	relayURL := data.RelayURL
	if relayURL == "" {
		relayURL = resolveRelayBaseURL(loadedConfig())
	}

	if token != "" {
		SetActiveAuth(token, relayURL)
	}

	id := req.FileName
	if id == "" {
		id = "mirasim.json"
	}

	return pluginapi.AuthParseResponse{
		Handled: true,
		Auth: pluginapi.AuthData{
			Provider:    pluginIdentifier,
			ID:          id,
			FileName:    id,
			StorageJSON: req.RawJSON,
			Metadata: map[string]any{
				"type":      pluginIdentifier,
				"token":     token,
				"relay_url": relayURL,
			},
		},
	}, nil
}

func handleAuthLoginStart(req pluginapi.AuthLoginStartRequest) (pluginapi.AuthLoginStartResponse, error) {
	state := generateRandomState()
	baseURL := req.BaseURL

	// Authorization URL:
	// Points to Mirasim web auth or login gateway
	var authURL string
	if baseURL != "" {
		authURL = fmt.Sprintf("https://auth.mirasim.ai/?redirect_uri=%s&state=%s", url.QueryEscape(baseURL), state)
	} else {
		authURL = fmt.Sprintf("https://auth.mirasim.ai/?state=%s", state)
	}

	return pluginapi.AuthLoginStartResponse{
		Provider:  pluginIdentifier,
		URL:       authURL,
		State:     state,
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Metadata: map[string]any{
			"state":        state,
			"redirect_uri": baseURL,
		},
	}, nil
}

type oauthCallbackFilePayload struct {
	Code  string `json:"code"`
	State string `json:"state"`
	Error string `json:"error"`
}

func handleAuthLoginPoll(ctx context.Context, req pluginapi.AuthLoginPollRequest) (pluginapi.AuthLoginPollResponse, error) {
	cfg := loadedConfig()
	relayURL := resolveRelayBaseURL(cfg)

	// 1. Check for callback file written by CLIProxyAPI when user pastes callback URL in WebUI panel
	authDir := req.Host.AuthDir
	if authDir != "" && req.State != "" {
		waitFile := filepath.Join(authDir, fmt.Sprintf(".oauth-%s-%s.oauth", pluginIdentifier, req.State))
		if data, err := os.ReadFile(waitFile); err == nil {
			_ = os.Remove(waitFile)

			var cb oauthCallbackFilePayload
			if errDecode := json.Unmarshal(data, &cb); errDecode == nil {
				if cb.Error != "" {
					return pluginapi.AuthLoginPollResponse{
						Status:  pluginapi.AuthLoginStatusError,
						Message: fmt.Sprintf("authorization failed: %s", cb.Error),
					}, nil
				}

				token := extractTokenFromCode(cb.Code)
				if token != "" {
					SetActiveAuth(token, relayURL)

					storageData, _ := json.Marshal(map[string]any{
						"type":      pluginIdentifier,
						"token":     token,
						"relay_url": relayURL,
					})

					return pluginapi.AuthLoginPollResponse{
						Status: pluginapi.AuthLoginStatusSuccess,
						Auth: pluginapi.AuthData{
							Provider:    pluginIdentifier,
							ID:          "mirasim.json",
							FileName:    "mirasim.json",
							StorageJSON: storageData,
							Metadata: map[string]any{
								"type":      pluginIdentifier,
								"token":     token,
								"relay_url": relayURL,
							},
						},
					}, nil
				}
			}
		}
	}

	// 2. Check if local token is discoverable from settings
	if tok := discoverLocalToken(); tok != "" {
		SetActiveAuth(tok, relayURL)

		storageData, _ := json.Marshal(map[string]any{
			"type":      pluginIdentifier,
			"token":     tok,
			"relay_url": relayURL,
		})

		return pluginapi.AuthLoginPollResponse{
			Status: pluginapi.AuthLoginStatusSuccess,
			Auth: pluginapi.AuthData{
				Provider:    pluginIdentifier,
				ID:          "mirasim.json",
				FileName:    "mirasim.json",
				StorageJSON: storageData,
				Metadata: map[string]any{
					"type":      pluginIdentifier,
					"token":     tok,
					"relay_url": relayURL,
				},
			},
		}, nil
	}

	// 3. Otherwise still waiting for user callback or token
	return pluginapi.AuthLoginPollResponse{
		Status: pluginapi.AuthLoginStatusPending,
	}, nil
}

func handleAuthRefresh(req pluginapi.AuthRefreshRequest) (pluginapi.AuthRefreshResponse, error) {
	token, relayURL := GetActiveAuth()
	if token == "" {
		var data struct {
			Token    string `json:"token"`
			RelayURL string `json:"relay_url"`
		}
		if len(req.StorageJSON) > 0 {
			_ = json.Unmarshal(req.StorageJSON, &data)
			token = data.Token
			relayURL = data.RelayURL
		}
	}

	storageData, _ := json.Marshal(map[string]any{
		"type":      pluginIdentifier,
		"token":     token,
		"relay_url": relayURL,
	})

	return pluginapi.AuthRefreshResponse{
		Auth: pluginapi.AuthData{
			Provider:    pluginIdentifier,
			ID:          req.AuthID,
			FileName:    req.AuthID,
			StorageJSON: storageData,
			Metadata:    req.Metadata,
		},
		NextRefreshAfter: time.Now().Add(24 * time.Hour),
	}, nil
}
