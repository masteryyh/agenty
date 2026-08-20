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
	maxShellCommands              = 4
	maxShellConcurrency           = 4
	shellWaitDelay                = 500 * time.Millisecond
)

type shellArguments struct {
	Commands        []string `json:"commands"`
	TimeoutMs       *int64   `json:"timeout_ms,omitempty"`
	MaxOutputLength *int64   `json:"max_output_length,omitempty"`
}

type shellTool struct{}

func (tool *shellTool) Definition() agentloop.ToolDefinition {
	return agentloop.ToolDefinition{
		Type: agentloop.ToolTypeShell,
		Name: "shell",
		Description: "Execute up to 4 shell commands in parallel. " +
			"Uses zsh on macOS, bash on Linux, and sh as a fallback when the preferred shell is unavailable. " +
			"On Windows, uses pwsh.exe or powershell.exe when available, then cmd.exe.",
		InputSchema: objectSchema(map[string]agentloop.JSONSchema{
			"commands": {
				Type:     agentloop.JSONSchemaTypeArray,
				MinItems: new(uint64(1)),
				MaxItems: new(uint64(maxShellCommands)),
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
	jobs := make(chan shellJob)
	workerCount := min(len(arguments.Commands), maxShellConcurrency)
	var waitGroup sync.WaitGroup
	waitGroup.Add(workerCount)
	for range workerCount {
		go func() {
			defer waitGroup.Done()
			for job := range jobs {
				results[job.index] = executeShellCommand(ctx, callContext.Cwd, job.command, timeout, outputLimit)
			}
		}()
	}
	for index, command := range arguments.Commands {
		jobs <- shellJob{index: index, command: command}
	}
	close(jobs)
	waitGroup.Wait()

	return conversation.Content{conversation.ShellCallOutputBlock{
		MaxOutputLength: outputLimit,
		Output:          results,
	}}, nil
}

type shellJob struct {
	index   int
	command string
}

func normalizeShellArguments(arguments shellArguments) (time.Duration, int64, error) {
	if len(arguments.Commands) == 0 {
		return 0, 0, fmt.Errorf("commands must contain at least one command")
	}
	if len(arguments.Commands) > maxShellCommands {
		return 0, 0, fmt.Errorf("commands must contain at most %d commands", maxShellCommands)
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
	prepareShellProcess(process)
	if strings.TrimSpace(cwd) != "" {
		process.Dir = cwd
	}
	stdout := newShellOutputBuffer(outputLimit)
	stderr := newShellOutputBuffer(outputLimit)
	process.Stdout = &stdout
	process.Stderr = &stderr
	err := process.Run()

	if errors.Is(commandContext.Err(), context.DeadlineExceeded) {
		capturedStdout, capturedStderr := truncateShellOutput(stdout.String(), stderr.String(), outputLimit)
		return conversation.ShellCommandOutput{
			Stdout:  capturedStdout,
			Stderr:  capturedStderr,
			Outcome: conversation.ShellOutcome{Type: "timeout"},
		}
	}
	if err == nil {
		code := int64(0)
		capturedStdout, capturedStderr := truncateShellOutput(stdout.String(), stderr.String(), outputLimit)
		return conversation.ShellCommandOutput{
			Stdout:  capturedStdout,
			Stderr:  capturedStderr,
			Outcome: conversation.ShellOutcome{Type: "exit", ExitCode: &code},
		}
	}

	if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
		code := int64(exitError.ExitCode())
		capturedStdout, capturedStderr := truncateShellOutput(stdout.String(), stderr.String(), outputLimit)
		return conversation.ShellCommandOutput{
			Stdout:  capturedStdout,
			Stderr:  capturedStderr,
			Outcome: conversation.ShellOutcome{Type: "exit", ExitCode: &code},
		}
	}
	_, _ = stderr.Write([]byte(err.Error()))
	capturedStdout, capturedStderr := truncateShellOutput(stdout.String(), stderr.String(), outputLimit)
	code := int64(-1)
	return conversation.ShellCommandOutput{
		Stdout:  capturedStdout,
		Stderr:  capturedStderr,
		Outcome: conversation.ShellOutcome{Type: "exit", ExitCode: &code},
	}
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
