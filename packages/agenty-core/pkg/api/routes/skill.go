package routes

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	domainskill "github.com/masteryyh/agenty-core/pkg/domain/skill"
	"github.com/masteryyh/agenty-core/pkg/infra/skill"
)

type skillListResult struct {
	Skills      []domainskill.Entry      `json:"skills"`
	Diagnostics []domainskill.Diagnostic `json:"diagnostics"`
}

type SkillRoutes struct {
	api      *API
	registry *skill.Registry
}

func newSkillRoutes(api *API, registry *skill.Registry) *SkillRoutes {
	return &SkillRoutes{api: api, registry: registry}
}

func (r *SkillRoutes) RegisterRoutes(v1 *gin.RouterGroup) {
	v1.GET("/skills", r.List)
}

func (r *SkillRoutes) List(c *gin.Context) {
	execute(r.api, c, http.StatusOK, nil, nil, func(_ context.Context, _ struct{}) (any, error) {
		if r.registry == nil {
			return skillListResult{Skills: []domainskill.Entry{}, Diagnostics: []domainskill.Diagnostic{}}, nil
		}
		return skillListResult{
			Skills:      r.registry.Entries(),
			Diagnostics: r.registry.Diagnostics(),
		}, nil
	})
}
