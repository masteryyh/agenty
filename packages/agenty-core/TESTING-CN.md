# agenty-core 测试指南

本文档说明当前 HTTP/2 core 测试。英文版见 TESTING.md。

## 测试范围

| 范围 | 测试环境 | 覆盖内容 | 构建标签 |
| --- | --- | --- | --- |
| Domain 与 Application | 内存值和 repository fake | 聚合不变量、初始化、provider/session 用例、round 生命周期、取消、审批决策、并发和错误映射 | 默认 |
| HTTP 响应与 Stream Broker | httptest、本地 HTTP/2、合成时间 | 统一响应体、任意值/null 数据、编码失败、全双工流命令、topic 共享序号、重放、游标过期、快照、慢消费者和持久化交接 | 默认 |
| Config、日志、存储 | 临时目录和本地 SQLite | 配置/env 合并、日志初始化、JSONL transcript、projection 重建和 repository 持久化 | 默认 |
| 完整装配 | 隔离文件系统和 SQLite | Repository 初始化及 application 到 storage 的完整流程 | integration |
| 可执行 E2E | 真实 core 子进程和隔离数据目录 | REST 生命周期、真实 HTTP/2 传输、事件顺序、CRUD 旅程、provider fixture、取消、并发、重启持久化和关闭 stdin 行为 | e2e |
| Bun 与编译 CLI 传输 | Windows 命名管道、真实 Bun 和编译 CLI | 双向 Node HTTP/2、事件流持续打开时发 REST 请求、编译后 CLI 握手 | Windows 可选 |
| Bootstrap 生命周期 | 真实释放的 core/CLI 子进程 | 同目录附着、不同目录并行所有者、CLI/core/bootstrap 退出后的清理 | Windows 忽略测试 |

E2E 测试 client 只使用公开 HTTP 资源，不导入 core 内部实现包，并检查 REST 路径与稳定错误码。
项目不再包含 JSON-RPC dispatcher 或分块组装器。

## 测试环境

- 需要 Go 1.26 或更新版本。
- SQLite 依赖要求启用 CGO 并安装 C 编译器。
- 测试使用临时数据根目录，不会访问或修改用户的 ~/.agenty。
- 修改进程环境的测试不会并行运行。
- E2E 子进程使用各自的 AGENTY_DATA_DIR 和日志配置。
- Provider E2E fixture 会绑定 loopback HTTP 端口；禁止本地监听的沙箱需要在允许的环境中重跑。
- 可选真实 provider 测试读取 OPENAI_API_KEY、ANTHROPIC_API_KEY 和 GEMINI_API_KEY。缺少 Key
  时跳过对应测试；已配置但无效的 Key 会正常失败。
- Windows 命名管道测试需要设置 AGENTY_BUN_BIN 和 AGENTY_COMPILED_CLI。未同时设置时，可选
  集成测试会跳过。
- Bootstrap 生命周期测试会启动并终止真实子进程，因此标记为 ignored；可在 Windows 开发
  环境中显式选择运行。
- 本地 MCP stdio fixture 是外部子进程，使用隔离的测试资源。

## 命令

在 packages/agenty-core 目录运行 Go 命令：

- go test ./...
- go test -tags=integration ./...
- go test -tags=e2e -count=1 -parallel=8 ./test/e2e
- go test -race -tags=e2e -count=1 -parallel=4 ./test/e2e
- go test -race -count=1 ./...
- go test -shuffle=on -count=10 ./...

从仓库根目录可以运行 pnpm core:test、core:test:integration、core:test:e2e、
core:test:e2e:race、core:test:race 或 core:test:repeat。

要运行 Windows Bun/CLI 命名管道检查，设置 AGENTY_BUN_BIN 和 AGENTY_COMPILED_CLI，
然后运行 httpapi package 中名为 TestBunAndCompiledCLIUseNamedPipeHTTP2Bidirectionally
的测试。

Bootstrap 单元测试使用 packages/agenty-bootstrap 下的 cargo test。CLI 检查使用
pnpm cli:typecheck 和 CLI package 的 Bun 测试。使用 pnpm bootstrap:build 或 pnpm build
构建整合后的 launcher。

## 验证边界

标准 Go 测试使用本地 fixture，不需要 provider 凭据。传输测试通过只说明命名管道上的
HTTP/2 连接和全双工行为正常；单独的传输测试不能证明完整 provider 用户旅程。E2E suite
会使用本地 provider fixture 实际启动 core 进程。Release 签名、macOS/Linux 进程生命周期和
真实上游 provider 可用性，需要对应的运行环境。
