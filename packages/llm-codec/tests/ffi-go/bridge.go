package bridge

/*
#cgo CFLAGS: -I${SRCDIR}/../../include
#include "llm_codec.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type ffiError struct {
	Code    int32
	Message string
}

func (e *ffiError) Error() string { return fmt.Sprintf("llm-codec %d: %s", e.Code, e.Message) }

func takeError(status C.int32_t, failure *C.LlmCodecError) error {
	defer C.llm_codec_error_free(failure)
	if status == 0 {
		return nil
	}
	return &ffiError{Code: int32(status), Message: string(C.GoBytes(unsafe.Pointer(failure.message.ptr), C.int(failure.message.len)))}
}

func load(directory string) (uint64, []byte, error) {
	input := C.CBytes([]byte(directory))
	defer C.free(input)
	var handle C.LlmCodecHandle
	var metadata C.LlmCodecBytes
	var failure C.LlmCodecError
	status := C.llm_codec_model_load((*C.uint8_t)(input), C.size_t(len(directory)), nil, 0, &handle, &metadata, &failure)
	defer C.llm_codec_bytes_free(&metadata)
	if err := takeError(status, &failure); err != nil {
		return 0, nil, err
	}
	return uint64(handle), C.GoBytes(unsafe.Pointer(metadata.ptr), C.int(metadata.len)), nil
}

func encode(handle uint64, text string, special bool) ([]uint32, error) {
	input := C.CBytes([]byte(text))
	defer C.free(input)
	var tokens C.LlmCodecTokens
	var failure C.LlmCodecError
	var flag C.uint8_t
	if special {
		flag = 1
	}
	status := C.llm_codec_tokenize(C.LlmCodecHandle(handle), (*C.uint8_t)(input), C.size_t(len(text)), flag, &tokens, &failure)
	defer C.llm_codec_tokens_free(&tokens)
	if err := takeError(status, &failure); err != nil {
		return nil, err
	}
	return append([]uint32{}, unsafe.Slice((*uint32)(unsafe.Pointer(tokens.ptr)), int(tokens.len))...), nil
}

func decode(handle uint64, tokens []uint32, skip bool) (string, error) {
	var input *C.uint32_t
	if len(tokens) > 0 {
		input = (*C.uint32_t)(unsafe.Pointer(&tokens[0]))
	}
	var text C.LlmCodecBytes
	var failure C.LlmCodecError
	var flag C.uint8_t
	if skip {
		flag = 1
	}
	status := C.llm_codec_decode(C.LlmCodecHandle(handle), input, C.size_t(len(tokens)), flag, &text, &failure)
	defer C.llm_codec_bytes_free(&text)
	if err := takeError(status, &failure); err != nil {
		return "", err
	}
	return string(C.GoBytes(unsafe.Pointer(text.ptr), C.int(text.len))), nil
}

func render(handle uint64, request string) (string, error) {
	input := C.CBytes([]byte(request))
	defer C.free(input)
	var text C.LlmCodecBytes
	var failure C.LlmCodecError
	status := C.llm_codec_chat_render(C.LlmCodecHandle(handle), (*C.uint8_t)(input), C.size_t(len(request)), &text, &failure)
	defer C.llm_codec_bytes_free(&text)
	if err := takeError(status, &failure); err != nil {
		return "", err
	}
	return string(C.GoBytes(unsafe.Pointer(text.ptr), C.int(text.len))), nil
}

func resolveTemplates(directory string) ([]byte, error) {
	input := C.CBytes([]byte(directory))
	defer C.free(input)
	var info C.LlmCodecBytes
	var failure C.LlmCodecError
	status := C.llm_codec_template_resolve((*C.uint8_t)(input), C.size_t(len(directory)), nil, 0, &info, &failure)
	defer C.llm_codec_bytes_free(&info)
	if err := takeError(status, &failure); err != nil {
		return nil, err
	}
	return C.GoBytes(unsafe.Pointer(info.ptr), C.int(info.len)), nil
}

func release(handle uint64) error {
	var failure C.LlmCodecError
	return takeError(C.llm_codec_model_release(C.LlmCodecHandle(handle), &failure), &failure)
}

func abiVersion() uint32 { return uint32(C.llm_codec_abi_version()) }
