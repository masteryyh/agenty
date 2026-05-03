# Tool Registration and Implementation Patterns

## Tool Interface (`pkg/chat/tools/tool.go`)

```go
type Tool interface {
    Definition() ToolDefinition
    Execute(ctx context.Context, tcc ToolCallContext, arguments string) (string, error)
}

type ToolCallContext struct {
    AgentID   uuid.UUID
    SessionID uuid.UUID
    ModelID   uuid.UUID
    ModelCode string
    Cwd       string
}
```

## Steps to Add a New Built-in Tool

### 1. Define the Tool Description String in `pkg/consts/`

Long tool descriptions belong in `pkg/consts/prompts.go`:

```go
const (
    FindSkillToolDescription = `Search for available skills by keyword...`
    MyNewToolDescription     = `Does something specific...`
)
```

### 2. Create a New File in `pkg/chat/tools/builtin/`

Name the file `<tool_name>.go`, e.g., `foo.go`:

```go
/*
Copyright © 2026 masteryyh <yyh991013@163.com>
...（Apache license header）
*/

package builtin

import (
    "context"
    "fmt"
    "strings"

    json "github.com/bytedance/sonic"
    "github.com/masteryyh/agenty/pkg/tools"
    "github.com/masteryyh/agenty/pkg/consts"
    "github.com/masteryyh/agenty/pkg/services"
)

type FooTool struct {
    fooService *services.FooService
}

func (t *FooTool) Definition() tools.ToolDefinition {
    return tools.ToolDefinition{
        Name:        "foo_tool",
        Description: consts.FooToolDescription,
        Parameters: tools.ToolParameters{
            Type: "object",
            Properties: map[string]tools.ParameterProperty{
                "query": {
                    Type:        "string",
                    Description: "The search query.",
                },
                "limit": {
                    Type:        "integer",
                    Description: "Max number of results (default: 10).",
                },
            },
            Required: []string{"query"},
        },
    }
}

func (t *FooTool) Execute(ctx context.Context, tcc tools.ToolCallContext, arguments string) (string, error) {
    var args struct {
        Query string `json:"query"`
        Limit int    `json:"limit"`
    }
    if err := json.Unmarshal([]byte(arguments), &args); err != nil {
        return "", fmt.Errorf("invalid arguments: %w", err)
    }
    if strings.TrimSpace(args.Query) == "" {
        return "", fmt.Errorf("query cannot be empty")
    }
    if args.Limit <= 0 {
        args.Limit = 10
    }

    results, err := t.fooService.Search(ctx, &tcc.SessionID, args.Query, args.Limit)
    if err != nil {
        return "", fmt.Errorf("search failed: %w", err)
    }
    if len(results) == 0 {
        return "No results found.", nil
    }

    var sb strings.Builder
    fmt.Fprintf(&sb, "Found %d result(s):\n\n", len(results))
    for i, r := range results {
        fmt.Fprintf(&sb, "%d. **%s**\n", i+1, r.Name)
        fmt.Fprintf(&sb, "   %s\n\n", r.Description)
    }
    return sb.String(), nil
}
```

### 3. Register in `RegisterAll` in `builtin.go`

```go
func RegisterAll(registry *tools.Registry) {
    // existing tools...
    registry.Register(&FindSkillTool{skillService: services.GetSkillService()})

    // new tool
    registry.Register(&FooTool{fooService: services.GetFooService()})
}
```

## Parameter Type Reference

```go
// String
"query": tools.ParameterProperty{Type: "string", Description: "..."}

// Integer
"limit": tools.ParameterProperty{Type: "integer", Description: "..."}

// Boolean
"recursive": tools.ParameterProperty{Type: "boolean", Description: "..."}

// String enum (document allowed values in Description)
"scope": tools.ParameterProperty{
    Type:        "string",
    Description: `Scope filter. One of: "global", "project", "all" (default: "all").`,
}

// Array
"paths": tools.ParameterProperty{
    Type:        "array",
    Items:       &tools.ParameterProperty{Type: "string"},
    Description: "List of file paths.",
}
```

## Argument Parsing Pattern (Anonymous Struct)

```go
var args struct {
    Query     string `json:"query"`
    Limit     int    `json:"limit"`
    Recursive bool   `json:"recursive"`
    Scope     string `json:"scope"`
}
if err := json.Unmarshal([]byte(arguments), &args); err != nil {
    return "", fmt.Errorf("invalid arguments: %w", err)
}
// Apply defaults
if args.Limit <= 0 {
    args.Limit = 10
}
if args.Scope == "" {
    args.Scope = "all"
}
```

## Using `ToolCallContext`

```go
sessionID := tcc.SessionID   // current session UUID
cwd       := tcc.Cwd         // working directory for file operations
modelCode := tcc.ModelCode   // currently active model code
```

## Return Value Conventions

- **Success with data**: Return Markdown-formatted human-readable text.
- **Success with no data**: Return a short explanatory string, e.g. `"No results found."`.
- **Failure**: Return `("", error)`; the Registry wraps it into `ToolResult{IsError: true}` automatically.
- **Long text truncation**: Truncate at a reasonable length with `"..."`:

```go
func truncate(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen-3] + "..."
}
```
