//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSessionMetadataIsPersistedButHiddenFromClients(t *testing.T) {
	t.Parallel()

	fixture := newProviderFixture(t, func(request providerRequest) providerReply {
		return providerSuccess("openai_completions", "metadata reply", request.Call)
	})
	dataDir := t.TempDir()
	ctx, cancel := testContext(t)
	defer cancel()
	process := startCoreAt(t, dataDir, coreEnv(dataDir))
	client := newAgentyClient(process)

	session, err := createExecutionResources(
		ctx,
		client,
		fixture,
		"openai_completions",
		"metadata",
	)
	requireNoError(t, err)
	_, err = client.SetSessionCwd(ctx, session.ID, stringPointer("/workspace/metadata"))
	requireNoError(t, err)
	started, err := client.StartSession(ctx, session.ID, []ContentInput{{
		Type: "text",
		Text: "hello",
	}})
	requireNoError(t, err)
	completed, err := client.WaitForRoundStatus(ctx, session.ID, started.RoundID, "completed")
	requireNoError(t, err)

	request := waitForProviderCall(t, ctx, fixture.requests, 1)
	messages := providerMessages(request)
	if len(messages) != 3 {
		t.Fatalf("provider messages = %+v, want system, metadata and user messages", messages)
	}
	systemText := providerMessageContent(messages[0])
	for _, dynamicValue := range []string{
		"<cwd>/workspace/metadata</cwd>",
		"<model>metadata-model</model>",
		"<provider>metadata-provider</provider>",
		"<reasoning-effort>off</reasoning-effort>",
	} {
		if strings.Contains(systemText, dynamicValue) {
			t.Errorf("system prompt contains dynamic metadata %q: %q", dynamicValue, systemText)
		}
	}
	metadataText := providerMessageContent(messages[1])
	if !strings.Contains(metadataText, "<metadata>") ||
		!strings.Contains(metadataText, "<cwd>/workspace/metadata</cwd>") ||
		!strings.Contains(metadataText, "<model>metadata-model</model>") ||
		!strings.Contains(metadataText, "<provider>metadata-provider</provider>") {
		t.Fatalf("provider metadata message = %q", metadataText)
	}
	if got := providerMessageContent(messages[2]); got != "hello" {
		t.Fatalf("provider user message = %q, want hello after metadata", got)
	}

	if len(completed.Rounds) != 1 || len(completed.Rounds[0].Messages) != 2 {
		t.Fatalf("public completed session = %+v, want metadata hidden", completed)
	}
	for _, message := range completed.Rounds[0].Messages {
		for _, block := range message.Content {
			if strings.Contains(block.Text, "<metadata>") {
				t.Fatalf("public session exposed metadata message = %+v", message)
			}
		}
	}

	paths, err := filepath.Glob(filepath.Join(dataDir, "sessions", "*", "*", "*", session.ID+".jsonl"))
	requireNoError(t, err)
	if len(paths) != 1 {
		t.Fatalf("transcript paths = %v, want one JSONL transcript", paths)
	}
	transcript, err := os.ReadFile(paths[0])
	requireNoError(t, err)
	transcriptText := string(transcript)
	if !strings.Contains(transcriptText, `"visibility":"hidden"`) ||
		!strings.Contains(transcriptText, "<metadata>") ||
		!strings.Contains(transcriptText, "hello") {
		t.Fatalf("transcript does not contain hidden metadata and user message: %s", transcriptText)
	}
}

func providerMessages(request providerRequest) []map[string]any {
	values, ok := request.Body["messages"].([]any)
	if !ok {
		return nil
	}
	messages := make([]map[string]any, 0, len(values))
	for _, value := range values {
		message, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		messages = append(messages, message)
	}
	return messages
}

func providerMessageContent(message map[string]any) string {
	content := message["content"]
	switch value := content.(type) {
	case string:
		return value
	case []any:
		var builder strings.Builder
		for _, part := range value {
			partMap, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := partMap["text"].(string); ok {
				builder.WriteString(text)
			}
		}
		return builder.String()
	default:
		return ""
	}
}

func TestAgentLoopExecutesThroughEveryProviderProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		apiType string
		prefix  string
	}{
		{name: "OpenAI Responses", apiType: "openai", prefix: "responses"},
		{name: "OpenAI Chat Completions", apiType: "openai_completions", prefix: "chat"},
		{name: "Anthropic Messages", apiType: "anthropic", prefix: "anthropic"},
		{name: "Google GenAI", apiType: "gemini", prefix: "gemini"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fixture := newProviderFixture(t, func(request providerRequest) providerReply {
				return providerSuccess(tt.apiType, "provider matrix reply", request.Call)
			})
			ctx, cancel := testContext(t)
			defer cancel()
			client := newAgentyClient(startCore(t))

			session, err := createExecutionResources(
				ctx,
				client,
				fixture,
				tt.apiType,
				tt.prefix,
			)
			requireNoError(t, err)
			started, err := client.StartSession(ctx, session.ID, []ContentInput{{
				Type: "text",
				Text: "Use the configured provider.",
			}})
			requireNoError(t, err)
			completed, err := client.WaitForRoundStatus(
				ctx,
				session.ID,
				started.RoundID,
				"completed",
			)
			requireNoError(t, err)
			assertCompletedRound(
				t,
				completed,
				started.RoundID,
				"provider matrix reply",
			)

			request := waitForProviderCall(
				t,
				ctx,
				fixture.requests,
				1,
			)
			if !requestMatchesAPI(request, tt.apiType) {
				t.Fatalf(
					"provider request = %s %s, does not match %s",
					request.Method,
					request.Path,
					tt.apiType,
				)
			}
			wantTools := []string{
				"delete_file", "glob", "grep", "ls", "patch_file", "read_file", "shell", "write_file",
			}
			if tt.apiType == "openai" {
				wantTools = []string{"apply_patch", "glob", "grep", "ls", "read_file", "shell"}
			}
			if names := providerToolNames(request, tt.apiType); !slices.Equal(names, wantTools) {
				t.Errorf("provider tools = %q, want %q", names, wantTools)
			}
		})
	}
}

