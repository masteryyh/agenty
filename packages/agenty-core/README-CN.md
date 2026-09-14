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
| MCP servers | `~/.agenty/mcp/<server-name>.json` | 每个 server 一个简洁配置文件，权限 `0600` |
| MCP OAuth 凭据 | `~/.agenty/mcp-auth/<server-name>.json` | OAuth token，权限 `0600`，与 server 配置分开 |
| Core 日志 | `~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log` | 结构化文本诊断信息（JSONL 模式下为 `core.jsonl`） |

Session 的 messages 和 rounds 永远不会存入 SQLite；`sessions` 表是摘要投影，可以通过
重放 JSONL transcript 重建。它当前的配置投影包括已选模型、模型的 `context_window` 和
reasoning effort。

## 领域层

领域层按 bounded context 拆分。Aggregates 之间只通过 identity 相互引用（conversation
系列使用 UUIDv7，providers 使用路径安全的 code，models 使用可保留上游字符的 code）。

```
pkg/domain/
├── shared/        Shared kernel: Code, ModelRef, ReasoningEffort, Metadata, Event, ID
├── conversation/  Session aggregate (Session -> Round -> Message), content blocks, events
├── catalog/       Provider aggregate (Provider -> Model)
└── mcp/           MCP server 配置、状态和工具投影
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

`pkg/infra/modelcall/` 负责一次无状态的 provider-neutral 模型调用。它只接收连接与模型配置
（base URL、API type、API key、model code 和能力）以及面向模型的上下文和 tools，返回已解析的
response 与 stream event。`pkg/infra/agentloop/` 负责原子化的 model/tool loop，并直接调用
`modelcall`。loop 负责连续调用、tool dispatch、usage 统计、取消和迭代上限，不包含 session
repository、model catalog、prompt renderer、tool registry、MCP client、skill scanner 或压缩策略。
`pkg/infra/session` 负责协调多个 session、持久化 round、资源、事件分发和 shutdown。

session host 构造 request，调用原子 loop，将 assistant response 和 tool result 转为 session
message，并将每个运行时事件交给 `OnEvent` 消费者。自定义 model 未填写时默认使用 `8192`
最大输出 token；内置 model 使用嵌入 catalog 中的精确限制。`pkg/infra/prompt` 负责渲染基础 system prompt；`pkg/infra/compaction`
负责 token 估算、阈值策略和临时 summary conversation，自动压缩与 `/compact` 共用同一
executor。压缩请求保持原有 system 和消息前缀，只在内存中追加一条 user 压缩指令，清空所有
工具定义，并通过 `modelcall.Call` 只调用模型一次。模型返回 tool call 时压缩失败，既不会执行也
不会持久化该调用。当前单个 round 最多执行 20 次 LLM/tool 迭代。
生产 registry 位于 `pkg/infra/tools`，实现 `ToolRuntime` port；同一批次内每个 tool call
并行执行，结果按调用顺序返回。`pkg/infra/tools/builtin/` 提供 `read_file`、`apply_patch`、`grep`、`glob`
和 `ls`，由 `cmd/main.go` 显式注册。`apply_patch` 会调用同名 Rust 可执行文件完成 V4A
解析和原子化文件修改。支持 free-form tool 的 provider 会收到模型工具定义；其他 provider
会在 system prompt 中收到通过 `shell` 执行同一命令的说明。相对路径基于该 round 捕获的
session 工作目录解析。

Session host 暴露生命周期，原子 loop 暴露单次调用的 hook port。`pkg/infra/middleware` 定义
平铺 hook 的 `Middleware` 结构和 `MiddlewareManager`；manager 按注册顺序收集非空 hook，
只编译一次，编译后拒绝继续注册。当前时机为 `BeforeSessionStart`、`BeforeRound`、
`AfterRound`、`BeforeModelCall`、`AfterModelCall`、`BeforeToolCall`、`AfterToolCall`、
`AfterSessionStop` 和 `OnEvent`。`BeforeRound` 在持久化 round 分配前执行，因此 middleware 可以在
round 写入前原子地修改输入内容、system prompt、tool runtime，或排队追加隐藏上下文。
Hook 可以替换为派生的 `context.Context`；manager 会沿链传递它，loop 会在后续模型和工具
调用中继续使用。生命周期 context 类型与编译后的生命周期 bridge 位于
`pkg/infra/middleware`；原子 loop
只接收执行期间所需的 model/tool hook bridge。AgentLoop 和各个 hook context 都能通过 event
emitter 广播事件，`OnEvent` 是这些事件共用的消费路径。

生产 middleware 与基础设施实现放在同一包：`infra/skill` 负责扫描 skill 并将会话级
skill catalog 拼接到 system prompt，`infra/mcp` 在每个轮次提供当前 MCP tool 快照，
`infra/metadata` 注入全量或变化的会话 metadata，`infra/compaction` 负责自动压缩策略，
`infra/tools` 负责可动态更新的 tool registry。`infra/storage` 在 `OnEvent` 中持久化 pending
session events，`infra/rpc` 将运行时事件投影为 CLI notification。`cmd/main.go` 按 Skill、MCP、
Metadata、Compaction、Storage、RPC notification 顺序注册它们，再把编译后的 hook chain 交给
`infra/session.Engine`，因此客户端收到 notification 前数据已经写入。

## 基础设施层

基础设施层（`pkg/infra/`）基于文件系统 + SQLite 存储模型实现领域 repositories。

```
pkg/infra/
├── config/             将配置文件和 env override 合并到单例中；解析 data-dir 路径
├── initialize/         OpenRepositories：一次性初始化所有 stores
├── catalogdata/        内嵌的 provider/model JSON
├── modelcall/          无状态模型调用 contract、protocol adapter 与 response 解析
├── agentloop/          直接调用 modelcall 的原子 model/tool loop
├── logging/            slog 初始化、环境配置解析和按日生成日志路径
├── storage/            Repository 实现 + SQLite connection factory
│   ├── db.go           OpenDB/OpenIsolatedDB + sessions schema
│   ├── catalog.go      CatalogRepository（内置 provider 与自定义 provider JSON）
│   ├── conversation.go ConversationRepository（JSONL transcript + SQLite projection）
│   └── middleware.go   Session 持久化事件消费者
├── compaction/         自动压缩 middleware 和请求阈值策略
├── mcp/                MCP client registry 及 stdio/Streamable HTTP/SSE transport
├── middleware/         Middleware contract、hook context 和 MiddlewareManager
├── metadata/            Session metadata middleware 和隐藏上下文消息
├── prompt/             基础 system prompt renderer
├── session/            Session execution host、持久化 round lifecycle 和 event dispatch
├── skill/              Skill discovery registry 和 skill middleware
├── tools/              可变基础设施 tool registry 和 round 快照
└── rpc/                stdio JSON-RPC 2.0 接口层和 notification 事件消费者
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

