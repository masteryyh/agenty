package agentloop

import (
	"fmt"
	"os"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/utils"
)

func fullMetadataXML(session *conversation.Session) (string, error) {
	if session.CurrentModel == nil || session.CurrentModel.IsZero() {
		return "", nil
	}

	cwd := ""
	if session.Cwd != nil {
		cwd = *session.Cwd
	} else {
		resolved, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve metadata cwd: %w", err)
		}
		cwd = resolved
	}

	metadata := conversation.SessionMetadata{
		Cwd:             cwd,
		Model:           session.CurrentModel.ModelCode.String(),
		Provider:        session.CurrentModel.ProviderCode.String(),
		Timezone:        utils.TimezoneName(),
		ReasoningEffort: string(session.CurrentReasoningEffort),
	}
	return metadata.XML()
}

func metadataForRound(
	session *conversation.Session,
	round conversation.Round,
) (conversation.Content, error) {
	cwd, err := roundCwd(round)
	if err != nil {
		return nil, fmt.Errorf("resolve metadata cwd: %w", err)
	}

	current := conversation.SessionMetadata{
		Cwd:             cwd,
		Model:           round.Model.ModelCode.String(),
		Provider:        round.Model.ProviderCode.String(),
		Timezone:        utils.TimezoneName(),
		ReasoningEffort: string(round.ReasoningEffort),
	}
	update := current.Diff(session.LastMetadata())
	if update.Empty() {
		return conversation.Content{}, nil
	}

	text, err := update.XML()
	if err != nil {
		return nil, fmt.Errorf("encode metadata XML: %w", err)
	}
	return conversation.Text(text), nil
}

func roundCwd(round conversation.Round) (string, error) {
	if round.Cwd != nil {
		return *round.Cwd, nil
	}

	return os.Getwd()
}
