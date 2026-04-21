# Service Layer Patterns

## Service Struct and Singleton Initialization

```go
type FooService struct {
    db *gorm.DB
    mu sync.RWMutex
    // other dependencies...
}

var (
    fooService *FooService
    fooOnce    sync.Once
)

func GetFooService() *FooService {
    fooOnce.Do(func() {
        fooService = &FooService{
            db: conn.GetDB(),
        }
    })
    return fooService
}
```

## GORM Basic Usage

### Always Attach Context

```go
// ✅ Correct
s.db.WithContext(ctx).First(&model, id)
s.db.WithContext(ctx).Create(&model)
s.db.WithContext(ctx).Save(&model)

// ❌ Wrong: missing context
s.db.First(&model, id)
```

### Transactions

```go
err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&foo).Error; err != nil {
        return err
    }
    if err := tx.Model(&bar).Updates(map[string]any{
        "field":      value,
        "updated_at": time.Now(),
    }).Error; err != nil {
        return err
    }
    return nil
})
```

### Record Not Found

```go
var model Foo
err := s.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
if errors.Is(err, gorm.ErrRecordNotFound) {
    return nil, customerrors.ErrFooNotFound
}
if err != nil {
    return nil, fmt.Errorf("failed to query foo: %w", err)
}
```

### Conditional Update (Prevent Empty Updates)

```go
result := s.db.WithContext(ctx).Model(&model).Updates(map[string]any{
    "name":       newName,
    "updated_at": time.Now(),
})
if result.Error != nil {
    return fmt.Errorf("failed to update: %w", result.Error)
}
```

## Raw Query Rules (Critical)

### `Raw().Rows()` Must Use `?` — Never Use `$N`

GORM counts `?` placeholders to determine how many arguments to bind. Using `$N` directly results in a count of 0, causing PostgreSQL to report "expected N arguments, got 0".

```go
// ✅ Correct: ? placeholders
rows, err := s.db.WithContext(ctx).Raw(`
    SELECT id, name, score
    FROM items
    WHERE name @@@ ? OR description @@@ ?
    ORDER BY score DESC
    LIMIT ?
`, query, query, limit).Rows()

// ❌ Wrong: $N placeholders
rows, err := s.db.WithContext(ctx).Raw(`
    SELECT id, name FROM items
    WHERE name @@@ $1 LIMIT $2
`, query, limit).Rows()
```

**Note**: when the same argument appears multiple times in the SQL, pass it multiple times in the args:
```go
// Three ? in the SQL → three arguments
.Raw(sql, query, query, limit)
```

### `Exec()` Can Use `$N`

`Exec()` passes SQL directly to the pgx driver, so `$N` works correctly:

```go
// ✅ $N works fine with Exec
deleteSQL := fmt.Sprintf(`DELETE FROM %s WHERE skill_md_path = $1`, tableName)
s.db.WithContext(ctx).Exec(deleteSQL, skillMDPath)

upsertSQL := fmt.Sprintf(`
    INSERT INTO %s (id, name, path)
    VALUES (uuidv7(), $1, $2)
    ON CONFLICT (path) DO UPDATE SET name = EXCLUDED.name
`, tableName)
s.db.WithContext(ctx).Exec(upsertSQL, name, path)
```

## ParadeDB BM25 Search

### Index Creation (After AutoMigrate in `pkg/conn/db.go`)

```go
// Static global table index
dbConn.Exec(`CREATE INDEX IF NOT EXISTS idx_skills_bm25
    ON skills
    USING bm25 (id, name, description)
    WITH (key_field = 'id')`)

// Dynamic session table index
createIndexSQL := fmt.Sprintf(`
    CREATE INDEX IF NOT EXISTS idx_%s_bm25
    ON %s
    USING bm25 (id, name, description)
    WITH (key_field = 'id')
`, indexSuffix, tableName)
```

### BM25 Search Query (Use `?` + Repeated Arguments)

```go
searchSQL := `
    SELECT id, name, description, skill_md_path, paradedb.score(id) as score
    FROM skills
    WHERE name @@@ ? OR description @@@ ?
    ORDER BY score DESC
    LIMIT ?
`
rows, err := s.db.WithContext(ctx).Raw(searchSQL, query, query, limit).Rows()
if err != nil {
    return nil, fmt.Errorf("skill search failed: %w", err)
}
defer rows.Close()

var results []models.SkillSearchResult
for rows.Next() {
    var r models.SkillSearchResult
    if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.SkillMDPath, &r.Score); err != nil {
        continue
    }
    results = append(results, r)
}
return results, nil
```

### Session Dynamic Table Search (Includes `scope` Column)

```go
searchSQL := fmt.Sprintf(`
    SELECT id, name, description, skill_md_path, scope, paradedb.score(id) as score
    FROM %s
    WHERE name @@@ ? OR description @@@ ?
    ORDER BY score DESC
    LIMIT ?
`, tableName)
rows, err := s.db.WithContext(ctx).Raw(searchSQL, query, query, limit).Rows()
// Scan includes &r.Scope
rows.Scan(&r.ID, &r.Name, &r.Description, &r.SkillMDPath, &r.Scope, &r.Score)
```

## Paginated Queries

Use the `pkg/utils/pagination` package:

```go
import "github.com/masteryyh/agenty/pkg/utils/pagination"

func (s *FooService) List(ctx context.Context, req *pagination.PageRequest) (*pagination.PagedResponse[FooDto], error) {
    var total int64
    var items []Foo

    query := s.db.WithContext(ctx).Model(&Foo{})
    query.Count(&total)
    query.Offset(req.Offset()).Limit(req.PageSize).Find(&items)

    dtos := make([]FooDto, len(items))
    for i, item := range items {
        dtos[i] = *item.ToDto()
    }
    return pagination.NewPagedResponse(dtos, total, req), nil
}
```

## File System Watching (fsnotify)

Use a debounce map to prevent duplicate processing of rapid file events:

```go
func (s *Service) runWatcher(ctx context.Context) {
    debounce := make(map[string]time.Time)
    const debounceDuration = 500 * time.Millisecond

    for {
        select {
        case <-ctx.Done():
            return
        case event, ok := <-s.watcher.Events:
            if !ok {
                return
            }
            if time.Since(debounce[event.Name]) < debounceDuration {
                continue
            }
            debounce[event.Name] = time.Now()
            s.handleEvent(ctx, event)
        case err, ok := <-s.watcher.Errors:
            if !ok {
                return
            }
            slog.ErrorContext(ctx, "watcher error", "error", err)
        }
    }
}
```
