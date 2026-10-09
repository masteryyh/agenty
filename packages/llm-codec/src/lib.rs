pub mod error;
pub mod ffi;
mod gguf;
pub mod metadata;
pub mod model;
pub mod template;

pub use error::{Error, ErrorCode, Result};
pub use model::{LoadOptions, Model};
pub use template::{resolve_templates, RenderRequest, TemplateInfo};

pub const ABI_VERSION: u32 = 1;
pub const MAX_INPUT_BYTES: usize = 16 * 1024 * 1024;
pub(crate) const MAX_CONFIG_BYTES: usize = 128 * 1024 * 1024;
pub(crate) const MAX_TEMPLATE_BYTES: usize = 1024 * 1024;
pub(crate) const MAX_OUTPUT_BYTES: usize = 32 * 1024 * 1024;
