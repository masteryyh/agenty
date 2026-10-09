use std::collections::BTreeMap;
use std::path::Path;

use serde::Serialize;
use serde_json::Value;
use tokenizers::{AddedToken, Tokenizer};

use crate::error::{Error, ErrorCode, Result};
use crate::gguf;
use crate::template::TemplateInfo;

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SpecialToken {
    pub id: u32,
    pub content: String,
    pub roles: Vec<String>,
    pub is_special: bool,
    pub normalized: Option<bool>,
    pub single_word: Option<bool>,
    pub lstrip: Option<bool>,
    pub rstrip: Option<bool>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ModelMetadata {
    pub schema_version: u32,
    pub model_name: String,
    pub model_name_source: String,
    pub model_type: Option<String>,
    pub vocab_size: usize,
    pub bos_token_id: Option<u32>,
    pub eos_token_id: Option<u32>,
    pub eos_token_ids: Vec<u32>,
    pub stop_token_ids: Vec<u32>,
    pub pad_token_id: Option<u32>,
    pub unk_token_id: Option<u32>,
    pub special_tokens: Vec<SpecialToken>,
    pub tokenizer_source: String,
    pub chat_templates: Vec<TemplateInfo>,
    #[serde(skip)]
    token_context: BTreeMap<String, Value>,
}

pub(crate) struct Sources<'a> {
    pub config: &'a Value,
    pub tokenizer_config: &'a Value,
    pub special_map: &'a Value,
    pub generation_config: &'a Value,
    pub gguf: &'a gguf::Metadata,
}

