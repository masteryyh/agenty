package llmcodec

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionCodecStreamingLifetimeAndErrors(t *testing.T) {
	directory, err := filepath.Abs("../../../../llm-codec/tests/fixtures/basic")
	if err != nil {
		t.Fatal(err)
	}
	model, err := Load(directory, LoadOptions{IgnoreGGUF: true})
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := model.Decoder()
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	text := "你好 world"
	tokens, err := model.Tokenize(text)
	if err != nil {
		t.Fatal(err)
	}
	// The decoder owns an independent Arc and must outlive the model handle.
	if err := model.Close(); err != nil {
		t.Fatal(err)
	}
	var result strings.Builder
	for _, token := range tokens {
		part, err := decoder.Push([]uint32{token}, false)
		if err != nil {
			t.Fatal(err)
		}
		result.WriteString(part)
	}
	tail, err := decoder.Push(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	result.WriteString(tail)
	if !strings.Contains(result.String(), "你好") {
		t.Fatalf("lost Unicode output: %q", result.String())
	}
	if _, err := decoder.Push(nil, false); err == nil {
		t.Fatal("finished decoder accepted more input")
	}
	if _, err := model.Tokenize("closed"); err == nil {
		t.Fatal("released model accepted input")
	}
}
