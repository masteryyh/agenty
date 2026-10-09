"""Regenerate byte-level fixtures with pinned Hugging Face/Jinja versions.

uv run --with transformers==4.57.1 --with tokenizers==0.22.2 \
    --with jinja2==3.1.6 python scripts/generate-reference.py
"""

import json
from importlib.metadata import version
from pathlib import Path

from transformers import PreTrainedTokenizerFast

ROOT = Path(__file__).resolve().parents[1]
FIXTURES = ROOT / "tests" / "fixtures"
VERSIONS = {name: version(name) for name in ("transformers", "tokenizers", "jinja2")}
assert VERSIONS == {
    "transformers": "4.57.1", "tokenizers": "0.22.2", "jinja2": "3.1.6"
}, VERSIONS

tokenizer = PreTrainedTokenizerFast(
    tokenizer_file=str(FIXTURES / "basic" / "tokenizer.json"),
    bos_token="[BOS]", eos_token="[EOS]", unk_token="[UNK]",
)


def normalize(request):
    systems = [request["systemPrompt"]] if request.get("systemPrompt") else []
    messages = []
    for message in request["messages"]:
        if message["role"] == "system":
            if message.get("content"):
                systems.append(message["content"])
            continue
        content = message.get("content")
        if content is None and message["role"] == "assistant":
            content = ""
        item = {"role": message["role"], "content": content}
        for source, target in (
            ("reasoningContent", "reasoning_content"),
            ("toolCallId", "tool_call_id"), ("name", "name"),
        ):
            if source in message:
                item[target] = message[source]
        if message.get("toolCalls"):
            item["tool_calls"] = [
                {"id": call["id"], "type": "function", "function": {
                    "name": call["function"]["name"],
                    "arguments": call["function"]["arguments"],
                }} for call in message["toolCalls"]
            ]
        messages.append(item)
    if systems:
        messages.insert(0, {"role": "system", "content": "\n\n".join(systems)})
    return messages


tool = {"type": "function", "function": {
    "name": "weather", "description": "查询天气", "parameters": {
        "type": "object", "properties": {"city": {"type": "string"}},
        "required": ["city"],
    },
}}
basic = (FIXTURES / "basic" / "chat_template.jinja").read_text()
qwen = (FIXTURES / "qwen3.jinja").read_text()
cases = [
    ("basic-system", basic, {
        "systemPrompt": "Keep whitespace. ", "messages": [
            {"role": "system", "content": "中文约束"},
            {"role": "user", "content": " Hello\n世界\t"},
        ], "addGenerationPrompt": True,
    }),
    ("json-python-format", "{{ payload | tojson }}", {
        "messages": [{"role": "user", "content": ""}], "templateKwargs": {
            "payload": {"z": "中文😀<&/", "a": [1, True, None, 1e-7, 1e20, 1e16, 1e15, 1e-4, 1e-5, 1e-6, -0.0, 1.25e-8, 1.2e15, 5e-324]},
        },
    }),
    ("json-ascii-indent", "{{ payload | tojson(ensure_ascii=true, sort_keys=true, indent=2) }}", {
        "messages": [{"role": "user", "content": ""}], "templateKwargs": {
            "payload": {"z": "中文😀", "a": [{"b": 2, "a": 1}, []]},
        },
    }),
    ("qwen-thinking-default", qwen, {
        "messages": [{"role": "user", "content": "你好"}], "addGenerationPrompt": True,
    }),
    ("qwen-thinking-off", qwen, {
        "systemPrompt": "Answer briefly.", "messages": [{"role": "user", "content": "Hello"}],
        "addGenerationPrompt": True, "reasoningEffort": "off",
    }),
    ("qwen-tools", qwen, {
        "systemPrompt": "Use tools.", "tools": [tool], "messages": [
            {"role": "user", "content": "北京天气？"},
            {"role": "assistant", "content": None, "reasoningContent": "需要查询。", "toolCalls": [
                {"id": "call1", "function": {"name": "weather", "arguments": {"city": "北京"}}},
            ]},
            {"role": "tool", "toolCallId": "call1", "content": "晴，20°C"},
        ], "addGenerationPrompt": True,
    }),
    ("qwen-historical-reasoning", qwen, {
        "messages": [
            {"role": "user", "content": "First"},
            {"role": "assistant", "content": "<think>old reasoning</think>\n\nFirst answer"},
            {"role": "user", "content": "Next"},
            {"role": "assistant", "content": "Answer", "reasoningContent": "current reasoning"},
        ], "addGenerationPrompt": False,
    }),
    ("complete-tool-projection", "{{ messages | tojson }}\n{{ tools | tojson }}", {
        "tools": [tool], "messages": [
            {"role": "assistant", "content": None, "reasoningContent": "思考", "name": "agent", "toolCalls": [
                {"id": "call1", "function": {"name": "weather", "arguments": {"city": "北京"}}},
            ]},
            {"role": "tool", "toolCallId": "call1", "content": "晴"},
        ],
    }),
]
render_cases = []
for name, template, request in cases:
    kwargs = dict(request.get("templateKwargs", {}))
    if request.get("reasoningEffort") == "off":
        kwargs["enable_thinking"] = False
    result = tokenizer.apply_chat_template(
        normalize(request), chat_template=template, tokenize=False,
        tools=request.get("tools") or None,
        add_generation_prompt=request.get("addGenerationPrompt", False), **kwargs,
    )
    render_cases.append({"name": name, "template": template, "request": request, "expected": result})

encode_cases = []
for text in ("", "Hello world 你好", " Hello\nworld\t你好 ", "[BOS]Hello[EOS]", "😀\u0000"):
    for add_special_tokens in (False, True):
        ids = tokenizer.encode(text, add_special_tokens=add_special_tokens)
        encode_cases.append({
            "text": text, "addSpecialTokens": add_special_tokens, "ids": ids,
            "decoded": tokenizer.decode(ids, skip_special_tokens=False),
            "decodedSkipSpecial": tokenizer.decode(ids, skip_special_tokens=True),
        })

output = {"versions": VERSIONS, "renderCases": render_cases, "encodeCases": encode_cases}
(FIXTURES / "huggingface-reference.json").write_text(
    json.dumps(output, ensure_ascii=False, indent=4) + "\n"
)
print(f"Generated {len(render_cases)} render and {len(encode_cases)} tokenizer reference cases")
