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
	goruntime "runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/spf13/cobra"
)

type initOptions struct {
	providerRef        string
	apiKey             string
	apiKeyEnv          string
	baseURL            string
	modelRef           string
	embeddingModelRef  string
	webSearchProvider  string
	webSearchAPIKey    string
	webSearchAPIKeyEnv string
	firecrawlBaseURL   string
}

type initResult struct {
	Initialized            bool   `json:"initialized"`
	DatabaseType           string `json:"databaseType"`
	DatabasePath           string `json:"databasePath,omitempty"`
	SQLiteVectorPath       string `json:"sqliteVectorPath,omitempty"`
	SQLiteVectorDownloaded bool   `json:"sqliteVectorDownloaded,omitempty"`
	Provider               string `json:"provider"`
	DefaultModel           string `json:"defaultModel"`
	EmbeddingModel         string `json:"embeddingModel,omitempty"`
	WebSearchProvider      string `json:"webSearchProvider,omitempty"`
	DefaultAgent           string `json:"defaultAgent"`
}

func newInitCmd() *cobra.Command {
	var opts initOptions

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize local agenty state and core configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.providerRef, "provider", "", "Provider name or ID to configure")
	cmd.Flags().StringVar(&opts.apiKey, "api-key", "", "API key for the selected provider")
	cmd.Flags().StringVar(&opts.apiKeyEnv, "api-key-env", "", "Environment variable containing the provider API key")
	cmd.Flags().StringVar(&opts.baseURL, "base-url", "", "Override base URL for the selected provider")
	cmd.Flags().StringVar(&opts.modelRef, "model", "", "Default chat model reference (provider/name, code, or ID)")
	cmd.Flags().StringVar(&opts.embeddingModelRef, "embedding-model", "", "Embedding model reference (provider/name, code, or ID)")
	cmd.Flags().StringVar(&opts.webSearchProvider, "web-search-provider", "", "Web search provider: disabled, tavily, brave, or firecrawl")
	cmd.Flags().StringVar(&opts.webSearchAPIKey, "web-search-api-key", "", "API key for the selected web search provider")
	cmd.Flags().StringVar(&opts.webSearchAPIKeyEnv, "web-search-api-key-env", "", "Environment variable containing the web search provider API key")
	cmd.Flags().StringVar(&opts.firecrawlBaseURL, "firecrawl-base-url", "", "Optional Firecrawl base URL override")
	cmd.MarkFlagsMutuallyExclusive("api-key", "api-key-env")
	cmd.MarkFlagsMutuallyExclusive("web-search-api-key", "web-search-api-key-env")

	return cmd
}