- `ProviderService`：provider CRUD 以及 model 子资源操作
  （`AddModel`/`RemoveModel`）。
- `InitializeService`：首次运行状态、默认模型持久化和完成校验。
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
| Skill | `skill.list` |
| Provider | `provider.create`, `provider.get`, `provider.list`, `provider.listModels`, `provider.update`, `provider.delete`, `provider.addModel`, `provider.removeModel` |
| Session | `session.create`, `session.get`, `session.list`, `session.delete`, `session.setTitle`, `session.setModel`, `session.setReasoningEffort`, `session.setCwd`, `session.start`, `session.compact`, `session.stop` |
| MCP | `mcp.list`, `mcp.get`, `mcp.logs`, `mcp.create`, `mcp.update`, `mcp.enable`, `mcp.reconnect`, `mcp.login`, `mcp.logout`, `mcp.remove` |
| Chunk | `chunk.begin`, `chunk.part`, `chunk.commit`, `chunk.abort` |

MCP server 配置位于 `<AGENTY_DATA_DIR>/mcp/<server-name>.json`，文件名就是 server 名称。
server 名称只允许 ASCII 字母、数字、`_` 和 `-`，且大小写不敏感（`GitHub` 与 `github`
冲突）。
`stdio` 使用 `command`、字符串数组形式的 `args`（每个进程参数一个元素）和可选的 `env`；`http` 和 `sse` 使用 Streamable HTTP 或 legacy SSE 的
`url` 和可选 `headers`。配置不保存 `cwd` 字段；`env` 和 `headers` 的值会在连接时展开 core 进程环境变量。

