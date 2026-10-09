use std::collections::BTreeMap;
use std::fs::File;
use std::io::{BufReader, Read, Seek, SeekFrom};
use std::path::{Component, Path, PathBuf};

use serde_json::Value;

use crate::error::{Error, ErrorCode, Result};

pub(crate) type Metadata = BTreeMap<String, Value>;

pub(crate) fn select_file(directory: &Path, requested: Option<&str>) -> Result<Option<PathBuf>> {
    if let Some(name) = requested {
        let path = Path::new(name);
        if path.components().count() != 1
            || !matches!(path.components().next(), Some(Component::Normal(_)))
            || path.extension().is_none_or(|e| e != "gguf")
        {
            return Err(Error::new(
                ErrorCode::InvalidArgument,
                "ggufFile must be a .gguf filename inside the model directory",
            ));
        }
        return Ok(Some(directory.join(path)));
    }
    let mut files = Vec::new();
    for entry in std::fs::read_dir(directory)
        .map_err(|e| Error::new(ErrorCode::Io, format!("{}: {e}", directory.display())))?
    {
        let entry = entry.map_err(|e| Error::new(ErrorCode::Io, e.to_string()))?;
        if entry.path().extension().is_some_and(|e| e == "gguf")
            && entry
                .file_type()
                .map_err(|e| Error::new(ErrorCode::Io, e.to_string()))?
                .is_file()
        {
            files.push(entry.path());
        }
    }
    files.sort();
    if files.len() <= 1 {
        return Ok(files.pop());
    }
    // A split set is unambiguous only when every shard belongs to the same set.
    let first_name = files[0].file_name().unwrap().to_string_lossy();
    if let Some((prefix, count)) = first_name.rsplit_once("-00001-of-") {
        if let Some(count) = count
            .strip_suffix(".gguf")
            .and_then(|v| v.parse::<usize>().ok())
        {
            if count == files.len()
                && (1..=count).zip(&files).all(|(i, path)| {
                    path.file_name().unwrap().to_string_lossy()
                        == format!("{prefix}-{i:05}-of-{count:05}.gguf")
                })
            {
                return Ok(Some(files.remove(0)));
            }
        }
    }
    Err(Error::new(
        ErrorCode::AmbiguousModelFile,
        "multiple GGUF models found; specify ggufFile",
    ))
}

struct Reader {
    file: BufReader<File>,
    size: u64,
    big_endian: bool,
}

impl Reader {
    fn bytes<const N: usize>(&mut self) -> Result<[u8; N]> {
        let mut bytes = [0; N];
        self.file
            .read_exact(&mut bytes)
            .map_err(|e| Error::new(ErrorCode::InvalidGguf, e.to_string()))?;
        self.budget()?;
        Ok(bytes)
    }

    fn u32(&mut self) -> Result<u32> {
        let bytes = self.bytes()?;
        Ok(if self.big_endian {
            u32::from_be_bytes(bytes)
        } else {
            u32::from_le_bytes(bytes)
        })
    }

    fn u64(&mut self) -> Result<u64> {
        let bytes = self.bytes()?;
        Ok(if self.big_endian {
            u64::from_be_bytes(bytes)
        } else {
            u64::from_le_bytes(bytes)
        })
    }

    fn budget(&mut self) -> Result<()> {
        let position = self
            .file
            .stream_position()
            .map_err(|e| Error::new(ErrorCode::Io, e.to_string()))?;
        if position > crate::MAX_CONFIG_BYTES as u64 {
            return Err(Error::new(
                ErrorCode::ResourceLimit,
                "GGUF metadata exceeds size limit",
            ));
        }
        Ok(())
    }

    fn skip(&mut self, length: u64) -> Result<()> {
        let start = self
            .file
            .stream_position()
            .map_err(|e| Error::new(ErrorCode::Io, e.to_string()))?;
        let end = start
            .checked_add(length)
            .filter(|end| *end <= self.size)
            .ok_or_else(|| Error::new(ErrorCode::InvalidGguf, "GGUF value extends beyond file"))?;
        self.file
            .seek(SeekFrom::Start(end))
            .map_err(|e| Error::new(ErrorCode::Io, e.to_string()))?;
        self.budget()
    }

    fn string(&mut self, limit: usize) -> Result<String> {
        let length = self.u64()?;
        if length > limit as u64 {
            return Err(Error::new(
                ErrorCode::ResourceLimit,
                "GGUF string exceeds size limit",
            ));
        }
        let mut bytes = vec![0; length as usize];
        self.file
            .read_exact(&mut bytes)
            .map_err(|e| Error::new(ErrorCode::InvalidGguf, e.to_string()))?;
        self.budget()?;
        String::from_utf8(bytes).map_err(|e| Error::new(ErrorCode::InvalidGguf, e.to_string()))
    }

