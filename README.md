# CLIProxyAPI Plugin: Mirasim (`cliproxy-plugin-mirasim`)

Mirasim 本地核心引擎在 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 平台上的标准动态链接库插件（遵循官方 `pluginabi.ABIVersion` 与 `SchemaVersion` 标准）。

---

## 插件特性

- **纯粹模型命名空间 (`mirasim/{model_id}`)**：
  - 彻底屏蔽 Mirasim 内部的 Harness / Agent 概念（如 `pi`, `claude`, `codex`, `zcode` 等内部执行器）。
  - 外部 API 消费方使用纯粹的标准模型 ID（如 `mirasim/claude-3-7-sonnet`, `mirasim/gpt-4o`, `mirasim/gemini-2.5-pro`）。
  - 不存在 `mirasim/agent:model` 杂质语法；插件根据模型 ID 自动完成底层引擎路由。
- **完全兼容管理面板操作与回填 OAuth 地址**：
  - **面板识别**：声明标准 `AuthProvider` 规范，CLIProxyAPI WebUI 控制面板自动出现 Mirasim OAuth 登录卡片。
  - **远程回填回调地址**：远程服务器（VPS / Docker）部署时，浏览器在本地无法直连远程服务器的回环地址。用户只需将跳转后的完整 URL（带 `code=...`、`token=...` 或 `#token=...`）复制粘贴回控制面板，面板自动提取 Token 并在服务器端保存为 `mirasim.json`。
  - **本地实例静默发现**：若本机运行着 Mirasim 桌面端，授权探针自动提取 Token 并瞬间激活。
- **双向实时流式与思考链 (Thinking Process)**：
  - 支持 CLIProxyAPI `host.stream.emit` 与 `host.stream.close` 实时双向流机制。
  - 自动捕获思考过程输出为 OpenAI 规范的 `reasoning_content`。

---

## 支持的模型列表 (Model IDs)

| 请求模型标识 (Model ID) | 底层驱动模型 | 说明 |
| :--- | :--- | :--- |
| `mirasim/claude-3-7-sonnet` | Claude 3.7 Sonnet | 支持全思考链推理 |
| `mirasim/claude-3-5-sonnet` | Claude 3.5 Sonnet | 高性能代码生成 |
| `mirasim/claude-3-5-haiku` | Claude 3.5 Haiku | 极速轻量模型 |
| `mirasim/gpt-4o` | GPT-4o | OpenAI 多模态旗舰模型 |
| `mirasim/gpt-4o-mini` | GPT-4o Mini | 经济高效模型 |
| `mirasim/o1` | OpenAI o1 | 深度推理模型 |
| `mirasim/o3-mini` | OpenAI o3-mini | 紧凑型推理模型 |
| `mirasim/gemini-2.5-pro` | Gemini 2.5 Pro | 超长上下文与高级分析 |
| `mirasim/gemini-2.0-flash` | Gemini 2.0 Flash | 毫秒级极速响应 |
| `mirasim/kimi-k1.5` | Kimi k1.5 | 月之暗面长文本模型 |
| `mirasim/qwen-2.5-coder-32b`| Qwen 2.5 Coder 32B | 通义千问代码大模型 |
| `mirasim/grok-2` | Grok 2 | xAI 智能模型 |
| `mirasim/glm-4` | GLM-4 | 智谱旗舰模型 |
| `mirasim/{任意模型ID}` | 动态透传 | 插件自动解包并交由 Mirasim 核心调度 |

---

## 控制面板与 OAuth 回填操作流程

### 1. 本地部署（本机直接运行）
1. 启动 Mirasim 桌面端或通过插件自动拉起后台服务。
2. 在 CLIProxyAPI WebUI 控制面板中点击 **Mirasim 登录**。
3. 插件自动探测到活跃 Token，面板显示“登录成功”。

### 2. 远程部署（VPS / 云服务器 / Docker 容器）
1. 在远程 CLIProxyAPI 管理面板点击 **Mirasim 登录**（或调用 `GET /v0/management/mirasim-auth-url`）。
2. 在本地浏览器打开返回的授权 URL。
3. 授权跳转后，即使页面显示“无法访问”（因为目标为远程回环地址），直接**复制本地浏览器地址栏中的完整 URL**。
4. 回到 CLIProxyAPI 管理面板，将复制的 URL 粘贴到“回填回调地址”输入框中提交（面板调用 `POST /v0/management/oauth-callback`）。
5. 插件自动解析 URL 中的 Token 参数，并在远程服务器将凭证写入 `mirasim.json`，完成授权绑定。

---

## 编译方式

### Windows:
```cmd
build.bat
# 或执行:
CGO_ENABLED=1 go build -buildmode=c-shared -o mirasim.dll .
```

### Linux / macOS:
```bash
make build
# 或执行:
CGO_ENABLED=1 go build -buildmode=c-shared -o mirasim.so .
```

---

## CLIProxyAPI 配置

将生成的 `mirasim.dll`（Windows）或 `mirasim.so`（Linux）放入 CLIProxyAPI 的 `plugins/` 目录，并在 CLIProxyAPI 的 `config.yaml` 中配置启用：

```yaml
plugins:
  enabled: true
  dir: "plugins" # 包含 mirasim.dll / mirasim.so
  configs:
    mirasim:
      enabled: true
      priority: 1
      port: 4939        # Mirasim 本地端口
      auto_spawn: true  # 未检测到后台时自动拉起后台服务
```

---

## 开源协议

本项目采用 MIT 协议开源。
