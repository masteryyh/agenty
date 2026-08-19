package builtin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

const (
	defaultShellTimeout     int64 = 60_000
	defaultShellOutputLimit int64 = 4_096
	maxShellTimeout         int64 = 120_000
	maxShellOutputLimit     int64 = 1 << 20
)

type shellArguments struct {
	Commands        []string `json:"commands"`
	TimeoutMs       *int64   `json:"timeout_ms,omitempty"`
	MaxOutputLength *int64   `json:"max_output_length,omitempty"`
}

type shellTool struct{}

func (tool *shellTool) Definition() agentloop.ToolDefinition {
	return agentloop.ToolDefinition{
		Type:        agentloop.ToolTypeShell,
		Name:        "shell",
		Description: "Execute one or more shell commands in parallel.",
		InputSchema: objectSchema(map[string]agentloop.JSONSchema{
			"commands": {
				Type:     agentloop.JSONSchemaTypeArray,
				MinItems: new(uint64(1)),
				Items:    &agentloop.JSONSchema{Type: agentloop.JSONSchemaTypeString},
			},
			"timeout_ms": {
				Type:        agentloop.JSONSchemaTypeInteger,
				Description: "Maximum wall-clock time in milliseconds for each command.",
				Minimum:     new(float64(1)),
				Maximum:     new(float64(maxShellTimeout)),
			},
			"max_output_length": {
				Type:        agentloop.JSONSchemaTypeInteger,
				Description: "Maximum number of UTF-8 characters captured from each command.",
				Minimum:     new(float64(1)),
				Maximum:     new(float64(maxShellOutputLimit)),
			},
		}, []string{"commands"}),
		Strict: true,
	}
}

func (tool *shellTool) Execute(
	ctx context.Context,
	callContext agentloop.CallContext,
	input []byte,
) (conversation.Content, error) {
	var arguments shellArguments
	if err := decodeArguments(input, &arguments); err != nil {
		return nil, err
	}
	timeout, outputLimit, err := normalizeShellArguments(arguments)
	if err != nil {
		return nil, err
	}

	results := make([]conversation.ShellCommandOutput, len(arguments.Commands))
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(arguments.Commands))
	for index, command := range arguments.Commands {
		go func() {
			defer waitGroup.Done()
			results[index] = executeShellCommand(ctx, callContext.Cwd, command, timeout, outputLimit)
		}()
	}
	waitGroup.Wait()

	return conversation.Content{conversation.ShellCallOutputBlock{
		MaxOutputLength: outputLimit,
		Output:          results,
	}}, nil
}

func normalizeShellArguments(arguments shellArguments) (time.Duration, int64, error) {
	if len(arguments.Commands) == 0 {
		return 0, 0, fmt.Errorf("commands must contain at least one command")
	}
	for index, command := range arguments.Commands {
		if strings.TrimSpace(command) == "" {
			return 0, 0, fmt.Errorf("commands[%d] must not be empty", index)
		}
	}

	timeoutMs := defaultShellTimeout
	if arguments.TimeoutMs != nil {
		timeoutMs = *arguments.TimeoutMs
	}
	if timeoutMs < 1 || timeoutMs > maxShellTimeout {
		return 0, 0, fmt.Errorf("timeout_ms must be between 1 and %d", maxShellTimeout)
	}

	outputLimit := defaultShellOutputLimit
	if arguments.MaxOutputLength != nil {
		outputLimit = *arguments.MaxOutputLength
	}
	if outputLimit < 1 || outputLimit > maxShellOutputLimit {
		return 0, 0, fmt.Errorf("max_output_length must be between 1 and %d", maxShellOutputLimit)
	}

	return time.Duration(timeoutMs) * time.Millisecond, outputLimit, nil
}

func executeShellCommand(
	parent context.Context,
	cwd string,
	command string,
	timeout time.Duration,
	outputLimit int64,
) conversation.ShellCommandOutput {
	commandContext, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	process := newShellCommand(commandContext, command)
	if strings.TrimSpace(cwd) != "" {
		process.Dir = cwd
	}
	stdout := newShellOutputBuffer(outputLimit)
	stderr := newShellOutputBuffer(outputLimit)
	process.Stdout = &stdout
	process.Stderr = &stderr
	err := process.Run()

	capturedStdout, capturedStderr := truncateShellOutput(stdout.String(), stderr.String(), outputLimit)
	result := conversation.ShellCommandOutput{Stdout: capturedStdout, Stderr: capturedStderr}
	if errors.Is(commandContext.Err(), context.DeadlineExceeded) {
		result.Outcome = conversation.ShellOutcome{Type: "timeout"}
		return result
	}
	if err == nil {
		code := int64(0)
		result.Outcome = conversation.ShellOutcome{Type: "exit", ExitCode: &code}
		return result
	}

	if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
		code := int64(exitError.ExitCode())
		result.Outcome = conversation.ShellOutcome{Type: "exit", ExitCode: &code}
		return result
	}
	result.Outcome = conversation.ShellOutcome{Type: "exit", ExitCode: new(int64(-1))}
	return result
}

type shellOutputBuffer struct {
	buffer    bytes.Buffer
	remaining int64
}

func newShellOutputBuffer(characterLimit int64) shellOutputBuffer {
	return shellOutputBuffer{remaining: characterLimit * utf8.UTFMax}
}

func (buffer *shellOutputBuffer) Write(data []byte) (int, error) {
	written := len(data)
	if buffer.remaining <= 0 {
		return written, nil
	}
	length := min(int64(len(data)), buffer.remaining)
	_, _ = buffer.buffer.Write(data[:length])
	buffer.remaining -= length
	return written, nil
}

func (buffer *shellOutputBuffer) String() string {
	return buffer.buffer.String()
}

func truncateShellOutput(stdout, stderr string, limit int64) (string, string) {
	stdoutLength := int64(utf8.RuneCountInString(stdout))
	if stdoutLength >= limit {
		return string([]rune(stdout)[:limit]), ""
	}
	remaining := limit - stdoutLength
	if int64(utf8.RuneCountInString(stderr)) <= remaining {
		return stdout, stderr
	}
	return stdout, string([]rune(stderr)[:remaining])
}