func runInit(cmd *cobra.Command, opts initOptions) error {
	opts.providerRef = strings.TrimSpace(opts.providerRef)
	opts.modelRef = strings.TrimSpace(opts.modelRef)
	opts.embeddingModelRef = strings.TrimSpace(opts.embeddingModelRef)
	opts.baseURL = strings.TrimSpace(opts.baseURL)
	opts.webSearchProvider = strings.TrimSpace(opts.webSearchProvider)
	opts.firecrawlBaseURL = strings.TrimSpace(opts.firecrawlBaseURL)

	if opts.providerRef == "" {
		return withExitCode(fmt.Errorf("--provider is required"), 2)
	}
	if opts.modelRef == "" {
		return withExitCode(fmt.Errorf("--model is required"), 2)
	}

	providerAPIKey, err := resolveSecretFlag(opts.apiKey, opts.apiKeyEnv, "provider API key")
	if err != nil {
		return withExitCode(err, 2)
	}
	webSearchAPIKey, err := resolveSecretFlag(opts.webSearchAPIKey, opts.webSearchAPIKeyEnv, "web search API key")
	if err != nil {
		return withExitCode(err, 2)
	}

	sqliteInfo, err := inspectLocalSQLitePaths()
	if err != nil {
		return err
	}
	if !quietOutput {
		fmt.Fprintln(cmd.ErrOrStderr(), "Initializing local database and runtime...")
	}

	runtime, cfg, err := initLocalRuntime(cmd.Context(), false, true)
	if err != nil {
		return err
	}
	defer runtime.Close()

	if cfg.DB != nil && cfg.DB.Type == config.DatabaseTypeSQLite && !quietOutput {
		if sqliteInfo.extensionMissingBefore {
			fmt.Fprintf(cmd.ErrOrStderr(), "Downloaded sqlite-vector extension to %s\n", sqliteInfo.extensionPath)
		} else {
			fmt.Fprintf(cmd.ErrOrStderr(), "sqlite-vector extension is available at %s\n", sqliteInfo.extensionPath)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "SQLite database is ready at %s\n", sqliteInfo.databasePath)
	}

	provider, err := resolveProviderReference(runtime.Backend, opts.providerRef)
	if err != nil {
		return err
	}
	if provider.APIKeyCensored == "<not set>" && providerAPIKey == "" {
		return withExitCode(fmt.Errorf("provider %q is not configured; use --api-key or --api-key-env", provider.Name), 2)
	}

	if providerAPIKey != "" || opts.baseURL != "" {
		update := &models.UpdateModelProviderDto{}
		if providerAPIKey != "" {
			update.APIKey = providerAPIKey
		}
		if opts.baseURL != "" {
			update.BaseURL = opts.baseURL
		}
		updated, err := runtime.Backend.UpdateProvider(provider.ID, update)
		if err != nil {
			return err
		}
		provider = updated
	}
	if !quietOutput {
		fmt.Fprintf(cmd.ErrOrStderr(), "Configured provider: %s\n", provider.Name)
	}

	chatModel, err := resolveModelReference(runtime.Backend, opts.modelRef, false)
	if err != nil {
		return err
	}
	if chatModel.Provider == nil || chatModel.Provider.ID != provider.ID {
		return withExitCode(fmt.Errorf("model %q does not belong to provider %q", modelDisplayName(*chatModel), provider.Name), 2)
	}
	if !modelConfigured(*chatModel) {
		return withExitCode(fmt.Errorf("model %q is not configured because its provider API key is missing", modelDisplayName(*chatModel)), 2)
	}
	setDefault := true
	if err := runtime.Backend.UpdateModel(chatModel.ID, &models.UpdateModelDto{DefaultModel: &setDefault}); err != nil {
		return err
	}
	if !quietOutput {
		fmt.Fprintf(cmd.ErrOrStderr(), "Set default chat model: %s\n", modelDisplayName(*chatModel))
	}

	agentName, err := ensureDefaultAgent(runtime.Backend, []uuid.UUID{chatModel.ID})
	if err != nil {
		return err
	}
	if !quietOutput {
		fmt.Fprintf(cmd.ErrOrStderr(), "Updated default agent: %s\n", agentName)
	}

	embeddingDisplayName := ""
	if opts.embeddingModelRef != "" {
		embeddingModel, err := resolveModelReference(runtime.Backend, opts.embeddingModelRef, true)
		if err != nil {
			return err
		}
		if !modelConfigured(*embeddingModel) {
			return withExitCode(fmt.Errorf("embedding model %q is not configured because its provider API key is missing", modelDisplayName(*embeddingModel)), 2)
		}
		if _, err := runtime.Backend.UpdateSystemSettings(&models.UpdateSystemSettingsDto{EmbeddingModelID: &embeddingModel.ID}); err != nil {
			return err
		}
		embeddingDisplayName = modelDisplayName(*embeddingModel)
		if !quietOutput {
			fmt.Fprintf(cmd.ErrOrStderr(), "Set embedding model: %s\n", embeddingDisplayName)
		}
	}

	webSearchProvider, err := configureInitWebSearch(runtime.Backend, opts, webSearchAPIKey)
	if err != nil {
		return err
	}
	if webSearchProvider != "" && !quietOutput {
		fmt.Fprintf(cmd.ErrOrStderr(), "Configured web search provider: %s\n", webSearchProvider)
	}

	if err := runtime.Backend.SetInitialized(); err != nil {
		return err
	}

	result := initResult{
		Initialized:       true,
		DatabaseType:      cfg.DB.Type,
		Provider:          provider.Name,
		DefaultModel:      modelDisplayName(*chatModel),
		EmbeddingModel:    embeddingDisplayName,
		WebSearchProvider: webSearchProvider,
		DefaultAgent:      agentName,
	}
	if cfg.DB.Type == config.DatabaseTypeSQLite {
		result.DatabasePath = sqliteInfo.databasePath
		result.SQLiteVectorPath = sqliteInfo.extensionPath
		result.SQLiteVectorDownloaded = sqliteInfo.extensionMissingBefore
	}

	if outputJSON {
		return writeJSON(cmd, result)
	}

	rows := [][2]string{
		{"Initialized", strconv.FormatBool(result.Initialized)},
		{"Database type", result.DatabaseType},
		{"Provider", result.Provider},
		{"Default model", result.DefaultModel},
		{"Default agent", result.DefaultAgent},
	}
	if result.DatabasePath != "" {
		rows = append(rows, [2]string{"Database path", result.DatabasePath})
	}
	if result.SQLiteVectorPath != "" {
		rows = append(rows, [2]string{"sqlite-vector path", result.SQLiteVectorPath})
		rows = append(rows, [2]string{"sqlite-vector downloaded", strconv.FormatBool(result.SQLiteVectorDownloaded)})
	}
	if result.EmbeddingModel != "" {
		rows = append(rows, [2]string{"Embedding model", result.EmbeddingModel})
	}
	if result.WebSearchProvider != "" {
		rows = append(rows, [2]string{"Web search provider", result.WebSearchProvider})
	}
	if err := writeKeyValues(cmd, rows); err != nil {
		return err
	}
	if quietOutput {
		return nil
	}
	return writeLine(cmd, "Initialization completed.")
}

