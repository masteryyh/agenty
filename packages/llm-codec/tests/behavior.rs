use std::fs;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use llm_codec::{resolve_templates, ErrorCode, LoadOptions, Model, RenderRequest};
use serde_json::{json, Value};

fn fixture() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR")).join("tests/fixtures/basic")
}
fn copy_fixture() -> tempfile::TempDir {
    let directory = tempfile::tempdir().unwrap();
    for name in [
        "tokenizer.json",
        "tokenizer_config.json",
        "config.json",
        "chat_template.jinja",
    ] {
        fs::copy(fixture().join(name), directory.path().join(name)).unwrap();
    }
    directory
}
fn request(value: Value) -> RenderRequest {
    serde_json::from_value(value).unwrap()
}
fn load(path: &Path) -> Model {
    Model::load(path, &LoadOptions::default()).unwrap()
}
fn set_template(path: &Path, text: &str) {
    fs::write(path.join("chat_template.jinja"), text).unwrap();
}
fn failure<T>(result: llm_codec::Result<T>) -> ErrorCode {
    match result {
        Ok(_) => panic!("expected failure"),
        Err(e) => e.code,
    }
}

#[test]
fn pinned_huggingface_reference_matches_bytes_and_ids() {
    let reference: Value = serde_json::from_str(
        &fs::read_to_string(
            fixture()
                .parent()
                .unwrap()
                .join("huggingface-reference.json"),
        )
        .unwrap(),
    )
    .unwrap();
    for case in reference["renderCases"].as_array().unwrap() {
        let directory = copy_fixture();
        set_template(directory.path(), case["template"].as_str().unwrap());
        let model = load(directory.path());
        assert_eq!(
            model.render(&request(case["request"].clone())).unwrap(),
            case["expected"].as_str().unwrap(),
            "{}",
            case["name"],
        );
    }
    let model = load(&fixture());
    for case in reference["encodeCases"].as_array().unwrap() {
        let ids: Vec<u32> = serde_json::from_value(case["ids"].clone()).unwrap();
        assert_eq!(
            model
                .tokenize(
                    case["text"].as_str().unwrap(),
                    case["addSpecialTokens"].as_bool().unwrap()
                )
                .unwrap(),
            ids
        );
        assert_eq!(
            model.decode(&ids, false).unwrap(),
            case["decoded"].as_str().unwrap()
        );
        assert_eq!(
            model.decode(&ids, true).unwrap(),
            case["decodedSkipSpecial"].as_str().unwrap()
        );
    }
}

#[test]
fn exact_ids_special_tokens_and_metadata() {
    let model = load(&fixture());
    assert_eq!(
        model.tokenize("Hello world 你好", false).unwrap(),
        [3, 4, 5]
    );
    assert_eq!(model.tokenize("Hello world", true).unwrap(), [1, 3, 4, 2]);
    assert_eq!(model.decode(&[1, 3, 4, 2], true).unwrap(), "Hello world");
    assert_eq!(
        model.decode(&[1, 3, 4, 2], false).unwrap(),
        "[BOS] Hello world [EOS]"
    );
    assert_eq!(model.metadata.unk_token_id, Some(0));
    assert_eq!(model.metadata.bos_token_id, Some(1));
    assert_eq!(model.metadata.eos_token_id, Some(2));
    assert_eq!(model.metadata.model_name, "llm-codec-test-model");
    assert_eq!(model.metadata.tokenizer_source, "tokenizer.json");
    assert_eq!(
        failure(model.decode(&[99], true)),
        ErrorCode::InvalidTokenId
    );
}

#[test]
fn multiple_stop_ids_and_conflicts_are_preserved() {
    let directory = copy_fixture();
    fs::write(
        directory.path().join("generation_config.json"),
        r#"{"eos_token_id":[2,0,2]}"#,
    )
    .unwrap();
    let model = load(directory.path());
    assert_eq!(model.metadata.stop_token_ids, [2, 0]);
    fs::write(
        directory.path().join("config.json"),
        r#"{"eos_token_id":5}"#,
    )
    .unwrap();
    assert_eq!(
        failure(Model::load(directory.path(), &LoadOptions::default())),
        ErrorCode::TokenIdConflict
    );
}

