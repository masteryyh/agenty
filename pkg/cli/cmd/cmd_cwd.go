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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
)

func handleCwdCmd(b backend.Backend, bridge *UIBridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	if len(args) == 0 {
		session, err := b.GetSession(sessionID)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to get session: %w", err)
		}
		if session.Cwd == nil {
			bridge.Info("No working directory set. Usage: /cwd <path>")
		} else {
			bridge.Info("Working directory: %s", styleGreen.Render(*session.Cwd))
		}
		return CommandResult{Handled: true}, nil
	}

	if args[0] == "clear" {
		if err := b.SetSessionCwd(sessionID, nil, nil); err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to clear working directory: %w", err)
		}
		bridge.Success("Working directory cleared")
		return CommandResult{Handled: true}, nil
	}

	input := args[0]
	var dirPath string
	if input == "~" || strings.HasPrefix(input, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to get home directory: %w", err)
		}
		if input == "~" {
			dirPath = homeDir
		} else {
			dirPath = filepath.Join(homeDir, input[2:])
		}
	} else {
		var err error
		dirPath, err = filepath.Abs(input)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("invalid path: %w", err)
		}
	}

	info, err := os.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			bridge.Error("Directory does not exist: %s", dirPath)
			return CommandResult{Handled: true}, nil
		}
		return CommandResult{Handled: true}, fmt.Errorf("failed to stat path: %w", err)
	}

	if !info.IsDir() {
		bridge.Error("Path is not a directory: %s", dirPath)
		return CommandResult{Handled: true}, nil
	}

	var agentsMD *string
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		candidate := filepath.Join(dirPath, name)
		data, readErr := os.ReadFile(candidate)
		if readErr == nil {
			content := string(data)
			agentsMD = &content
			bridge.Info("Loaded %s", styleGreen.Render(candidate))
			break
		}
	}

	if err := b.SetSessionCwd(sessionID, &dirPath, agentsMD); err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to set working directory: %w", err)
	}

	if agentsMD != nil {
		bridge.Success("Working directory set to %s (with project instructions)", styleGreen.Render(dirPath))
	} else {
		bridge.Success("Working directory set to %s (no AGENTS.md or CLAUDE.md found)", styleGreen.Render(dirPath))
	}
	return CommandResult{Handled: true}, nil
}
