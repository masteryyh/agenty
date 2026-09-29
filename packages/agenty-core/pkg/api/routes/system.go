package routes

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty-core/pkg/api/response"
)

func registerSystemRoutes(v1 *gin.RouterGroup, api *API) {
	v1.GET("/system", api.systemInfo)
	v1.POST("/system/shutdown", api.requestShutdown)
}

func (api *API) systemInfo(context *gin.Context) {
	info := api.info
	if api.stream != nil {
		info.EventStreamID = api.stream.StreamID()
	}
	if api.routes.initialization != nil && api.routes.initialization.service != nil {
		info.Initialized = api.routes.initialization.service.Already(context.Request.Context()).Initialized
	}
	response.Success(context.Writer, http.StatusOK, info)
}

func (api *API) requestShutdown(context *gin.Context) {
	api.BeginShutdown()
	response.Success(context.Writer, http.StatusAccepted, map[string]bool{"shuttingDown": true})
	if api.shutdown != nil {
		go func() {
			time.Sleep(25 * time.Millisecond)
			api.shutdown()
		}()
	}
}
