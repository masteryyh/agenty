use std::path::Path;

use serde::{Deserialize, Serialize};
use tokenizers::Tokenizer;

use crate::error::{read_limited, read_optional_json, Error, ErrorCode, Result};
use crate::metadata::ModelMetadata;
use crate::template::{RenderRequest, Templates};

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct LoadOptions {
    pub gguf_file: Option<String>,
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

        let config = read_optional_json(&directory.join("config.json"))?.unwrap_or_default();
        let tokenizer_config =
            read_optional_json(&directory.join("tokenizer_config.json"))?.unwrap_or_default();
        let special_map =
            read_optional_json(&directory.join("special_tokens_map.json"))?.unwrap_or_default();
        let generation_config =
            read_optional_json(&directory.join("generation_config.json"))?.unwrap_or_default();
        let gguf_path = crate::gguf::select_file(directory, options.gguf_file.as_deref())?;
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
}
