# agenty-core 测试指南

本文档说明 agenty-core 当前的测试情况和运行方式。英文版本见
[TESTING.md](./TESTING.md)。

## §1. 测试范围

| 范围 | 测试环境 | 覆盖行为 | 默认运行 |
| --- | --- | --- | --- |
| Domain | 仅内存值 | 聚合不变量、Session 状态转换与 replay、event 和 content 序列化、Provider model 生命周期、slug 和 thinking 校验 | 是 |
| Application | 内存 repository fake | Agent、Provider 和 Session 用例、输入校验、partial update、错误映射和 pending event 生命周期 | 是 |
| RPC | buffer、fake handler 和合成时间 | JSON-RPC/NDJSON framing、notification、batch、非法请求、单行限制、chunk 组装与清理 | 是 |
| Config 与 storage | `t.TempDir()`、真实文件和本地 SQLite | 配置发现、JSON repository、append-only transcript、SQLite projection 和 schema 初始化 | 是 |
| 完整装配 | 隔离的文件系统和 SQLite 状态 | repository 初始化，以及 RPC 到 application 再到 storage 的完整流程 | 启用 `integration` 时 |

当前 `integration` 构建标签会启用：

- `pkg/infra/initialize/initialize_test.go`，验证完整 repository 初始化和生命周期。
- `pkg/infra/rpc/adapter/adapter_test.go`，验证完整 RPC adapter 流程，其中包括分块输入。

测试套件有意跳过纯 DTO、简单结构体构造、薄 getter，以及只做字段赋值的构造器，
包括 `Agent.New`、`NewID`、`ModelRef.String` 和 `TokenUsage.Add`。命令装配和会终止
进程的 signal 路径也不属于单元测试范围。

## §2. 测试环境

- 需要 Go 1.26 或更高版本。
- `github.com/mattn/go-sqlite3` 要求启用 CGO 并提供可用的 C 编译器。
- 文件系统和 SQLite 测试使用独立临时目录，不会访问用户的 `~/.agenty` 目录。
- Application 测试使用互不共享的内存 repository fake。
- 设置 `AGENTY_DATA_DIR` 的测试不会并行运行，因为环境变量是进程级状态。
- Chunk 过期测试使用 `testing/synctest`，不等待真实时间。

Go 命令应在 `packages/agenty-core/` 下运行。模块 pnpm 命令可以在该目录直接运行；
从仓库根目录执行时，使用对应的 `pnpm core:*` 命令。

## §3. 运行测试

| 模块命令 | 根目录命令 | 用途 |
| --- | --- | --- |
| `pnpm test` | `pnpm core:test` | 所有不带 `integration` 或 `e2e` build tag 的测试 |
| `pnpm test:integration` | `pnpm core:test:integration` | 默认测试加 integration 测试 |
| `pnpm test:race` | `pnpm core:test:race` | 使用 race detector 且不复用结果缓存的默认测试 |
| `pnpm test:repeat` | `pnpm core:test:repeat` | 运行十次 shuffle，检查测试隔离性 |

未来的端到端测试必须使用 `e2e` build tag，确保 `pnpm core:test` 始终运行除复杂集成
测试和端到端环境测试以外的完整快速测试集。

对应的 Go 命令为：

```sh
go test ./...
go test -tags=integration ./...
go test -race -count=1 ./...
go test -shuffle=on -count=10 ./...
```

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

当前 integration 测试全部使用本地文件和 SQLite，不需要网络服务或单独维护的数据库。

测试涉及两个实现边界：

- `ConversationRepository.Save` 在 JSONL 追加成功、SQLite projection 更新失败时没有
  跨存储回滚。
- `Server.Serve` 取消后，阻塞在 input 上的 goroutine 只有在底层 reader 关闭后才会
  退出。
