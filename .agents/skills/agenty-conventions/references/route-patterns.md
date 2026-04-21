# Route and Response Patterns

## Route Struct and Singleton

```go
type FooRoutes struct {
    service *services.FooService
}

var (
    fooRoutes *FooRoutes
    fooOnce   sync.Once
)

func GetFooRoutes() *FooRoutes {
    fooOnce.Do(func() {
        fooRoutes = &FooRoutes{
            service: services.GetFooService(),
        }
    })
    return fooRoutes
}

func (r *FooRoutes) RegisterRoutes(router *gin.RouterGroup) {
    g := router.Group("/foos")
    {
        g.GET("", r.List)
        g.POST("", r.Create)
        g.GET("/:id", r.Get)
        g.PUT("/:id", r.Update)
        g.DELETE("/:id", r.Delete)
    }
}
```

## Registering Routes in routes.go

```go
func SetupRoutes(router *gin.Engine) {
    api := router.Group("/api/v1")
    // ...
    GetFooRoutes().RegisterRoutes(api)
}
```

## Response Helpers (`pkg/utils/response`)

```go
import "github.com/masteryyh/agenty/pkg/utils/response"

// Success response
response.OK(c, data)         // HTTP 200, payload in "data" field
response.OK(c, []FooDto{})   // empty slice is fine too

// Error response (automatically unwraps BusinessError, falls back to 500)
response.Failed(c, err)

// Abort in middleware (terminates the handler chain)
response.Abort(c, err)
```

Response body shape:
```json
{
  "code": 200,
  "message": "ok",
  "data": { ... }
}
```

## Business Errors (`pkg/customerrors`)

Use predefined errors directly:

```go
import "github.com/masteryyh/agenty/pkg/customerrors"

// In route handlers
response.Failed(c, customerrors.ErrNotFound)
response.Failed(c, customerrors.ErrInvalidParams)
response.Failed(c, customerrors.ErrUnauthorized)

// In service layer (bubble up; route handler calls response.Failed)
return nil, customerrors.ErrFooNotFound

// Custom error (when nothing in the predefined list fits)
return nil, customerrors.NewBusinessError(409, "foo already exists")
```

Add new predefined errors in `pkg/customerrors/errors.go`:

```go
var (
    ErrFooNotFound = NewBusinessError(http.StatusNotFound, "foo not found")
    ErrFooInUse    = NewBusinessError(http.StatusBadRequest, "foo is in use")
)
```

## Request Binding

### JSON Body

```go
type createFooRequest struct {
    Name        string `json:"name" binding:"required"`
    Description string `json:"description"`
}

func (r *FooRoutes) Create(c *gin.Context) {
    var req createFooRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        response.Failed(c, customerrors.ErrInvalidParams)
        return
    }
    // ...
}
```

### Query Parameters

```go
func (r *FooRoutes) List(c *gin.Context) {
    page, _     := strconv.Atoi(c.DefaultQuery("page", "1"))
    pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
    // ...
}

// Optional UUID query parameter
var sessionID *uuid.UUID
if s := c.Query("sessionId"); s != "" {
    parsed, err := uuid.Parse(s)
    if err != nil {
        response.Failed(c, customerrors.ErrInvalidParams)
        return
    }
    sessionID = &parsed
}
```

### Path Parameters

```go
func (r *FooRoutes) Get(c *gin.Context) {
    id, err := uuid.Parse(c.Param("id"))
    if err != nil {
        response.Failed(c, customerrors.ErrInvalidParams)
        return
    }
    // ...
}
```

## Middleware

```go
// pkg/middleware/auth.go — authentication middleware pattern
func Auth() gin.HandlerFunc {
    return func(c *gin.Context) {
        token := c.GetHeader("Authorization")
        if token == "" {
            response.Abort(c, customerrors.ErrUnauthorized)
            return
        }
        // validate...
        c.Next()
    }
}
```

## HTTP Helper Functions (`pkg/conn/http_helpers.go`)

Used by `remote.go` and any service that needs to call external HTTP endpoints:

```go
import "github.com/masteryyh/agenty/pkg/conn"

// JSON request + JSON response
result, err := conn.Get[ResponseType](ctx, conn.HTTPRequest{
    URL:     "https://api.example.com/v1/items",
    Params:  map[string]string{"page": "1"},
    Headers: map[string]string{"Authorization": "Bearer " + token},
})

result, err := conn.Post[ResponseType](ctx, conn.HTTPRequest{
    URL:     "https://api.example.com/v1/items",
    Body:    requestBody,
    Headers: map[string]string{"Authorization": "Bearer " + token},
})

// SSE streaming response
ch, err := conn.PostSSE(ctx, conn.HTTPRequest{
    URL:     streamURL,
    Body:    streamRequest,
    Headers: map[string]string{"Authorization": "Bearer " + token},
})
for event := range ch {
    if event.Err != nil { break }
    // handle event.Data
}
```

**Never** create your own `http.Client`. Always use the shared client via `conn.GetHTTPClient()`:

```go
client := conn.GetHTTPClient()
resp, err := client.Do(req)
```
