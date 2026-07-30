# agenty-core

[English](README.md) | [简体中文](README-CN.md)

Agenty 的下一代核心运行时，正在从零构建以替代 `agenty-runtime`。它围绕本地优先的
存储模型（文件系统 + SQLite）和领域驱动设计（DDD）领域层进行设计。

## 存储模型

文件系统是真实数据源，SQLite 是查询侧投影。

| 数据 | 位置 | 作用 |
| --- | --- | --- |
| Session transcript | `~/.agenty/sessions/<yyyy>/<mm>/<dd>/<session-id>.jsonl` | 写模型，即 append-only event log（真实数据源） |
| Session index | `~/.agenty/agenty.sqlite` -> `sessions` | 读模型，用于快速列表和搜索的投影 |
| 全局配置 | `~/.agenty/config.json` | 应用配置 |
| Providers | `~/.agenty/providers/<slug>/provider.json` | Catalog aggregate |
| Models | `~/.agenty/providers/<slug>/models/<model-slug>.json` | Catalog aggregate member |
| Agents | `~/.agenty/agents/<slug>.json` | Agent aggregate |
| Core 日志 | `~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log` | 结构化文本诊断信息（JSONL 模式下为 `core.jsonl`） |

Session 的 messages 和 rounds 永远不会存入 SQLite；`sessions` 表是摘要投影，可以通过
重放 JSONL transcript 重建。它当前的配置投影包括已选模型、模型的 `context_window` 和
reasoning effort。

## 领域层

领域层按 bounded context 拆分。Aggregates 之间只通过 identity 相互引用（conversation
系列使用 UUIDv7，agents、providers 和 models 使用 kebab-case slugs）。

```
pkg/domain/
├── shared/        Shared kernel: Slug, ModelRef, ReasoningEffort, Metadata, Event, ID
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
`medium`、`high`、`xhigh` 和 `max`。模型通过 `reasoningEffortMapping` 对象保存映射，
其中 key 是 provider 原生 effort 名称，value 是 Agenty effort 等级：

```json
{
  "reasoningEffortMapping": {
    "none": "off",
    "minimal": "low",
    "low": "low",
    "medium": "medium",
    "high": "high"
  }
}
```

该映射允许多个原生 effort 归一化到同一个 Agenty effort。映射中没有启用任何 effort
的模型不支持 reasoning。只有上述六个 Agenty 等级可以作为映射值；原生 effort 名称
由 provider 自行定义。

## 基础设施层

基础设施层（`pkg/infra/`）基于文件系统 + SQLite 存储模型实现领域 repositories。

```
pkg/infra/
├── config/             将配置文件和 env override 合并到单例中；解析 data-dir 路径
├── initialize/         OpenRepositories：一次性初始化所有 stores
├── logging/            slog 初始化、环境配置解析和按日生成日志路径
├── storage/            Repository 实现 + SQLite connection factory
│   ├── db.go           OpenDB/OpenIsolatedDB + sessions schema
│   ├── agent.go        AgentRepository（agent JSON 文件）
│   ├── catalog.go      CatalogRepository（provider/model JSON 文件、DeleteModel）
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
| Agent | `agent.create`, `agent.get`, `agent.list`, `agent.update`, `agent.delete` |
| Provider | `provider.create`, `provider.get`, `provider.list`, `provider.update`, `provider.delete`, `provider.addModel`, `provider.removeModel` |
| Session | `session.create`, `session.get`, `session.list`, `session.delete`, `session.setTitle`, `session.setModel`, `session.setReasoningEffort`, `session.setCwd` |
| Chunk | `chunk.begin`, `chunk.part`, `chunk.commit`, `chunk.abort` |

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
$ echo '{"jsonrpc":"2.0","id":1,"method":"agent.create","params":{"slug":"dev","name":"Dev"}}' | go run ./cmd
{"jsonrpc":"2.0","id":1,"result":{"slug":"dev","name":"Dev",...}}
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
RPC-to-disk 路径使用 `integration` build tag。

`test/e2e` package 只构建一次 `cmd`，通过 stdio 启动真实 binary，并为每个并行测试
进程分配独立的 `AGENTY_DATA_DIR`。它覆盖公开的 Agent、Provider/Model、Session、
JSON-RPC、chunking、startup、restart persistence 和 process isolation contracts，
不会访问用户的数据目录。

所有文件系统和 SQLite 测试都使用每个测试独立的临时目录。修改 `AGENTY_DATA_DIR` 的
测试不会并行运行，因为环境变量属于进程级全局状态。

完整测试策略和命令指南请参阅中文版 [TESTING-CN.md](TESTING-CN.md)，英文版见
[TESTING.md](TESTING.md)。

## 状态

领域层、基础设施层、应用层和 stdio JSON-RPC 接口层均已实现。目前尚未实现基于该 core
的 HTTP API 和 CLI 集成。
