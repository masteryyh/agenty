# agenty-core

[English](README.md) | [简体中文](README-CN.md)

Agenty 的核心运行时。它围绕本地优先的存储模型（文件系统 + SQLite）和领域驱动设计
（DDD）领域层进行设计。

## 存储模型

文件系统是真实数据源，SQLite 是查询侧投影。

| 数据 | 位置 | 作用 |
| --- | --- | --- |
| Session transcript | `~/.agenty/sessions/<yyyy>/<mm>/<dd>/<session-id>.jsonl` | 写模型，即 append-only event log（真实数据源） |
| Session index | `~/.agenty/agenty.sqlite` -> `sessions` | 读模型，用于快速列表和搜索的投影 |
| 全局配置 | `~/.agenty/config.json` | 应用配置 |
| Providers | 内置 catalog 固化在 core；自定义 provider 使用 `~/.agenty/providers/<provider-code>.json` | 内置元数据/模型只读，内置 provider 文件仅保存 API key |
| Agents | `~/.agenty/agents/<code>.json` | Agent aggregate |
| Core 日志 | `~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log` | 结构化文本诊断信息（JSONL 模式下为 `core.jsonl`） |

Session 的 messages 和 rounds 永远不会存入 SQLite；`sessions` 表是摘要投影，可以通过
重放 JSONL transcript 重建。它当前的配置投影包括已选模型、模型的 `context_window` 和
reasoning effort。

## 领域层

领域层按 bounded context 拆分。Aggregates 之间只通过 identity 相互引用（conversation
系列使用 UUIDv7，agents 和 providers 使用路径安全的 code，models 使用可保留上游字符的 code）。

```
pkg/domain/
├── shared/        Shared kernel: Code, ModelRef, ReasoningEffort, Metadata, Event, ID
├── conversation/  Session aggregate (Session -> Round -> Message), content blocks, events
├── agent/         Agent aggregate
└── catalog/       Provider aggregate (Provider -> Model)
```

Conversation transcript 采用 event sourcing：每一行 JSONL 都是一个 domain event
（`session_started`、`session_model_set`、`session_reasoning_effort_set`、
`session_cwd_set`、`round_started`、`message_appended`、`round_ended` 等），并且可以
通过 `conversation.ReplaySession` 重建 `Session` aggregate。Session 保存未来 rounds
使用的当前配置，而 `RoundStarted` 会快照记录该 round 使用的模型、context window、
reasoning effort 和工作目录。

### Reasoning effort

Agenty 对外提供且仅提供六个与 provider 无关的 reasoning effort 等级：`off`、`low`、
`medium`、`high`、`xhigh` 和 `max`。reasoning 模型通过 `reasoningEfforts` 数组保存
实际支持的启用等级：

```json
{
  "reasoningEfforts": ["low", "medium", "high", "xhigh", "max"]
}
```

显式空数组表示非 reasoning 模型；上游未返回 capability 数据时默认使用五个启用等级。
provider adapter 会原样发送选择的等级，不支持的等级通过正常 round error 流程返回。
`minimal` 等 provider 特有等级不对外开放。

## Agent loop 运行时

`pkg/agentloop/` 是独立的 Agent 运行时模块。它拥有 provider-neutral 的模型调用
contract、tool contract、JSON Schema、线程安全的 tool registry，以及统一管理多个
session 的 `Engine`。不同 session 可以并行执行，同一 session 只允许一个 active round；
`Engine` 统一管理所有运行中 round 的取消和 shutdown。

每次 loop 会解析 Agent system prompt，重建有效会话上下文，通过选定 provider adapter
转换上游数据结构，调用 LLM、持久化 assistant 响应，并在返回 tool calls 时继续循环。
自定义 model 未填写时默认使用 `8192` 最大输出 token；内置 model 使用嵌入 catalog
中的精确限制。估算上下文达到
`contextWindow` 的 `90%` 时自动压缩；TUI 的 `/compact`
可以手动触发同一流程。压缩会在 `session_compacted` 事件中只保存生成的总结和压缩审计数据；
重放和构造模型请求时，再从 transcript 动态计算最多三条最近 user 消息、总结、metadata
以及最多五条最近 assistant 消息，原始 JSONL transcript 不变。保留消息会移除 reasoning
和未配对的 tool-use block。当前单个 round 最多执行 20 次 LLM/tool 迭代。压缩请求
保持原有 system、消息和工具前缀不变，只在内存中追加一条 user 压缩指令；压缩过程中的
工具调用和结果也只保存在临时缓冲区。切换到上下文窗口较小的 model 时，如果达到目标
窗口的 90%，先使用当前 model 压缩，必要时裁剪保留消息以适配目标窗口，再写入 model
切换事件。
共享 tool registry 实现 `ToolRuntime` port；同一批次内每个 tool call 并行执行，结果按
调用顺序返回。`pkg/agentloop/builtin/` 提供 `read_file`、`apply_patch`、`grep`、`glob`
和 `ls`，由 `cmd/main.go` 显式注册。`apply_patch` 会调用同名 Rust 可执行文件完成 V4A
解析和原子化文件修改。支持 free-form tool 的 provider 会收到模型工具定义；其他 provider
会在 system prompt 中收到通过 `shell` 执行同一命令的说明。相对路径基于该 round 捕获的
session 工作目录解析。

