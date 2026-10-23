// Package llmcodec owns the production cgo boundary to the Rust codec.
package llmcodec

/*
#cgo CFLAGS: -I${SRCDIR}/../../../../llm-codec/include
#include "llm_codec.h"
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"

	json "github.com/bytedance/sonic"
)

type Metadata struct {
	VocabSize    int      `json:"vocabSize"`
	StopTokenIDs []uint32 `json:"stopTokenIds"`
	ModelType    string   `json:"modelType"`
}

type LoadOptions struct {
	IgnoreGGUF       bool   `json:"ignoreGguf"`
	GGUFMetadataPath string `json:"ggufMetadataPath,omitempty"`
}

type Model struct {
	mu       sync.RWMutex
	handle   C.LlmCodecHandle
	Metadata Metadata
}

type Error struct {
	Code    int32
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("llm-codec %d: %s", e.Code, e.Message) }

func takeError(status C.int32_t, failure *C.LlmCodecError) error {
	defer C.llm_codec_error_free(failure)
	if status == 0 {
		return nil
	}
	return &Error{int32(status), string(C.GoBytes(unsafe.Pointer(failure.message.ptr), C.int(failure.message.len)))}
}

func bytePointer(data []byte) *C.uint8_t {
	if len(data) == 0 {
		return nil
	}
	return (*C.uint8_t)(unsafe.Pointer(&data[0]))
}

func Load(directory string, options LoadOptions) (*Model, error) {
	if C.llm_codec_abi_version() != 2 {
		return nil, fmt.Errorf("llm-codec ABI 2 required")
	}
	path := []byte(directory)
	opts, err := json.Marshal(options)
	if err != nil {
		return nil, err
	}
	var handle C.LlmCodecHandle
	var metadata C.LlmCodecBytes
	var failure C.LlmCodecError
	status := C.llm_codec_model_load(bytePointer(path), C.size_t(len(path)), bytePointer(opts), C.size_t(len(opts)), &handle, &metadata, &failure)
	defer C.llm_codec_bytes_free(&metadata)
	if err := takeError(status, &failure); err != nil {
		return nil, err
	}
	model := &Model{handle: handle}
	if err := json.Unmarshal(C.GoBytes(unsafe.Pointer(metadata.ptr), C.int(metadata.len)), &model.Metadata); err != nil {
		_ = model.Close()
		return nil, fmt.Errorf("decode codec metadata: %w", err)
	}
	return model, nil
}

func (m *Model) ChatEncode(request any) ([]uint32, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var tokens C.LlmCodecTokens
	var failure C.LlmCodecError
	status := C.llm_codec_chat_encode(m.handle, bytePointer(data), C.size_t(len(data)), &tokens, &failure)
	defer C.llm_codec_tokens_free(&tokens)
	if err := takeError(status, &failure); err != nil {
		return nil, err
	}
	return append([]uint32{}, unsafe.Slice((*uint32)(unsafe.Pointer(tokens.ptr)), int(tokens.len))...), nil
}

func (m *Model) Tokenize(text string) ([]uint32, error) {
	data := []byte(text)
	m.mu.RLock()
	defer m.mu.RUnlock()
	var tokens C.LlmCodecTokens
	var failure C.LlmCodecError
	status := C.llm_codec_tokenize(m.handle, bytePointer(data), C.size_t(len(data)), 0, &tokens, &failure)
	defer C.llm_codec_tokens_free(&tokens)
	if err := takeError(status, &failure); err != nil {
		return nil, err
	}
	return append([]uint32{}, unsafe.Slice((*uint32)(unsafe.Pointer(tokens.ptr)), int(tokens.len))...), nil
}

func (m *Model) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handle == 0 {
		return nil
	}
	var failure C.LlmCodecError
	err := takeError(C.llm_codec_model_release(m.handle, &failure), &failure)
	m.handle = 0
	return err
}

type Decoder struct {
	mu     sync.Mutex
	handle C.LlmCodecHandle
}

func (m *Model) Decoder() (*Decoder, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var handle C.LlmCodecHandle
	var failure C.LlmCodecError
	if err := takeError(C.llm_codec_decoder_create(m.handle, &handle, &failure), &failure); err != nil {
		return nil, err
	}
	return &Decoder{handle: handle}, nil
}

func (d *Decoder) Push(ids []uint32, finish bool) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var input *C.uint32_t
	if len(ids) > 0 {
		input = (*C.uint32_t)(unsafe.Pointer(&ids[0]))
	}
	var flag C.uint8_t
	if finish {
		flag = 1
	}
	var output C.LlmCodecBytes
	var failure C.LlmCodecError
	status := C.llm_codec_decoder_push(d.handle, input, C.size_t(len(ids)), flag, &output, &failure)
	defer C.llm_codec_bytes_free(&output)
	if err := takeError(status, &failure); err != nil {
		return "", err
	}
	return string(C.GoBytes(unsafe.Pointer(output.ptr), C.int(output.len))), nil
}

func (d *Decoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle == 0 {
		return nil
	}
	var failure C.LlmCodecError
	err := takeError(C.llm_codec_decoder_release(d.handle, &failure), &failure)
	d.handle = 0
	return err
}