中心 registry 会以异步、有限并发方式连接启用的 server，并记录 `connecting`、`connected`、
`auth-required` 和 `error` 状态，通过 `mcp.event` notification 推送变化。Streamable HTTP
使用官方 SDK 的 OAuth authorization-code handler，自动进行动态 client 注册；`mcp.login`
会把 loopback callback 授权 URL 交给 CLI，并将 token 及刷新所需的 OAuth client 配置保存到 `mcp-auth`。
工具名称统一为 `mcp__<server-name>__<tool-name>`；不符合 provider 安全 ASCII 集合的符号会替换为 `_`，并截断到 64 个字符。每个 session round 开始时快照当时已连接的工具，因此较晚
完成的连接从下一轮开始可见。core 退出时会关闭所有 MCP session 及 stdio 子进程。
`mcp.logs` 返回 server 最近的有界内存诊断日志，包括连接错误和 stdio 子进程 stderr；配置中的敏感值会脱敏，日志不会持久化到磁盘。

`skill.list` 返回已发现的 skill registry 和非致命诊断信息。core 先扫描
`<AGENTY_DATA_DIR>/skills`（默认是 `~/.agenty/skills`），再扫描 `~/.agents/skills` 和
`~/.claude/skills`；前面目录中相同的文件系统目录名会覆盖后面的结果。skill 的展示名称和
描述来自 `SKILL.md` frontmatter 的 `name` 与 `description`。如果 frontmatter 的 name 与
目录名不同，skill 仍可被显式引用，但不会进入自动 system prompt catalog，并会产生警告。

`provider.list` 可选接收 `{providerCode}`。不传时，core 会并行获取所有已配置且 catalog
为空的 provider；传入时只会获取指定 provider。`provider.listModels` 接收同样的
`{providerCode}`，供直接调用者使用 core 内置的发现流程。成功结果仅缓存于运行中的 core 进程，
有效期为 8 小时，不写入磁盘，core 重启后不会保留。过期缓存仍作为旧数据返回，下一次 list 时
再刷新。它会兼容常见的 `id`、名称和 token 限制字段，自动跟随 provider 分页；上下文窗口或最大
输出 token 缺失或不为正数时分别使用 `256000` 和 `65536`，缺少 reasoning 能力时返回空的
`reasoningEfforts` 数组。事务性 `apply_patch` 锁位于 `~/.agenty/locks/`，每个锁都会记录 helper
进程 PID 和完整目标路径。

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
$ echo '{"jsonrpc":"2.0","id":1,"method":"provider.list","params":{}}' | go run ./cmd
{"jsonrpc":"2.0","id":1,"result":[...]}
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
pnpm core:tidyup            # 仅对 agenty-core 运行 go fmt、go vet 和 go mod tidy
pnpm core:clean             # 清理 agenty-core 构建产物
```

在仓库根目录运行 `pnpm tidyup`，可对所有 Go 模块执行相同的整理命令。

目前有意不提供本地 service 命令。End-to-end tests 使用 `e2e` build tag，因此不会进入
默认 `core:test` suite。

## 测试

默认 suite 覆盖 domain behavior、使用内存 repository fakes 的 application services、
protocol framing、配置和隔离的 storage adapter contracts。完整 repository 装配和
RPC-to-disk 路径使用 `integration` build tag。同一 build tag 还会启用可选的 LLM SDK
真实集成测试；测试直接读取环境中的 `OPENAI_API_KEY`、`ANTHROPIC_API_KEY` 和
`GEMINI_API_KEY`，未配置对应 Key 时会提示并跳过，不计为失败。

`test/e2e` package 只构建一次 `cmd`，通过 stdio 启动真实 binary，并为每个并行测试
进程分配独立的 `AGENTY_DATA_DIR`。它覆盖公开的 Provider/Model、Session、
agent loop 启停与并行执行、JSON-RPC、chunking、startup、restart persistence 和
process isolation contracts，不会访问用户的数据目录。

所有文件系统和 SQLite 测试都使用每个测试独立的临时目录。修改 `AGENTY_DATA_DIR` 的
测试不会并行运行，因为环境变量属于进程级全局状态。

完整测试策略和命令指南请参阅中文版 [TESTING-CN.md](TESTING-CN.md)，英文版见
[TESTING.md](TESTING.md)。

## 状态

领域层、agent loop 运行时、基础设施层、应用层和 stdio JSON-RPC 接口层均已实现。
基础设施层还提供 OpenAI Responses、OpenAI Chat Completions、Anthropic Messages 和 Google
GenAI 的统一无状态非流式/流式模型调用。执行引擎直接调用 `modelcall`；尚未实现 streaming
agent turn 传输、命令和 todo 工具、基于该 core 的 HTTP API 和 CLI 集成。
