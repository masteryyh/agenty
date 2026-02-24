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
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/chat/tools"
)

type RunShellCommandTool struct{}

func (t *RunShellCommandTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "run_shell_command",
		Description: "Execute a shell command on the local machine and return its output. Supports Windows (PowerShell), Linux, and macOS (sh). Returns stdout, stderr, and exit code.",
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"command": {
					Type:        "string",
					Description: "The shell command to execute. On Windows this runs via PowerShell -Command, on Linux/macOS via sh -c.",
				},
				"workdir": {
					Type:        "string",
					Description: "Optional working directory for the command. Defaults to the current working directory if not specified.",
				},
				"timeout": {
					Type:        "integer",
					Description: "Optional timeout in seconds. Defaults to 30. The command will be killed if it exceeds this duration.",
				},
			},
			Required: []string{"command"},
		},
	}
}

func (t *RunShellCommandTool) Execute(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Command string `json:"command"`
		Workdir string `json:"workdir,omitempty"`
		Timeout int    `json:"timeout,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if strings.TrimSpace(args.Command) == "" {
		return "", fmt.Errorf("command must not be empty")
	}

	timeout := 30
	if args.Timeout > 0 {
		timeout = args.Timeout
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(execCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", args.Command)
	} else {
		cmd = exec.CommandContext(execCtx, "sh", "-c", args.Command)
	}

	if args.Workdir != "" {
		dirInfo, err := os.Stat(args.Workdir)
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("working directory does not exist: %s", args.Workdir)
			}
			return "", fmt.Errorf("failed to access working directory: %w", err)
		}
		if !dirInfo.IsDir() {
			return "", fmt.Errorf("working directory path is not a directory: %s", args.Workdir)
		}
		cmd.Dir = args.Workdir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if execCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after %d seconds", timeout)
		} else {
			return "", fmt.Errorf("failed to run command: %w", runErr)
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Exit Code: %d\n", exitCode)
	if stdout.Len() > 0 {
		sb.WriteString("Stdout:\n")
		sb.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		sb.WriteString("Stderr:\n")
		sb.WriteString(stderr.String())
	}
	if stdout.Len() == 0 && stderr.Len() == 0 {
		sb.WriteString("(no output)")
	}

	return sb.String(), nil
}