    fn skip_value(&mut self, kind: u32) -> Result<()> {
        match kind {
            0 | 1 | 7 => self.skip(1),
            2 | 3 => self.skip(2),
            4..=6 => self.skip(4),
            10..=12 => self.skip(8),
            8 => {
                let length = self.u64()?;
                self.skip(length)
            }
            9 => {
                let item_type = self.u32()?;
                let count = self.u64()?;
                let width = match item_type {
                    0 | 1 | 7 => 1,
                    2 | 3 => 2,
                    4..=6 => 4,
                    10..=12 => 8,
                    8 => 0,
                    _ => {
                        return Err(Error::new(
                            ErrorCode::InvalidGguf,
                            "invalid GGUF array type",
                        ))
                    }
                };
                if width > 0 {
                    self.skip(count.checked_mul(width).ok_or_else(|| {
                        Error::new(ErrorCode::InvalidGguf, "GGUF array length overflow")
                    })?)
                } else {
                    if count > crate::MAX_CONFIG_BYTES as u64 / 8 {
                        return Err(Error::new(
                            ErrorCode::ResourceLimit,
                            "GGUF array exceeds size limit",
                        ));
                    }
                    for _ in 0..count {
                        self.skip_value(8)?;
                    }
                    Ok(())
                }
            }
            _ => Err(Error::new(
                ErrorCode::InvalidGguf,
                format!("unknown GGUF value type {kind}"),
            )),
        }
    }
}

pub(crate) fn read(path: &Path) -> Result<Metadata> {
    let file = File::open(path).map_err(|e| {
        Error::new(
            if e.kind() == std::io::ErrorKind::NotFound {
                ErrorCode::ModelNotFound
            } else {
                ErrorCode::Io
            },
            format!("{}: {e}", path.display()),
        )
    })?;
    let size = file
        .metadata()
        .map_err(|e| Error::new(ErrorCode::Io, e.to_string()))?
        .len();
    let mut reader = Reader {
        file: BufReader::new(file),
        size,
        big_endian: false,
    };
    match &reader.bytes::<4>()? {
        b"GGUF" => {}
        b"FUGG" => {
            reader.big_endian = true;
        }
        _ => {
            return Err(Error::new(
                ErrorCode::UnsupportedModelFormat,
                "invalid GGUF magic",
            ))
        }
    }
    let version = reader.u32()?;
    if !matches!(version, 2 | 3) {
        return Err(Error::new(
            ErrorCode::UnsupportedModelFormat,
            format!("unsupported GGUF version {version}"),
        ));
    }
    let _tensor_count = reader.u64()?;
    let count = reader.u64()?;
    if count > 100_000 {
        return Err(Error::new(
            ErrorCode::ResourceLimit,
            "too many GGUF metadata fields",
        ));
    }
    let mut metadata = Metadata::new();
    let mut keys = std::collections::BTreeSet::new();
    for _ in 0..count {
        let key = reader.string(65535)?;
        if !keys.insert(key.clone()) {
            return Err(Error::new(
                ErrorCode::InvalidGguf,
                format!("duplicate GGUF key {key}"),
            ));
        }
        let kind = reader.u32()?;
        let text = matches!(key.as_str(), "general.name" | "general.architecture")
            || key == "tokenizer.chat_template"
            || key.starts_with("tokenizer.chat_template.");
        let token_id = matches!(
            key.as_str(),
            "tokenizer.ggml.bos_token_id"
                | "tokenizer.ggml.eos_token_id"
                | "tokenizer.ggml.padding_token_id"
                | "tokenizer.ggml.unknown_token_id"
                | "tokenizer.ggml.eot_token_id"
                | "tokenizer.ggml.eom_token_id"
        );
        let flag = matches!(
            key.as_str(),
            "tokenizer.ggml.add_bos_token" | "tokenizer.ggml.add_eos_token"
        );
        let split = matches!(key.as_str(), "split.no" | "split.count");
        let value = if text && kind == 8 {
            Some(Value::String(reader.string(crate::MAX_TEMPLATE_BYTES)?))
        } else if (token_id || split) && kind == 4 {
            Some(Value::from(reader.u32()?))
        } else if (token_id || split) && kind == 10 {
            Some(Value::from(reader.u64()?))
        } else if split && kind == 2 {
            let bytes = reader.bytes()?;
            Some(Value::from(if reader.big_endian {
                u16::from_be_bytes(bytes)
            } else {
                u16::from_le_bytes(bytes)
            }))
        } else if flag && kind == 7 {
            let flag = reader.bytes::<1>()?[0];
            if flag > 1 {
                return Err(Error::new(ErrorCode::InvalidGguf, "invalid GGUF boolean"));
            }
            Some(Value::Bool(flag == 1))
        } else if text || token_id || flag || split {
            return Err(Error::new(
                ErrorCode::InvalidGguf,
                format!("invalid value type for {key}"),
            ));
        } else {
            // In particular, never extract tokenizer.ggml.tokens or tokenizer.huggingface.json.
            reader.skip_value(kind)?;
            None
        };
        if let Some(value) = value {
            metadata.insert(key, value);
        }
    }
    if metadata
        .get("split.no")
        .and_then(Value::as_u64)
        .is_some_and(|n| n != 0)
    {
        return Err(Error::new(
            ErrorCode::UnsupportedModelFormat,
            "select the first GGUF shard for metadata",
        ));
    }
    Ok(metadata)
}
