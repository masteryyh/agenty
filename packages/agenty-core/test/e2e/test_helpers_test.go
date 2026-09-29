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

func requireAPIError(t *testing.T, err error, code string) *APIError {
	t.Helper()

	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error = %v, want HTTP API error %q", err, code)
	}
	if apiError.Code != code {
		t.Fatalf("API error code = %q, want %q: %s", apiError.Code, code, apiError.Message)
	}
	return apiError
}

func createExecutionResources(
	ctx context.Context,
	client *agentyClient,
	fixture *providerFixture,
	apiType string,
	prefix string,
) (Session, error) {
	providerCode := prefix + "-provider"
	modelCode := prefix + "-model"

	if _, err := client.CreateProvider(ctx, ProviderCreateInput{
		Code:    providerCode,
		Name:    "E2E Provider",
		Type:    apiType,
		BaseURL: fixture.BaseURL(apiType),
		APIKey:  "test-key",
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
		ProviderCode:  providerCode,
		ModelCode:     modelCode,
		ContextWindow: 128_000,
	})
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}