impl ModelMetadata {
    pub(crate) fn load(
        directory: &Path,
        tokenizer: &Tokenizer,
        sources: Sources<'_>,
        chat_templates: Vec<TemplateInfo>,
    ) -> Result<Self> {
        let added: BTreeMap<u32, AddedToken> =
            tokenizer.get_added_tokens_decoder().into_iter().collect();
        let mut tokens = BTreeMap::new();
        for (&id, token) in &added {
            if token.special {
                tokens.insert(
                    id,
                    SpecialToken {
                        id,
                        content: token.content.clone(),
                        roles: Vec::new(),
                        is_special: true,
                        normalized: Some(token.normalized),
                        single_word: Some(token.single_word),
                        lstrip: Some(token.lstrip),
                        rstrip: Some(token.rstrip),
                    },
                );
            }
        }
        let mut context = BTreeMap::new();
        let mut ids = BTreeMap::new();
        for (role, gguf_key) in [
            ("bos", "bos_token_id"),
            ("eos", "eos_token_id"),
            ("pad", "padding_token_id"),
            ("unk", "unknown_token_id"),
        ] {
            let token_name = format!("{role}_token");
            let mut id = None;
            let mut text = None;
            for source in [sources.tokenizer_config, sources.special_map] {
                if let Some(value) = source.get(&token_name).filter(|v| !v.is_null()) {
                    let content = token_content(value)?;
                    let resolved = tokenizer.token_to_id(content).ok_or_else(|| {
                        Error::new(
                            ErrorCode::TokenIdConflict,
                            format!("{token_name} {content:?} is missing from tokenizer.json"),
                        )
                    })?;
                    merge_id(&mut id, resolved, &token_name)?;
                    text = Some(content.to_owned());
                }
            }
            if let Some(value) = sources.config.get(format!("{role}_token_id")) {
                let values = parse_ids(value)?;
                // Multi-EOS generation settings are not the tokenizer's single eos_token.
                if values.len() == 1 {
                    merge_id(&mut id, values[0], &token_name)?;
                }
                if let Some(id) = id {
                    if !values.is_empty() && !values.contains(&id) {
                        return Err(Error::new(
                            ErrorCode::TokenIdConflict,
                            format!("{token_name} disagrees with config.json"),
                        ));
                    }
                }
            }
            if let Some(value) = sources.gguf.get(&format!("tokenizer.ggml.{gguf_key}")) {
                merge_id(&mut id, scalar_id(value)?, &token_name)?;
            }
            if let Some(id) = id {
                let content = tokenizer.id_to_token(id).ok_or_else(|| {
                    Error::new(
                        ErrorCode::TokenIdConflict,
                        format!("{token_name} ID {id} is missing from tokenizer.json"),
                    )
                })?;
                if text.as_ref().is_some_and(|text| *text != content) {
                    return Err(Error::new(
                        ErrorCode::TokenIdConflict,
                        format!("{token_name} text disagrees with ID {id}"),
                    ));
                }
                context.insert(token_name.clone(), Value::String(content.clone()));
                add_role(&mut tokens, &added, id, content, token_name);
            }
            ids.insert(role, id);
        }
        // Hugging Face exposes all named special tokens, including cls/sep/mask,
        // as template variables. Keep their roles even when IDs are shared.
        for source in [sources.tokenizer_config, sources.special_map] {
            if let Some(object) = source.as_object() {
                for (name, value) in object {
                    if !name.ends_with("_token") || value.is_null() {
                        continue;
                    }
                    let content = token_content(value)?;
                    let id = tokenizer.token_to_id(content).ok_or_else(|| {
                        Error::new(
                            ErrorCode::TokenIdConflict,
                            format!("{name} {content:?} is missing from tokenizer.json"),
                        )
                    })?;
                    let text = Value::String(content.to_owned());
                    if context.get(name).is_some_and(|previous| previous != &text) {
                        return Err(Error::new(
                            ErrorCode::TokenIdConflict,
                            format!("conflicting {name} values"),
                        ));
                    }
                    context.insert(name.clone(), text);
                    add_role(&mut tokens, &added, id, content.to_owned(), name.clone());
                }
            }
        }
        for source in [sources.tokenizer_config, sources.special_map] {
            if let Some(values) = source
                .get("additional_special_tokens")
                .filter(|v| !v.is_null())
            {
                let values = values.as_array().ok_or_else(|| {
                    Error::new(
                        ErrorCode::InvalidJson,
                        "additional_special_tokens must be an array",
                    )
                })?;
                let mut strings = Vec::new();
                for value in values {
                    let content = token_content(value)?;
                    let id = tokenizer.token_to_id(content).ok_or_else(|| {
                        Error::new(
                            ErrorCode::TokenIdConflict,
                            format!("additional special token {content:?} is missing"),
                        )
                    })?;
                    add_role(
                        &mut tokens,
                        &added,
                        id,
                        content.to_owned(),
                        "additional_special_tokens".to_owned(),
                    );
                    strings.push(Value::String(content.to_owned()));
                }
                if let Some(previous) = context.insert(
                    "additional_special_tokens".to_owned(),
                    Value::Array(strings.clone()),
                ) {
                    if previous != Value::Array(strings) {
                        return Err(Error::new(
                            ErrorCode::TokenIdConflict,
                            "additional_special_tokens disagree across configuration files",
                        ));
                    }
                }
            }
        }
        let mut eos_ids = sources
            .config
            .get("eos_token_id")
            .map(parse_ids)
            .transpose()?
            .unwrap_or_default();
        if let Some(id) = ids["eos"] {
            if !eos_ids.contains(&id) {
                eos_ids.push(id);
            }
        }
        let mut stop_ids = sources
            .generation_config
            .get("eos_token_id")
            .filter(|v| !v.is_null())
            .map(parse_ids)
            .transpose()?
            .unwrap_or_else(|| eos_ids.clone());
        for role in ["eot", "eom"] {
            if let Some(value) = sources.gguf.get(&format!("tokenizer.ggml.{role}_token_id")) {
                let id = scalar_id(value)?;
                if !stop_ids.contains(&id) {
                    stop_ids.push(id);
                }
            }
        }
        for id in eos_ids.iter().chain(&stop_ids) {
            if tokenizer.id_to_token(*id).is_none() {
                return Err(Error::new(
                    ErrorCode::TokenIdConflict,
                    format!("stop/EOS ID {id} is missing from tokenizer.json"),
                ));
            }
        }
        let (name, name_source) = if let Some(name) = sources
            .gguf
            .get("general.name")
            .and_then(Value::as_str)
            .filter(|s| !s.is_empty())
        {
            (name.to_owned(), "gguf:general.name")
        } else if let Some(name) = sources
            .config
            .get("_name_or_path")
            .and_then(Value::as_str)
            .filter(|s| !s.is_empty())
        {
            (name.to_owned(), "config.json:_name_or_path")
        } else {
            (
                directory
                    .file_name()
                    .unwrap_or(directory.as_os_str())
                    .to_string_lossy()
                    .into_owned(),
                "directory",
            )
        };
        Ok(Self {
            schema_version: 1,
            model_name: name,
            model_name_source: name_source.to_owned(),
            model_type: sources
                .config
                .get("model_type")
                .and_then(Value::as_str)
                .or_else(|| {
                    sources
                        .gguf
                        .get("general.architecture")
                        .and_then(Value::as_str)
                })
                .map(str::to_owned),
            vocab_size: tokenizer.get_vocab_size(true),
            bos_token_id: ids["bos"],
            eos_token_id: ids["eos"],
            eos_token_ids: eos_ids,
            stop_token_ids: stop_ids,
            pad_token_id: ids["pad"],
            unk_token_id: ids["unk"],
            special_tokens: tokens.into_values().collect(),
            tokenizer_source: "tokenizer.json".to_owned(),
            chat_templates,
            token_context: context,
        })
    }

