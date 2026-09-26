package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
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

	pendingSessionsMu sync.Mutex
	pendingSessions   = make(map[string]string) // state -> session token

	authServerMu   sync.Mutex
	authServerAddr string
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

func recordPendingSession(state, token string) {
	token = extractTokenFromCode(token)
	if state == "" || token == "" {
		return
	}
	pendingSessionsMu.Lock()
	defer pendingSessionsMu.Unlock()
	pendingSessions[state] = token
}

func takePendingSession(state string) string {
	if state == "" {
		return ""
	}
	pendingSessionsMu.Lock()
	defer pendingSessionsMu.Unlock()
	tok, ok := pendingSessions[state]
	if ok {
		delete(pendingSessions, state)
		return tok
	}
	return ""
}

func extractTokenFromCode(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	keys := []string{
		"token", "access_token", "session", "session_token", "sessionToken",
		"mirachannelToken", "mirachannel_token", "code", "key", "auth",
	}

	// 1. If it's a URL with hash fragments (e.g. #token=... or #access_token=...)
	if strings.Contains(raw, "#") {
		parts := strings.SplitN(raw, "#", 2)
		if len(parts) == 2 {
			if q, err := url.ParseQuery(parts[1]); err == nil {
				for _, k := range keys {
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
			for _, k := range keys {
				if v := q.Get(k); v != "" {
					return strings.TrimSpace(v)
				}
			}
		}
		if q, err := url.ParseQuery(raw); err == nil {
			for _, k := range keys {
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
	if prov != "" && prov != pluginIdentifier && prov != "mirasim" {
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
	if prov != pluginIdentifier && prov != "mirasim" && data.Type != pluginIdentifier && data.Type != "mirasim" && !strings.Contains(strings.ToLower(req.FileName), "mirasim") {
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

func ensureAuthServerStarted() (string, error) {
	authServerMu.Lock()
	defer authServerMu.Unlock()

	if authServerAddr != "" {
		return authServerAddr, nil
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("failed to start local auth listener: %w", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port
	authServerAddr = fmt.Sprintf("http://127.0.0.1:%d", port)

	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleAuthSubmitHTTP(w, r)
			return
		}
		state := r.URL.Query().Get("state")
		redirectURI := r.URL.Query().Get("redirect_uri")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(renderAuthHTML(state, redirectURI)))
	})
	mux.HandleFunc("/auth/submit", func(w http.ResponseWriter, r *http.Request) {
		handleAuthSubmitHTTP(w, r)
	})

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		_ = srv.Serve(ln)
	}()

	return authServerAddr, nil
}

func handleAuthSubmitHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	var payload struct {
		State string `json:"state"`
		Token string `json:"token"`
	}

	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		_ = json.NewDecoder(r.Body).Decode(&payload)
	} else {
		_ = r.ParseForm()
		payload.State = r.FormValue("state")
		payload.Token = r.FormValue("token")
		if payload.Token == "" {
			_ = json.NewDecoder(r.Body).Decode(&payload)
		}
	}

	if payload.State == "" {
		payload.State = r.URL.Query().Get("state")
	}
	if payload.Token == "" {
		payload.Token = r.URL.Query().Get("token")
		if payload.Token == "" {
			payload.Token = r.URL.Query().Get("session")
		}
	}

	token := extractTokenFromCode(payload.Token)
	if payload.State == "" || token == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "state and token are required"})
		return
	}

	recordPendingSession(payload.State, token)
	SetActiveAuth(token, resolveRelayBaseURL(loadedConfig()))

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "session recorded successfully"})
}