func resolveSecretFlag(value string, envName string, label string) (string, error) {
	value = strings.TrimSpace(value)
	envName = strings.TrimSpace(envName)
	if value != "" && envName != "" {
		return "", fmt.Errorf("%s cannot use both direct value and env var", label)
	}
	if value != "" {
		return value, nil
	}
	if envName == "" {
		return "", nil
	}
	resolved := os.Getenv(envName)
	if strings.TrimSpace(resolved) == "" {
		return "", fmt.Errorf("environment variable %s is empty or not set", envName)
	}
	return resolved, nil
}

type sqliteBootstrapInfo struct {
	databasePath           string
	extensionPath          string
	extensionMissingBefore bool
}

func inspectLocalSQLitePaths() (*sqliteBootstrapInfo, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, withExitCode(fmt.Errorf("failed to resolve user config dir: %w", err), 1)
	}
	agentyDir := filepath.Join(configDir, "agenty")
	extensionPath := filepath.Join(agentyDir, "vector"+localSQLiteExtensionSuffix())
	_, statErr := os.Stat(extensionPath)
	return &sqliteBootstrapInfo{
		databasePath:           filepath.Join(agentyDir, "agenty.db"),
		extensionPath:          extensionPath,
		extensionMissingBefore: os.IsNotExist(statErr),
	}, nil
}

func localSQLiteExtensionSuffix() string {
	switch goruntime.GOOS {
	case "darwin":
		return ".dylib"
	case "windows":
		return ".dll"
	default:
		return ".so"
	}
}

func resolveProviderReference(b backend.Backend, ref string) (*models.ModelProviderDto, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, withExitCode(fmt.Errorf("provider reference is required"), 2)
	}

	providers, err := listProvidersAll(b)
	if err != nil {
		return nil, err
	}

	if id, err := uuid.Parse(ref); err == nil {
		for _, provider := range providers {
			if provider.ID == id {
				return &provider, nil
			}
		}
		return nil, withExitCode(fmt.Errorf("provider not found: %s", ref), 2)
	}

	var matches []models.ModelProviderDto
	for _, provider := range providers {
		if strings.EqualFold(provider.Name, ref) {
			matches = append(matches, provider)
		}
	}
	switch len(matches) {
	case 0:
		return nil, withExitCode(fmt.Errorf("provider not found: %s", ref), 2)
	case 1:
		return &matches[0], nil
	default:
		return nil, withExitCode(fmt.Errorf("provider name is ambiguous: %s; use provider ID instead", ref), 2)
	}
}

func resolveModelReference(b backend.Backend, ref string, embedding bool) (*models.ModelDto, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, withExitCode(fmt.Errorf("model reference is required"), 2)
	}

	modelsList, err := listModelsAll(b)
	if err != nil {
		return nil, err
	}

	filtered := make([]models.ModelDto, 0, len(modelsList))
	for _, model := range modelsList {
		if model.EmbeddingModel == embedding {
			filtered = append(filtered, model)
		}
	}

	if id, err := uuid.Parse(ref); err == nil {
		for _, model := range filtered {
			if model.ID == id {
				return &model, nil
			}
		}
		return nil, withExitCode(fmt.Errorf("model not found: %s", ref), 2)
	}

	if strings.Contains(ref, "/") {
		parts := strings.SplitN(ref, "/", 2)
		providerName := strings.TrimSpace(parts[0])
		modelName := strings.TrimSpace(parts[1])
		var matches []models.ModelDto
		for _, model := range filtered {
			if model.Provider == nil {
				continue
			}
			if strings.EqualFold(model.Provider.Name, providerName) && strings.EqualFold(model.Name, modelName) {
				matches = append(matches, model)
			}
		}
		switch len(matches) {
		case 0:
			return nil, withExitCode(fmt.Errorf("model not found: %s", ref), 2)
		case 1:
			return &matches[0], nil
		default:
			return nil, withExitCode(fmt.Errorf("model reference is ambiguous: %s; use model ID instead", ref), 2)
		}
	}

	var codeMatches []models.ModelDto
	for _, model := range filtered {
		if strings.EqualFold(model.Code, ref) {
			codeMatches = append(codeMatches, model)
		}
	}
	if len(codeMatches) == 1 {
		return &codeMatches[0], nil
	}
	if len(codeMatches) > 1 {
		return nil, withExitCode(fmt.Errorf("model code is ambiguous: %s; use provider/name or model ID instead", ref), 2)
	}

	var nameMatches []models.ModelDto
	for _, model := range filtered {
		if strings.EqualFold(model.Name, ref) {
			nameMatches = append(nameMatches, model)
		}
	}
	switch len(nameMatches) {
	case 0:
		return nil, withExitCode(fmt.Errorf("model not found: %s", ref), 2)
	case 1:
		return &nameMatches[0], nil
	default:
		return nil, withExitCode(fmt.Errorf("model name is ambiguous: %s; use provider/name or model ID instead", ref), 2)
	}
}