    pub(crate) fn template_tokens(&self) -> &BTreeMap<String, Value> {
        &self.token_context
    }
}

fn scalar_id(value: &Value) -> Result<u32> {
    value
        .as_u64()
        .and_then(|v| u32::try_from(v).ok())
        .ok_or_else(|| Error::new(ErrorCode::InvalidJson, "token ID must be a uint32"))
}

fn parse_ids(value: &Value) -> Result<Vec<u32>> {
    if value.is_null() {
        return Ok(Vec::new());
    }
    if let Some(values) = value.as_array() {
        let mut ids = Vec::new();
        for value in values {
            let id = scalar_id(value)?;
            if !ids.contains(&id) {
                ids.push(id);
            }
        }
        Ok(ids)
    } else {
        Ok(vec![scalar_id(value)?])
    }
}

fn token_content(value: &Value) -> Result<&str> {
    value
        .as_str()
        .or_else(|| value.get("content").and_then(Value::as_str))
        .ok_or_else(|| {
            Error::new(
                ErrorCode::InvalidJson,
                "special token must be a string or an object containing content",
            )
        })
}

fn merge_id(current: &mut Option<u32>, id: u32, name: &str) -> Result<()> {
    if current.is_some_and(|previous| previous != id) {
        return Err(Error::new(
            ErrorCode::TokenIdConflict,
            format!("conflicting {name} IDs"),
        ));
    }
    *current = Some(id);
    Ok(())
}

fn add_role(
    tokens: &mut BTreeMap<u32, SpecialToken>,
    added_tokens: &BTreeMap<u32, AddedToken>,
    id: u32,
    content: String,
    role: String,
) {
    let token = tokens.entry(id).or_insert_with(|| {
        let added = added_tokens.get(&id);
        SpecialToken {
            id,
            content,
            roles: Vec::new(),
            is_special: added.as_ref().is_some_and(|t| t.special),
            normalized: added.as_ref().map(|t| t.normalized),
            single_word: added.as_ref().map(|t| t.single_word),
            lstrip: added.as_ref().map(|t| t.lstrip),
            rstrip: added.as_ref().map(|t| t.rstrip),
        }
    });
    if !token.roles.contains(&role) {
        token.roles.push(role);
    }
}
