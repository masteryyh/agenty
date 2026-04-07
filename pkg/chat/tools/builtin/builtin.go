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

package builtin

import (
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/utils/signal"
)

func RegisterAll(registry *tools.Registry) {
	registry.Register(&ReadFileTool{})
	registry.Register(&WriteFileTool{})
	registry.Register(&ListDirectoryTool{})
	registry.Register(&ReplaceInFileTool{})
	registry.Register(&RunShellCommandTool{})
	registry.Register(&UpdateSoulTool{agentService: services.GetAgentService()})
	registry.Register(&TodoTool{})

	embeddingSvc := services.GetEmbeddingService()
	if embeddingSvc.IsEnabled(signal.GetBaseContext()) {
		knowledgeSvc := services.GetKnowledgeService()
		webSearchSvc := services.GetWebSearchService()
		registry.Register(&SaveMemoryTool{
			knowledgeService: knowledgeSvc,
		})
		registry.Register(&SearchTool{
			knowledgeService: knowledgeSvc,
			webSearchService: webSearchSvc,
			evaluator:        services.GetSearchEvaluator(),
		})
	}
}
