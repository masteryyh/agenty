# llm-codec

Rust static library for local model tokenization and chat-template rendering.
It exports a versioned C ABI for the future Go core inference path. The production
core does not call it yet; `tests/ffi-go` exercises the release archive through cgo.

## Build and verify

From the repository root:

```bash
pnpm llm-codec:build
pnpm llm-codec:test
pnpm llm-codec:lint
```

The build generates `include/llm_codec.h` with cbindgen and writes
`bin/<rust-target>/libllm_codec.a` plus `native-static-libs.txt`. Link the
archive and the target-specific flags in that file. Do not hardcode flags from
another target. `GOOS`/`GOARCH` take precedence over `OS`/`ARCH`. The workspace
dependency orders this library's build before core.

Targets: macOS amd64/arm64, Linux GNU amd64/arm64, Windows amd64 GNU. Install the
corresponding Rust target before cross compilation. Windows cgo requires
MinGW-w64 or LLVM-MinGW; the archive uses `x86_64-pc-windows-gnu`. An MSVC archive
is not interchangeable. `CC`/`CXX` select the native compiler. A static archive
still depends on the target's system libraries.

The implementation uses Hugging Face `tokenizers` 0.22.2 and MiniJinja 2.24 with
Python-method compatibility. Runtime operations stay local and never download
model files or execute Python.

## Model directory and template selection

`tokenizer.json` is required for loading. Its normalizer, pre-tokenizer, model,
post-processor, decoder, and added-token definitions remain authoritative. Saved
padding and truncation are disabled so an inference prompt is not silently modified.
Encoding and decoding take explicit special-token flags.

Optional files are `config.json`, `tokenizer_config.json`, `special_tokens_map.json`,
and `generation_config.json`. Token strings and IDs are cross-checked against
`tokenizer.json`; conflicts fail loading. A missing ID is JSON `null`, so ID zero
remains valid. Metadata includes `schemaVersion`, `modelName`, `modelNameSource`,
`modelType`, `vocabSize`, `bosTokenId`, `eosTokenId`, `eosTokenIds`, `stopTokenIds`,
`padTokenId`, `unkTokenId`, `specialTokens`, `tokenizerSource`, and `chatTemplates`.

`eosTokenId` identifies the single tokenizer EOS when available. `eosTokenIds`
preserves the model's complete EOS list. `stopTokenIds` uses generation-config
EOS settings when present and includes GGUF EOT/EOM IDs. Special-token records
contain text, ID, semantic roles, and tokenizer matching attributes. Named
special tokens become template string variables.

Template discovery combines these sources:

1. `chat_template.jinja` overrides the `default` template.
2. `tokenizer_config.json` supplies a string or named-template array. Files in
   `additional_chat_templates/<name>.jinja` override that name.
3. When neither source supplies a template, GGUF supplies
   `tokenizer.chat_template` and `tokenizer.chat_template.<name>`.

All discovered templates compile during loading. Missing templates return
`TEMPLATE_NOT_FOUND`; invalid higher-priority files fail rather than silently
falling back. Rendering selects explicit `templateName`, then `tool_use` when
tools are present, then `default`, then the sole template. Otherwise selection
is ambiguous and needs an explicit name.

Only GGUF v2/v3 key/value metadata is read; tensor data and vocabulary tables are
skipped. GGUF never supplies a tokenizer, including `tokenizer.huggingface.json`.
A GGUF-only directory can resolve templates independently, but model loading
returns `TOKENIZER_NOT_FOUND`. Safetensors/PyTorch weights do not provide a
supported fallback here. Python-only implementations such as special DeepSeek
scripts are unsupported.

One GGUF is selected automatically. A complete, consistently named split set
selects its first shard. Multiple quantizations require load options such as
`{"ggufFile":"model-q4.gguf"}`. Model loading reads selected GGUF metadata even
when file-based templates exist and reports ambiguity or conflicting IDs.

## C ABI and ownership

The generated header is the source of truth for signatures and numeric codes:

| Entry point | Output |
| --- | --- |
| `llm_codec_abi_version` | ABI version, currently 1 |
| `llm_codec_model_load` | Opaque `uint64_t` handle and metadata JSON |
| `llm_codec_template_resolve` | Validated `{name, source, text}` array; no tokenizer required |
| `llm_codec_tokenize` | `uint32_t` token array |
| `llm_codec_decode` | UTF-8 text |
| `llm_codec_chat_render` | UTF-8 prompt |
| `llm_codec_model_release` | Releases the handle |
| `llm_codec_bytes_free`, `llm_codec_tokens_free`, `llm_codec_error_free` | Release Rust-owned outputs |

Inputs are pointer/length pairs consumed only during the call. Text and paths
are UTF-8, outputs are not NUL-terminated, and embedded NUL in text is allowed.
`NULL` is accepted for a zero-length input and optional `out_error`. Other outputs
must be writable, aligned, and non-overlapping. Initialize output slots to zero
and free them before reuse. Never use C `free` on Rust outputs or copy an owning
output struct. Free helpers zero the same slot, making repeated freeing of that
zeroed slot safe.

Every operation returns an `int32_t` status: zero is success. Optional per-call
`LlmCodecError` contains that code and an owned UTF-8 diagnostic. Free it on success
and failure. There is no thread-local last-error state.