## 基础设施层

基础设施层（`pkg/infra/`）基于文件系统 + SQLite 存储模型实现领域 repositories。

```
pkg/infra/
├── config/             将配置文件和 env override 合并到单例中；解析 data-dir 路径
├── initialize/         OpenRepositories：一次性初始化所有 stores
├── catalogdata/        内嵌的 provider/model JSON
├── llm/                实现 agentloop caller contract 的 provider SDK adapters
├── logging/            slog 初始化、环境配置解析和按日生成日志路径
├── storage/            Repository 实现 + SQLite connection factory
│   ├── db.go           OpenDB/OpenIsolatedDB + sessions schema
│   ├── agent.go        AgentRepository（agent JSON 文件）
│   ├── catalog.go      CatalogRepository（内置 provider 与自定义 provider JSON）
│   └── conversation.go ConversationRepository（JSONL transcript + SQLite projection）
└── rpc/                stdio JSON-RPC 2.0 接口层
    ├── message.go      Request/Response/Notification/Error/ID wire types
    ├── codes.go        标准错误码 + server-defined 错误码
    ├── handler.go      Handler interface + Dispatcher
    ├── server.go       NDJSON-over-stdio Server（batch、notifications、cancel）
    └── adapter/        application services -> JSON-RPC method handlers
```

## 应用层

应用层（`pkg/application/`）包含编排 domain aggregates 和 repositories 的 use-case
services，将失败归类为少量业务错误码，并保证 mutation 与 event-sourced Session
aggregate 一致（load -> mutate -> save -> clear pending events）。

每个 service 只依赖其 use cases 所需的最小 repository interface。生产环境装配文件系统
和 SQLite repositories；单元测试则使用隔离的内存 fakes，不会打开文件或数据库。

- `AgentService`：agent CRUD（`Create`/`Get`/`List`/`Update`/`Delete`）。
- `ProviderService`：provider CRUD 以及 model 子资源操作
  （`AddModel`/`RemoveModel`）。
- `InitializeService`：首次运行状态和完成校验；provider/model/agent 数据通过各自的正式服务写入。
- `SessionService`：session CRUD 和配置修改
  （`SetTitle`/`SetModel`/`SetReasoningEffort`/`SetCwd`）。

`application.Error` 携带一个 `Code`（NotFound/AlreadyExists/Validation/Internal），接口层
会将其映射为结构化 JSON-RPC 错误码。

## 接口层

接口层是一个 stdio JSON-RPC 2.0 server。协议核心位于 `pkg/infra/rpc/`，method adapters
位于 `pkg/infra/rpc/adapter/`。入口 `cmd/main.go` 会打开 repositories、装配 services、
注册 handlers 并开始处理请求。

传输格式是逐行 JSON（NDJSON）：stdin 每行一条 JSON-RPC message，stdout 每行一条
response。每一行都必须是由 `json.Marshal` 生成的单个紧凑 JSON value（不能使用
`MarshalIndent`）；未转义的控制字符或多行 JSON 会把一条 message 拆成多行并破坏
framing。Notifications（没有 `id` 的 requests）不会产生 response；batches（arrays）
会产生单个 array response。诊断信息写入 core 日志文件，确保 stdout 始终是纯净的
JSON-RPC stream。Server 会在 stdin EOF、SIGINT 或 SIGTERM 时关闭。stderr 仅用于报告
导致日志文件自身无法初始化或关闭的失败。

### 日志

`agenty-core` 使用标准库 `slog` package。日志会追加写入进程启动日期对应的
`~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log`。设置 `AGENTY_DATA_DIR` 后，`logs` 目录也会
移动到该数据目录下。

