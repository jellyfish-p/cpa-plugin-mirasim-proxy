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
	"github.com/router-for-me/cliproxy-plugin-mirasim/mirasim"
)

var (
	currentAuthTokenMu sync.RWMutex
	currentAuthToken   string
	currentWsURL       string
)

func SetActiveAuth(token, wsURL string) {
	currentAuthTokenMu.Lock()
	defer currentAuthTokenMu.Unlock()
	currentAuthToken = token
	currentWsURL = wsURL

	// If client exists, update its URL
	mirasimClientMu.Lock()
	if mirasimClient != nil && wsURL != "" {
		mirasimClient.UpdateURL(wsURL)
	}
	mirasimClientMu.Unlock()
}

func GetActiveAuth() (token, wsURL string) {
	currentAuthTokenMu.RLock()
	defer currentAuthTokenMu.RUnlock()
	return currentAuthToken, currentWsURL
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

	// If it's a URL or contains query string
	if strings.Contains(raw, "?") || strings.Contains(raw, "&") || strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		if u, err := url.Parse(raw); err == nil {
			q := u.Query()
			for _, k := range []string{"token", "code", "access_token", "key", "auth"} {
				if v := q.Get(k); v != "" {
					return strings.TrimSpace(v)
				}
			}
		}
		if q, err := url.ParseQuery(raw); err == nil {
			for _, k := range []string{"token", "code", "access_token", "key", "auth"} {
				if v := q.Get(k); v != "" {
					return strings.TrimSpace(v)
				}
			}
		}
	}

	return raw
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
		Type   string `json:"type"`
		Token  string `json:"token"`
		WsURL  string `json:"ws_url"`
		Port   int    `json:"port"`
		APIKey string `json:"api_key"`
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

	port := data.Port
	if port <= 0 {
		port = loadedConfig().MirasimPort
	}

	wsURL := data.WsURL
	if wsURL == "" && token != "" {
		wsURL = fmt.Sprintf("ws://127.0.0.1:%d/ws?token=%s", port, token)
	}

	if token != "" {
		SetActiveAuth(token, wsURL)
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
				"type":  pluginIdentifier,
				"token": token,
				"port":  port,
			},
		},
	}, nil
}

func handleAuthLoginStart(req pluginapi.AuthLoginStartRequest) (pluginapi.AuthLoginStartResponse, error) {
	cfg := loadedConfig()
	port := cfg.MirasimPort

	state := generateRandomState()
	baseURL := req.BaseURL

	// Authorization URL:
	// Points to Mirasim web interface or auth endpoint
	var authURL string
	if baseURL != "" {
		authURL = fmt.Sprintf("http://127.0.0.1:%d/?redirect_uri=%s&state=%s", port, url.QueryEscape(baseURL), state)
	} else {
		authURL = fmt.Sprintf("http://127.0.0.1:%d/?state=%s", port, state)
	}

	return pluginapi.AuthLoginStartResponse{
		Provider:  pluginIdentifier,
		URL:       authURL,
		State:     state,
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Metadata: map[string]any{
			"state":        state,
			"redirect_uri": baseURL,
			"port":         port,
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
	port := cfg.MirasimPort

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
					wsURL := fmt.Sprintf("ws://127.0.0.1:%d/ws?token=%s", port, token)
					SetActiveAuth(token, wsURL)

					storageData, _ := json.Marshal(map[string]any{
						"type":   pluginIdentifier,
						"token":  token,
						"ws_url": wsURL,
						"port":   port,
					})

					return pluginapi.AuthLoginPollResponse{
						Status: pluginapi.AuthLoginStatusSuccess,
						Auth: pluginapi.AuthData{
							Provider:    pluginIdentifier,
							ID:          "mirasim.json",
							FileName:    "mirasim.json",
							StorageJSON: storageData,
							Metadata: map[string]any{
								"type":  pluginIdentifier,
								"token": token,
							},
						},
					}, nil
				}
			}
		}
	}

	// 2. Check if local Mirasim instance is actively running
	instances, err := mirasim.FindActiveInstances()
	if err == nil && len(instances) > 0 {
		inst := instances[0]
		SetActiveAuth(inst.Token, inst.WsURL)

		storageData, _ := json.Marshal(map[string]any{
			"type":   pluginIdentifier,
			"token":  inst.Token,
			"ws_url": inst.WsURL,
			"port":   inst.Port,
		})

		return pluginapi.AuthLoginPollResponse{
			Status: pluginapi.AuthLoginStatusSuccess,
			Auth: pluginapi.AuthData{
				Provider:    pluginIdentifier,
				ID:          "mirasim.json",
				FileName:    "mirasim.json",
				StorageJSON: storageData,
				Metadata: map[string]any{
					"type":  pluginIdentifier,
					"token": inst.Token,
					"port":  inst.Port,
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
	// Re-verify or reload active credentials
	token, wsURL := GetActiveAuth()
	if token == "" {
		var data struct {
			Token string `json:"token"`
			WsURL string `json:"ws_url"`
		}
		if len(req.StorageJSON) > 0 {
			_ = json.Unmarshal(req.StorageJSON, &data)
			token = data.Token
			wsURL = data.WsURL
		}
	}

	storageData, _ := json.Marshal(map[string]any{
		"type":   pluginIdentifier,
		"token":  token,
		"ws_url": wsURL,
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
