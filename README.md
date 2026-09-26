# CLIProxyAPI Plugin: Mirasim Proxy (`cpa-plugin-mirasim-proxy`)

Mirasim 在 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 平台上的纯净直传 Provider 插件（遵循官方 `pluginabi.ABIVersion` 与 `SchemaVersion` 标准）。

---

## 架构核心原则：纯粹 API 直连与报文翻译，彻底杜绝本地 Agent 污染

- **零本地 Agent 调度，零环境污染**：
  - 彻底移除本地进程唤起、Mirasim server 守护与 WebSocket 驱动的本地 Agent 逻辑。
  - 用户在 `pi` 或其他客户端中作为 API Provider 使用时，插件**仅负责模型报文的网络直传与协议翻译**，绝不在本地机器或当前工作空间中执行任何本地工具调用、代码修改或命令调度。
- **复用官方成熟报文翻译路径 (`sdk/translator`)**：
  - 基于 CLIProxyAPI 官方成熟的报文翻译架构，将接收到的标准 OpenAI 格式报文（来自 `pi`、curl 或第三方应用）直接翻译为上游网关所需的请求协议（Anthropic Messages、OpenAI Responses 等）。
  - 上游返回的 SSE 事件流直接翻译为标准 OpenAI `chat.completion.chunk` 实时回传给调用方，原生支持思考链（Reasoning / Thinking）流式输出。
- **借鉴 Codex 客户端模型加载架构**：
  - 采用独立的结构化模型元数据 JSON（`models/mirasim_models.json`）与 Go `//go:embed` 作为离线基底。
  - 具备线程安全 Catalog 快照存储与动态版本控制。
  - 支持后台定时轮询与授权实时动态发现（`model.for_auth`），账号可用模型自动同步上游。
  - 纯粹面向模型 ID，对外提供标准模型命名空间（`mirasim-proxy/{model_id}` 及兼容 `mirasim/{model_id}`）。
- **完全兼容管理控制台与 OAuth 回填操作**：
  - 支持官方 WebUI 面板一键发起授权。
  - 远程服务器（VPS / 容器）部署时，支持在管理面板中直接回填重定向地址完成 Token 萃取与持久化。

---

## 支持的模型列表 (Model IDs)

| 请求模型标识 (Model ID) | 上游协议 | 说明 |
| :--- | :--- | :--- |
| `mirasim-proxy/claude-3-7-sonnet` | Anthropic Messages | Claude 3.7 Sonnet（原生支持思考链推理） |
| `mirasim-proxy/claude-3-5-sonnet` | Anthropic Messages | Claude 3.5 Sonnet |
| `mirasim-proxy/claude-3-5-haiku` | Anthropic Messages | Claude 3.5 Haiku |
| `mirasim-proxy/claude-opus-5-5` | Anthropic Messages | Claude Opus 5.5 |
| `mirasim-proxy/claude-sonnet-5` | Anthropic Messages | Claude Sonnet 5 |
| `mirasim-proxy/claude-haiku-4-5` | Anthropic Messages | Claude Haiku 4.5 |
| `mirasim-proxy/gpt-4o` | OpenAI / Responses | GPT-4o 旗舰模型 |
| `mirasim-proxy/gpt-4o-mini` | OpenAI / Responses | GPT-4o Mini |
| `mirasim-proxy/o1` | OpenAI / Responses | o1 深度推理模型 |
| `mirasim-proxy/o3-mini` | OpenAI / Responses | o3-mini 推理模型 |
| `mirasim-proxy/gpt-6-astra` | OpenAI / Responses | GPT-6 Astra |
| `mirasim-proxy/gpt-5.6-sol` | OpenAI / Responses | GPT-5.6 Sol |
| `mirasim-proxy/gemini-2.5-pro` | Gemini | Gemini 2.5 Pro |
| `mirasim-proxy/gemini-2.0-flash` | Gemini | Gemini 2.0 Flash |
| `mirasim-proxy/deepseek-chat` | OpenAI-compatible | DeepSeek V3 |
| `mirasim-proxy/deepseek-reasoner`| OpenAI-compatible | DeepSeek R1（支持深度思考） |
| `mirasim-proxy/deepseek-flash` | OpenAI-compatible | DeepSeek V4.1 Flash |
| `mirasim-proxy/kimi-k1.5` | OpenAI-compatible | Kimi k1.5 长文本模型 |
| `mirasim-proxy/kimi-k3` | OpenAI-compatible | Kimi k3 新一代高智力模型 |
| `mirasim-proxy/qwen-2.5-coder-32b`| OpenAI-compatible | 通义千问代码大模型 |
| `mirasim-proxy/qwen-2.5-72b` | OpenAI-compatible | 通义千问 72B |
| `mirasim-proxy/grok-2` | OpenAI-compatible | xAI Grok 2 |
| `mirasim-proxy/glm-4` | OpenAI-compatible | 智谱 GLM-4 |
| `mirasim-proxy/glm-5.3-flash` | OpenAI-compatible | 智谱 GLM-5.3 Flash |

---

## 控制面板与 OAuth / Session 登录操作流程

1. **发起授权**：
   在 CLIProxyAPI 管理面板点击 **Mirasim Proxy 登录**（或请求 `GET /v0/management/mirasim-proxy-auth-url`），获取授权 URL。
2. **多模式登录支持**：
   在浏览器中打开授权链接，系统提供以下三种登录授权选项：
   - 🐱 **GitHub 登录**：通过 GitHub OAuth 授权（跳转至官方登录页）。
   - 🌐 **Google 登录**：通过 Google OAuth 授权（跳转至官方登录页）。
   - 🔑 **直接输入 Session (网页兼容模式)**：适用于远程服务器、容器或受限网络环境。直接在网页中填入已有的 Mirasim 访问凭据（JWT Token 或 `mirachannelToken`），点击“提交并授权”即可自动完成绑定。
3. **回填回调地址 (远程部署 / 容器场景)**：
   若处于无头或远程环境，第三方 OAuth 授权跳转完成后（或直接在页面中输入 Session 生成回调地址后），复制完整重定向 URL，粘贴至管理控制台的“回填回调地址”输入框并提交（前端调用 `POST /v0/management/oauth-callback`）。
4. **自动完成绑定**：
   插件解析凭据信息并持久化写入 `mirasim-proxy.json`。后续外部客户端发送的请求将自动携带该 Token 鉴权。

---

## 编译方式

### Windows:
```cmd
build.bat
# 或执行:
CGO_ENABLED=1 go build -buildmode=c-shared -o mirasim-proxy.dll .
```

### Linux / macOS:
```bash
make build
# 或执行:
CGO_ENABLED=1 go build -buildmode=c-shared -o mirasim-proxy.so .
```

---

## CLIProxyAPI 配置示例

在 CLIProxyAPI 的 `config.yaml` 中配置加载：

```yaml
plugins:
  enabled: true
  dir: "plugins" # 包含 mirasim-proxy.dll / mirasim-proxy.so
  configs:
    mirasim-proxy:
      enabled: true
      priority: 1
      # Mirasim 上游网关（默认: https://relay.mirasim.ai）
      relay_url: "https://relay.mirasim.ai"
      # OAuth 登录选项 (默认: "all", 网页选择页提供 GitHub/Google/直接输入Session 三种方式; 可选 "github" 或 "google" 直跳)
      oauth_provider: "all"
      # 可选：手动指定 Token（亦可通过面板 OAuth 完成配置）
      # token: "YOUR_MIRASIM_TOKEN"
```

---

## 开源协议

本项目采用 [MIT](LICENSE) 协议开源。