func handleManagementRequest(req pluginapi.ManagementRequest) pluginapi.ManagementResponse {
	path := strings.TrimSpace(req.Path)
	if strings.HasSuffix(path, "/submit") || req.Method == http.MethodPost {
		var payload struct {
			State string `json:"state"`
			Token string `json:"token"`
		}
		_ = json.Unmarshal(req.Body, &payload)
		if payload.State == "" {
			payload.State = req.Query.Get("state")
		}
		if payload.Token == "" {
			payload.Token = req.Query.Get("token")
			if payload.Token == "" {
				payload.Token = req.Query.Get("session")
			}
		}
		token := extractTokenFromCode(payload.Token)
		if payload.State != "" && token != "" {
			recordPendingSession(payload.State, token)
			SetActiveAuth(token, resolveRelayBaseURL(loadedConfig()))
			return pluginapi.ManagementResponse{
				StatusCode: http.StatusOK,
				Headers:    http.Header{"Content-Type": []string{"application/json"}},
				Body:       []byte(`{"ok":true,"message":"session recorded successfully"}`),
			}
		}
		return pluginapi.ManagementResponse{
			StatusCode: http.StatusBadRequest,
			Headers:    http.Header{"Content-Type": []string{"application/json"}},
			Body:       []byte(`{"ok":false,"error":"state and token required"}`),
		}
	}

	state := req.Query.Get("state")
	redirectURI := req.Query.Get("redirect_uri")
	htmlContent := renderAuthHTML(state, redirectURI)

	return pluginapi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:       []byte(htmlContent),
	}
}

