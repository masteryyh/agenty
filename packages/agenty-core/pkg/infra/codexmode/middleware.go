// Package codexmode selects the session's built-in file-tool dialect.
package codexmode

import (
	"context"
	"fmt"
	"strings"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	inframiddleware "github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	infratools "github.com/masteryyh/agenty-core/pkg/infra/tools"
)

const textEditorToolName = "str_replace_based_edit_tool"

var codexBuiltinToolNames = map[string]struct{}{
	"apply_patch": {},
	"glob":        {},
	"grep":        {},
	"ls":          {},
	"read_file":   {},
	"shell":       {},
}

// Config controls the middleware's provider-neutral prompt guidance.
type Config struct{}

func NewMiddleware(_ Config) inframiddleware.Middleware {
	return inframiddleware.Middleware{
		Name:               "codex-mode",
		BeforeSessionStart: beforeSessionStart,
		BeforeRound:        beforeRound,
	}
}

func beforeSessionStart(_ context.Context, state *inframiddleware.SessionStartContext) error {
	if state == nil || state.Session == nil {
		return nil
	}
	if err := validateProvider(state.Session, state.Provider); err != nil {
		return err
	}
	if state.Tools != nil {
		*state.Tools = selectTools(*state.Tools, state.Session.CurrentToolDialect())
	}
	if state.SystemPrompt != nil {
		*state.SystemPrompt = appendPrompt(*state.SystemPrompt, state.Session.CurrentToolDialect())
	}
	return nil
}

func beforeRound(_ context.Context, state *inframiddleware.RoundContext) error {
	if state == nil || state.Session == nil {
		return nil
	}
	if err := validateProvider(state.Session, state.Provider); err != nil {
		return err
	}
	if state.Tools != nil {
		*state.Tools = selectTools(*state.Tools, state.Session.CurrentToolDialect())
	}
	if state.SystemPrompt != nil {
		*state.SystemPrompt = appendPrompt(*state.SystemPrompt, state.Session.CurrentToolDialect())
	}
	return nil
}

func validateProvider(session *conversation.Session, provider *catalog.Provider) error {
	if session.CurrentToolDialect() != conversation.ToolDialectCodex {
		return nil
	}
	if provider == nil || provider.Type != catalog.APIOpenAI {
		return fmt.Errorf("Codex Mode requires a Responses API provider")
	}
	return nil
}

func selectTools(runtime agentloop.ToolRuntime, dialect conversation.ToolDialect) agentloop.ToolRuntime {
	if runtime == nil {
		return nil
	}
	return infratools.Filter(runtime, func(definition modelcall.ToolDefinition) bool {
		switch dialect.Normalized() {
		case conversation.ToolDialectCodex:
			_, isCodexBuiltin := codexBuiltinToolNames[definition.Name]
			return isCodexBuiltin || strings.HasPrefix(definition.Name, "mcp__")
		default:
			return definition.Name != "read_file" && definition.Name != "apply_patch"
		}
	})
}

func appendPrompt(prompt string, dialect conversation.ToolDialect) string {
	prompt = strings.ReplaceAll(prompt, "\n\n"+defaultPrompt, "")
	prompt = strings.ReplaceAll(prompt, "\n\n"+codexPrompt, "")
	if prompt == defaultPrompt || prompt == codexPrompt {
		prompt = ""
	}
	section := defaultPrompt
	if dialect.Normalized() == conversation.ToolDialectCodex {
		section = codexPrompt
	}
	base := strings.TrimRight(prompt, " \t\r\n")
	if base == "" {
		return section
	}
	return base + "\n\n" + section
}

const defaultPrompt = `<file-editing>
Use str_replace_based_edit_tool for every local file or directory read and every text edit. Its view command reads files or lists directories; use str_replace only when the old text occurs exactly once, create only for a new path, and insert after the requested 1-based line.
</file-editing>`

const codexPrompt = `<file-editing>
Use read_file to inspect local files. Use the free-form apply_patch tool for file modifications. Its input must be a complete V4A patch, including the opening *** Begin Patch and closing *** End Patch markers.
</file-editing>`
