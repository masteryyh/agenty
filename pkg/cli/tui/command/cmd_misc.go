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
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
)

func handleExitCmd(b backend.Backend, bridge Bridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	bridge.Info("Goodbye!")
	return CommandResult{Handled: true, ShouldExit: true}, nil
}

func handleThinkCmd(b backend.Backend, bridge Bridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	if len(args) == 0 {
		if state.Thinking {
			if state.ThinkingLevel != "" {
				bridge.Info("Thinking is %s (level: %s)", styleGreen.Render("on"), styleYellow.Render(state.ThinkingLevel))
			} else {
				bridge.Info("Thinking is %s", styleGreen.Render("on"))
			}
		} else {
			bridge.Info("Thinking is %s", styleRed.Render("off"))
		}
		bridge.Info("Usage: /think [off|<level>]")
		return CommandResult{Handled: true}, nil
	}

	arg := strings.ToLower(args[0])
	if arg == "off" {
		state.Thinking = false
		state.ThinkingLevel = ""
		bridge.Success("Thinking disabled")
		return CommandResult{Handled: true}, nil
	}

	supportedLevelsPtr, _ := b.GetModelThinkingLevels(modelID)
	var supportedLevels []string
	if supportedLevelsPtr != nil {
		supportedLevels = *supportedLevelsPtr
	}
	valid := false
	for _, l := range supportedLevels {
		if l == arg {
			valid = true
			break
		}
	}
	if !valid {
		if len(supportedLevels) == 0 {
			bridge.Error("Model does not support thinking")
		} else {
			bridge.Error("Unknown level: %s. Supported: %s", arg, strings.Join(supportedLevels, ", "))
		}
		return CommandResult{Handled: true}, nil
	}

	state.Thinking = true
	state.ThinkingLevel = arg
	bridge.Success("Thinking enabled (level: %s)", styleYellow.Render(arg))
	return CommandResult{Handled: true}, nil
}

func handleHelpCmd(b backend.Backend, bridge Bridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	bridge.PrintCommandHints(state.LocalMode)
	return CommandResult{Handled: true}, nil
}
