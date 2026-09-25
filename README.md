# CLIProxyAPI Plugin: Mirasim (`cliproxy-plugin-mirasim`)

Mirasim 本地核心引擎在 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 平台上的标准动态链接库插件（遵循官方 `pluginabi.ABIVersion` 与 `SchemaVersion` 标准）。

---

## 插件特性

- **模型命名空间隔离 (`mirasim/{model}`)**：
  注入的所有模型均带 `mirasim/` 前缀（如 `mirasim/pi`, `mirasim/claude-3-7-sonnet`, `mirasim/codex` 等），无需配置或控制 `default_agent`，请求中的模型标识即直接决定路由目标。
- **全生命周期 AuthProvider 支持**：
  - **远程 OAuth 支持**：支持无头服务器、远程 VPS、Docker 容器部署场景。
  - **面板填入回调 URL**：在 CLIProxyAPI WebUI 控制面板或通过 `/v0/management/oauth-callback` 填入浏览器重定向后的完整回调 URL，插件自动解析 Token 与 State 完成凭证归档。
  - **本地实例极速接管**：若本机已启动 Mirasim 桌面端，授权探针自动提取 Token，实现一键登录。
- **双向实时流式与思考链 (Thinking Process)**：
  - 支持 CLIProxyAPI `host.stream.emit` 与 `host.stream.close` 实时双向流机制。
  - 自动捕获 Agent 的思考过程输出为 OpenAI 规范的 `reasoning_content`。
- **纯粹 API 路由体系**：
  不侵入默认全局 Agent，将 Mirasim 的底层交互与各能力模型纯净封装为标准 OpenAI 兼容接口。

---

## 支持的模型列表 (Model IDs)

| 请求模型标识 (Model ID) | 路由目标 | 说明 |
| :--- | :--- | :--- |
| `mirasim/pi` | Pi Coding Agent | 官方 Pi 智能编程助手 |
| `mirasim/claude` | Claude Code | 默认 Claude Code Agent |
| `mirasim/claude-3-7-sonnet` | Claude Code | Claude 3.7 Sonnet |
| `mirasim/claude-3-5-sonnet` | Claude Code | Claude 3.5 Sonnet |
| `mirasim/codex` | Codex | 默认 Codex Agent |
| `mirasim/gpt-4o` | Codex | GPT-4o 交互模型 |
| `mirasim/kimi` | Kimi | Kimi Code Agent |
| `mirasim/qwen` | Qwen | Qwen Code Agent |
| `mirasim/grok` | Grok | Grok Build Agent |
| `mirasim/zcode` | ZCode | 智谱 ZCode CLI Agent |
| `mirasim/default` | 默认 Agent | Mirasim 默认路由 |
| `mirasim/{agent}:{model}` | 自定义 | 任意指定 Mirasim 支持的 Agent 及子模型 |

---

## 编译方式

### 前置要求
- Go 1.22+
- C 语言编译器（Windows 下推荐 MinGW-w64 GCC，Linux/macOS 下使用 GCC / Clang）

### 编译为动态链接库插件

#### Windows:
```cmd
build.bat
# 或手动执行:
CGO_ENABLED=1 go build -buildmode=c-shared -o mirasim.dll .
```

#### Linux / macOS:
```bash
make build
# 或手动执行:
CGO_ENABLED=1 go build -buildmode=c-shared -o mirasim.so .
```

---

## CLIProxyAPI 配置

将生成的 `mirasim.dll`（Windows）或 `mirasim.so`（Linux）放入 CLIProxyAPI 的 `plugins/` 目录，并在 CLIProxyAPI 的 `config.yaml` 中配置启用：

```yaml
plugins:
  enabled: true
  dir: "plugins" # 包含 mirasim.dll / mirasim.so 的目录
  configs:
    mirasim:
      enabled: true
      priority: 1
      # Local Mirasim 后端端口（默认 4939）
      port: 4939
      # 未检测到后台时是否自动拉起后台服务（默认 true）
      auto_spawn: true
```

---

## OAuth 远程登录与控制面板回调

在部署于远程服务器或 Docker 容器时，用户本地浏览器访问远程的回调地址会因回环地址而无法直接打通：

1. **发起登录**：
   在 CLIProxyAPI 管理面板点击 Mirasim 登录，或请求 `GET /v0/management/mirasim-auth-url` 获取授权 URL。
2. **浏览器授权**：
   在本地浏览器打开返回的授权 URL。
3. **面板填入回调 URL**：
   浏览器跳转完成后，直接从浏览器地址栏复制完整的回调 URL，粘贴至管理面板的“回调 URL”输入框中（面板将调用 `POST /v0/management/oauth-callback`），或通过 curl 提交：
   ```http
   POST /v0/management/oauth-callback
   Content-Type: application/json
   Authorization: Bearer <management-key>

   {"provider":"mirasim","redirect_url":"<完整的回调URL>"}
   ```
4. **自动完成绑定**：
   插件将自动从回调数据中提取 Token，并将其持久化保存为 `mirasim.json`。后续所有发送至 `mirasim/{model}` 的 API 请求均将自动挂载该认证凭证。

---

## 开源协议

本项目采用 MIT 协议开源。
