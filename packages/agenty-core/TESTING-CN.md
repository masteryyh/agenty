# agenty-core 测试指南

本文档说明 agenty-core 当前的测试情况和运行方式。英文版本见
[TESTING.md](./TESTING.md)。

## §1. 测试范围

| 范围 | 测试环境 | 覆盖行为 | 默认运行 |
| --- | --- | --- | --- |
| Domain | 仅内存值 | 聚合不变量、Session 状态转换与 replay、event 和 content 序列化、Provider model 生命周期、slug 和 reasoning effort 映射校验 | 是 |
| Application | 内存 repository fake | Agent、Provider 和 Session 用例；execution loop 完成、tool continuation、model 输出 token 上限、多 session 并行、取消、shutdown、输入校验、错误映射和 pending event 生命周期 | 是 |
| 内置工具 | `t.TempDir()` 和真实文件系统操作 | 注册、相对路径解析、范围读取、创建/覆盖、精确 patch、单文件删除、正则搜索、递归 glob、目录列表、输出限制和错误路径 | 是 |
| RPC | buffer、fake handler 和合成时间 | JSON-RPC/NDJSON framing、notification、batch、非法请求、单行限制、chunk 组装与清理 | 是 |
| Config、logging 与 storage | `t.TempDir()`、真实文件和本地 SQLite | 配置文件与 env override 合并、单例 Manager、日志等级/格式/路径选择、JSON repository、append-only transcript、SQLite projection 和 schema 初始化 | 是 |
| 完整装配 | 隔离的文件系统和 SQLite 状态 | repository 初始化，以及包括异步 session start/stop 在内的 RPC 到 application 再到 storage 完整流程 | 启用 `integration` 时 |
| 可执行 E2E | 真实 `cmd` 子进程、独立数据目录、typed IPC client、本地 provider fixtures 和可选真实上游 | 完整用户旅程、全部 28 个公开 RPC methods、有序 session event notifications、四类 provider 协议、内置工具 definitions、连续多轮、完成/失败/取消、同通道并发、运行中退出、重启持久化和 stdio 协议边界 | 启用 `e2e` 时 |

当前 `integration` 构建标签会启用：

- `pkg/infra/initialize/initialize_test.go`，验证完整 repository 初始化和生命周期。
- `pkg/infra/rpc/adapter/adapter_test.go`，验证完整 RPC adapter 流程，其中包括分块输入。

`e2e` build tag 会启用 `test/e2e`。`TestMain` 只构建一次 core 二进制，每个测试使用
唯一的 `AGENTY_DATA_DIR` 启动自己的进程。测试侧 typed client 使用公开 NDJSON 协议，
支持并发 request ID 路由、notification、batch 和 chunk，不导入 core 内部实现包；
`blackbox_test.go` 会持续检查这一依赖边界。

测试套件有意跳过纯 DTO、简单结构体构造、薄 getter，以及只做字段赋值的构造器，
包括 `Agent.New`、`NewID`、`ModelRef.String` 和 `TokenUsage.Add`。命令装配和会终止
进程的 signal 路径也不属于单元测试范围。

## §2. 测试环境

- 需要 Go 1.26 或更高版本。
- `github.com/mattn/go-sqlite3` 要求启用 CGO 并提供可用的 C 编译器。
- 文件系统和 SQLite 测试使用独立临时目录，不会访问用户的 `~/.agenty` 目录。
- Application 测试使用互不共享的内存 repository fake。
- 设置 `AGENTY_DATA_DIR`、`AGENTY_LOG_LEVEL` 或 `AGENTY_LOG_FORMAT` 的测试不会并行
  运行，因为环境变量是进程级状态。
- E2E 测试把数据目录设置到各自子进程的环境中，并清空日志环境变量，使子进程由配置
  文件（默认 info/text）驱动；不修改测试 runner 的环境，因此独立业务流程可以安全
  使用 `t.Parallel()`，日志也只会写入各自的隔离数据目录。
- Agent loop E2E 使用本地 `httptest` HTTP server 分别模拟 OpenAI Responses、OpenAI Chat
  Completions、Anthropic Messages 和 Google GenAI。执行环境必须允许绑定 loopback 端口；
  沙箱若拒绝 `listen`，需要在允许绑定的环境复跑相同命令。
- `TestLiveProviderConversationsThroughIPC` 使用同一 typed client 通过真实 core 子进程发起
  可选真实上游对话。每个 provider 独立检查对应 API Key；Key 未设置或只包含空白时用
  `t.Skip` 跳过该子测试，其他已配置 provider 继续运行。已设置但无效的 Key 会正常失败，
  不会被当作“未配置”静默跳过。
- Chunk 过期测试使用 `testing/synctest`，不等待真实时间。

Go 命令应在 `packages/agenty-core/` 下运行。模块 pnpm 命令可以在该目录直接运行；
从仓库根目录执行时，使用对应的 `pnpm core:*` 命令。

## §3. 运行测试

