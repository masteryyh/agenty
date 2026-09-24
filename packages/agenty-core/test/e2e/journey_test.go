//go:build e2e

package e2e_test

import (
	"context"
	"net/http"
	"testing"
)

func TestClientJourneyCoversPublicRPCSurfaceAcrossRestart(t *testing.T) {
	t.Parallel()

	fixture := newProviderFixture(t, func(request providerRequest) providerReply {
		if request.Method == http.MethodGet {
			return providerReply{Body: `{"object":"list","data":[{"id":"fixture-model"}]}`}
		}
		messages := providerMessages(request)
		if len(messages) > 0 && providerMessageContent(messages[len(messages)-1]) == "wait" {
			return providerReply{WaitForCancel: true}
		}
		return providerSuccess("openai_completions", "reply from turn", request.Call)
	})
	dataDir := t.TempDir()
	ctx, cancel := testContext(t)
	defer cancel()

	firstProcess := startCoreAt(t, dataDir, coreEnv(dataDir))
	first := newAgentyClient(firstProcess)
	already, err := first.InitializeAlready(ctx)
	requireNoError(t, err)
	if already.Initialized {
		t.Fatal("fresh data dir reported initialized")
	}
	_, err = first.CreateProvider(ctx, ProviderCreateInput{
		Code:     "setup-provider",
		Name:     "Setup Provider",
		Type:     "openai_completions",
		BaseURL:  fixture.BaseURL("openai_completions"),
		APIKey:   "test-key",
		Metadata: map[string]any{"source": "initialize"},
	})
	requireNoError(t, err)
	_, err = first.AddModel(ctx, ModelInput{
		ProviderCode:    "setup-provider",
		ModelCode:       "setup-model",
		Name:            "Setup Model",
		ContextWindow:   64_000,
		MaxOutputTokens: 8_192,
		IsDefault:       true,
	})
	requireNoError(t, err)
	already, err = first.CompleteInitialization(ctx, "setup-provider", "setup-model")
	requireNoError(t, err)
	if !already.Initialized {
		t.Fatal("completed initialization reported false")
	}
	_, err = first.DeleteProvider(ctx, "setup-provider")
	requireNoError(t, err)

	provider, err := first.CreateProvider(ctx, ProviderCreateInput{
		Code:     "local-openai",
		Name:     "Local OpenAI",
		Type:     "openai_completions",
		BaseURL:  fixture.BaseURL("openai_completions"),
		Metadata: map[string]any{"environment": "e2e"},
	})
	requireNoError(t, err)
	providerName := "Local OpenAI Compatible"
	providerAPIKey := "test-key"
	provider, err = first.UpdateProvider(ctx, ProviderUpdateInput{
		Code:   "local-openai",
		Name:   &providerName,
		APIKey: &providerAPIKey,
	})
	requireNoError(t, err)
	if provider.Name != providerName {
		t.Fatalf("updated provider = %+v", provider)
	}
	providers, err := first.ListProviders(ctx)
	requireNoError(t, err)
	var legacyProvider *Provider
	var localProvider *Provider
	for index := range providers {
		if providers[index].Code == "openai_legacy" {
			legacyProvider = &providers[index]
		}
		if providers[index].Code == "local-openai" {
			localProvider = &providers[index]
		}
	}
	if legacyProvider == nil || !legacyProvider.Builtin || !legacyProvider.Official || legacyProvider.Name != "OpenAI (Legacy API)" || legacyProvider.Type != "openai_completions" {
		t.Fatalf("legacy provider = %+v", legacyProvider)
	}
	if localProvider == nil {
		t.Fatalf("providers = %+v", providers)
	}
	_, err = first.GetProvider(ctx, "local-openai")
	requireNoError(t, err)

	provider, err = first.AddModel(ctx, ModelInput{
		ProviderCode:    "local-openai",
		ModelCode:       "primary-model",
		Name:            "Primary Model",
		ContextWindow:   128_000,
		MaxOutputTokens: 100_000,
		MultiModal:      true,
		IsDefault:       true,
	})
	requireNoError(t, err)
	var primaryModel *Model
	for index := range provider.Models {
		if provider.Models[index].Code == "primary-model" {
			primaryModel = &provider.Models[index]
			break
		}
	}
	if primaryModel == nil || primaryModel.MaxOutputTokens != 100_000 {
		t.Fatalf("provider models = %+v", provider.Models)
	}
	_, err = first.AddModel(ctx, ModelInput{
		ProviderCode:    "local-openai",
		ModelCode:       "temporary-model",
		Name:            "Temporary Model",
		ContextWindow:   64_000,
		MaxOutputTokens: 8_192,
	})
	requireNoError(t, err)
	primary, err := first.CreateSession(ctx, SessionCreateInput{
		ProviderCode:  "local-openai",
		ModelCode:     "primary-model",
		ContextWindow: 128_000,
	})
	requireNoError(t, err)
	secondary, err := first.CreateSession(ctx, SessionCreateInput{
		ProviderCode:  "local-openai",
		ModelCode:     "primary-model",
		ContextWindow: 64_000,
	})
	requireNoError(t, err)
	_, err = first.SetSessionTitle(ctx, primary.ID, "Plan the release")
	requireNoError(t, err)
	_, err = first.SetSessionModel(ctx, primary.ID, ModelRef{
		ProviderCode: "local-openai",
		ModelCode:    "temporary-model",
	})
	requireNoError(t, err)
	_, err = first.SetSessionModel(ctx, primary.ID, ModelRef{
		ProviderCode: "local-openai",
		ModelCode:    "primary-model",
	})
	requireNoError(t, err)
	_, err = first.SetSessionReasoningEffort(ctx, primary.ID, "high")
	requireNoError(t, err)
	workspace := "/tmp/agenty-e2e-workspace"
	_, err = first.SetSessionCwd(ctx, primary.ID, &workspace)
	requireNoError(t, err)
	_, err = first.SetSessionCwd(ctx, primary.ID, nil)
	requireNoError(t, err)

	summaries, err := first.ListSessions(ctx, SessionListInput{
		Limit:  1,
		Offset: 1,
	})
	requireNoError(t, err)
	if len(summaries) != 1 {
		t.Fatalf("session summaries = %+v", summaries)
	}

	firstRound, err := first.StartSession(ctx, primary.ID, []ContentInput{{
		Type: "text",
		Text: "Prepare a release checklist.",
	}})
	requireNoError(t, err)
	ended, err := first.WaitForRoundEvent(ctx, primary.ID, firstRound.RoundID, "round_ended")
	requireNoError(t, err)
	if ended.Status != "completed" || ended.Usage == nil || ended.Usage.Total != 5 {
		t.Fatalf("round ended event = %+v", ended)
	}
	completed, err := first.WaitForRoundStatus(
		ctx,
		primary.ID,
		firstRound.RoundID,
		"completed",
	)
	requireNoError(t, err)
	assertCompletedRound(
		t,
		completed,
		firstRound.RoundID,
		"reply from turn",
	)

	requireNoError(t, firstProcess.Close())
	secondProcess := startCoreAt(t, dataDir, coreEnv(dataDir))
	second := newAgentyClient(secondProcess)

	reloaded, err := second.GetSession(ctx, primary.ID)
	requireNoError(t, err)
	if reloaded.Title == nil || *reloaded.Title != "Plan the release" || len(reloaded.Rounds) != 1 {
		t.Fatalf("reloaded session = %+v", reloaded)
	}
	secondRound, err := second.StartSession(ctx, primary.ID, []ContentInput{{
		Type: "text",
		Text: "Now prioritize it.",
	}})
	requireNoError(t, err)
	completed, err = second.WaitForRoundStatus(
		ctx,
		primary.ID,
		secondRound.RoundID,
		"completed",
	)
	requireNoError(t, err)
	if len(completed.Rounds) != 2 {
		t.Fatalf("round count after restart = %d, want 2", len(completed.Rounds))
	}
	firstProviderRequest := waitForProviderCall(
		t,
		ctx,
		fixture.requests,
		2,
	)
	secondProviderRequest := waitForProviderCall(
		t,
		ctx,
		fixture.requests,
		3,
	)
	if firstProviderRequest.Body["max_completion_tokens"] != float64(100_000) {
		t.Fatalf("max completion tokens = %v, want 100000", firstProviderRequest.Body["max_completion_tokens"])
	}
	if providerMessageCount(secondProviderRequest) <= providerMessageCount(firstProviderRequest) {
		t.Fatalf(
			"second provider request message count = %d, want more than first request %d",
			providerMessageCount(secondProviderRequest),
			providerMessageCount(firstProviderRequest),
		)
	}

	cancelSession, err := second.CreateSession(ctx, SessionCreateInput{
		ProviderCode:  "local-openai",
		ModelCode:     "primary-model",
		ContextWindow: 128_000,
	})
	requireNoError(t, err)
	cancelRound, err := second.StartSession(
		ctx,
		cancelSession.ID,
		[]ContentInput{{Type: "text", Text: "wait"}},
	)
	requireNoError(t, err)
	waitForProviderCall(
		t,
		ctx,
		fixture.requests,
		4,
	)
	models, err := second.ListProviderModels(ctx, "local-openai")
	requireNoError(t, err)
	var availablePrimary *AvailableModel
	for index := range models {
		if models[index].Code == "primary-model" {
			availablePrimary = &models[index]
			break
		}
	}
	if availablePrimary == nil || availablePrimary.ContextWindow != 128_000 || availablePrimary.MaxOutputTokens != 100_000 {
		t.Fatalf("provider models = %+v", models)
	}
	_, err = second.StartSession(
		ctx,
		cancelSession.ID,
		[]ContentInput{{Type: "text", Text: "duplicate"}},
	)
	requireRPCCode(t, err, errAlreadyExists)
	_, err = second.DeleteSession(ctx, cancelSession.ID)
	requireRPCCode(t, err, errAlreadyExists)
	stop, err := second.StopSession(ctx, cancelSession.ID)
	requireNoError(t, err)
	if !stop.StopRequested || stop.RoundID != cancelRound.RoundID {
		t.Fatalf("stop result = %+v", stop)
	}
	_, err = second.WaitForRoundStatus(
		ctx,
		cancelSession.ID,
		cancelRound.RoundID,
		"cancelled",
	)
	requireNoError(t, err)
	_, err = second.CompactSession(ctx, primary.ID)
	requireNoError(t, err)
	if _, err = second.EnableCodexMode(ctx, primary.ID); err == nil {
		t.Fatal("Codex Mode accepted an OpenAI Chat Completions session")
	}
	requireNoError(t, second.rpc.Call(ctx, "skill.list", map[string]any{}, nil))
	requireNoError(t, second.rpc.CallChunked(ctx, "skill.list", map[string]any{}, 1, nil))

	requireNoError(t, second.rpc.AbortChunk(ctx, "aborted-upload", "provider.list"))
	err = second.rpc.Call(
		ctx,
		"chunk.commit",
		map[string]any{"requestId": "aborted-upload"},
		nil,
	)
	requireRPCCode(t, err, errNotFound)

	_, err = second.DeleteSession(ctx, cancelSession.ID)
	requireNoError(t, err)
	_, err = second.DeleteSession(ctx, secondary.ID)
	requireNoError(t, err)
	_, err = second.DeleteSession(ctx, primary.ID)
	requireNoError(t, err)
	_, err = second.RemoveModel(ctx, "local-openai", "temporary-model")
	requireNoError(t, err)
	_, err = second.DeleteProvider(ctx, "local-openai")
	requireNoError(t, err)

	called := mergeMethodCounts(first.rpc, second.rpc)
	for _, method := range publicRPCMethods {
		if called[method] == 0 {
			t.Errorf("public RPC method %q was not exercised", method)
		}
	}
}

