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

package cmd

import (
	"github.com/masteryyh/agenty/pkg/cli/chatstate"
	"github.com/masteryyh/agenty/pkg/models"
)

func findDefaultAgent(agents []models.AgentDto) *models.AgentDto {
	return chatstate.FindDefaultAgent(agents)
}

func modelDisplayName(model models.ModelDto) string {
	return chatstate.ModelDisplayName(model)
}

func modelConfigured(model models.ModelDto) bool {
	return chatstate.ModelConfigured(model)
}

func modelSwitchable(m models.ModelDto) bool {
	return chatstate.ModelSwitchable(m)
}

func modelProviderConfigured(m models.ModelDto) bool {
	return chatstate.ModelProviderConfigured(m)
}