`AGENTY_LOG_LEVEL` / `logging.level` 接受 `debug`、`info`、`warn` 或 `error`，默认值为
`info`。`AGENTY_LOG_FORMAT` / `logging.format` 接受 `text` 或 `jsonl`，默认值为 `text`。
两者既可以写在配置文件的 `logging` section 中，也可以通过 `AGENTY_LOG_LEVEL` /
`AGENTY_LOG_FORMAT` 环境变量设置；设置环境变量时，其值优先于配置文件，因此可以在
不修改文件的情况下临时 override；环境变量为空或只含空白字符时则回退到配置文件值。
JSONL 模式使用 `slog.JSONHandler`，每行向 `core.jsonl` 写入一个 JSON object；text 模式
使用 `slog.TextHandler`。值不区分大小写并忽略首尾空白；不支持的值会导致启动失败。

启动期间，配置文件与环境变量只会合并一次，并写入进程级 `config.Manager` 单例；
`logging`、`initialize` 和其他模块都从这一统一数据源读取配置，不会各自解析环境变量或
路径。

超过 64 MiB 的输入行不会导致服务退出：server 会丢弃该行，返回 `-32003` message too
large（`id: null`、`data.maxLineBytes`），然后继续处理请求。由于被丢弃的行没有可解析的
`id`，sender 收到 `-32003` 后必须停止 pipelining，并通过下述分块上传协议重新发送
payload。

Methods 使用 `resource.action` 命名：

| 分组 | Methods |
| --- | --- |
| Initialize | `initialize.already`, `initialize.complete` |
| Agent | `agent.create`, `agent.get`, `agent.list`, `agent.update`, `agent.delete` |
| Provider | `provider.create`, `provider.get`, `provider.list`, `provider.listModels`, `provider.update`, `provider.delete`, `provider.addModel`, `provider.removeModel` |
| Session | `session.create`, `session.get`, `session.list`, `session.delete`, `session.setTitle`, `session.setModel`, `session.setReasoningEffort`, `session.setCwd`, `session.start`, `session.compact`, `session.stop` |
| Chunk | `chunk.begin`, `chunk.part`, `chunk.commit`, `chunk.abort` |

`provider.list` 可选接收 `{providerCode}`。不传时，core 会并行获取所有已配置且 catalog
为空的 provider；传入时只会获取指定 provider。`provider.listModels` 接收同样的
`{providerCode}`，供直接调用者使用 core 内置的发现流程。成功结果会缓存到
`~/.agenty/providers/.models/<provider-code>.json`，有效期为 8 小时，JSON 中保存 `expiresAt`
和标准化模型列表。过期缓存仍作为旧数据返回，下一次 list 时再刷新。它会兼容常见的 `id`、
名称和 token 限制字段，自动跟随 provider 分页；上下文窗口或最大输出 token 缺失或不为正数时
分别使用 `256000` 和 `65536`，缺少 reasoning 能力时返回空的 `reasoningEfforts` 数组。

`session.start` 接收 `{id, content}`，持久化 running round 后立即返回 round 标识和
`running` 状态，完整 agent turn 由引擎异步继续执行。执行期间，core 会写出
`session.event` JSON-RPC notifications，事件类型包括 `round_started`、
`message_appended`、`model_stream` 和 `round_ended`。每个事件都携带 `sessionId`、
`roundId` 和 round 内单调递增的 `sequence`；模型事件还包含 provider-neutral stream
event 和 agent loop 的 `iteration`。由于 round 与 request response 并发，notification
可能早于 `session.start` response 写出，因此 client 必须先订阅再发起请求，并把
notification 与 response 分开路由。`round_ended` 携带 `completed`、`failed` 或
`cancelled` 终态、token usage 和可选 error。

`session.stop` 接收 `{id}` 并请求取消。同一 session 重复启动，或在运行期间删除该
session，会返回 `already exists`；不同 sessions 可以并行运行。

`session.compact` 接收 `{id}`，基于当前会话临时追加一条 user 压缩指令执行总结请求。
执行期间通过 `session.compaction` notification 发出 `started`、`completed` 或 `failed`
状态，并写入只包含总结的 `session_compacted` 事件；user、metadata 和 assistant 上下文会在
重放时从 transcript 动态计算，公开 transcript projection 不会被替换。

### 分块上传

当 request 的 `params` 超过每行 64 MiB 的上限时，需要将其切分上传并由 server 组装，
然后再执行真正的 method：

