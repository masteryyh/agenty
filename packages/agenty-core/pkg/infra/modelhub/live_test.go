package modelhub

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLivePublicModelHubs(t *testing.T) {
	if os.Getenv("AGENTY_TEST_MODEL_HUB_LIVE") != "1" {
		t.Skip("set AGENTY_TEST_MODEL_HUB_LIVE=1 to check public APIs")
	}
	for _, source := range []Source{HuggingFace, ModelScope} {
		t.Run(string(source), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			catalog := NewCatalog()
			page, err := catalog.Search(ctx, source, "Qwen3.5-0.8B", "")
			if err != nil || len(page.Models) == 0 {
				t.Fatalf("search: %v %+v", err, page)
			}
			if page.Models[0].SizeBytes <= 0 {
				t.Fatalf("missing repository size: %+v", page.Models[0])
			}
			details, err := catalog.Inspect(ctx, source, "Qwen/Qwen3.5-0.8B")
			if err != nil || details.ModelType != "qwen3_5" || details.Architecture != "Dense" || len(details.Variants) == 0 {
				t.Fatalf("details: %v %+v", err, details)
			}
			file, ok := findFile(details.Files, "config.json")
			if !ok || strings.Contains(file.URL, "/main/") || strings.Contains(file.URL, "Revision=master") {
				t.Fatal("config download is not revision pinned")
			}
			m := NewManager(ctx, Options{Directory: t.TempDir()})
			defer m.cancel()
			if err := m.downloadFile(ctx, file, m.options.Directory+"/config.json", func(int64, string) {}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
