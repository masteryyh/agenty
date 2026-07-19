package builtin

import (
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/tools"
)

func RegisterAll(registry *tools.Registry) {
	registry.Register(&ReadFileTool{})
	registry.Register(&WriteFileTool{})
	registry.Register(&ListDirectoryTool{})
	registry.Register(&ReplaceInFileTool{})
	registry.Register(&RunShellCommandTool{})
	registry.Register(&UpdateSoulTool{agentService: services.GetAgentService()})
	registry.Register(&TodoTool{})

	registry.Register(&SaveMemoryTool{
		knowledgeService: services.GetKnowledgeService(),
	})

	registry.Register(&SearchTool{
		searchService: services.GetSearchService(),
	})

	registry.Register(&FetchTool{
		webFetchService: services.GetWebFetchService(),
	})

	registry.Register(&FindSkillTool{
		skillService: services.GetSkillService(),
	})
}
