# agenty-core

Go core 负责本地配置、provider/model catalog、持久会话、agent 执行、工具、skills 和 MCP
连接。CLI 通过本地 HTTP/2 访问 core；这里不再提供 stdio RPC 或远程客户端兼容层。

## 存储模型

文件系统是事实来源，SQLite 是可重建的查询投影。

| 数据 | 默认位置 | 用途 |
| --- | --- | --- |
| Session transcript | ~/.agenty/sessions/<yyyy>/<mm>/<dd>/<session-id>.jsonl | 只追加的领域事件日志 |
| Session 索引 | ~/.agenty/agenty.sqlite | 列表和搜索用的查询投影 |
| 全局配置 | ~/.agenty/config.json | 应用默认配置 |
| Provider | 内置 catalog 和 ~/.agenty/providers | 内置项只读；自定义 provider 使用 JSON 文件 |
| MCP server | ~/.agenty/mcp | 每个 server 一个配置文件 |
| MCP 凭据 | ~/.agenty/mcp-auth | OAuth 凭据与 server 配置分开保存 |
| 日志 | ~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log | 结构化诊断日志 |
| Runtime | ~/.agenty/runtime | 运行锁和发现元数据 |

AGENTY_DATA_DIR 用于切换数据根目录。Core 会先规范化目录并持有目录专属的进程锁，再初始化
业务存储。同一规范化目录只能有一个 core 所有者，不同目录可以并行运行。Runtime metadata
只是发现线索，客户端仍会核对 core 的真实握手信息。

## 架构

- pkg/domain：provider-neutral 的 conversation、catalog、MCP 和 shared types。
- pkg/application：初始化、provider 和 session 用例。
- pkg/infra/modelcall：把各 provider 协议适配到无状态 model-call 接口。
- pkg/infra/agentloop：model/tool 循环、usage 统计、取消和迭代限制。
- pkg/infra/session：并发 session、持久 round 和事件 hook。
- pkg/infra/storage：JSONL repository、SQLite projection 和持久化 hook。
- pkg/api/routes：统一注册 Gin REST 路由和 handler，并承载 HTTP/2 流端点。
- pkg/api/middlewares：提供通用 HTTP 请求日志和 panic 恢复。
- pkg/infra/httpapi：提供事件重放、快照、会话同步屏障和执行事件投影。
- pkg/infra/instance：数据目录规范化、运行锁和 IPC 地址。
- cmd/main.go：装配 repository、中间件、application services 和 listener。

Session transcript JSONL 是事实来源。发布对应流事件前先持久化变更；SQLite 可通过回放
transcript 重建。

## 本地 API

Core 使用 HTTP/2 prior knowledge；macOS/Linux 通过 Unix domain socket，Windows 通过
byte-mode 命名管道。CLI 使用 Node 的 HTTP/2 和 net API。Bootstrap 启动或附着 core，核对
数据目录及协议身份后，把真实终端 stdio 交给 CLI。

Bootstrap 只向子进程提供以下 IPC 环境变量：

- AGENTY_TRANSPORT：uds 或 named_pipe。
- AGENTY_CORE_ADDR：socket 路径或命名管道地址。
- AGENTY_DATA_DIR：规范化数据目录。
- AGENTY_IPC_VERSION：IPC 兼容版本。

API contract 为 v1。GET /v1/system 返回进程身份、规范化数据目录、初始化状态、API contract、
IPC 版本和事件流 ID。Bootstrap IPC 环境缺失或不兼容时，CLI 会明确报错。

JSON CRUD 资源使用 lowerCamelCase 字段。普通响应统一为 `{"code":200,"message":"ok","data":...}`：
code 与 HTTP 状态一致，data 可为任意 JSON 值，包括 null。失败响应保留 HTTP 错误状态，在
message 中返回错误信息，data 为 null，并包含 not_found 等稳定的 errorCode。CLI 和 bootstrap
读取 data；NDJSON 事件帧仍采用独立格式。路由分组：

| 资源 | 路由 |
| --- | --- |
| 初始化 | GET、POST /v1/initialization |
| Provider 和 Model | /v1/providers 及 /v1/providers/{code}/models |
| Session 和 Round | /v1/sessions、session 设置、/rounds、/rounds/{roundId}/cancel、/compact |
| Skills | GET /v1/skills |
| MCP | /v1/mcp 及 server 的 get/update/logs/enable/reconnect/login/logout |
| 工具审批 | POST /v1/tool-approvals/{approvalId}/resolution |
| Core 生命周期 | GET /v1/system、POST /v1/system/shutdown |

