use std::fmt;

#[repr(i32)]
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ErrorCode {
    Ok = 0,
    InvalidArgument = 1001,
    InvalidUtf8 = 1002,
    InvalidJson = 1003,
    InvalidHandle = 1004,
    ModelNotFound = 1101,
    Io = 1102,
    AmbiguousModelFile = 1103,
    UnsupportedModelFormat = 1104,
    InvalidGguf = 1105,
    TokenizerNotFound = 1201,
    InvalidTokenizer = 1202,
    TokenIdConflict = 1203,
    EncodeFailed = 1204,
    DecodeFailed = 1205,
    InvalidTokenId = 1206,
    TemplateNotFound = 1301,
    AmbiguousTemplate = 1302,
    TemplateSyntax = 1303,
    UnsupportedTemplateFeature = 1304,
    TemplateRender = 1305,
    InvalidRequest = 1401,
    UnsupportedContent = 1402,
    UnsupportedOption = 1403,
    ResourceLimit = 1501,
    Internal = 1502,
    Panic = 1503,
}

#[derive(Debug)]
pub struct Error {
    pub code: ErrorCode,
    pub message: String,
}

impl Error {
    pub fn new(code: ErrorCode, message: impl Into<String>) -> Self {
        Self {
            code,
            message: message.into(),
        }
    }
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{:?}: {}", self.code, self.message)
    }
}

impl std::error::Error for Error {}

pub type Result<T> = std::result::Result<T, Error>;

pub(crate) fn read_optional_json(path: &std::path::Path) -> Result<Option<serde_json::Value>> {
    match read_limited(path, crate::MAX_CONFIG_BYTES) {
        Ok(bytes) => {
            let value: serde_json::Value = serde_json::from_slice(&bytes).map_err(|e| {
                Error::new(ErrorCode::InvalidJson, format!("{}: {e}", path.display()))
            })?;

            if !value.is_object() {
                return Err(Error::new(
                    ErrorCode::InvalidJson,
                    format!("{} must contain a JSON object", path.display()),
                ));
            }
            Ok(Some(value))
        }
        Err(e) if e.code == ErrorCode::ModelNotFound => Ok(None),
        Err(e) => Err(e),
    }
}

pub(crate) fn read_limited(path: &std::path::Path, limit: usize) -> Result<Vec<u8>> {
    use std::io::Read;
    let file = std::fs::File::open(path).map_err(|e| {
        let code = if e.kind() == std::io::ErrorKind::NotFound {
            ErrorCode::ModelNotFound
        } else {
            ErrorCode::Io
        };
        Error::new(code, format!("{}: {e}", path.display()))
    })?;
    let mut bytes = Vec::new();
    file.take(limit as u64 + 1)
        .read_to_end(&mut bytes)
        .map_err(|e| Error::new(ErrorCode::Io, format!("{}: {e}", path.display())))?;
    if bytes.len() > limit {
        return Err(Error::new(
            ErrorCode::ResourceLimit,
            format!("{} exceeds {limit} bytes", path.display()),
        ));
    }
    Ok(bytes)
}
