package localinference

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"

	json "github.com/bytedance/sonic"
	domain "github.com/masteryyh/agenty-core/pkg/domain/localinference"
)

const BundleIndexURL = "https://github.com/masteryyh/agenty/releases/download/workers-v0.1.0/index.json"

type Bundle struct {
	Name    string         `json:"name"`
	Backend domain.Backend `json:"backend"`
	Version string         `json:"version"`
	URL     string         `json:"url"`
	SHA256  string         `json:"sha256"`
	OS      string         `json:"os"`
	Arch    string         `json:"arch"`
}

func AvailableBundles(ctx context.Context) ([]Bundle, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, BundleIndexURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("worker release index unavailable (HTTP %d)", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 2<<20 {
		return nil, fmt.Errorf("worker release index exceeds limit")
	}
	var index struct {
		SchemaVersion int      `json:"schemaVersion"`
		Bundles       []Bundle `json:"bundles"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	if index.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported worker release index version")
	}
	bundles := make([]Bundle, 0)
	for _, bundle := range index.Bundles {
		if bundle.OS == runtime.GOOS && bundle.Arch == runtime.GOARCH && bundle.Backend.Valid() {
			bundles = append(bundles, bundle)
		}
	}
	return bundles, nil
}
