# Data Model and DTO Patterns

## GORM Model Definition

```go
type Foo struct {
    ID          uuid.UUID      `gorm:"type:uuid;default:uuidv7();primaryKey"`
    Name        string         `gorm:"type:varchar(255);not null"`
    Description string         `gorm:"type:text;not null"`
    Metadata    datatypes.JSON `gorm:"type:jsonb"`
    ParentID    *uuid.UUID     `gorm:"type:uuid"`           // nullable → pointer
    CreatedAt   time.Time      `gorm:"autoCreateTime:milli"`
    UpdatedAt   time.Time      `gorm:"autoUpdateTime:milli"`
    DeletedAt   *time.Time                                  // soft delete
}

func (Foo) TableName() string {
    return "foos"
}
```

### Field Type Reference

| Go Type | GORM Tag | PostgreSQL Type |
|---|---|---|
| `uuid.UUID` | `type:uuid;default:uuidv7()` | UUID |
| `string` | `type:varchar(255)` | VARCHAR(255) |
| `string` | `type:text` | TEXT |
| `bool` | `default:false` | BOOLEAN |
| `int64` | `not null;default:0` | BIGINT |
| `datatypes.JSON` | `type:jsonb` | JSONB |
| `time.Time` | `autoCreateTime:milli` | TIMESTAMPTZ |
| `*time.Time` | _(no extra tag)_ | TIMESTAMPTZ NULL |
| `*string` | `type:text` | TEXT NULL |
| `*uuid.UUID` | `type:uuid` | UUID NULL |

## DTO Design

```go
type FooDto struct {
    ID          uuid.UUID  `json:"id"`
    Name        string     `json:"name"`
    Description string     `json:"description"`
    ParentID    *uuid.UUID `json:"parentId,omitempty"`  // optional fields use omitempty
    CreatedAt   time.Time  `json:"createdAt"`
    UpdatedAt   time.Time  `json:"updatedAt"`
}

// Create request DTO
type CreateFooDto struct {
    Name        string  `json:"name" binding:"required"`
    Description string  `json:"description"`
    ParentID    *string `json:"parentId"`
}

// Update request DTO (all fields optional)
type UpdateFooDto struct {
    Name        *string `json:"name"`
    Description *string `json:"description"`
}
```

## Model → DTO Conversion

```go
func (m *Foo) ToDto() *FooDto {
    return &FooDto{
        ID:          m.ID,
        Name:        m.Name,
        Description: m.Description,
        ParentID:    m.ParentID,
        CreatedAt:   m.CreatedAt,
        UpdatedAt:   m.UpdatedAt,
    }
}
```

## AutoMigrate Registration (`pkg/conn/db.go`)

Append the new model to the `AutoMigrate` call in `InitDB`:

```go
if err := dbConn.AutoMigrate(
    &models.SystemSettings{},
    &models.ModelProvider{},
    &models.Model{},
    // ... existing models ...
    &models.Foo{},   // add here
); err != nil {
    return fmt.Errorf("failed to migrate database: %w", err)
}
```

Create indexes after AutoMigrate:

```go
if err := dbConn.Exec(`CREATE INDEX IF NOT EXISTS idx_foos_bar ON foos (bar_field)`).Error; err != nil {
    return fmt.Errorf("failed to create index: %w", err)
}
```

## JSONB Field Handling

```go
import (
    json "github.com/bytedance/sonic"
    "gorm.io/datatypes"
)

// Write
metaBytes, _ := json.Marshal(metaMap)
model.Metadata = datatypes.JSON(metaBytes)

// Read as map
var meta map[string]string
json.Unmarshal(model.Metadata, &meta)

// Read as slice
var levels []string
json.Unmarshal(model.ThinkingLevels, &levels)
```

## Dynamic Unlogged Tables (Session-Scoped)

Session tables are not managed by AutoMigrate; create them manually with `Exec`:

```go
func (s *Service) createSessionTable(ctx context.Context, sessionID uuid.UUID) error {
    tableName := fmt.Sprintf("session_items_%s", strings.ReplaceAll(sessionID.String(), "-", ""))

    createSQL := fmt.Sprintf(`
        CREATE UNLOGGED TABLE IF NOT EXISTS %s (
            id          UUID         PRIMARY KEY DEFAULT uuidv7(),
            name        VARCHAR(255) NOT NULL,
            scope       VARCHAR(20)  NOT NULL DEFAULT 'global',
            created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
            updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
        )
    `, tableName)

    if err := s.db.WithContext(ctx).Exec(createSQL).Error; err != nil {
        return fmt.Errorf("failed to create session table: %w", err)
    }

    // BM25 index — failure is non-fatal (warn only)
    indexSQL := fmt.Sprintf(`
        CREATE INDEX IF NOT EXISTS idx_%s_bm25
        ON %s
        USING bm25 (id, name)
        WITH (key_field = 'id')
    `, strings.ReplaceAll(sessionID.String(), "-", ""), tableName)

    if err := s.db.WithContext(ctx).Exec(indexSQL).Error; err != nil {
        slog.WarnContext(ctx, "failed to create BM25 index on session table",
            "table", tableName, "error", err)
    }
    return nil
}
```

## Backend Interface Extension

Every new feature must be reflected in all three files:

**1. `pkg/backend/backend.go` (interface declaration)**
```go
type Backend interface {
    // ...existing methods...
    ListFoos(sessionID uuid.UUID) ([]models.FooDto, error)
    GetFoo(fooID uuid.UUID) (*models.FooDto, error)
}
```

**2. `pkg/backend/local.go` (local implementation — calls the service)**
```go
func (l *LocalBackend) ListFoos(sessionID uuid.UUID) ([]models.FooDto, error) {
    return l.fooSvc.List(signal.GetBaseContext(), sessionID)
}
```

**3. `pkg/backend/remote.go` (remote implementation — calls the HTTP API)**
```go
func (r *RemoteBackend) ListFoos(sessionID uuid.UUID) ([]models.FooDto, error) {
    return r.client.ListFoos(sessionID)
}
```