创建 round 会立即返回 202 Accepted 和 round ID，执行在后台继续。取消请求同时指定 session 和
round，因此迟到的旧取消请求不会停止后续 round。同一 session 同时只能执行一个 round，不同
session 可以并行。客户端断开不会取消已接受的任务。

## 事件流

POST /v1/stream 是真正的双向 HTTP/2 流。客户端保持请求 body 打开，持续发送换行分隔的订阅/
取消订阅命令；core 尽早 flush 响应头，并随事件到达持续写出换行分隔的 frame。此分帧仅用于
事件流，不是 RPC envelope 或 JSON 隧道。

支持的 topic 是 mcp 和 session:<sessionId>。每个 topic 有共享给所有消费者的单调递增 sequence。
游标由不透明 streamId 和最后成功应用的 sequence 组成。客户端忽略重复事件并检测缺口。带有效
游标订阅会先重放保留事件，再原子切换到实时通知。游标已过期、属于其他 stream 或超前于 topic
时，core 会先发当前快照和一致游标，再继续发送实时事件。Core 重启后 streamId 改变；客户端
根据持久状态和当前运行状态重新同步。

快照包含 session 投影、当前执行状态及审批状态。有限的重放存储和消费者队列避免慢消费者阻塞
执行；慢消费者会断开，并可从游标恢复或请求新快照。Round 事件包括持久化消息、模型输出、
工具与审批变化、压缩状态和终态。领域 round sequence、JSONL sequence 与 HTTP 事件 topic
sequence 各自独立。

## 功能

Provider 支持 CRUD、model discovery、添加/删除 model 和全局初始化。Session 支持创建、列表、
读取、删除、标题/model/reasoning/cwd/权限配置、Codex mode、异步 round、取消和压缩。Skills
从配置的数据目录和用户 skills 目录发现。MCP 支持 stdio、Streamable HTTP 和 legacy SSE 外部
server transport。CLI 启动的 MCP stdio 子进程与 core IPC 无关。

权限中间件支持 ask、auto 和 yolo。待审批状态保存在内存中，第一个有效决策生效，resolved
事件会同步给所有客户端。每个 round 启动时固定当时的工作目录和工具快照。

Model/tool loop 使用 OpenAI Responses、OpenAI Chat Completions、Anthropic Messages 和 Google
GenAI adapter。模型输出会实时推送给订阅者，完成的消息和 round 生命周期会持久化。

## 开发与测试

从仓库根目录运行工作区脚本：

- pnpm core:build
- pnpm core:test
- pnpm core:test:integration
- pnpm core:test:e2e
- pnpm core:test:e2e:race
- pnpm core:test:race
- pnpm core:test:repeat
- pnpm core:tidyup

Core 的 SQLite 驱动使用 cgo，因此构建必须启用 CGO。构建脚本默认设置 `CGO_ENABLED=1`，
并拒绝 `CGO_ENABLED=0`。Windows 需要 MinGW-w64 或 LLVM-MinGW 等 GCC 兼容编译器；MSVC
`cl.exe` 不能用于构建 core。构建前可在当前进程中设置 `CC`，如果工具链还需要其他程序，
也把其 `bin` 目录临时加入当前进程的 `PATH`：

```powershell
$env:CC = 'C:\path\to\llvm-mingw\bin\clang.exe'
$env:PATH = "C:\path\to\llvm-mingw\bin;$env:PATH"
pnpm core:build
```

`CC` 和 `CXX` 会传给 Go，并纳入 Turbo 构建缓存键。

在 packages/agenty-core 运行 Go 命令。默认测试覆盖 domain、用例、HTTP 路由与事件 broker、
middleware 和 storage。Integration 测试验证完整 repository 装配。e2e build tag 在隔离临时
数据目录中启动真实 core，验证 REST 请求、事件流、provider fixture、并发、取消、重启持久化
和进程行为。可选 live-provider 测试需要 provider 凭据；缺少 Key 时会跳过。

Windows HTTP/2 集成测试可以额外通过命名管道验证真实 Bun client 和编译后的 CLI，运行时需设置
AGENTY_BUN_BIN 与 AGENTY_COMPILED_CLI。真实启动/终止子进程的 bootstrap 生命周期测试默认忽略，
可在 Windows 开发环境中显式选择运行。

完整测试矩阵见 TESTING-CN.md，英文说明见 TESTING.md。
