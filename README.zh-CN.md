# Agenty

[English](./README.md)

Agenty 是一个本地优先的 AI agent 应用。当前产品链路由 `agenty-cli`、
`agenty-core`、Rust `file-editor` helper 和自解压 launcher `agenty-bootstrap` 组成。
Bootstrap 监督 core 和 CLI；CLI 通过本地 Unix domain socket 或 Windows 命名管道上的
HTTP/2 与 core 通信。

core 当前支持 provider/model 管理、持久化会话、模型流式输出、agent 工具循环、会话压缩、
内置文件工具、本地 Skills 和 MCP client 连接。memory 和远程客户端模式要等 core 提供对等
实现后再开放。

## 快速开始

从[最新 release](https://github.com/masteryyh/agenty/releases/latest)下载与你的系统和
架构匹配的压缩包，解压并安装 `agenty`：

```bash
chmod +x agenty
sudo install -m 755 agenty /usr/local/bin/agenty
agenty
```

首次运行时，launcher 会校验并释放内置 CLI、core 和 patch helper 到
`~/.agenty/bin` 下按版本区分的路径。Bootstrap 会为所选数据目录启动或附着 core，
再把真实终端 stdin/stdout/stderr 交给 CLI。初始化向导通过版本化 HTTP API 创建 provider
和聊天 model，并保存默认会话 model。

## 运行模型

launcher 内含三个 XZ 压缩 payload 及其解压内容的 SHA3-256 摘要。摘要一致的文件会直接
复用；缺失或不一致时会重新解压、校验并原子替换。释放路径由三个 payload 摘要共同确定，
因此新版本不会覆盖仍在运行的 core。Bootstrap 准备 `fileedit` 的 PATH、解析规范化数据目录、
获取该目录的 core 所有权或附着到兼容 core，完成 HTTP/2 握手后启动 CLI。一个规范化数据目录
只能由一个 core 所有，不同数据目录可以并行运行；附着的 CLI 不负责关闭已有 core。

Core 在 `/v1` 下提供初始化、provider/model、session/round、skills、MCP 和工具审批等普通
JSON endpoint。`POST /v1/stream` 是双向 HTTP/2 流，客户端可持续发送订阅/取消订阅命令，同时
接收事件。每个 topic 有单调递增序号和不透明 stream ID。客户端可用游标恢复；短期重放窗口过期
时，core 会先发送一致快照，再继续发送实时事件。CLI 断开不会取消已接受的 round。

TUI 当前开放 `/provider`、`/model`、`/mcp`、`/cwd`、`/effort`、`/status`、`/codex-mode`、
`/new`、`/resume`、`/help` 和 `/exit`。在输入框中输入 `$` 可以搜索并插入已安装的
Skill 结构化引用。core 会依次扫描数据目录下的 `skills/`、`~/.agents/skills` 和
`~/.claude/skills`；可以通过 `AGENTY_DATA_DIR` 更改第一个目录。

## 配置与存储

core 默认把数据保存在 `~/.agenty`。可向 bootstrap 传入 `--data-dir <path>`，或设置
`AGENTY_DATA_DIR` 以切换数据根目录。Bootstrap 会把规范化目录和本地 IPC 地址传给子进程。
主要文件如下：

| 数据 | 路径 |
| --- | --- |
| 配置 | `~/.agenty/config.json` |
| Skills | `~/.agenty/skills/`，然后是 `~/.agents/skills/` 和 `~/.claude/skills/` |
| 会话 transcript | `~/.agenty/sessions/<yyyy>/<mm>/<dd>/<session-id>.jsonl` |
| 会话索引 | `~/.agenty/agenty.sqlite` |
| Providers 和 models | 内置 catalog 固化在 core 二进制中；自定义 provider 使用 `~/.agenty/providers/<provider-code>.json`，内置 provider 文件仅保存 API key |
| MCP server 配置 | `~/.agenty/mcp/<server-name>.json`（每个 server 一个简洁 JSON 文件，权限 `0600`） |
| MCP OAuth 凭据 | `~/.agenty/mcp-auth/<server-name>.json`（权限 `0600`，与配置分开保存） |
| 模型发现缓存 | 仅保存在运行中的 core 进程内，有效期 8 小时；按需刷新，core 重启后不会保留 |
| Patch 事务锁 | `~/.agenty/locks/` |
| 日志 | `~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log` |

`AGENTY_LOG_LEVEL` 接受 `debug`、`info`、`warn` 或 `error`；
`AGENTY_LOG_FORMAT` 接受 `text` 或 `jsonl`。

`/mcp` 可管理 stdio、Streamable HTTP 和 legacy SSE server。server 文件包含 `type`、
`enabled`，以及按 transport 选择的 `command`/`args`/`env` 或 `url`/`headers` 字段，
其中 `args` 是每个进程参数一个元素的 JSON 字符串数组。不保存工作目录；环境变量和
header 值可以引用环境变量；TUI 中的 headers 位于默认收起的 `Advanced Options` 下。
Streamable HTTP 使用 Go SDK 提供的 OAuth authorization-code 流程，自动进行动态 client
registration 和 loopback 浏览器登录；`/mcp` 会打开授权 URL 并显示连接状态。SSE 继续兼容已有
server，但会在 TUI 中标记为 deprecated。连接在后台以有限并发启动；每个 round 开始时会快照
当时已连接的工具，较晚完成连接的 server 从下一轮开始可用。

## 开发

```bash
pnpm install
pnpm cli:dev
```

`pnpm dev`（同 `pnpm cli:dev`）通过 Turbo 先构建本机支持的推理 worker，随后构建 core、CLI
和 bootstrap，再以真实终端 stdio 启动打包后的程序。worker 产物保留在
`packages/llama-cpp-worker/bin/llama-cpp-worker`（Windows 带 `.exe`）及 Apple Silicon 上的
`packages/mlx-worker/bin/mlx-worker`。MLX 入口使用同目录中的私有 CPython 和锁定依赖。
构建需要 C/C++ 编译器与 CMake 或 uv；MLX 另需 uv。

启动器新增 `--llama-cpp-worker <path>` 和 `--mlx-worker <path>`，也支持等号形式。
`dev` 自动传入构建路径，用户显式参数可覆盖。路径仅作用于当前 core 进程，不写入长期配置；
本地推理仍需启用。数据目录已有 core 运行时，需要先关闭其 bootstrap，或使用独立的
`--data-dir` 才能应用路径。常用定向命令：

```bash
pnpm core:test
pnpm core:test:integration
pnpm core:test:e2e
pnpm cli:typecheck
pnpm bootstrap:test
pnpm llm-codec:build
pnpm llm-codec:test
pnpm build
pnpm run update
pnpm tidyup
pnpm clean
pnpm deepclean
```

构建版本优先来自进程环境中的 `AGENTY_VERSION`，其次读取被忽略的根目录 `.env`，两者都
没有时默认为 `dev`。如需固定本地版本，可把 `.env.example` 复制为 `.env`，直接运行
`pnpm build` 即可构建完整 launcher。默认构建本机支持的 worker、file-editor、llm-codec 静态库、core、CLI 和 bootstrap；
worker ZIP 打包复用各模块 `bin` 产物，worker 仍独立于 launcher 分发。
Inspector 通过 `pnpm inspector:build` 单独构建。

内置 local 推理 provider 通过 Rust
[llm-codec 静态库](./packages/llm-codec/README.md) 完成模板渲染、分词和增量解码，
再调用独立的 MLX 或 llama.cpp worker 计算 token。初始化向导、`/provider` 和 CLI
支持安装 worker 及启用本地推理。界面展示已注册模型并支持编辑，手动注册已有文件保留在 CLI 中。
默认关闭；worker 发行包单独构建和发布。

初始化向导的本地推理页面和 `/model` 支持浏览 Hugging Face、ModelScope 的公开模型，搜索并选择
Safetensors 或 GGUF 量化版本。下载确认前展示兼容性、保存目录、目标分区用量及剩余可用空间；
列表仅展示名称、参数量与仓库整体文件大小，缺失信息显示 `UNKNOWN`。导航到末项时自动加载下一页，
重新进入面板自动刷新；回车或点击模型直接在列表上方打开下载弹窗，多版本仓库在弹窗内选择版本。
选择后检查兼容性和容量，通过后才可点击 Download。Esc 返回，Ctrl+P 查看下载进度。
空间不足或不支持推理时禁止下载。权重保存在数据目录的 `models` 下，
进度按文件展示，支持取消和通过重新选择同一资源续传。校验与预检通过后自动添加到 local provider。
完成页可启用本地推理并返回 `/model` 选择模型；向导内回到设置页面保存后启用。
浏览面板关闭后，下载会在 core 存活期间继续；退出应用会中断下载，部分文件保留供后续续传。
当前自动添加支持 Qwen3、Qwen3.5 文本推理；自动使用仓库自带的 tokenizer，缺失时尝试从声明的基础模型
仓库获取。找不到匹配 tokenizer 时提示无法推理并禁止下载。私有或需授权的仓库暂不支持登录下载。

Core 使用 CGO 构建 SQLite。Windows 需要通过 `CC` 配置 MinGW-w64 或 LLVM-MinGW 等 GCC
兼容编译器；MSVC `cl.exe` 不受支持。进程级配置示例见 [core 构建说明](./packages/agenty-core/README-CN.md)。

`pnpm run update` 会更新 pnpm、Go 和 Cargo 模块的依赖；`pnpm tidyup` 会在所有 Go 模块中依次
运行 `go fmt`、`go vet` 和 `go mod tidy`。`pnpm clean` 清理根目录及所有模块的构建产物；
`pnpm deepclean` 进一步清理本地缓存、依赖 store、`node_modules` 和其他生成的临时文件，
保留环境文件与已跟踪的锁文件。

## 许可证

本项目使用 Apache License 2.0，详见 [LICENSE](./LICENSE)。

Copyright (c) 2026 masteryyh

## 会话调试工具

[Agenty Inspector](./packages/agenty-inspector/README.md) 是独立的只读网页调试工具。
执行 `pnpm inspector:dev` 后访问 `http://127.0.0.1:5173`；也可执行
`pnpm inspector:build` 与 `pnpm inspector:start`，使用默认端口 4318 的独立程序。
它沿用 `AGENTY_DATA_DIR`（默认 `~/.agenty`），展示消息、原始事件、工具关联和诊断问题，
无需启动 core。
