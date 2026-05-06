# Go Syntax and Coding Conventions

## Apache 2.0 License Header (Required in Every New Go Source File)

```go
/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
```

## Type Conventions

### Use `any` Instead of `interface{}`

```go
// ✅ Correct
func Process(val any) error { ... }
m := map[string]any{"key": value}

// ❌ Wrong
func Process(val interface{}) error { ... }
m := map[string]interface{}{"key": value}
```

### Built-in Functions (Go 1.21+)

```go
// ✅ Use built-in min/max
n := min(a, b)
n := max(a, b, c)

// ❌ Do not implement manually
func minInt(a, b int) int { if a < b { return a }; return b }

// ❌ Do not use if/else
if n < a {
    n = a
} else if n > b {
    n = b
}
```

## String Building

### Use `fmt.Fprintf` Instead of `sb.WriteString`

```go
// ✅ Correct: use fmt.Fprintf
var sb strings.Builder
fmt.Fprintf(&sb, "Found %d result(s):\n\n", len(results))
for i, r := range results {
    fmt.Fprintf(&sb, "%d. %s\n", i+1, r.Name)
    fmt.Fprintf(&sb, "   Score: %.2f\n", r.Score)
}
return sb.String(), nil

// ❌ Wrong: do not use sb.WriteString for formatted strings
sb.WriteString(fmt.Sprintf("Found %d result(s):\n\n", len(results)))
```

## JSON Serialization

The project uses `github.com/bytedance/sonic` as its JSON library. **Always import it aliased as `json`**:

```go
import (
    json "github.com/bytedance/sonic"
)

// Usage is identical to the standard library
data, err := json.Marshal(v)
err = json.Unmarshal(b, &v)
```

JSON field names and JSON tags for API/tool request and response payloads must use lowerCamelCase:

```go
type SearchResultDto struct {
    SearchID     string `json:"searchId"`
    RelativePath string `json:"relativePath,omitempty"`
    StartLine    int    `json:"startLine,omitempty"`
}
```

## Error Handling

```go
// Wrap errors to preserve the call chain
return fmt.Errorf("failed to create skill: %w", err)

// Check for specific errors
if errors.Is(err, gorm.ErrRecordNotFound) { ... }
if errors.As(err, &businessErr) { ... }

// Business errors (see pkg/customerrors/errors.go)
return customerrors.ErrNotFound
return customerrors.NewBusinessError(400, "custom message")
```

## Context Propagation

`context.Context` must be the first parameter of any I/O-intensive or blocking operation:

```go
func (s *FooService) DoSomething(ctx context.Context, id uuid.UUID) (*Foo, error) {
    return s.db.WithContext(ctx).First(&foo, id).Error
}
```

## Logging (slog)

```go
// Use the Context variant when a context is available
slog.InfoContext(ctx, "skill scanned", "count", len(skills))
slog.WarnContext(ctx, "watcher error", "path", path, "error", err)
slog.ErrorContext(ctx, "failed to upsert", "error", err)

// Without a context
slog.Info("service initialized")
slog.Error("panic recovered", "error", r)
```

## Safe Goroutine Launch

**Never** use bare `go func()`. Always use the `safe` package:

```go
import "github.com/masteryyh/agenty/pkg/utils/safe"

// With context (cancellable; panics trigger automatic restart)
safe.GoSafeWithCtx("watcher-name", ctx, s.runWatcher)

// Without context (uses the global base context)
safe.GoSafe("background-job", s.runJob)

// Fire-and-forget goroutine that should not restart
safe.GoOnce("cleanup", func() { ... })
```

## Singleton Pattern

Services, routes, and similar singletons follow this exact pattern:

```go
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

## Avoid Over-Encapsulation

Inline simple logic that is only used once — do not extract it into a helper function:

```go
// ✅ Inline it
rows, err := s.db.WithContext(ctx).Raw(`SELECT ...`, args...).Rows()

// ❌ Unnecessary wrapper
func (s *Service) queryRows(ctx context.Context, sql string, args ...any) (*sql.Rows, error) {
    return s.db.WithContext(ctx).Raw(sql, args...).Rows()
}
```

## Global Base Context

In local/daemon mode, use `signal.GetBaseContext()` to obtain the global context (requires `signal.SetupContext()` to have been called first):

```go
import "github.com/masteryyh/agenty/pkg/utils/signal"

ctx := signal.GetBaseContext()
```

`LocalBackend` methods that have no context parameter use it internally:

```go
func (l *LocalBackend) ListProviders(page, pageSize int) (...) {
    return l.providerSvc.ListProviders(signal.GetBaseContext(), pageReq(page, pageSize))
}
```
