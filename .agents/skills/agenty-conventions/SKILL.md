---
name: agenty-conventions
description: Project-specific coding conventions and patterns for the agenty repository. Covers Go 1.26 style rules, GORM query patterns, Gin route conventions, tool registration, model/DTO design, and correct ParadeDB BM25 search usage. Must be loaded whenever adding new code to the agenty codebase.
license: Apache-2.0
metadata:
  author: masteryyh
  version: "1.0.0"
  domain: project-conventions
  triggers: agenty, Go service, GORM, Gin route, tool registration, BM25, ParadeDB, service pattern, model DTO
  role: specialist
  scope: implementation
  output-format: code
---

# agenty Project Coding Conventions

agenty is an AI Agent application written in Go 1.26. It consists of a backend service (Gin + GORM + PostgreSQL/ParadeDB) and a CLI client, supporting tool calling, agentic looping, and a skills system.

## Hard Rules (Non-Negotiable)

1. **All new Go source files must include the Apache 2.0 license header** (see `references/go-conventions.md`)
2. **No redundant comments** unless explicitly requested by the user
3. **No summary documents or planning documents**
4. **Always reply to the user in Simplified Chinese**
5. Use `any` instead of `interface{}`
6. Use `fmt.Fprintf` instead of `strings.Builder.WriteString()`
7. GORM `Raw().Rows()` must use `?` placeholders — **never use `$1`/`$2` directly**

## Package Structure Quick Reference

| Package Path | Responsibility |
|---|---|
| `pkg/models/` | GORM data models + DTO structs |
| `pkg/services/` | Business logic, singleton pattern |
| `pkg/routes/` | Gin route handlers, singleton pattern |
| `pkg/chat/tools/builtin/` | Built-in tool implementations |
| `pkg/chat/tools/tool.go` | Tool interface definition and global registry |
| `pkg/conn/` | DB, HTTP client, SSE, GORM logger |
| `pkg/customerrors/` | Business error definitions |
| `pkg/utils/response/` | Gin unified response helpers |
| `pkg/utils/safe/` | Safe goroutine launcher |
| `pkg/utils/signal/` | Global base context |
| `pkg/consts/` | Constants; tool description strings go here |
| `pkg/backend/` | Backend interface + Local/Remote implementations |

## Reference Index

| Topic | File | Load When |
|---|---|---|
| Go syntax conventions | `references/go-conventions.md` | Creating or modifying any Go file |
| Service layer patterns | `references/service-patterns.md` | Adding/modifying `pkg/services/` |
| Route patterns | `references/route-patterns.md` | Adding/modifying `pkg/routes/` |
| Tool patterns | `references/tool-patterns.md` | Adding a new built-in tool |
| Database patterns | `references/database-patterns.md` | Working with GORM, BM25, or ParadeDB |

## Implementation Checklist

Verify each item when adding new functionality:

- [ ] New Go files have the Apache 2.0 license header
- [ ] `any` is used instead of `interface{}`
- [ ] `strings.Builder` uses `fmt.Fprintf`, not `sb.WriteString`
- [ ] GORM `Raw().Rows()` uses `?` placeholders
- [ ] New services are singletons initialized with `sync.Once`, exposing a `GetXxxService()` function
- [ ] New routes are singletons implementing `RegisterRoutes(*gin.RouterGroup)`
- [ ] Route responses use `response.OK` / `response.Failed`
- [ ] Errors use business error types from `customerrors`
- [ ] All blocking operations take `context.Context` as the first parameter
- [ ] Background goroutines use `safe.GoSafe` or `safe.GoSafeWithCtx`
- [ ] New Backend interface methods are implemented in both `local.go` and `remote.go`