func assertCompletedRound(t *testing.T, session Session, roundID, text string) {
	t.Helper()

	for _, round := range session.Rounds {
		if round.ID != roundID {
			continue
		}
		if round.Status != "completed" || len(round.Messages) != 2 {
			t.Fatalf("completed round = %+v", round)
		}
		assistant := round.Messages[1]
		if assistant.Role != "assistant" || len(assistant.Content) != 1 || assistant.Content[0].Text != text {
			t.Fatalf("assistant message = %+v", assistant)
		}
		if round.Usage != (TokenUsage{Input: 2, Output: 3, Total: 5}) {
			t.Fatalf("round usage = %+v", round.Usage)
		}
		return
	}
	t.Fatalf("round %s not found in session", roundID)
}

func waitForProviderCall(
	t *testing.T,
	ctx context.Context,
	requests <-chan providerRequest,
	call int,
) providerRequest {
	t.Helper()

	for {
		select {
		case request := <-requests:
			if request.Call == call {
				return request
			}
		case <-ctx.Done():
			t.Fatalf("provider call %d did not arrive: %v", call, ctx.Err())
		}
	}
}

func providerMessageCount(request providerRequest) int {
	messages, ok := request.Body["messages"].([]any)
	if !ok {
		return 0
	}
	return len(messages)
}
