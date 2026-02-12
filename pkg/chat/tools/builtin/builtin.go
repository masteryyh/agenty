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
	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/services"
)

func RegisterAll(registry *tools.Registry) {
	cfg := config.GetConfigManager().GetConfig()

	registry.Register(&ReadFileTool{cfg: cfg})
	registry.Register(&WriteFileTool{cfg: cfg})
	registry.Register(&ListDirectoryTool{cfg: cfg})

	memoryService := services.GetMemoryService()
	if memoryService.IsEnabled() {
		registry.Register(&SaveMemoryTool{memoryService: memoryService})
		registry.Register(&SearchMemoryTool{memoryService: memoryService})
	}
}