#[test]
fn nested_text_eos_and_tokenizer_flags_do_not_conflict_with_chat_eos() {
    let directory = copy_fixture();
    fs::write(
        directory.path().join("config.json"),
        r#"{"model_type":"qwen3_5","text_config":{"eos_token_id":0}}"#,
    )
    .unwrap();
    fs::write(
        directory.path().join("tokenizer_config.json"),
        r#"{"eos_token":"[EOS]","add_bos_token":false,"add_eos_token":false}"#,
    )
    .unwrap();
    let model = load(directory.path());
    assert_eq!(model.metadata.eos_token_id, Some(2));
    assert_eq!(model.metadata.stop_token_ids, [0, 2]);
    let rendered = request(
        json!({"messages":[{"role":"user","content":"Hello world"}],"addGenerationPrompt":true}),
    );
    assert_eq!(
        model.chat_encode(&rendered).unwrap(),
        model
            .tokenize(&model.render(&rendered).unwrap(), false)
            .unwrap()
    );
}

#[test]
fn named_special_tokens_and_shared_roles_are_preserved() {
    let directory = copy_fixture();
    fs::write(directory.path().join("special_tokens_map.json"),
        r#"{"cls_token":"[BOS]","sep_token":{"content":"[EOS]"},"mask_token":"[UNK]","pad_token":"[EOS]"}"#).unwrap();
    set_template(
        directory.path(),
        "{{ cls_token }}{{ sep_token }}{{ mask_token }}{{ pad_token }}",
    );
    let model = load(directory.path());
    assert_eq!(
        model.render(&request(json!({"messages":[]}))).unwrap(),
        "[BOS][EOS][UNK][EOS]"
    );
    assert_eq!(model.metadata.pad_token_id, Some(2));
    assert!(model
        .metadata
        .special_tokens
        .iter()
        .any(|token| token.id == 2
            && token.roles.contains(&"sep_token".to_owned())
            && token.roles.contains(&"pad_token".to_owned())));
}

#[test]
fn malformed_config_and_missing_tokenizer_fail_atomically() {
    let directory = copy_fixture();
    fs::write(directory.path().join("tokenizer_config.json"), "[]").unwrap();
    assert_eq!(
        failure(Model::load(directory.path(), &LoadOptions::default())),
        ErrorCode::InvalidJson
    );
    fs::remove_file(directory.path().join("tokenizer.json")).unwrap();
    assert_eq!(
        failure(Model::load(directory.path(), &LoadOptions::default())),
        ErrorCode::TokenizerNotFound
    );
}

#[test]
fn loaded_truncation_and_padding_are_disabled() {
    let directory = copy_fixture();
    let path = directory.path().join("tokenizer.json");
    let mut value: Value = serde_json::from_slice(&fs::read(&path).unwrap()).unwrap();
    value["truncation"] =
        json!({"direction":"Right","max_length":1,"strategy":"LongestFirst","stride":0});
    value["padding"] = json!({"strategy":{"Fixed":8},"direction":"Right","pad_to_multiple_of":null,"pad_id":0,"pad_type_id":0,"pad_token":"[UNK]"});
    fs::write(path, serde_json::to_vec(&value).unwrap()).unwrap();
    assert_eq!(
        load(directory.path())
            .tokenize("Hello world 你好", false)
            .unwrap(),
        [3, 4, 5]
    );
}

#[test]
fn render_text_is_exact_and_special_tokens_are_not_duplicated() {
    let model = load(&fixture());
    let text = model
        .render(&request(
            json!({"messages":[{"role":"user","content":"Hello"}],"addGenerationPrompt":true}),
        ))
        .unwrap();
    assert_eq!(text, "[BOS]user: Hello[EOS]assistant: ");
    let ids = model.tokenize(&text, false).unwrap();
    assert_eq!(ids.iter().filter(|id| **id == 1).count(), 1);
    assert_eq!(ids.iter().filter(|id| **id == 2).count(), 1);
}

#[test]
fn system_prompt_and_tool_exchange_are_projected_once() {
    let directory = copy_fixture();
    set_template(directory.path(), "{{ messages|tojson }}");
    let model = load(directory.path());
    let text = model.render(&request(json!({"systemPrompt":"first","messages":[{"role":"system","content":"second"},{"role":"assistant","content":null,"reasoningContent":"thinking","toolCalls":[{"id":"call-1","function":{"name":"lookup","arguments":{"key":"值"}}}]},{"role":"tool","content":"found","toolCallId":"call-1"}]}))).unwrap();
    let messages: Value = serde_json::from_str(&text).unwrap();
    assert_eq!(
        messages[0],
        json!({"role":"system","content":"first\n\nsecond"})
    );
    assert_eq!(messages[1]["reasoning_content"], "thinking");
    assert_eq!(
        messages[1]["tool_calls"][0]["function"]["arguments"],
        json!({"key":"值"})
    );
    assert_eq!(messages[2]["tool_call_id"], "call-1");
    assert_eq!(messages.as_array().unwrap().len(), 3);
}

#[test]
fn python_methods_json_format_and_clock_match_reference_bytes() {
    let directory = copy_fixture();
    set_template(directory.path(), "{{ messages[0].content.strip().upper() }}|{{ template_data|tojson }}|{{ template_data|tojson(ensure_ascii=true,separators=(',', ':'),sort_keys=true) }}|{{ strftime_now('%Y-%m-%d') }}");
    let model = load(directory.path());
    let text = model.render(&request(json!({"messages":[{"role":"user","content":" hello "}],"currentTime":"2026-10-08T12:00:00+08:00","templateKwargs":{"template_data":{"z":"<你好😀>","a":1}}}))).unwrap();
    assert_eq!(text, "HELLO|{\"z\": \"<你好😀>\", \"a\": 1}|{\"a\":1,\"z\":\"<\\u4f60\\u597d\\ud83d\\ude00>\"}|2026-10-08");
    set_template(directory.path(), "{{ data|tojson(indent=2) }}");
    assert_eq!(
        load(directory.path())
            .render(&request(
                json!({"messages":[],"templateKwargs":{"data":{"a":[1,2]}}})
            ))
            .unwrap(),
        "{\n  \"a\": [\n    1,\n    2\n  ]\n}"
    );
}

#[test]
fn named_templates_and_source_precedence() {
    let directory = copy_fixture();
    fs::write(directory.path().join("tokenizer_config.json"), r#"{"chat_template":[{"name":"default","template":"legacy"},{"name":"tool_use","template":"{{ tools[0].function.name }}"}]}"#).unwrap();
    let model = load(directory.path());
    assert!(model
        .metadata
        .chat_templates
        .iter()
        .find(|info| info.name == "default")
        .unwrap()
        .source
        .ends_with("chat_template.jinja"));
    assert_eq!(model.render(&request(json!({"messages":[],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]}))).unwrap(), "lookup");
    fs::create_dir(directory.path().join("additional_chat_templates")).unwrap();
    fs::write(
        directory
            .path()
            .join("additional_chat_templates/tool_use.jinja"),
        "standalone",
    )
    .unwrap();
    assert_eq!(
        load(directory.path())
            .render(&request(json!({"messages":[],"templateName":"tool_use"})))
            .unwrap(),
        "standalone"
    );
    set_template(directory.path(), "{% if %}");
    assert_eq!(
        failure(Model::load(directory.path(), &LoadOptions::default())),
        ErrorCode::TemplateSyntax
    );
}

#[test]
fn ambiguity_and_unsupported_tags_are_explicit() {
    let directory = copy_fixture();
    fs::remove_file(directory.path().join("chat_template.jinja")).unwrap();
    fs::write(
        directory.path().join("tokenizer_config.json"),
        r#"{"chat_template":[{"name":"one","template":"one"},{"name":"two","template":"two"}]}"#,
    )
    .unwrap();
    assert_eq!(
        failure(load(directory.path()).render(&request(json!({"messages":[]})))),
        ErrorCode::AmbiguousTemplate
    );
    set_template(directory.path(), "{% generation %}x{% endgeneration %}");
    assert_eq!(
        failure(Model::load(directory.path(), &LoadOptions::default())),
        ErrorCode::UnsupportedTemplateFeature
    );
}

#[test]
fn unsupported_content_options_and_template_exceptions() {
    let directory = copy_fixture();
    let model = load(directory.path());
    assert_eq!(
        failure(model.render(&request(
            json!({"messages":[{"role":"user","content":[{"type":"image"}]}]})
        ))),
        ErrorCode::UnsupportedContent
    );
    assert_eq!(
        failure(model.render(&request(
            json!({"messages":[],"templateKwargs":{"bos_token":"fake"}})
        ))),
        ErrorCode::InvalidRequest
    );
    assert_eq!(
        failure(model.render(&request(json!({"messages":[],"reasoningEffort":"high"})))),
        ErrorCode::UnsupportedOption
    );
    set_template(
        directory.path(),
        "{{ raise_exception('role order invalid') }}",
    );
    let error = load(directory.path())
        .render(&request(json!({"messages":[]})))
        .unwrap_err();
    assert_eq!(error.code, ErrorCode::TemplateRender);
    assert!(error.message.contains("role order invalid"));
    set_template(directory.path(), "{{ reasoning_effort }}");
    assert_eq!(
        load(directory.path())
            .render(&request(json!({"messages":[],"reasoningEffort":"high"})))
            .unwrap(),
        "high"
    );
    set_template(directory.path(), "{{ enable_thinking }}");
    assert_eq!(
        load(directory.path())
            .render(&request(json!({"messages":[],"reasoningEffort":"off"})))
            .unwrap(),
        "False"
    );
}

#[test]
fn byte_level_tokenizer_roundtrips_unicode_whitespace_and_nul() {
    use tokenizers::models::bpe::BPE;
    use tokenizers::pre_tokenizers::byte_level::ByteLevel;
    use tokenizers::Tokenizer;
    let directory = copy_fixture();
    let mut alphabet: Vec<char> = ByteLevel::alphabet().into_iter().collect();
    alphabet.sort_unstable();
    let vocab: ahash::AHashMap<String, u32> = alphabet
        .into_iter()
        .enumerate()
        .map(|(id, c)| (c.to_string(), id as u32))
        .collect();
    let mut tokenizer = Tokenizer::new(
        BPE::builder()
            .vocab_and_merges(vocab, Vec::new())
            .build()
            .unwrap(),
    );
    tokenizer.with_pre_tokenizer(Some(ByteLevel::new(false, true, false)));
    tokenizer.with_decoder(Some(tokenizers::decoders::byte_level::ByteLevel::default()));
    tokenizer
        .save(directory.path().join("tokenizer.json"), false)
        .unwrap();
    fs::remove_file(directory.path().join("config.json")).unwrap();
    fs::write(directory.path().join("tokenizer_config.json"), "{}").unwrap();
    set_template(directory.path(), "{{ messages[0].content }}");
    let model = load(directory.path());
    let text = " 你好\t😀\r\n\0 e\u{301}  ";
    assert_eq!(
        model
            .decode(&model.tokenize(text, false).unwrap(), false)
            .unwrap(),
        text
    );
}

#[test]
fn model_calls_are_isolated_across_threads_and_models() {
    let first = Arc::new(load(&fixture()));
    let directory = copy_fixture();
    set_template(directory.path(), "second");
    let second = Arc::new(load(directory.path()));
    let threads: Vec<_> = (0..8)
        .map(|i| {
            let model = if i % 2 == 0 {
                first.clone()
            } else {
                second.clone()
            };
            std::thread::spawn(move || {
                for _ in 0..50 {
                    assert_eq!(model.tokenize("Hello world", false).unwrap(), [3, 4]);
                    let text = model.render(&request(json!({"messages":[]}))).unwrap();
                    assert_eq!(text, if i % 2 == 0 { "[BOS]" } else { "second" });
                }
            })
        })
        .collect();
    for thread in threads {
        thread.join().unwrap();
    }
}

fn gguf_file(path: &Path, big_endian: bool, entries: &[(&str, Value)]) {
    let mut bytes = Vec::new();
    let u32_bytes = |v: u32| {
        if big_endian {
            v.to_be_bytes()
        } else {
            v.to_le_bytes()
        }
    };
    let u64_bytes = |v: u64| {
        if big_endian {
            v.to_be_bytes()
        } else {
            v.to_le_bytes()
        }
    };
    bytes.extend_from_slice(if big_endian { b"FUGG" } else { b"GGUF" });
    bytes.extend_from_slice(&u32_bytes(3));
    bytes.extend_from_slice(&u64_bytes(0));
    bytes.extend_from_slice(&u64_bytes(entries.len() as u64));
    for (key, value) in entries {
        bytes.extend_from_slice(&u64_bytes(key.len() as u64));
        bytes.extend_from_slice(key.as_bytes());
        if let Some(text) = value.as_str() {
            bytes.extend_from_slice(&u32_bytes(8));
            bytes.extend_from_slice(&u64_bytes(text.len() as u64));
            bytes.extend_from_slice(text.as_bytes());
        } else {
            bytes.extend_from_slice(&u32_bytes(4));
            bytes.extend_from_slice(&u32_bytes(value.as_u64().unwrap() as u32));
        }
    }
    fs::write(path, bytes).unwrap();
}

#[test]
fn gguf_templates_and_metadata_work_without_importing_gguf_tokenizers() {
    for big_endian in [false, true] {
        let directory = copy_fixture();
        fs::remove_file(directory.path().join("chat_template.jinja")).unwrap();
        gguf_file(
            &directory.path().join("model.gguf"),
            big_endian,
            &[
                ("general.name", json!("GGUF display name")),
                ("tokenizer.chat_template", json!("from gguf")),
                (
                    "tokenizer.huggingface.json",
                    json!("invalid tokenizer JSON intentionally ignored"),
                ),
                ("tokenizer.ggml.bos_token_id", json!(1)),
            ],
        );
        let model = load(directory.path());
        assert_eq!(model.metadata.model_name, "GGUF display name");
        assert_eq!(model.tokenize("Hello", false).unwrap(), [3]);
        assert_eq!(
            model.render(&request(json!({"messages":[]}))).unwrap(),
            "from gguf"
        );
        fs::remove_file(directory.path().join("tokenizer.json")).unwrap();
        assert_eq!(
            failure(Model::load(directory.path(), &LoadOptions::default())),
            ErrorCode::TokenizerNotFound
        );
        assert_eq!(
            resolve_templates(directory.path(), &LoadOptions::default()).unwrap()[0].text,
            "from gguf"
        );
    }
}

#[test]
fn gguf_ambiguity_corruption_and_shards() {
    let directory = copy_fixture();
    for name in ["a.gguf", "b.gguf"] {
        gguf_file(
            &directory.path().join(name),
            false,
            &[("tokenizer.chat_template", json!("gguf"))],
        );
    }
    assert_eq!(
        failure(Model::load(directory.path(), &LoadOptions::default())),
        ErrorCode::AmbiguousModelFile
    );
    Model::load(
        directory.path(),
        &LoadOptions {
            gguf_file: Some("a.gguf".to_owned()),
            ..Default::default()
        },
    )
    .unwrap();
    fs::write(directory.path().join("a.gguf"), b"GGUF").unwrap();
    assert_eq!(
        failure(Model::load(
            directory.path(),
            &LoadOptions {
                gguf_file: Some("a.gguf".to_owned()),
                ..Default::default()
            }
        )),
        ErrorCode::InvalidGguf
    );
    fs::remove_file(directory.path().join("a.gguf")).unwrap();
    fs::remove_file(directory.path().join("b.gguf")).unwrap();
    gguf_file(
        &directory.path().join("model-00001-of-00002.gguf"),
        false,
        &[("split.no", json!(0)), ("split.count", json!(2))],
    );
    gguf_file(
        &directory.path().join("model-00002-of-00002.gguf"),
        false,
        &[("split.no", json!(1)), ("split.count", json!(2))],
    );
    load(directory.path());
}

#[test]
fn template_fuel_limit_is_enforced() {
    let directory = copy_fixture();
    set_template(
        directory.path(),
        "{% for i in range(100000) %}{% for j in range(10) %}{% set value=j %}{% endfor %}{% endfor %}",
    );
    assert_eq!(
        failure(load(directory.path()).render(&request(json!({"messages":[]})))),
        ErrorCode::ResourceLimit
    );
}
