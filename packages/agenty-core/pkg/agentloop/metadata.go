package agentloop

import (
	"fmt"
	"os"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/utils"
)

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
		Model:           round.Model.ModelSlug.String(),
		Provider:        round.Model.ProviderSlug.String(),
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