Handles are registry IDs rather than Rust addresses. Models are immutable and
calls can run concurrently. A call that acquires the model before release may
finish; later calls return `INVALID_HANDLE`. Output buffers remain valid after
model release until freed. Loading publishes a handle only after all data validates.
Rust panics are contained at operational FFI boundaries; caller pointer violations
remain outside the ABI contract.

```c
#include "llm_codec.h"
#include <string.h>

LlmCodecHandle handle = 0;
LlmCodecBytes metadata = {0};
LlmCodecTokens tokens = {0};
LlmCodecError error = {0};
const char *directory = "/models/example";
int32_t status = llm_codec_model_load((const uint8_t *)directory, strlen(directory),
                                NULL, 0, &handle, &metadata, &error);
llm_codec_bytes_free(&metadata);
llm_codec_error_free(&error);
if (status == 0) {
    const char *text = "Hello";
    status = llm_codec_tokenize(handle, (const uint8_t *)text, strlen(text),
                          0, &tokens, &error);
    /* Copy/use tokens.ptr[0..tokens.len] only when status == 0. */
    llm_codec_tokens_free(&tokens);
    llm_codec_error_free(&error);
    llm_codec_model_release(handle, &error);
    llm_codec_error_free(&error);
}
```

See `tests/ffi-go/bridge.go` for cgo examples. It is a test bridge, not a
production core API.

## Render request

The FFI accepts lowerCamelCase JSON. Unknown request/message fields fail validation.

```json
{
    "systemPrompt": "Answer briefly.",
    "messages": [
        {"role": "user", "content": "北京天气？"},
        {
            "role": "assistant",
            "content": null,
            "reasoningContent": "需要查询天气。",
            "toolCalls": [{
                "id": "call_1",
                "function": {"name": "weather", "arguments": {"city": "北京"}}
            }]
        },
        {"role": "tool", "toolCallId": "call_1", "content": "晴，20°C"}
    ],
    "tools": [{
        "type": "function",
        "function": {
            "name": "weather",
            "parameters": {"type": "object", "properties": {"city": {"type": "string"}}}
        }
    }],
    "addGenerationPrompt": true,
    "templateKwargs": {"enable_thinking": false}
}
```

`messages` is required. Other fields are optional; `systemPrompt` and `tools`
default to empty, `addGenerationPrompt` defaults to false. Roles are `system`,
`user`, `assistant`, and `tool`. Content is text, with `null` allowed for assistant
messages. Image/audio content is unsupported. Function-call arguments are JSON
objects, not serialized JSON strings. Tool results require `toolCallId`.

The top-level system prompt and all system messages are joined once with `\n\n`
and placed at the start. Other order is preserved. Message fields become
`reasoning_content`, `tool_calls`, and `tool_call_id`; tool calls gain
`type: "function"`.

Optional `templateName` selects a template. `templateKwargs` passes model-specific
variables, but cannot overwrite messages, tools, special tokens, or helpers.
`reasoningEffort` accepts `off`, `low`, `medium`, `high`, `xhigh`, or `max` and is
passed only to templates referencing `reasoning_effort`. `off` can instead disable
`enable_thinking`. Other grades are never silently converted to a boolean;
unsupported mappings return `UNSUPPORTED_OPTION`. For Qwen's boolean control,
use explicit `templateKwargs.enable_thinking`.

Rendering disables HTML escaping, enables whitespace trimming and Python-method
compatibility, and supplies `raise_exception`, `strftime_now`, and Python-style
`tojson` formatting (Unicode, separators, indentation, sorting, float notation).
Optional `currentTime` is an RFC3339 clock override; otherwise the local clock is
used. MiniJinja is not a full Python/Jinja runtime. Unsupported methods, filters,
functions, and tags such as `{% generation %}` return explicit template errors.

Tokenize rendered prompts with `add_special_tokens=0`: templates commonly emit
BOS/EOS markers already. Decode follows the tokenizer decoder; lossy normalizers
do not guarantee original text round trips.

## Limits and verification

Text/request inputs are limited to 16 MiB, token inputs to 4M IDs, configuration
files and scanned GGUF metadata to 128 MiB, templates to 1 MiB each, template
collections to 64 names/8 MiB, and outputs to 32 MiB. Rendering uses fuel
1,000,000 and recursion limit 100. These are operational bounds, not a promise
that every intermediate allocation is bounded.

Stable error groups: 1001–1004 arguments/UTF-8/JSON/handle; 1101–1105 model
files/GGUF; 1201–1206 tokenizer/token IDs; 1301–1305 templates; 1401–1403
request/content/options; 1501–1503 limits/internal/panic. Generated
`LlmCodecErrorCode` constants list the exact values.

Rust tests cover byte-level tokenization, templates, metadata conflicts, GGUF
corruption/selection, ownership, and release races. Go tests link the release
archive and exercise multilingual data, errors, and concurrency. Golden references
include an actual Qwen3 template and pinned Transformers 4.57.1, Tokenizers 0.22.2,
and Jinja2 3.1.6 outputs. Optional regeneration instructions are in
`scripts/generate-reference.py`; Python is only needed for that test-data step.
See `tests/fixtures/README.md` for attribution.
