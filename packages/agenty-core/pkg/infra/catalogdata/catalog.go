package catalogdata

import (
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"strings"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

//go:embed providers.json
var providersJSON []byte

func LoadProviders() ([]*catalog.Provider, error) {
	var providers []*catalog.Provider
	if err := json.Unmarshal(providersJSON, &providers); err != nil {
		return nil, fmt.Errorf("catalogdata: decode embedded providers: %w", err)
	}
	if len(providers) == 0 {
		return nil, errors.New("catalogdata: embedded providers are empty")
	}

	seen := make(map[shared.Code]struct{}, len(providers))
	for index, provider := range providers {
		if provider == nil {
			return nil, fmt.Errorf("catalogdata: provider %d is null", index)
		}
		if err := validateProvider(provider); err != nil {
			return nil, fmt.Errorf("catalogdata: provider %d: %w", index, err)
		}
		if _, ok := seen[provider.Code]; ok {
			return nil, fmt.Errorf("catalogdata: duplicate provider code %q", provider.Code)
		}

		seen[provider.Code] = struct{}{}
		provider.Builtin = true
		if provider.Models == nil {
			provider.Models = make([]catalog.Model, 0)
		}
		for modelIndex := range provider.Models {
			provider.Models[modelIndex].ReasoningEfforts = shared.NormalizeReasoningEfforts(
				provider.Models[modelIndex].ReasoningEfforts,
			)
		}
	}
	return providers, nil
}

func validateProvider(provider *catalog.Provider) error {
	if !provider.Code.Valid() {
		return errors.New("invalid provider code")
	}
	if strings.TrimSpace(provider.Name) == "" {
		return errors.New("provider name is empty")
	}
	if !provider.Type.Valid() {
		return fmt.Errorf("invalid API type %q", provider.Type)
	}
	if strings.TrimSpace(provider.BaseURL) == "" {
		return errors.New("base URL is empty")
	}
	if err := validateRelativeURL(provider.ModelsURL); err != nil {
		return fmt.Errorf("models URL: %w", err)
	}
	if err := validateRelativeURL(provider.TokenCountURL); err != nil {
		return fmt.Errorf("token count URL: %w", err)
	}
	for index := range provider.Models {
		model := &provider.Models[index]
		if model.Code.IsZero() || strings.TrimSpace(model.Name) == "" {
			return fmt.Errorf("model %d has an empty code or name", index)
		}
		if model.ContextWindow <= 0 || model.MaxOutputTokens <= 0 {
			return fmt.Errorf("model %q has invalid token limits", model.Code)
		}
		if err := validateReasoningEfforts(model); err != nil {
			return err
		}
	}
	return nil
}

func validateReasoningEfforts(model *catalog.Model) error {
	seen := make(map[shared.ReasoningEffort]struct{}, len(model.ReasoningEfforts))
	for _, effort := range model.ReasoningEfforts {
		if !effort.Valid() || !effort.Enabled() {
			return fmt.Errorf("model %q has invalid reasoning effort %q", model.Code, effort)
		}
		if _, ok := seen[effort]; ok {
			return fmt.Errorf("model %q repeats reasoning effort %q", model.Code, effort)
		}
		seen[effort] = struct{}{}
	}
	return nil
}

func validateRelativeURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.IsAbs() || parsed.Host != "" {
		return errors.New("must be relative to the provider base URL")
	}
	return nil
}
