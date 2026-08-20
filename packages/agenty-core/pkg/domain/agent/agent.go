package agent

import (
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

const BaseSystemPrompt = `<basic>
You are an AI agent running inside Agenty, an open source agent harness. You and the user share the same workspace and collaborate to achieve the user's goals.

A piece of prompt called Soul will be given to you, which defines your interactive style, attitude to everything, problem solving approaches etc. You must follow your Soul and meet user's expectations at best effort.

Sometimes there will be a piece of XML data that follows user's message, which contains basic session and harness config. This kind of data are given by harness and they will look like this:

` + "```xml" + `
<metadata>
	<cwd>~/Documents</cwd>
	<model>deepseek-v4-pro</model>
	<provider>deepseek</provider>
	<timezone>Asia/Shanghai</timezone>
	<reasoning-effort>high</reasoning-effort>
</metadata>
` + "```" + `

You will receive this at the very beginning of the session, and maybe more after if something has changed by user or harness. You must follow these messages and treat them as truth.
</basic>

<soul>
{{ .Soul }}
</soul>`

var baseSystemPromptTemplate = template.Must(
	template.New("agent_system_prompt").Parse(BaseSystemPrompt),
)

type Agent struct {
	Code                   shared.Code            `json:"code"`
	Name                   string                 `json:"name"`
	Description            string                 `json:"description,omitempty"`
	Soul                   string                 `json:"soul"`
	DefaultModel           *shared.ModelRef       `json:"defaultModel,omitempty"`
	DefaultContextWindow   int64                  `json:"defaultContextWindow"`
	DefaultReasoningEffort shared.ReasoningEffort `json:"defaultReasoningEffort,omitempty"`
	IsDefault              bool                   `json:"isDefault"`
	Metadata               shared.Metadata        `json:"metadata,omitempty"`
	CreatedAt              time.Time              `json:"createdAt"`
	UpdatedAt              time.Time              `json:"updatedAt"`
}

func New(code, name string) (*Agent, error) {
	s, err := shared.NewCode(code)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	return &Agent{
		Code:      s,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (a *Agent) ResolveSystemPrompt() (string, error) {
	data := struct {
		Soul string
	}{Soul: a.Soul}

	var prompt strings.Builder
	if err := baseSystemPromptTemplate.Execute(&prompt, data); err != nil {
		return "", fmt.Errorf("agent: resolve system prompt: %w", err)
	}
	return prompt.String(), nil
}
