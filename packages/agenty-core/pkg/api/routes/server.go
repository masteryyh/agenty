package routes

import (
	"bytes"
	"context"
	stdjson "encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"

	json "github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty-core/pkg/api/middlewares"
	"github.com/masteryyh/agenty-core/pkg/api/response"
	"github.com/masteryyh/agenty-core/pkg/infra/event"
)

const maxRequestBytes = 4 << 20

type SystemInfo struct {
	DataDir       string `json:"dataDir"`
	IPCVersion    string `json:"ipcVersion"`
	APIContract   string `json:"apiContract"`
	ProcessID     int    `json:"processId"`
	Initialized   bool   `json:"initialized"`
	EventStreamID string `json:"eventStreamId"`
}

type API struct {
	routes   *APIRoutes
	info     SystemInfo
	stream   *event.StreamBroker
	shutdown func()
	logger   *slog.Logger
	router   http.Handler
	stopping atomic.Bool
}

func NewServer(deps Dependencies, info SystemInfo, stream *event.StreamBroker, shutdown func()) *API {
	api := &API{
		info:     info,
		stream:   stream,
		shutdown: shutdown,
		logger:   slog.Default(),
	}
	api.routes = newAPIRoutes(api, deps)
	api.router = api.buildRouter()
	return api
}

func (api *API) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	api.router.ServeHTTP(writer, request)
}

func (api *API) BeginShutdown() {
	api.stopping.Store(true)
}

func (api *API) buildRouter() http.Handler {
	engine := gin.New()
	engine.Use(middlewares.RequestLogger(), middlewares.Recovery())
	engine.UseRawPath = true
	engine.RedirectTrailingSlash = false
	engine.HandleMethodNotAllowed = true
	engine.NoRoute(func(c *gin.Context) {
		writeAPIError(c.Writer, notFound("API route not found"))
	})
	engine.NoMethod(func(c *gin.Context) {
		writeAPIError(c.Writer, &APIError{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "HTTP method is not allowed for this route"})
	})
	api.routes.RegisterRoutes(engine, api)
	return engine
}

func execute[T any](api *API, c *gin.Context, status int, pathFields map[string]string, queryFields []string, action func(context.Context, T) (any, error)) {
	writer := c.Writer
	request := c.Request
	if api.stopping.Load() && request.Method != http.MethodGet {
		writeAPIError(writer, &APIError{Status: http.StatusServiceUnavailable, Code: "shutting_down", Message: "core is shutting down and is not accepting changes"})
		return
	}

	for _, pathValue := range pathFields {
		request.SetPathValue(pathValue, c.Param(pathValue))
	}
	params, err := requestParams(writer, request, pathFields, queryFields)
	if err != nil {
		writeAPIError(writer, err)
		return
	}
	ctx := request.Context()
	if api.stream != nil && request.PathValue("id") != "" && sessionMutationUsesRepository(request.Method, c.FullPath()) {
		var release func()
		ctx, release = api.stream.TopicBarrier(ctx, "session:"+request.PathValue("id"))
		defer release()
	}
	var input T
	if err := decodeParams(params, &input); err != nil {
		writeAPIError(writer, invalidParams("invalid params: "+err.Error()))
		return
	}
	value, err := action(ctx, input)
	if err != nil {
		writeAPIError(writer, err)
		return
	}
	response.Success(writer, status, value)
}

func requestParams(writer http.ResponseWriter, request *http.Request, pathFields map[string]string, queryFields []string) (stdjson.RawMessage, error) {
	params := make(map[string]stdjson.RawMessage)
	if request.Body != nil {
		body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, maxRequestBytes))
		if err != nil {
			return nil, invalidParams("request body is too large or unreadable")
		}
		if len(bytes.TrimSpace(body)) > 0 {
			if err := json.Unmarshal(body, &params); err != nil {
				return nil, invalidParams("request body must be a JSON object: " + err.Error())
			}
			if params == nil {
				return nil, invalidParams("request body must be a JSON object")
			}
		}
	}
	for key, pathValue := range pathFields {
		encoded, _ := json.Marshal(request.PathValue(pathValue))
		params[key] = encoded
	}
	for _, key := range queryFields {
		if value := request.URL.Query().Get(key); value != "" {
			if key == "limit" || key == "offset" {
				parsed, err := parsePositiveInt(value)
				if err != nil {
					return nil, err
				}
				params[key], _ = json.Marshal(parsed)
			} else {
				params[key], _ = json.Marshal(value)
			}
		}
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return nil, invalidParams("could not encode request parameters")
	}
	return encoded, nil
}

func writeAPIError(writer http.ResponseWriter, err error) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		mapped := toAPIError(err)
		if !errors.As(mapped, &apiErr) {
			apiErr = &APIError{Status: http.StatusInternalServerError, Code: "internal", Message: mapped.Error()}
		}
	}
	response.Failure(writer, apiErr.Status, apiErr.Code, apiErr.Message)
}

func parsePositiveInt(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, invalidParams("expected a non-negative integer")
	}
	return parsed, nil
}
