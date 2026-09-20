// Package metadata injects the session metadata understood by the model as
// hidden developer or user messages.
package metadata

import (
	"context"
	"fmt"
	"os"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	inframiddleware "github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/utils"
)

type sessionMetadataKey struct{}

func NewMiddleware() inframiddleware.Middleware {
	return inframiddleware.Middleware{
		Name:               "metadata",
		BeforeSessionStart: beforeSessionStart,
		BeforeRound:        beforeRound,
	}
}

func beforeSessionStart(
	ctx context.Context,
	state *inframiddleware.SessionStartContext,
) error {
	if state == nil || state.Session == nil || state.AppendHiddenMessage == nil {
		return nil
	}

	metadata, err := buildMetadata(state.Session, state.Provider, state.Model)
	if err != nil {
		return err
	}

	if metadata.Diff(nil).Empty() {
		return nil
	}

	text, err := metadata.XML()
	if err != nil {
		return fmt.Errorf("encode session metadata: %w", err)
	}

	role := messageRole(state.Provider)
	if err := state.AppendHiddenMessage(
		role,
		conversation.Text(text),
		map[string]any{"kind": "metadata", "scope": "session"},
	); err != nil {
		return err
	}

	baseContext := state.Context
	if baseContext == nil {
		baseContext = ctx
	}
	if baseContext == nil {
		baseContext = context.Background()
	}
	state.Context = context.WithValue(baseContext, sessionMetadataKey{}, metadata)
	return nil
}

func beforeRound(
	ctx context.Context,
	state *inframiddleware.RoundContext,
) error {
	if state == nil || state.Session == nil || state.AppendHiddenMessage == nil {
		return nil
	}

	metadata, err := buildMetadata(state.Session, state.Provider, state.Model)
	if err != nil {
		return err
	}
	if metadata.Diff(nil).Empty() {
		return nil
	}

	previous, ok := contextMetadata(state.Context)
	if !ok {
		previousMetadata := state.Session.LastMetadata()
		if previousMetadata != nil {
			previous = *previousMetadata
			ok = true
		}
	}
	if ok && metadata == previous {
		return nil
	}

	update := metadata.Diff(nil)
	if ok {
		update = metadata.Diff(&previous)
	}
	if update.Empty() {
		return nil
	}

	text, err := update.XML()
	if err != nil {
		return fmt.Errorf("encode round metadata: %w", err)
	}
	if err := state.AppendHiddenMessage(
		messageRole(state.Provider),
		conversation.Text(text),
		map[string]any{"kind": "metadata", "scope": "round"},
	); err != nil {
		return err
	}
	return nil
}

func buildMetadata(
	session *conversation.Session,
	provider *catalog.Provider,
	model *catalog.Model,
) (conversation.SessionMetadata, error) {
	if session == nil {
		return conversation.SessionMetadata{}, nil
	}

	cwd := ""
	if session.Cwd != nil {
		cwd = *session.Cwd
	} else {
		resolved, err := os.Getwd()
		if err != nil {
			return conversation.SessionMetadata{}, fmt.Errorf("resolve metadata cwd: %w", err)
		}
		cwd = resolved
	}
	result := conversation.SessionMetadata{
		Cwd:             cwd,
		ReasoningEffort: string(session.CurrentReasoningEffort),
		PermissionMode:  session.CurrentPermissionMode(),
		Timezone:        timezoneName(),
	}
	if session.CurrentModel != nil {
		result.Provider = session.CurrentModel.ProviderCode.String()
		result.Model = session.CurrentModel.ModelCode.String()
	}
	if provider != nil {
		result.Provider = provider.Code.String()
	}
	if model != nil {
		result.Model = model.Code.String()
	}
	return result, nil
}

func messageRole(provider *catalog.Provider) conversation.Role {
	if provider != nil && provider.SupportsDeveloperMessages() {
		return conversation.RoleDeveloper
	}
	return conversation.RoleUser
}

func contextMetadata(ctx context.Context) (conversation.SessionMetadata, bool) {
	if ctx == nil {
		return conversation.SessionMetadata{}, false
	}
	metadata, ok := ctx.Value(sessionMetadataKey{}).(conversation.SessionMetadata)
	return metadata, ok
}

func timezoneName() string {
	return utils.TimezoneName()
}
