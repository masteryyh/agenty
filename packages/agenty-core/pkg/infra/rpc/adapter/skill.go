package adapter

import (
	"context"
	"encoding/json"

	domainskill "github.com/masteryyh/agenty-core/pkg/domain/skill"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
	"github.com/masteryyh/agenty-core/pkg/infra/skill"
)

type skillListResult struct {
	Skills      []domainskill.Entry      `json:"skills"`
	Diagnostics []domainskill.Diagnostic `json:"diagnostics"`
}

func RegisterSkillHandlers(d *rpc.Dispatcher, registry *skill.Registry) {
	d.Register("skill.list", func(_ context.Context, _ json.RawMessage) (any, error) {
		if registry == nil {
			return skillListResult{Skills: []domainskill.Entry{}, Diagnostics: []domainskill.Diagnostic{}}, nil
		}
		return skillListResult{
			Skills:      registry.Entries(),
			Diagnostics: registry.Diagnostics(),
		}, nil
	})
}
