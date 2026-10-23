use std::path::Path;
use std::sync::Arc;

use serde::{Deserialize, Serialize};
use tokenizers::Tokenizer;

use crate::error::{read_limited, read_optional_json, Error, ErrorCode, Result};
use crate::metadata::ModelMetadata;
use crate::template::{RenderRequest, Templates};

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct LoadOptions {
    pub gguf_file: Option<String>,
    pub gguf_metadata_path: Option<std::path::PathBuf>,
    #[serde(default)]
    pub ignore_gguf: bool,
}

pub struct Model {
    tokenizer: Tokenizer,
    templates: Templates,
    pub metadata: ModelMetadata,
}

impl Model {
    pub fn load(directory: &Path, options: &LoadOptions) -> Result<Self> {
        if !directory.is_dir() {
            return Err(Error::new(
                ErrorCode::ModelNotFound,
                format!("model directory not found: {}", directory.display()),
            ));
        }
        let path = directory.join("tokenizer.json");
        let bytes = read_limited(&path, crate::MAX_CONFIG_BYTES).map_err(|e| {
            if e.code == ErrorCode::ModelNotFound {
                Error::new(
                    ErrorCode::TokenizerNotFound,
                    format!(
                        "{} is required; GGUF tokenizers are not supported",
                        path.display()
                    ),
                )
            } else {
                e
            }
        })?;
        let mut tokenizer = Tokenizer::from_bytes(&bytes).map_err(|e| {
            Error::new(
                ErrorCode::InvalidTokenizer,
                format!("{}: {e}", path.display()),
            )
        })?;
        tokenizer.with_padding(None);
        tokenizer
            .with_truncation(None)
            .map_err(|e| Error::new(ErrorCode::InvalidTokenizer, e.to_string()))?;

        let mut config = read_optional_json(&directory.join("config.json"))?.unwrap_or_default();
        // Multimodal checkpoints keep language-model metadata in text_config.
        if let Some(text) = config
            .get("text_config")
            .and_then(|v| v.as_object())
            .cloned()
        {
            if let Some(root) = config.as_object_mut() {
                for (key, value) in text {
                    // text_config's pretraining EOS can differ from the chat
                    // tokenizer EOS. Metadata records both as stopping IDs.
                    if key != "eos_token_id" {
                        root.entry(key).or_insert(value);
                    }
                }
            }
        }
        let tokenizer_config =
            read_optional_json(&directory.join("tokenizer_config.json"))?.unwrap_or_default();
        let special_map =
            read_optional_json(&directory.join("special_tokens_map.json"))?.unwrap_or_default();
        let generation_config =
            read_optional_json(&directory.join("generation_config.json"))?.unwrap_or_default();
        let gguf_path = selected_gguf(directory, options)?;
        let gguf = gguf_path
            .as_deref()
            .map(crate::gguf::read)
            .transpose()?
            .unwrap_or_default();
        let templates = Templates::load(directory, &tokenizer_config, &gguf, gguf_path.as_deref())?;
        let metadata = ModelMetadata::load(
            directory,
            &tokenizer,
            crate::metadata::Sources {
                config: &config,
                tokenizer_config: &tokenizer_config,
                special_map: &special_map,
                generation_config: &generation_config,
                gguf: &gguf,
            },
            templates.info(),
        )?;
        Ok(Self {
            tokenizer,
            templates,
            metadata,
        })
    }

    pub fn tokenize(&self, text: &str, add_special_tokens: bool) -> Result<Vec<u32>> {
        if text.len() > crate::MAX_INPUT_BYTES {
            return Err(Error::new(
                ErrorCode::ResourceLimit,
                "text exceeds input size limit",
            ));
        }
        let ids = self
            .tokenizer
            .encode(text, add_special_tokens)
            .map(|encoding| encoding.get_ids().to_vec())
            .map_err(|e| Error::new(ErrorCode::EncodeFailed, e.to_string()))?;
        if ids.len() > crate::MAX_OUTPUT_BYTES / 4 {
            return Err(Error::new(
                ErrorCode::ResourceLimit,
                "token output exceeds size limit",
            ));
        }
        Ok(ids)
    }

