package modelcall

import (
	"context"
	"errors"
	"fmt"

	domain "github.com/masteryyh/agenty-core/pkg/domain/localinference"
	"github.com/masteryyh/agenty-core/pkg/infra/llmcodec"
)

// CountLocalTokens renders the complete prompt, including ordered tool schemas.
// Counting does not acquire a worker lease, so compaction can call the same model.
func CountLocalTokens(ctx context.Context, model ModelCallConfig, request ModelCallRequest) (int64, error) {
	input, err := localRequest(model, request)
	if err != nil {
		return 0, err
	}
	var count int64
	var failures []error
	valid := false
	for _, resource := range model.Local.Resources {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if preference := model.Local.BackendPreference; preference != "" && preference != "auto" && preference != string(resource.Backend) {
			continue
		}
		options := llmcodec.LoadOptions{IgnoreGGUF: resource.Backend != domain.LlamaCpp}
		if resource.Backend == domain.LlamaCpp {
			options.GGUFMetadataPath = resource.WeightsPath
		}
		codec, err := llmcodec.Load(resource.CodecDirectory, options)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		ids, err := codec.ChatEncode(input)
		_ = codec.Close()
		if err != nil {
			failures = append(failures, err)
			continue
		}
		count = max(count, int64(len(ids)))
		valid = true
	}
	if !valid {
		return 0, fmt.Errorf("count local prompt: %w", errors.Join(failures...))
	}
	return count, nil
}
