//go:build e2e

package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func testContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), processTimeout)
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func requireRPCCode(t *testing.T, err error, code int) *RPCError {
	t.Helper()

	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf(
			"error = %v, want RPC code %d",
			err,
			code,
		)
	}
	if rpcErr.Code != code {
		t.Fatalf(
			"RPC code = %d, want %d: %s",
			rpcErr.Code,
			code,
			rpcErr.Message,
		)
	}
	return rpcErr
}

func stringPointer(value string) *string {
	return &value
}

func createExecutionResources(
	ctx context.Context,
	client *agentyClient,
	fixture *providerFixture,
	apiType string,
	prefix string,
) (Session, error) {
	agentCode := prefix + "-agent"
	providerCode := prefix + "-provider"
	modelCode := prefix + "-model"

	if _, err := client.CreateAgent(ctx, AgentCreateInput{
		Code: agentCode,
		Name: "E2E Agent",
		Soul: "Answer the user clearly.",
	}); err != nil {
		return Session{}, fmt.Errorf("create agent: %w", err)
	}
	if _, err := client.CreateProvider(ctx, ProviderCreateInput{
		Code:         providerCode,
		Name:         "E2E Provider",
		Type:         apiType,
		BaseURL:      fixture.BaseURL(apiType),
		APIKey:       "test-key",
		FreeFormTool: apiType == "openai",
	}); err != nil {
		return Session{}, fmt.Errorf("create provider: %w", err)
	}
	if _, err := client.AddModel(ctx, ModelInput{
		ProviderCode:    providerCode,
		ModelCode:       modelCode,
		Name:            "E2E Model",
		ContextWindow:   128_000,
		MaxOutputTokens: 8_192,
	}); err != nil {
		return Session{}, fmt.Errorf("add model: %w", err)
	}

	session, err := client.CreateSession(ctx, SessionCreateInput{
		AgentCode:     agentCode,
		ProviderCode:  providerCode,
		ModelCode:     modelCode,
		ContextWindow: 128_000,
	})
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

func mergeMethodCounts(clients ...*rpcClient) map[string]int {
	merged := map[string]int{}
	for _, client := range clients {
		for method, count := range client.CalledMethods() {
			merged[method] += count
		}
	}
	return merged
}
