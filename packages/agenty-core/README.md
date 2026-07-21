# agenty-core

The next-generation core runtime for Agenty, being built from scratch to replace
`agenty-runtime`. It is designed around a local-first storage model (filesystem +
SQLite) and a Domain-Driven Design (DDD) domain layer.

## Storage model

The filesystem is the source of truth; SQLite is a query-side projection.

| Data | Location | Role |
| --- | --- | --- |
| Session transcript | `~/.agenty/sessions/<yyyy>/<mm>/<dd>/<session-id>.jsonl` | Write model — append-only event log (source of truth) |
| Session index | `~/.agenty/agenty.sqlite` → `sessions` | Read model — projection for fast listing/search |
| Global config | `~/.agenty/config.json` | Application configuration |
| Providers | `~/.agenty/providers/<slug>/provider.json` | Catalog aggregate |
| Models | `~/.agenty/providers/<slug>/models/<model-slug>.json` | Catalog aggregate member |
| Agents | `~/.agenty/agents/<slug>.json` | Agent aggregate |

A session's messages and rounds are never stored in SQLite; the `sessions` table is a
summary projection that can be rebuilt by replaying the JSONL transcript. Its current
configuration projection includes the selected model, its `context_window`, and the
thinking effort.

## Domain layer

The domain layer is split by bounded context. Aggregates reference each other only by
identity (UUIDv7 for the conversation family, kebab-case slugs for agents, providers,
and models).

```
pkg/domain/
├── shared/        Shared kernel: Slug, ModelRef, ThinkingEffort, Metadata, Event, ID
├── conversation/  Session aggregate (Session → Round → Message), content blocks, events
├── agent/         Agent aggregate
└── catalog/       Provider aggregate (Provider → Model)
```

The conversation transcript is event-sourced: each JSONL line is a domain event
(`session_started`, `session_model_set`, `session_thinking_effort_set`,
`session_cwd_set`, `round_started`, `message_appended`, `round_ended`, ...), and a
`Session` aggregate can be reconstructed with `conversation.ReplaySession`. A Session
holds the current configuration for future rounds, while `RoundStarted` snapshots the
model, context window, thinking effort, and working directory used by that round.

## Infrastructure layer

The infrastructure layer (`pkg/infra/`) implements the domain repositories using the
filesystem + SQLite storage model.

```
pkg/infra/
├── config/             Load ~/.agenty/config.json and resolve data-dir paths
├── initialize/         OpenRepositories: one-call setup of all stores
└── storage/            Repository implementations + SQLite singleton
    ├── db.go           OpenDB/GetDB/CloseDB: singleton *sql.DB + sessions schema
    ├── agent.go        AgentRepository (agent JSON files)
    ├── catalog.go      CatalogRepository (provider/model JSON files)
    └── conversation.go ConversationRepository (JSONL transcript + SQLite projection)
```

## Status

The domain and infrastructure layers are implemented. The application and interface
layers (use-case services, HTTP API, CLI integration) are not yet implemented.
