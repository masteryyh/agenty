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
		if request.Call == 3 {
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
	_, err = first.CreateAgent(ctx, AgentCreateInput{
		Code:                 "setup-agent",
		Name:                 "Setup Agent",
		DefaultModel:         &ModelRef{ProviderCode: "setup-provider", ModelCode: "setup-model"},
		DefaultContextWindow: 64_000,
		IsDefault:            true,
	})
	requireNoError(t, err)
	already, err = first.CompleteInitialization(ctx, "setup-agent", "setup-provider", "setup-model")
	requireNoError(t, err)
	if !already.Initialized {
		t.Fatal("completed initialization reported false")
	}
	_, err = first.DeleteProvider(ctx, "setup-provider")
	requireNoError(t, err)
	_, err = first.DeleteAgent(ctx, "setup-agent")
	requireNoError(t, err)

	createdAgent, err := first.CreateAgent(ctx, AgentCreateInput{
		Code:                   "daily-assistant",
		Name:                   "Daily Assistant",
		Description:            "Helps with daily work",
		Soul:                   "Be concise and verify facts.",
		DefaultModel:           &ModelRef{ProviderCode: "local-openai", ModelCode: "primary-model"},
		DefaultContextWindow:   128_000,
		DefaultReasoningEffort: "high",
		IsDefault:              true,
		Metadata:               map[string]any{"team": "platform"},
	})
	requireNoError(t, err)
	if createdAgent.Code != "daily-assistant" || createdAgent.CreatedAt.IsZero() {
		t.Fatalf("created agent = %+v", createdAgent)
	}
	_, err = first.CreateAgent(ctx, AgentCreateInput{Code: "daily-assistant", Name: "duplicate"})
	requireRPCCode(t, err, errAlreadyExists)

	updatedAgent, err := first.UpdateAgent(ctx, AgentUpdateInput{
		Code:        "daily-assistant",
		Name:        stringPointer("Senior Daily Assistant"),
		Description: stringPointer(""),
		Metadata:    map[string]any{"team": "runtime"},
	})
	requireNoError(t, err)
	if updatedAgent.Name != "Senior Daily Assistant" || updatedAgent.Description != "" {
		t.Fatalf("updated agent = %+v", updatedAgent)
	}
	gotAgent, err := first.GetAgent(ctx, "daily-assistant")
	requireNoError(t, err)
	if gotAgent.Metadata["team"] != "runtime" {
		t.Fatalf("agent metadata = %+v", gotAgent.Metadata)
	}
	agents, err := first.ListAgents(ctx)
	requireNoError(t, err)
	if len(agents) != 1 {
		t.Fatalf("agents = %+v", agents)
	}

	provider, err := first.CreateProvider(ctx, ProviderCreateInput{
		Code:     "local-openai",
		Name:     "Local OpenAI",
		Type:     "openai_completions",
		BaseURL:  fixture.BaseURL("openai_completions"),
		APIKey:   "test-key",
		Metadata: map[string]any{"environment": "e2e"},
	})
	requireNoError(t, err)
	providerName := "Local OpenAI Compatible"
	provider, err = first.UpdateProvider(ctx, ProviderUpdateInput{
		Code: "local-openai",
		Name: &providerName,
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
	if len(provider.Models) != 1 || provider.Models[0].MaxOutputTokens != 100_000 {
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
		AgentCode:     "daily-assistant",
		ProviderCode:  "local-openai",
		ModelCode:     "primary-model",
		ContextWindow: 128_000,
	})
	requireNoError(t, err)
	secondary, err := first.CreateSession(ctx, SessionCreateInput{
		AgentCode:     "daily-assistant",
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
		AgentCode: "daily-assistant",
		Limit:     1,
		Offset:    1,
	})
	requireNoError(t, err)
	if len(summaries) != 1 || summaries[0].AgentCode != "daily-assistant" {
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
		1,
	)
	secondProviderRequest := waitForProviderCall(
		t,
		ctx,
		fixture.requests,
		2,
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
		AgentCode:     "daily-assistant",
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
		3,
	)
	models, err := second.ListProviderModels(ctx, "local-openai")
	requireNoError(t, err)
	if len(models) != 1 || models[0].Code != "fixture-model" || models[0].ContextWindow != 256_000 || models[0].MaxOutputTokens != 65_536 || len(models[0].ReasoningEfforts) != 0 {
		t.Fatalf("discovered models = %+v", models)
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

	var chunkedAgent Agent
	err = second.rpc.CallChunked(ctx, "agent.create", AgentCreateInput{
		Code: "chunked-agent",
		Name: "Chunked Agent",
		Soul: "This payload is split by the client and reassembled by core.",
	}, 17, &chunkedAgent)
	requireNoError(t, err)
	if chunkedAgent.Code != "chunked-agent" {
		t.Fatalf("chunked agent = %+v", chunkedAgent)
	}
	requireNoError(t, second.rpc.AbortChunk(ctx, "aborted-upload", "agent.create"))
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
	_, err = second.DeleteAgent(ctx, "chunked-agent")
	requireNoError(t, err)
	_, err = second.DeleteProvider(ctx, "local-openai")
	requireNoError(t, err)
	_, err = second.DeleteAgent(ctx, "daily-assistant")
	requireNoError(t, err)
	_, err = second.GetAgent(ctx, "daily-assistant")
	requireRPCCode(t, err, errNotFound)

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
