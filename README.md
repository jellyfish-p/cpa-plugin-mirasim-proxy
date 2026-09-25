# CLIProxyAPI Plugin: Mirasim (`cliproxy-plugin-mirasim`)

Mirasim (桌面客户端 / 本地核心引擎) 的 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 动态链接库与 Provider 插件。

将 Mirasim 内部的本地交互引擎与多 Agent 架构（Pi, Claude Code, Codex, Kimi, Qwen, Grok 等）无缝桥接至 CLIProxyAPI 路由体系，允许用户通过 CLIProxyAPI 统一调用 Mirasim 提供的各 AI Agent 能力。

---

## 插件特性

- **官方标准 C ABI 规范**：完全遵循 CLIProxyAPI `pluginabi.ABIVersion` 与 `SchemaVersion` 标准开发，支持 `-buildmode=c-shared` 输出 `.dll` / `.so` 动态加载。
- **Capability 声明**：
  - `ModelProvider`：向 CLIProxyAPI 注册 `mirasim`, `pi`, `claude`, `codex`, `kimi`, `qwen`, `grok` 等模型。
  - `Executor`：支持 `chat-completions` 与 `openai.chat` 协议。
- **全实时流式输出**：
  - 支持 CLIProxyAPI `host.stream.emit` 与 `host.stream.close` 实时双向流机制。
  - 自动捕获思考链（Thinking / Reasoning Process）输出为 `reasoning_content`。
- **自动检测与实例守护**：
  - 自动扫描并复用本机已启动的 Mirasim 桌面端或 Server 进程。
  - 若无活跃实例，插件可自动在后台静默拉起 `server.cjs` 并管理生命周期。

---

## 构建方式

### 前置要求
- Go 1.22+
- C 编译器（Windows 下推荐 MinGW-w64 GCC，Linux/macOS 下使用 GCC / Clang）

### 编译为动态链接库插件

#### Windows:
```cmd
build.bat
# 或者手动执行:
CGO_ENABLED=1 go build -buildmode=c-shared -o mirasim.dll .
```

#### Linux / macOS:
```bash
make build
# 或者手动执行:
CGO_ENABLED=1 go build -buildmode=c-shared -o mirasim.so .
```

编译完成后将生成 `mirasim.dll`（Windows）或 `mirasim.so`（Linux）。

---

## CLIProxyAPI 配置

将生成的 `mirasim.dll`（或 `mirasim.so`）放入 CLIProxyAPI 的 `plugins/` 目录中，并在 CLIProxyAPI 的 `config.yaml` 中配置启用：

```yaml
plugins:
  enabled: true
  dir: "plugins" # 包含 mirasim.dll / mirasim.so 的目录
  configs:
    mirasim:
      enabled: true
      priority: 1
      # Mirasim 本地后端端口（默认 4939）
      port: 4939
      # 默认路由的 Agent (pi, claude, codex, kimi, qwen, grok)
      default_agent: "pi"
      # 是否在未检测到后台时自动拉起后台服务（默认 true）
      auto_spawn: true
```

启动 CLIProxyAPI 即可加载该插件并在 `/v1/models` 中看到 Mirasim 提供的模型，并直接通过 `/v1/chat/completions` 调用。

---

## 支持的模型列表

| 模型标识 (Model ID) | 归属提供商 | 说明 |
| :--- | :--- | :--- |
| `mirasim` | Mirasim | 默认 Agent 路由 |
| `pi` | Mirasim | Pi Coding Agent |
| `claude` | Anthropic | Claude Code Agent |
| `claude-3-7-sonnet` | Anthropic | Claude 3.7 Sonnet |
| `claude-3-5-sonnet` | Anthropic | Claude 3.5 Sonnet |
| `codex` / `gpt-4o` | OpenAI | ChatGPT Codex Agent |
| `kimi` | Moonshot | Kimi Code Agent |
| `qwen` | Alibaba | Qwen Code Agent |
| `grok` | xAI | Grok Build Agent |
| `zcode` | Zhipu | ZCode CLI Agent |

---

## 开源协议

本项目根据 MIT 协议发布。
