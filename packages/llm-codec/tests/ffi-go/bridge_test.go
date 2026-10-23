package bridge

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestStaticLibraryFromGo(t *testing.T) {
	directory, err := filepath.Abs("../fixtures/basic")
	if err != nil {
		t.Fatal(err)
	}
	handle, metadata, err := load(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := release(handle); err != nil {
			t.Error(err)
		}
	})
	var info struct {
		BosTokenID uint32 `json:"bosTokenId"`
		EosTokenID uint32 `json:"eosTokenId"`
		ModelName  string `json:"modelName"`
	}
	if err := json.Unmarshal(metadata, &info); err != nil {
		t.Fatal(err)
	}
	if abiVersion() != 2 || info.BosTokenID != 1 || info.EosTokenID != 2 || info.ModelName != "llm-codec-test-model" {
		t.Fatalf("unexpected ABI or metadata: %s", metadata)
	}
	tokens, err := encode(handle, "Hello world 你好", true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tokens, []uint32{1, 3, 4, 5, 2}) {
		t.Fatalf("tokens: %v", tokens)
	}
	text, err := decode(handle, tokens, true)
	if err != nil || text != "Hello world 你好" {
		t.Fatalf("decode: %q %v", text, err)
	}
	text, err = render(handle, `{"messages":[{"role":"user","content":"Hello"}],"addGenerationPrompt":true}`)
	if err != nil || text != "[BOS]user: Hello[EOS]assistant: " {
		t.Fatalf("render: %q %v", text, err)
	}
	if _, err := resolveTemplates(directory); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for range 8 {
		wait.Go(func() {
			for range 50 {
				if _, err := encode(handle, "Hello", false); err != nil {
					t.Error(err)
				}
				if _, err := render(handle, `{"messages":[]}`); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wait.Wait()
}

func TestErrorsAndTemplateOnlyDirectory(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "chat_template.jinja"), []byte("template only"), 0600); err != nil {
		t.Fatal(err)
	}
	handle, metadata, err := load(directory)
	var failure *ffiError
	if handle != 0 || metadata != nil || !errors.As(err, &failure) || failure.Code != 1201 {
		t.Fatalf("missing tokenizer: %d %s %v", handle, metadata, err)
	}
	if _, err := resolveTemplates(directory); err != nil {
		t.Fatal(err)
	}
	if _, err := encode(0, "Hello", false); !errors.As(err, &failure) || failure.Code != 1004 {
		t.Fatalf("invalid handle: %v", err)
	}
	fixture, _ := filepath.Abs("../fixtures/basic")
	handle, _, err = load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encode(handle, string([]byte{255}), false); !errors.As(err, &failure) || failure.Code != 1002 {
		t.Fatalf("invalid UTF-8: %v", err)
	}
	if _, err := render(handle, "{"); !errors.As(err, &failure) || failure.Code != 1003 {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, err := decode(handle, []uint32{99}, true); !errors.As(err, &failure) || failure.Code != 1206 {
		t.Fatalf("invalid token ID: %v", err)
	}
	if err := release(handle); err != nil {
		t.Fatal(err)
	}
	if _, err := encode(handle, "Hello", false); !errors.As(err, &failure) || failure.Code != 1004 {
		t.Fatalf("released handle: %v", err)
	}
}