func TestSingleIPCClientRunsSessionsConcurrently(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	var arrived atomic.Int32
	var releaseOnce sync.Once
	fixture := newProviderFixture(t, func(request providerRequest) providerReply {
		if arrived.Add(1) == 2 {
			releaseOnce.Do(func() {
				close(release)
			})
		}
		<-release
		return providerSuccess("openai_completions", fmt.Sprintf("parallel reply %d", request.Call), request.Call)
	})
	ctx, cancel := testContext(t)
	defer cancel()
	client := newAgentyClient(startCore(t))

	first, err := createExecutionResources(
		ctx,
		client,
		fixture,
		"openai_completions",
		"parallel",
	)
	requireNoError(t, err)
	second, err := client.CreateSession(ctx, SessionCreateInput{
		AgentCode:     "parallel-agent",
		ProviderCode:  "parallel-provider",
		ModelCode:     "parallel-model",
		ContextWindow: 128_000,
	})
	requireNoError(t, err)

	type startResult struct {
		start ExecutionStart
		err   error
	}
	results := make(chan startResult, 2)
	for _, sessionID := range []string{first.ID, second.ID} {
		go func() {
			started, startErr := client.StartSession(ctx, sessionID, []ContentInput{{
				Type: "text",
				Text: "Run concurrently.",
			}})
			results <- startResult{start: started, err: startErr}
		}()
	}

	starts := make([]ExecutionStart, 0, 2)
	for range 2 {
		result := <-results
		requireNoError(t, result.err)
		starts = append(starts, result.start)
	}
	for _, started := range starts {
		_, err := client.WaitForRoundStatus(
			ctx,
			started.SessionID,
			started.RoundID,
			"completed",
		)
		requireNoError(t, err)
	}
	if arrived.Load() != 2 {
		t.Fatalf("concurrent provider requests = %d, want 2", arrived.Load())
	}
}

func TestAgentLoopPersistsFailureVisibleToClient(t *testing.T) {
	t.Parallel()

	fixture := newProviderFixture(t, func(providerRequest) providerReply {
		return providerReply{
			Status: http.StatusBadRequest,
			Body:   `{"error":{"message":"fixture rejected request","type":"invalid_request_error"}}`,
		}
	})
	ctx, cancel := testContext(t)
	defer cancel()
	client := newAgentyClient(startCore(t))

	session, err := createExecutionResources(
		ctx,
		client,
		fixture,
		"openai_completions",
		"failure",
	)
	requireNoError(t, err)
	started, err := client.StartSession(
		ctx,
		session.ID,
		[]ContentInput{{Type: "text", Text: "fail"}},
	)
	requireNoError(t, err)
	failed, err := client.WaitForRoundStatus(
		ctx,
		session.ID,
		started.RoundID,
		"failed",
	)
	requireNoError(t, err)
	if len(failed.Rounds) != 1 || failed.Rounds[0].Error == nil || *failed.Rounds[0].Error == "" {
		t.Fatalf("failed round = %+v", failed.Rounds)
	}
}

func TestRunningRoundIsCancelledWhenCoreRestarts(t *testing.T) {
	t.Parallel()

	fixture := newProviderFixture(t, func(providerRequest) providerReply {
		return providerReply{WaitForCancel: true}
	})
	dataDir := t.TempDir()
	ctx, cancel := testContext(t)
	defer cancel()

	firstProcess := startCoreAt(t, dataDir, coreEnv(dataDir))
	first := newAgentyClient(firstProcess)
	session, err := createExecutionResources(
		ctx,
		first,
		fixture,
		"openai_completions",
		"restart",
	)
	requireNoError(t, err)
	started, err := first.StartSession(
		ctx,
		session.ID,
		[]ContentInput{{Type: "text", Text: "wait"}},
	)
	requireNoError(t, err)
	waitForProviderCall(
		t,
		ctx,
		fixture.requests,
		1,
	)
	requireNoError(t, firstProcess.Close())

	second := newAgentyClient(startCoreAt(t, dataDir, coreEnv(dataDir)))
	reloaded, err := second.GetSession(ctx, session.ID)
	requireNoError(t, err)
	if len(reloaded.Rounds) != 1 {
		t.Fatalf("reloaded rounds = %+v", reloaded.Rounds)
	}
	round := reloaded.Rounds[0]
	if round.ID != started.RoundID || round.Status != "cancelled" || round.Error != nil {
		t.Fatalf("reloaded interrupted round = %+v", round)
	}
}