| 模块命令 | 根目录命令 | 用途 |
| --- | --- | --- |
| `pnpm test` | `pnpm core:test` | 所有不带 `integration` 或 `e2e` build tag 的测试 |
| `pnpm test:integration` | `pnpm core:test:integration` | 默认测试加 integration 测试 |
| `pnpm test:e2e` | `pnpm core:test:e2e` | 最多八路并行的真实二进制 E2E 测试 |
| `pnpm test:e2e:race` | `pnpm core:test:e2e:race` | 同时启用 race detector 的 E2E harness 和 core 二进制 |
| `pnpm test:race` | `pnpm core:test:race` | 使用 race detector 且不复用结果缓存的默认测试 |
| `pnpm test:repeat` | `pnpm core:test:repeat` | 运行十次 shuffle，检查测试隔离性 |

端到端测试使用 `e2e` build tag，确保 `pnpm core:test` 始终是排除复杂集成和进程环境
之后的完整快速测试集。

对应的 Go 命令为：

```sh
go test ./...
go test -tags=integration ./...
go test -tags=e2e -count=1 -parallel=8 ./test/e2e
go test -race -tags=e2e -count=1 -parallel=4 ./test/e2e
go test -race -count=1 ./...
go test -shuffle=on -count=10 ./...
```

LLM 真实 integration 和可选 live E2E 用例读取以下环境变量：

- `OPENAI_API_KEY`，可选 `OPENAI_BASE_URL`、`OPENAI_RESPONSES_MODEL` 和
  `OPENAI_CHAT_MODEL`。
- `ANTHROPIC_API_KEY`，可选 `ANTHROPIC_BASE_URL` 和 `ANTHROPIC_MODEL`。
- `GEMINI_API_KEY`，可选 `GEMINI_BASE_URL` 和 `GEMINI_MODEL`。

Integration 中的每个 Provider 子测试都会验证 `Invoke` 和 `Stream`；live E2E 则通过
stdio IPC 运行一个真实非流式 conversation。两者都在缺少对应 API Key 时通过 `t.Skip`
显示提示并跳过，不会导致 suite 失败。请求/响应转换测试和本地 provider fixture E2E
仍完全离线，不需要凭证。

开发时可以定向运行 package 或单个测试：

```sh
go test ./pkg/domain/conversation
go test ./pkg/domain/conversation -run '^TestSessionLifecycleAndReplay$' -count=1
```

改动跨层行为时，运行带 race detector 的 integration 测试：

```sh
go test -race -tags=integration -count=1 ./...
```

如果沙箱中的默认 Go cache 不可写，指定一个可写缓存：

```sh
GOCACHE=/private/tmp/agenty-core-go-cache go test ./...
```

使用以下命令生成 coverage 报告：

```sh
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
go tool cover -html=coverage.out
```

## §4. 当前状态与边界

2026-07-22 验证的默认测试快照为 70.1% statement coverage。其中
`pkg/domain/conversation` 为 92.8%，`pkg/infra/rpc` 为 91.8%，
`pkg/application` 为 76.4%。模块总覆盖率包含有意不测试的简单构造和 wiring 代码，
因此这里只记录快照。

Storage/RPC integration 测试和全部 E2E 测试使用本地文件与 SQLite。可选 LLM integration
和 `TestLiveProviderConversationsThroughIPC` 会访问外部服务，并且只在环境中存在对应
Provider API Key 时执行。其余 E2E 用例使用本地 provider fixtures。E2E 聚焦可观察的进程
contract；穷举 parser 变体、真实 64 MiB 单行限制和 chunk assembler 输入校验继续由更快的
RPC 测试覆盖，不在子进程中用大 payload 重复。

E2E system 将 core 视为由 stdin、stdout、stderr、退出码和公开 provider HTTP 请求组成的
黑箱。完整用户旅程通过 typed client 创建和修改 Agent、Provider/Model 与 Session，跨进程
连续执行多轮会话，再通过 IPC 查询持久化结果；不会断言 SQLite、JSONL 或 repository 的
物理布局。Provider fixtures 覆盖 OpenAI Responses、OpenAI Chat Completions、Anthropic
Messages 和 Google GenAI，并验证 65,536 输出 token 裁剪、上游失败、同一 IPC client 的
并发 session、重复启动/运行中删除拒绝、stop 取消和运行中进程退出后的恢复状态。

当前 28 个公开 methods 由用户旅程统一覆盖：Initialize 2 个、Agent 5 个、Provider 7 个、
Session 10 个和 Chunk 4 个。Session events、batch、精确 request ID、malformed JSON 恢复、末行无换行、stdin
EOF 和启动失败保留为进程级协议场景；穷举 parser 和 chunk 非法输入仍由低层 RPC 测试负责。

测试涉及两个实现边界：

- `ConversationRepository.Save` 在 JSONL 追加成功、SQLite projection 更新失败时没有
  跨存储回滚。
- `Server.Serve` 取消后，阻塞在 input 上的 goroutine 只有在底层 reader 关闭后才会
  退出。