func listProvidersAll(b backend.Backend) ([]models.ModelProviderDto, error) {
	return listAllPages(func(page, pageSize int) (*pagination.PagedResponse[models.ModelProviderDto], error) {
		return b.ListProviders(page, pageSize)
	})
}

func listModelsAll(b backend.Backend) ([]models.ModelDto, error) {
	return listAllPages(func(page, pageSize int) (*pagination.PagedResponse[models.ModelDto], error) {
		return b.ListModels(page, pageSize)
	})
}

func listAllPages[T any](fetch func(page, pageSize int) (*pagination.PagedResponse[T], error)) ([]T, error) {
	pageNum := 1
	items := make([]T, 0)
	for {
		result, err := fetch(pageNum, 100)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return items, nil
		}
		items = append(items, result.Data...)
		if len(result.Data) == 0 || int64(len(items)) >= result.Total {
			return items, nil
		}
		pageNum++
	}
}

func ensureDefaultAgent(b backend.Backend, modelIDs []uuid.UUID) (string, error) {
	agents, err := b.ListAgents(1, 100)
	if err != nil {
		return "", err
	}

	if agent := findDefaultAgent(agents.Data); agent != nil {
		isDefault := true
		if err := b.UpdateAgent(agent.ID, &models.UpdateAgentDto{
			IsDefault: &isDefault,
			ModelIDs:  &modelIDs,
		}); err != nil {
			return "", err
		}
		return agent.Name, nil
	}

	soul := ""
	agent, err := b.CreateAgent(&models.CreateAgentDto{
		Name:      "default",
		Soul:      &soul,
		IsDefault: true,
		ModelIDs:  modelIDs,
	})
	if err != nil {
		return "", err
	}
	return agent.Name, nil
}

func configureInitWebSearch(b backend.Backend, opts initOptions, resolvedKey string) (string, error) {
	if opts.webSearchProvider == "" {
		return "", nil
	}

	provider := models.WebSearchProvider(strings.ToLower(opts.webSearchProvider))
	if !slices.Contains([]models.WebSearchProvider{
		models.WebSearchProviderDisabled,
		models.WebSearchProviderTavily,
		models.WebSearchProviderBrave,
		models.WebSearchProviderFirecrawl,
	}, provider) {
		return "", withExitCode(fmt.Errorf("unsupported web search provider: %s", opts.webSearchProvider), 2)
	}

	settings, err := b.GetSystemSettings()
	if err != nil {
		return "", err
	}

	dto := &models.UpdateSystemSettingsDto{WebSearchProvider: &provider}
	switch provider {
	case models.WebSearchProviderDisabled:
	case models.WebSearchProviderTavily:
		if resolvedKey == "" && !webSearchProviderConfigured(settings, provider) {
			return "", withExitCode(fmt.Errorf("web search provider %s requires --web-search-api-key or --web-search-api-key-env", provider), 2)
		}
		if resolvedKey != "" {
			dto.TavilyAPIKey = &resolvedKey
		}
	case models.WebSearchProviderBrave:
		if resolvedKey == "" && !webSearchProviderConfigured(settings, provider) {
			return "", withExitCode(fmt.Errorf("web search provider %s requires --web-search-api-key or --web-search-api-key-env", provider), 2)
		}
		if resolvedKey != "" {
			dto.BraveAPIKey = &resolvedKey
		}
	case models.WebSearchProviderFirecrawl:
		if resolvedKey == "" && !webSearchProviderConfigured(settings, provider) {
			return "", withExitCode(fmt.Errorf("web search provider %s requires --web-search-api-key or --web-search-api-key-env", provider), 2)
		}
		if resolvedKey != "" {
			dto.FirecrawlAPIKey = &resolvedKey
		}
		if opts.firecrawlBaseURL != "" {
			dto.FirecrawlBaseURL = &opts.firecrawlBaseURL
		}
	}

	if _, err := b.UpdateSystemSettings(dto); err != nil {
		return "", err
	}
	return string(provider), nil
}

func webSearchProviderConfigured(settings *models.SystemSettingsDto, provider models.WebSearchProvider) bool {
	if settings == nil {
		return false
	}
	switch provider {
	case models.WebSearchProviderTavily:
		return settings.TavilyAPIKey != ""
	case models.WebSearchProviderBrave:
		return settings.BraveAPIKey != ""
	case models.WebSearchProviderFirecrawl:
		return settings.FirecrawlAPIKey != ""
	default:
		return false
	}
}
