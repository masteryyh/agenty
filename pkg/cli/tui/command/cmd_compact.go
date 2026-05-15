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

package command

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
)

func handleCompactCmd(b backend.Backend, bridge Bridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	compacted, err := b.CompactSessionForModel(sessionID, modelID, true)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to compact conversation: %w", err)
	}
	if compacted {
		bridge.Success("Compacted conversation")
		return CommandResult{Handled: true}, nil
	}
	bridge.Warning("No compactable conversation prefix found")
	return CommandResult{Handled: true}, nil
}