func renderAuthHTML(state, redirectURI string) string {
	if redirectURI == "" {
		redirectURI = "http://127.0.0.1:8080/v0/management/oauth-callback"
	}
	githubURL := fmt.Sprintf("https://auth.mirasim.ai/auth/oauth/github/login?redirect_uri=%s&state=%s", url.QueryEscape(redirectURI), state)
	googleURL := fmt.Sprintf("https://auth.mirasim.ai/auth/oauth/google/login?redirect_uri=%s&state=%s", url.QueryEscape(redirectURI), state)

	stateJSON, _ := json.Marshal(state)
	redirectURIJSON, _ := json.Marshal(redirectURI)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Mirasim 授权登录</title>
<style>
  :root {
    --bg: #0d1117;
    --card-bg: #161b22;
    --border: #30363d;
    --text: #c9d1d9;
    --text-muted: #8b949e;
    --primary: #58a6ff;
    --primary-hover: #388bfd;
    --btn-primary-bg: #238636;
    --btn-primary-hover: #2ea043;
    --gh-bg: #21262d;
    --gh-hover: #30363d;
    --google-bg: #ffffff;
    --google-text: #1f2937;
    --google-hover: #f3f4f6;
    --success: #3fb950;
    --error: #f85149;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    background: var(--bg);
    color: var(--text);
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
    display: flex;
    justify-content: center;
    align-items: center;
    min-height: 100vh;
    padding: 24px 16px;
  }
  .card {
    background: var(--card-bg);
    border: 1px solid var(--border);
    border-radius: 12px;
    width: 100%%;
    max-width: 500px;
    padding: 32px 28px;
    box-shadow: 0 10px 30px rgba(0, 0, 0, 0.4);
  }
  .header { text-align: center; margin-bottom: 24px; }
  .logo-icon {
    width: 48px;
    height: 48px;
    background: linear-gradient(135deg, #3b82f6, #8b5cf6);
    border-radius: 12px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    margin-bottom: 12px;
    color: #fff;
    font-size: 24px;
    font-weight: bold;
    box-shadow: 0 4px 12px rgba(59, 130, 246, 0.3);
  }
  .title { font-size: 20px; font-weight: 700; color: #f0f6fc; margin-bottom: 6px; }
  .subtitle { font-size: 13px; color: var(--text-muted); line-height: 1.5; }
  .btn-group { display: flex; flex-direction: column; gap: 12px; margin-bottom: 20px; }
  .btn {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    width: 100%%;
    padding: 12px 16px;
    border-radius: 8px;
    font-size: 14px;
    font-weight: 600;
    text-decoration: none;
    cursor: pointer;
    transition: all 0.15s ease;
    border: none;
  }
  .btn-gh { background: var(--gh-bg); color: #c9d1d9; border: 1px solid var(--border); }
  .btn-gh:hover { background: var(--gh-hover); color: #fff; border-color: #8b949e; }
  .btn-google { background: var(--google-bg); color: var(--google-text); }
  .btn-google:hover { background: var(--google-hover); }
  .btn-primary { background: var(--btn-primary-bg); color: #fff; margin-top: 10px; }
  .btn-primary:hover { background: var(--btn-primary-hover); }
  .btn-secondary {
    background: #21262d;
    color: #c9d1d9;
    border: 1px solid var(--border);
    padding: 7px 12px;
    font-size: 12px;
    margin-top: 8px;
  }
  .btn-secondary:hover { background: #30363d; color: #fff; }
  .divider {
    display: flex;
    align-items: center;
    text-align: center;
    margin: 22px 0;
    color: var(--text-muted);
    font-size: 12px;
  }
  .divider::before, .divider::after {
    content: '';
    flex: 1;
    border-bottom: 1px solid var(--border);
  }
  .divider span { padding: 0 12px; font-weight: 500; }
  .section-session {
    background: rgba(255, 255, 255, 0.02);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 18px 16px;
  }
  .session-header {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 14px;
    font-weight: 600;
    color: #f0f6fc;
    margin-bottom: 6px;
  }
  .session-desc {
    font-size: 12px;
    color: var(--text-muted);
    margin-bottom: 12px;
    line-height: 1.45;
  }
  .input-text {
    width: 100%%;
    padding: 10px 12px;
    background: #0d1117;
    border: 1px solid var(--border);
    border-radius: 6px;
    color: #f0f6fc;
    font-size: 13px;
    outline: none;
    font-family: ui-monospace, SFMono-Regular, SF Mono, Menlo, monospace;
    transition: border-color 0.15s ease;
  }
  .input-text:focus { border-color: var(--primary); box-shadow: 0 0 0 3px rgba(88, 166, 255, 0.2); }
  .msg {
    margin-top: 12px;
    padding: 10px 12px;
    border-radius: 6px;
    font-size: 13px;
    line-height: 1.45;
  }
  .msg-error { background: rgba(248, 81, 73, 0.15); border: 1px solid var(--error); color: #ff7b72; }
  .msg-success { background: rgba(63, 185, 80, 0.15); border: 1px solid var(--success); color: #7ee787; }
  .msg-info { background: rgba(88, 166, 255, 0.15); border: 1px solid var(--primary); color: #79c0ff; }
  .callback-box {
    margin-top: 14px;
    padding: 12px;
    background: #0d1117;
    border: 1px solid var(--border);
    border-radius: 6px;
    font-size: 12px;
  }
  .callback-box label { display: block; color: var(--text-muted); margin-bottom: 6px; font-weight: 500; }
  .footer-tip {
    margin-top: 20px;
    text-align: center;
    font-size: 11px;
    color: var(--text-muted);
    line-height: 1.5;
  }
</style>
</head>
<body>
  <div class="card">
    <div class="header">
      <div class="logo-icon">M</div>
      <h1 class="title">Mirasim 授权登录</h1>
      <p class="subtitle">请选择第三方 OAuth 账号登录，或直接输入现有 Session 凭据进行绑定。</p>
    </div>

    <div class="btn-group">
      <!-- 选项 1: GitHub 登录 -->
      <a href="%s" class="btn btn-gh">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/>
        </svg>
        <span>使用 GitHub 账号登录</span>
      </a>

      <!-- 选项 2: Google 登录 -->
      <a href="%s" class="btn btn-google">
        <svg width="20" height="20" viewBox="0 0 24 24">
          <path fill="#4285F4" d="M23.745 12.27c0-.7-.06-1.4-.19-2.07H12v4.51h6.6c-.29 1.52-1.14 2.8-2.4 3.65v3.05h3.88c2.27-2.09 3.665-5.17 3.665-9.14z"/>
          <path fill="#34A853" d="M12 24c3.24 0 5.95-1.08 7.93-2.91l-3.88-3.05c-1.08.72-2.45 1.16-4.05 1.16-3.12 0-5.77-2.1-6.72-4.93H1.26v3.15C3.25 21.37 7.34 24 12 24z"/>
          <path fill="#FBBC05" d="M5.28 14.27c-.25-.72-.38-1.49-.38-2.27s.13-1.55.38-2.27V6.58H1.26C.46 8.16 0 9.98 0 12s.46 3.84 1.26 5.42l4.02-3.15z"/>
          <path fill="#EA4335" d="M12 4.75c1.77 0 3.35.61 4.6 1.8l3.42-3.42C17.95 1.19 15.24 0 12 0 7.34 0 3.25 2.63 1.26 6.58l4.02 3.15c.95-2.83 3.6-4.98 6.72-4.98z"/>
        </svg>
        <span>使用 Google 账号登录</span>
      </a>
    </div>

    <div class="divider">
      <span>或直接输入 Session (网页兼容模式)</span>
    </div>

    <!-- 选项 3: 直接输入 Session -->
    <div class="section-session">
      <div class="session-header">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="7.5" cy="15.5" r="5.5"></circle>
          <path d="m21 2-9.6 9.6"></path>
          <path d="m15.5 7.5 3 3L22 7l-3-3"></path>
        </svg>
        <span>输入 Session Token / 访问令牌</span>
      </div>
      <p class="session-desc">适用于远程部署、容器或网络受限环境。在此填入已有的 Mirasim Token（JWT Token、mirachannelToken 或完整回调链接）。</p>
      <input type="text" id="session-input" placeholder="输入 JWT Token (eyJ...) 或 mirachannelToken" class="input-text" autocomplete="off" />
      <button type="button" class="btn btn-primary" onclick="submitSession()">提交并授权</button>
      <div id="msg" class="msg" style="display:none;"></div>
      <div id="callback-box" class="callback-box" style="display:none;">
        <label>重定向回调地址 (若未能自动跳转，可复制粘贴至管理控制台):</label>
        <input type="text" id="callback-url" class="input-text" readonly />
        <button type="button" class="btn btn-secondary" onclick="copyCallback()">复制地址</button>
      </div>
    </div>

    <div class="footer-tip">
      授权完成后将自动完成绑定并保存至配置文件。<br>
      若在远程管理面板中使用，可复制重定向地址后在控制台提交回填。
    </div>
  </div>

  <script>
    const state = %s;
    const redirectUri = %s;

    async function submitSession() {
      const input = document.getElementById('session-input');
      const val = input.value.trim();
      const msg = document.getElementById('msg');
      if (!val) {
        msg.className = 'msg msg-error';
        msg.innerText = '请输入 Session Token！';
        msg.style.display = 'block';
        return;
      }
      msg.className = 'msg msg-info';
      msg.innerText = '正在提交并完成授权...';
      msg.style.display = 'block';

      // 1. Post to local auth server / resource route
      let submitUrl = window.location.pathname;
      if (!submitUrl.endsWith('/submit')) {
        submitUrl = submitUrl.replace(/\/+$/, '') + '/submit';
      }
      try {
        await fetch(submitUrl, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ state: state, token: val })
        });
      } catch (e) {
        console.warn('Local submit fetch warning:', e);
      }

      // 2. Build callback redirect URL
      let callbackURL = '';
      if (redirectUri) {
        try {
          const u = new URL(redirectUri, window.location.origin);
          u.searchParams.set('token', val);
          if (state) u.searchParams.set('state', state);
          callbackURL = u.toString();
        } catch (e) {
          const sep = redirectUri.includes('?') ? '&' : '?';
          callbackURL = redirectUri + sep + 'token=' + encodeURIComponent(val) + (state ? '&state=' + encodeURIComponent(state) : '');
        }
      }

      msg.className = 'msg msg-success';
      msg.innerText = '✓ 凭据已提交成功！';

      if (callbackURL) {
        document.getElementById('callback-box').style.display = 'block';
        document.getElementById('callback-url').value = callbackURL;
        setTimeout(() => {
          window.location.href = callbackURL;
        }, 600);
      }
    }

    function copyCallback() {
      const copyText = document.getElementById('callback-url');
      copyText.select();
      navigator.clipboard.writeText(copyText.value).then(() => {
        alert('回调地址已复制到剪贴板！');
      }).catch(() => {
        document.execCommand('copy');
        alert('回调地址已复制到剪贴板！');
      });
    }
  </script>
</body>
</html>`,
		html.EscapeString(githubURL),
		html.EscapeString(googleURL),
		string(stateJSON),
		string(redirectURIJSON),
	)
}

func handleAuthLoginStart(req pluginapi.AuthLoginStartRequest) (pluginapi.AuthLoginStartResponse, error) {
	state := generateRandomState()
	baseURL := req.BaseURL

	cfg := loadedConfig()
	provider := strings.ToLower(strings.TrimSpace(cfg.OAuthProvider))

	// Check metadata overrides (e.g. ?provider=github or ?oauth_provider=google)
	if req.Metadata != nil {
		if p, ok := req.Metadata["oauth_provider"].(string); ok && strings.TrimSpace(p) != "" {
			provider = strings.ToLower(strings.TrimSpace(p))
		} else if p, ok := req.Metadata["provider"].(string); ok && strings.TrimSpace(p) != "" {
			provider = strings.ToLower(strings.TrimSpace(p))
		}
	}

	var authURL string

	switch provider {
	case "github":
		if baseURL != "" {
			authURL = fmt.Sprintf("https://auth.mirasim.ai/auth/oauth/github/login?redirect_uri=%s&state=%s", url.QueryEscape(baseURL), state)
		} else {
			authURL = fmt.Sprintf("https://auth.mirasim.ai/auth/oauth/github/login?state=%s", state)
		}
	case "google":
		if baseURL != "" {
			authURL = fmt.Sprintf("https://auth.mirasim.ai/auth/oauth/google/login?redirect_uri=%s&state=%s", url.QueryEscape(baseURL), state)
		} else {
			authURL = fmt.Sprintf("https://auth.mirasim.ai/auth/oauth/google/login?state=%s", state)
		}
	default:
		// Default "all" / "web" / "page" / interactive selection page:
		// Offers GitHub, Google, and direct Session Token input
		redirectURI := baseURL
		if redirectURI == "" {
			redirectURI = "http://127.0.0.1:8080/v0/management/oauth-callback"
		}

		isRemoteHost := false
		if baseURL != "" {
			if u, errParse := url.Parse(baseURL); errParse == nil && u.Host != "" {
				h := u.Hostname()
				if h != "127.0.0.1" && h != "localhost" && h != "::1" && h != "" {
					isRemoteHost = true
					authURL = fmt.Sprintf("%s://%s/v0/resource/plugins/%s/auth?state=%s&redirect_uri=%s",
						u.Scheme, u.Host, pluginIdentifier, state, url.QueryEscape(redirectURI))
				}
			}
		}

		if !isRemoteHost {
			localAddr, err := ensureAuthServerStarted()
			if err == nil && localAddr != "" {
				authURL = fmt.Sprintf("%s/auth?state=%s&redirect_uri=%s", localAddr, state, url.QueryEscape(redirectURI))
			} else if baseURL != "" {
				if u, errParse := url.Parse(baseURL); errParse == nil && u.Host != "" {
					authURL = fmt.Sprintf("%s://%s/v0/resource/plugins/%s/auth?state=%s&redirect_uri=%s",
						u.Scheme, u.Host, pluginIdentifier, state, url.QueryEscape(redirectURI))
				} else {
					authURL = fmt.Sprintf("https://auth.mirasim.ai/auth/oauth/github/login?redirect_uri=%s&state=%s", url.QueryEscape(redirectURI), state)
				}
			} else {
				authURL = fmt.Sprintf("https://auth.mirasim.ai/auth/oauth/github/login?state=%s", state)
			}
		}
	}

	return pluginapi.AuthLoginStartResponse{
		Provider:  pluginIdentifier,
		URL:       authURL,
		State:     state,
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Metadata: map[string]any{
			"state":          state,
			"redirect_uri":   baseURL,
			"oauth_provider": provider,
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

	// 0. Check in-memory pending session (submitted directly via web page)
	if tok := takePendingSession(req.State); tok != "" {
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
				ID:          "mirasim-proxy.json",
				FileName:    "mirasim-proxy.json",
				StorageJSON: storageData,
				Metadata: map[string]any{
					"type":      pluginIdentifier,
					"token":     tok,
					"relay_url": relayURL,
				},
			},
		}, nil
	}

	// 1. Check for callback file written by CLIProxyAPI when user pastes callback URL in WebUI panel
	authDir := req.Host.AuthDir
	if authDir != "" && req.State != "" {
		waitFiles := []string{
			filepath.Join(authDir, fmt.Sprintf(".oauth-%s-%s.oauth", pluginIdentifier, req.State)),
			filepath.Join(authDir, fmt.Sprintf(".oauth-mirasim-%s.oauth", req.State)),
		}
		for _, waitFile := range waitFiles {
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
								ID:          "mirasim-proxy.json",
								FileName:    "mirasim-proxy.json",
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
