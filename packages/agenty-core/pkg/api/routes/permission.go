package routes

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty-core/pkg/infra/permission"
)

type PermissionRoutes struct {
	api     *API
	manager *permission.PermissionManager
}

func newPermissionRoutes(api *API, manager *permission.PermissionManager) *PermissionRoutes {
	return &PermissionRoutes{api: api, manager: manager}
}

func (r *PermissionRoutes) RegisterRoutes(v1 *gin.RouterGroup) {
	v1.POST("/tool-approvals/:approvalId/resolution", r.ResolveToolApproval)
}

func (r *PermissionRoutes) ResolveToolApproval(c *gin.Context) {
	execute(r.api, c, http.StatusOK, map[string]string{"approvalId": "approvalId"}, nil, func(ctx context.Context, resolution permission.Resolution) (any, error) {
		if err := r.manager.Resolve(ctx, resolution); err != nil {
			return nil, err
		}
		return resolution, nil
	})
}