1. `chunk.begin` `{requestId, method, totalSize?, chunkCount?}` 创建一个 session。
2. `chunk.part` `{requestId, index, data}` 追加一个 shard；index 必须从零开始连续递增。
   `data` 是 params JSON 文本原始切片的 base64，因此可以在任意位置切分。
3. `chunk.commit` `{requestId}` 按 index 顺序组装 shards，校验结果是合法 JSON，在进程内
   dispatch `method`，并使用 commit request 的 `id` 返回真实 method 的 result（或携带
   真实 method 错误码的结构化错误）。
4. `chunk.abort` `{requestId}` 取消正在进行的 session。

NDJSON 具有顺序保证，server 也只在单个 goroutine 上 dispatch，因此 sender 可以直接
pipeline `begin` + `part`s + `commit`，无需等待中间 responses。Sessions 保存在进程内存
中，空闲 5 分钟后会被回收；上传中断后必须使用新的 `chunk.begin` 重新开始。组装后的
payload 总大小上限为 256 MiB（超限时返回 `-32004`）。

错误码包括标准 JSON-RPC 错误码（`-32700` parse、`-32600` invalid request、`-32601`
method not found、`-32602` invalid params、`-32603` internal），以及 server-defined
`-32001` not found、`-32002` already exists、`-32003` message too large 和 `-32004`
chunk payload too large。Application validation errors 映射为 `-32602`。

示例：

```
$ echo '{"jsonrpc":"2.0","id":1,"method":"agent.create","params":{"code":"dev","name":"Dev"}}' | go run ./cmd
{"jsonrpc":"2.0","id":1,"result":{"code":"dev","name":"Dev",...}}
```

说明：`rpc` 和 `adapter` packages 使用 `encoding/json`（原生支持 RawMessage、无依赖），
而不是 sonic；application 和 storage 层仍按项目约定使用 sonic。

## 开发命令

在仓库根目录运行模块级命令：

```sh
pnpm core:build             # 编译所有 agenty-core packages
pnpm core:test              # 运行除 integration 和 e2e build tags 外的所有测试
pnpm core:test:integration  # 默认 suite + integration-tagged tests
pnpm core:test:e2e          # 在隔离进程中运行真实二进制 stdio workflows
pnpm core:test:e2e:race     # 对 e2e harness 和 core binary 启用 race detection
pnpm core:test:race         # 使用 race detector 运行默认 suite
pnpm core:test:repeat       # shuffle 后重复运行，检查隔离性
pnpm core:tidyup            # 运行 go fmt、go vet 和 go mod tidy
pnpm core:clean             # 清理该模块的 Go build 和 test caches
```

目前有意不提供本地 service 命令。End-to-end tests 使用 `e2e` build tag，因此不会进入
默认 `core:test` suite。

## 测试

默认 suite 覆盖 domain behavior、使用内存 repository fakes 的 application services、
protocol framing、配置和隔离的 storage adapter contracts。完整 repository 装配和
RPC-to-disk 路径使用 `integration` build tag。同一 build tag 还会启用可选的 LLM SDK
真实集成测试；测试直接读取环境中的 `OPENAI_API_KEY`、`ANTHROPIC_API_KEY` 和
`GEMINI_API_KEY`，未配置对应 Key 时会提示并跳过，不计为失败。

`test/e2e` package 只构建一次 `cmd`，通过 stdio 启动真实 binary，并为每个并行测试
进程分配独立的 `AGENTY_DATA_DIR`。它覆盖公开的 Agent、Provider/Model、Session、
agent loop 启停与并行执行、JSON-RPC、chunking、startup、restart persistence 和
process isolation contracts，不会访问用户的数据目录。

所有文件系统和 SQLite 测试都使用每个测试独立的临时目录。修改 `AGENTY_DATA_DIR` 的
测试不会并行运行，因为环境变量属于进程级全局状态。

完整测试策略和命令指南请参阅中文版 [TESTING-CN.md](TESTING-CN.md)，英文版见
[TESTING.md](TESTING.md)。

## 状态

领域层、agent loop 运行时、基础设施层、应用层和 stdio JSON-RPC 接口层均已实现。
基础设施层还提供
OpenAI Responses、OpenAI Chat Completions、Anthropic Messages 和 Google GenAI 的统一
非流式/流式 SDK 调用器。执行引擎当前使用非流式 caller；尚未实现 streaming agent turn
传输、命令和 todo 工具、基于该 core 的 HTTP API 和 CLI 集成。
