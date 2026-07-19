package routes

import (
	"regexp"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

type V1Routes struct {
	chatRoutes      *ChatRoutes
	providerRoutes  *ProviderRoutes
	modelRoutes     *ModelRoutes
	agentRoutes     *AgentRoutes
	memoryRoutes    *MemoryRoutes
	mcpServerRoutes *MCPServerRoutes
	systemRoutes    *SystemRoutes
	skillRoutes     *SkillRoutes
	gatewayRoutes   *GatewayRoutes
}

var (
	v1Routes *V1Routes
	v1Once   sync.Once
)

func GetV1Routes() *V1Routes {
	v1Once.Do(func() {
		v1Routes = &V1Routes{
			chatRoutes:      GetChatRoutes(),
			providerRoutes:  GetProviderRoutes(),
			modelRoutes:     GetModelRoutes(),
			agentRoutes:     GetAgentRoutes(),
			memoryRoutes:    GetMemoryRoutes(),
			mcpServerRoutes: GetMCPServerRoutes(),
			systemRoutes:    GetSystemRoutes(),
			skillRoutes:     GetSkillRoutes(),
			gatewayRoutes:   GetGatewayRoutes(),
		}
	})
	return v1Routes
}

func (r *V1Routes) RegisterRoutes(routerGroup *gin.RouterGroup) error {
	codeRegex := regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		if err := v.RegisterValidation("code", func(fl validator.FieldLevel) bool {
			return codeRegex.MatchString(fl.Field().String())
		}); err != nil {
			return err
		}
	}

	r.chatRoutes.RegisterRoutes(routerGroup)
	r.providerRoutes.RegisterRoutes(routerGroup)
	r.modelRoutes.RegisterRoutes(routerGroup)
	r.agentRoutes.RegisterRoutes(routerGroup)
	r.memoryRoutes.RegisterRoutes(routerGroup)
	r.mcpServerRoutes.RegisterRoutes(routerGroup)
	r.systemRoutes.RegisterRoutes(routerGroup)
	r.skillRoutes.RegisterRoutes(routerGroup)
	r.gatewayRoutes.RegisterRoutes(routerGroup)
	return nil
}
