# Data Model and Database Patterns

## GORM Model Definition

Persistent model structs should stay database-agnostic. Do not encode PostgreSQL-only types, defaults, indexes, or column constraints in `gorm` tags.

```go
type Foo struct {
    ID          uuid.UUID
    Name        string
    Description string
    Metadata    datatypes.JSON
    ParentID    *uuid.UUID
    CreatedAt   time.Time
    UpdatedAt   time.Time
    DeletedAt   *time.Time
}

func (Foo) TableName() string {
    return "foos"
}
```

Generate UUIDs in Go for new rows, normally through model `BeforeCreate` hooks in `pkg/models/hooks.go`. Keep JSON API tags lowerCamelCase and separate from persistence concerns.

## Schema SQL

Do not use GORM `AutoMigrate`. Static table DDL lives in embedded SQL files:

| Backend | File |
|---|---|
| PostgreSQL | `pkg/conn/db/postgres.sql` |
| SQLite | `pkg/conn/db/sqlite.sql` |

When adding a persistent table or index, update both files unless the feature is backend-specific. Use `CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS`.

PostgreSQL schema can use `UUID`, `JSONB`, `TIMESTAMPTZ`, `vector(1024)`, and ParadeDB BM25 indexes. SQLite schema should use `TEXT` UUIDs, `TEXT` JSON, `DATETIME`, FTS5 virtual tables for BM25-style search, and `BLOB` for sqlite-vector embeddings.

## Database Type Branches

Use `conn.GetDBType()` or the local service helper `usingSQLite()` when raw SQL must differ by backend. Prefer shared SQL where possible:

```go
query := "LOWER(content) LIKE LOWER(?)"
```

Use `CURRENT_TIMESTAMP` instead of `NOW()` in cross-backend raw SQL. For GORM expressions, use `conn.NowExpr()`.

## Raw Query Rules

GORM `Raw().Rows()` and `Exec()` should use `?` placeholders. Do not use `$1`/`$2`; SQLite does not share PostgreSQL parameter semantics, and this project standardizes on GORM binding.

```go
rows, err := s.db.WithContext(ctx).Raw(`
    SELECT id, name
    FROM items
    WHERE name = ?
    LIMIT ?
`, name, limit).Rows()
```

## Search Backends

Knowledge search must preserve the same user-facing contract across backends:

- PostgreSQL vector search uses pgvector ordering.
- SQLite vector search uses sqlite-vector and stores embeddings as Float32 BLOBs through `models.EmbeddingVector`.
- PostgreSQL BM25 uses ParadeDB.
- SQLite BM25-style search uses FTS5 virtual tables and `bm25(...)`.
- Keyword fallback should use backend-neutral SQL such as `LOWER(column) LIKE LOWER(?)`.

SQLite startup must verify FTS5 and sqlite-vector availability. If the sqlite-vector library is missing at `db.sqliteVectorExtensionPath` or the default user config path, startup should fetch the latest GitHub release, derive the asset name from the release version, OS, architecture, and Linux libc, then install it before loading. Asset names follow `vector-<os>[-musl]-<arch>-<version>.tar.gz`, where OS tags are `macos`, `linux`, `windows`, architecture tags are `x86_64` and `arm64`, and Linux musl uses `vector-linux-musl-...`. Do not silently degrade search features when the required extension cannot be installed or loaded.

## Dynamic Session Tables

Session skill tables are created manually in `SkillService`. Keep PostgreSQL and SQLite DDL branches together in the service. PostgreSQL session tables can use ParadeDB BM25 indexes; SQLite session tables need a sibling FTS5 table and triggers.

## DTO Design

```go
type FooDto struct {
    ID          uuid.UUID  `json:"id"`
    Name        string     `json:"name"`
    Description string     `json:"description"`
    ParentID    *uuid.UUID `json:"parentId,omitempty"`
    CreatedAt   time.Time  `json:"createdAt"`
    UpdatedAt   time.Time  `json:"updatedAt"`
}
```

Create/update DTOs should use lowerCamelCase JSON fields and Gin binding tags only for request validation.