    pub fn decode(&self, ids: &[u32], skip_special_tokens: bool) -> Result<String> {
        if ids.len() > crate::MAX_INPUT_BYTES / 4 {
            return Err(Error::new(
                ErrorCode::ResourceLimit,
                "token sequence exceeds input size limit",
            ));
        }
        for id in ids {
            if self.tokenizer.id_to_token(*id).is_none() {
                return Err(Error::new(
                    ErrorCode::InvalidTokenId,
                    format!("unknown token ID {id}"),
                ));
            }
        }
        let text = self
            .tokenizer
            .decode(ids, skip_special_tokens)
            .map_err(|e| Error::new(ErrorCode::DecodeFailed, e.to_string()))?;
        if text.len() > crate::MAX_OUTPUT_BYTES {
            return Err(Error::new(
                ErrorCode::ResourceLimit,
                "decoded text exceeds output size limit",
            ));
        }
        Ok(text)
    }

    pub fn render(&self, request: &RenderRequest) -> Result<String> {
        self.templates
            .render(request, self.metadata.template_tokens())
    }

    pub fn chat_encode(&self, request: &RenderRequest) -> Result<Vec<u32>> {
        self.tokenize(&self.render(request)?, false)
    }
}

pub(crate) fn selected_gguf(
    directory: &Path,
    options: &LoadOptions,
) -> Result<Option<std::path::PathBuf>> {
    if options.ignore_gguf {
        if options.gguf_file.is_some() || options.gguf_metadata_path.is_some() {
            return Err(Error::new(
                ErrorCode::InvalidArgument,
                "ignoreGguf conflicts with a GGUF selection",
            ));
        }
        return Ok(None);
    }
    if let Some(path) = &options.gguf_metadata_path {
        if options.gguf_file.is_some() || !path.is_absolute() {
            return Err(Error::new(
                ErrorCode::InvalidArgument,
                "ggufMetadataPath must be absolute and cannot accompany ggufFile",
            ));
        }
        return Ok(Some(path.clone()));
    }
    crate::gguf::select_file(directory, options.gguf_file.as_deref())
}

pub struct Decoder {
    model: Arc<Model>,
    pending: Vec<u32>,
    prefix: String,
    prefix_index: usize,
    all: Vec<u32>,
    emitted: String,
    finished: bool,
}

impl Decoder {
    pub fn new(model: Arc<Model>) -> Self {
        Self {
            model,
            pending: Vec::new(),
            prefix: String::new(),
            prefix_index: 0,
            all: Vec::new(),
            emitted: String::new(),
            finished: false,
        }
    }

    pub fn push(&mut self, ids: &[u32], finish: bool) -> Result<String> {
        if self.finished {
            return Err(Error::new(ErrorCode::InvalidHandle, "decoder has finished"));
        }
        if self.all.len().saturating_add(ids.len()) > crate::MAX_INPUT_BYTES / 4 {
            return Err(Error::new(
                ErrorCode::ResourceLimit,
                "decoder token limit exceeded",
            ));
        }
        for id in ids {
            if self.model.tokenizer.id_to_token(*id).is_none() {
                return Err(Error::new(
                    ErrorCode::InvalidTokenId,
                    format!("unknown token ID {id}"),
                ));
            }
        }
        let mut output = String::new();
        if !ids.is_empty() {
            output = tokenizers::tokenizer::step_decode_stream(
                &self.model.tokenizer,
                ids.to_vec(),
                false,
                &mut self.pending,
                &mut self.prefix,
                &mut self.prefix_index,
            )
            .map_err(|e| Error::new(ErrorCode::DecodeFailed, e.to_string()))?
            .unwrap_or_default();
            self.all.extend_from_slice(ids);
            self.emitted.push_str(&output);
        }
        if self.emitted.len() > crate::MAX_OUTPUT_BYTES {
            return Err(Error::new(
                ErrorCode::ResourceLimit,
                "decoder output limit exceeded",
            ));
        }
        if finish {
            let full = self.model.decode(&self.all, false)?;
            let tail = full.strip_prefix(&self.emitted).ok_or_else(|| {
                Error::new(
                    ErrorCode::DecodeFailed,
                    "stream decoder changed an already emitted prefix",
                )
            })?;
            output.push_str(tail);
            self.finished = true;
        }
        Ok(output)
    }
}
