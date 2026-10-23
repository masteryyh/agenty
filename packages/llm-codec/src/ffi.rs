use std::collections::HashMap;
use std::panic::{catch_unwind, AssertUnwindSafe};
use std::path::Path;
use std::ptr;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex, OnceLock};

use serde::de::DeserializeOwned;

use crate::{Error, ErrorCode, LoadOptions, Model, RenderRequest, Result};

pub type LlmCodecHandle = u64;

/// Rust-owned UTF-8 bytes. Not NUL-terminated. Free with llm_codec_bytes_free.
#[repr(C)]
pub struct LlmCodecBytes {
    pub ptr: *mut u8,
    pub len: usize,
}

impl Default for LlmCodecBytes {
    fn default() -> Self {
        Self {
            ptr: ptr::null_mut(),
            len: 0,
        }
    }
}

/// Rust-owned token IDs. Free with llm_codec_tokens_free.
#[repr(C)]
pub struct LlmCodecTokens {
    pub ptr: *mut u32,
    pub len: usize,
}

impl Default for LlmCodecTokens {
    fn default() -> Self {
        Self {
            ptr: ptr::null_mut(),
            len: 0,
        }
    }
}

/// Per-call error. Initialize to zero; free before reusing this output slot.
#[repr(C)]
#[derive(Default)]
pub struct LlmCodecError {
    pub code: i32,
    pub message: LlmCodecBytes,
}

static MODELS: OnceLock<Mutex<HashMap<LlmCodecHandle, Arc<Model>>>> = OnceLock::new();
static NEXT_HANDLE: AtomicU64 = AtomicU64::new(1);
type SharedDecoder = Arc<Mutex<crate::model::Decoder>>;
static DECODERS: OnceLock<Mutex<HashMap<LlmCodecHandle, SharedDecoder>>> = OnceLock::new();

fn decoders() -> &'static Mutex<HashMap<LlmCodecHandle, SharedDecoder>> {
    DECODERS.get_or_init(Mutex::default)
}

fn models() -> &'static Mutex<HashMap<LlmCodecHandle, Arc<Model>>> {
    MODELS.get_or_init(Mutex::default)
}

fn lookup(handle: LlmCodecHandle) -> Result<Arc<Model>> {
    models()
        .lock()
        .map_err(|_| Error::new(ErrorCode::Internal, "model registry lock poisoned"))?
        .get(&handle)
        .cloned()
        .ok_or_else(|| Error::new(ErrorCode::InvalidHandle, "unknown or released model handle"))
}

fn bytes_output(bytes: Vec<u8>) -> LlmCodecBytes {
    if bytes.is_empty() {
        return LlmCodecBytes::default();
    }
    let bytes = Box::into_raw(bytes.into_boxed_slice());
    LlmCodecBytes {
        ptr: bytes as *mut u8,
        len: unsafe { (&*bytes).len() },
    }
}

fn tokens_output(tokens: Vec<u32>) -> LlmCodecTokens {
    if tokens.is_empty() {
        return LlmCodecTokens::default();
    }
    let tokens = Box::into_raw(tokens.into_boxed_slice());
    LlmCodecTokens {
        ptr: tokens as *mut u32,
        len: unsafe { (&*tokens).len() },
    }
}

fn json_output(value: &impl serde::Serialize) -> Result<LlmCodecBytes> {
    let bytes =
        serde_json::to_vec(value).map_err(|e| Error::new(ErrorCode::Internal, e.to_string()))?;
    if bytes.len() > crate::MAX_OUTPUT_BYTES {
        return Err(Error::new(
            ErrorCode::ResourceLimit,
            "JSON output exceeds size limit",
        ));
    }
    Ok(bytes_output(bytes))
}

fn check_pointer<T>(pointer: *const T) -> Result<()> {
    if pointer.is_null() || !(pointer as usize).is_multiple_of(std::mem::align_of::<T>()) {
        return Err(Error::new(
            ErrorCode::InvalidArgument,
            "null or unaligned pointer",
        ));
    }
    Ok(())
}

unsafe fn input<'a, T>(pointer: *const T, length: usize, limit: usize) -> Result<&'a [T]> {
    if length > limit || length > isize::MAX as usize / std::mem::size_of::<T>() {
        return Err(Error::new(
            ErrorCode::ResourceLimit,
            "input exceeds size limit",
        ));
    }
    if length == 0 {
        return Ok(&[]);
    }
    check_pointer(pointer)?;
    Ok(std::slice::from_raw_parts(pointer, length))
}

unsafe fn text<'a>(pointer: *const u8, length: usize) -> Result<&'a str> {
    std::str::from_utf8(input(pointer, length, crate::MAX_INPUT_BYTES)?)
        .map_err(|e| Error::new(ErrorCode::InvalidUtf8, e.to_string()))
}

unsafe fn json_input<T: DeserializeOwned>(pointer: *const u8, length: usize) -> Result<T> {
    serde_json::from_str(text(pointer, length)?)
        .map_err(|e| Error::new(ErrorCode::InvalidJson, e.to_string()))
}

unsafe fn options(pointer: *const u8, length: usize) -> Result<LoadOptions> {
    if length == 0 {
        Ok(LoadOptions::default())
    } else {
        json_input(pointer, length)
    }
}

unsafe fn boundary(error: *mut LlmCodecError, operation: impl FnOnce() -> Result<()>) -> i32 {
    if !error.is_null() {
        if check_pointer(error).is_err() {
            return ErrorCode::InvalidArgument as i32;
        }
        ptr::write(error, LlmCodecError::default());
    }
    let result = catch_unwind(AssertUnwindSafe(operation)).unwrap_or_else(|payload| {
        let message = payload
            .downcast_ref::<String>()
            .map(String::as_str)
            .or_else(|| payload.downcast_ref::<&str>().copied())
            .unwrap_or("Rust panic");
        Err(Error::new(ErrorCode::Panic, message))
    });
    match result {
        Ok(()) => 0,
        Err(failure) => {
            let code = failure.code as i32;
            if !error.is_null() {
                ptr::write(
                    error,
                    LlmCodecError {
                        code,
                        message: bytes_output(failure.message.into_bytes()),
                    },
                );
            }
            code
        }
    }
}

#[no_mangle]
pub extern "C" fn llm_codec_abi_version() -> u32 {
    crate::ABI_VERSION
}

/// Load tokenizer.json and templates. options_json may be NULL with length zero.
/// # Safety
/// Inputs must be readable for their lengths. Output pointers must be writable,
/// empty, correctly aligned and non-overlapping. out_error may be NULL.
#[no_mangle]
pub unsafe extern "C" fn llm_codec_model_load(
    directory: *const u8,
    directory_len: usize,
    options_json: *const u8,
    options_len: usize,
    out_handle: *mut LlmCodecHandle,
    out_metadata: *mut LlmCodecBytes,
    out_error: *mut LlmCodecError,
) -> i32 {
    boundary(out_error, || {
        check_pointer(out_handle)?;
        check_pointer(out_metadata)?;
        ptr::write(out_handle, 0);
        ptr::write(out_metadata, LlmCodecBytes::default());
        let directory = text(directory, directory_len)?;
        if directory.is_empty() || directory.contains('\0') {
            return Err(Error::new(
                ErrorCode::InvalidArgument,
                "model directory must be a nonempty path without NUL",
            ));
        }
        let model = Arc::new(Model::load(
            Path::new(directory),
            &options(options_json, options_len)?,
        )?);
        let metadata = serde_json::to_vec(&model.metadata)
            .map_err(|e| Error::new(ErrorCode::Internal, e.to_string()))?;
        if metadata.len() > crate::MAX_OUTPUT_BYTES {
            return Err(Error::new(
                ErrorCode::ResourceLimit,
                "metadata exceeds output size limit",
            ));
        }
        let handle = NEXT_HANDLE
            .fetch_update(Ordering::Relaxed, Ordering::Relaxed, |current| {
                current.checked_add(1)
            })
            .map_err(|_| Error::new(ErrorCode::ResourceLimit, "model handle space exhausted"))?;
        models()
            .lock()
            .map_err(|_| Error::new(ErrorCode::Internal, "model registry lock poisoned"))?
            .insert(handle, model);
        ptr::write(out_metadata, bytes_output(metadata));
        ptr::write(out_handle, handle);
        Ok(())
    })
}

/// Resolve and validate templates without loading tokenizer.json.
/// # Safety
/// Inputs and outputs obey the same pointer and ownership rules as llm_codec_model_load.
#[no_mangle]
pub unsafe extern "C" fn llm_codec_template_resolve(
    directory: *const u8,
    directory_len: usize,
    options_json: *const u8,
    options_len: usize,
    out_templates: *mut LlmCodecBytes,
    out_error: *mut LlmCodecError,
) -> i32 {
    boundary(out_error, || {
        check_pointer(out_templates)?;
        ptr::write(out_templates, LlmCodecBytes::default());
        let directory = text(directory, directory_len)?;
        if directory.is_empty() || directory.contains('\0') {
            return Err(Error::new(
                ErrorCode::InvalidArgument,
                "invalid model directory",
            ));
        }
        let templates =
            crate::resolve_templates(Path::new(directory), &options(options_json, options_len)?)?;
        ptr::write(out_templates, json_output(&templates)?);
        Ok(())
    })
}

/// Encode UTF-8 text. add_special_tokens must be 0 or 1.
/// # Safety
/// text must be readable for text_len bytes; outputs must be empty, writable,
/// aligned and non-overlapping. out_error may be NULL.
#[no_mangle]
pub unsafe extern "C" fn llm_codec_tokenize(
    handle: LlmCodecHandle,
    source: *const u8,
    source_len: usize,
    add_special_tokens: u8,
    out_tokens: *mut LlmCodecTokens,
    out_error: *mut LlmCodecError,
) -> i32 {
    boundary(out_error, || {
        check_pointer(out_tokens)?;
        ptr::write(out_tokens, LlmCodecTokens::default());
        if add_special_tokens > 1 {
            return Err(Error::new(
                ErrorCode::InvalidArgument,
                "add_special_tokens must be 0 or 1",
            ));
        }
        let ids = lookup(handle)?.tokenize(text(source, source_len)?, add_special_tokens != 0)?;
        ptr::write(out_tokens, tokens_output(ids));
        Ok(())
    })
}

/// Decode token IDs. skip_special_tokens must be 0 or 1.
/// # Safety
/// ids must be aligned and readable for ids_len uint32 values; output slots obey
/// the ownership and pointer rules of llm_codec_tokenize.
#[no_mangle]
pub unsafe extern "C" fn llm_codec_decode(
    handle: LlmCodecHandle,
    ids: *const u32,
    ids_len: usize,
    skip_special_tokens: u8,
    out_text: *mut LlmCodecBytes,
    out_error: *mut LlmCodecError,
) -> i32 {
    boundary(out_error, || {
        check_pointer(out_text)?;
        ptr::write(out_text, LlmCodecBytes::default());
        if skip_special_tokens > 1 {
            return Err(Error::new(
                ErrorCode::InvalidArgument,
                "skip_special_tokens must be 0 or 1",
            ));
        }
        let decoded = lookup(handle)?.decode(
            input(ids, ids_len, crate::MAX_INPUT_BYTES / 4)?,
            skip_special_tokens != 0,
        )?;
        ptr::write(out_text, bytes_output(decoded.into_bytes()));
        Ok(())
    })
}

/// Render a lowerCamelCase request JSON. Tokenize this result with
/// add_special_tokens=0 to avoid duplicate BOS/EOS markers.
/// # Safety
/// request must be readable for request_len bytes; output slots obey the same
/// ownership and pointer rules as llm_codec_tokenize.
#[no_mangle]
pub unsafe extern "C" fn llm_codec_chat_render(
    handle: LlmCodecHandle,
    request: *const u8,
    request_len: usize,
    out_text: *mut LlmCodecBytes,
    out_error: *mut LlmCodecError,
) -> i32 {
    boundary(out_error, || {
        check_pointer(out_text)?;
        ptr::write(out_text, LlmCodecBytes::default());
        let request: RenderRequest = json_input(request, request_len)?;
        let rendered = lookup(handle)?.render(&request)?;
        ptr::write(out_text, bytes_output(rendered.into_bytes()));
        Ok(())
    })
}

/// Render and encode in one operation. The template supplies special tokens.
/// # Safety
/// Pointer and ownership rules are identical to llm_codec_tokenize.
#[no_mangle]
pub unsafe extern "C" fn llm_codec_chat_encode(
    handle: LlmCodecHandle,
    request: *const u8,
    request_len: usize,
    out_tokens: *mut LlmCodecTokens,
    out_error: *mut LlmCodecError,
) -> i32 {
    boundary(out_error, || {
        check_pointer(out_tokens)?;
        ptr::write(out_tokens, LlmCodecTokens::default());
        let request: RenderRequest = json_input(request, request_len)?;
        ptr::write(
            out_tokens,
            tokens_output(lookup(handle)?.chat_encode(&request)?),
        );
        Ok(())
    })
}

/// Create an independent decoder which retains its model after model_release.
/// # Safety
/// Outputs must be writable, aligned and empty, as with model_load.
#[no_mangle]
pub unsafe extern "C" fn llm_codec_decoder_create(
    model_handle: LlmCodecHandle,
    out_handle: *mut LlmCodecHandle,
    out_error: *mut LlmCodecError,
) -> i32 {
    boundary(out_error, || {
        check_pointer(out_handle)?;
        ptr::write(out_handle, 0);
        let decoder = Arc::new(Mutex::new(crate::model::Decoder::new(lookup(
            model_handle,
        )?)));
        let handle = NEXT_HANDLE
            .fetch_update(Ordering::Relaxed, Ordering::Relaxed, |current| {
                current.checked_add(1)
            })
            .map_err(|_| Error::new(ErrorCode::ResourceLimit, "handle space exhausted"))?;
        decoders()
            .lock()
            .map_err(|_| Error::new(ErrorCode::Internal, "decoder registry poisoned"))?
            .insert(handle, decoder);
        ptr::write(out_handle, handle);
        Ok(())
    })
}

/// Append IDs and optionally flush. A finished decoder accepts no further input.
/// # Safety
/// Pointer/length and output ownership rules are identical to llm_codec_decode.
#[no_mangle]
pub unsafe extern "C" fn llm_codec_decoder_push(
    handle: LlmCodecHandle,
    ids: *const u32,
    ids_len: usize,
    finish: u8,
    out_text: *mut LlmCodecBytes,
    out_error: *mut LlmCodecError,
) -> i32 {
    boundary(out_error, || {
        check_pointer(out_text)?;
        ptr::write(out_text, LlmCodecBytes::default());
        if finish > 1 {
            return Err(Error::new(
                ErrorCode::InvalidArgument,
                "finish must be 0 or 1",
            ));
        }
        let decoder = decoders()
            .lock()
            .map_err(|_| Error::new(ErrorCode::Internal, "decoder registry poisoned"))?
            .get(&handle)
            .cloned()
            .ok_or_else(|| Error::new(ErrorCode::InvalidHandle, "unknown decoder"))?;
        let output = decoder
            .lock()
            .map_err(|_| Error::new(ErrorCode::Internal, "decoder lock poisoned"))?
            .push(
                input(ids, ids_len, crate::MAX_INPUT_BYTES / 4)?,
                finish != 0,
            )?;
        ptr::write(out_text, bytes_output(output.into_bytes()));
        Ok(())
    })
}

/// Release a decoder; in-flight operations retain their own reference.
/// # Safety
/// out_error must be NULL or an empty, aligned writable error slot.
#[no_mangle]
pub unsafe extern "C" fn llm_codec_decoder_release(
    handle: LlmCodecHandle,
    out_error: *mut LlmCodecError,
) -> i32 {
    boundary(out_error, || {
        decoders()
            .lock()
            .map_err(|_| Error::new(ErrorCode::Internal, "decoder registry poisoned"))?
            .remove(&handle)
            .ok_or_else(|| Error::new(ErrorCode::InvalidHandle, "unknown decoder"))?;
        Ok(())
    })
}

/// Release a model. In-flight calls retain their own model reference.
/// # Safety
/// out_error, when non-NULL, must be an empty, aligned writable error slot.
#[no_mangle]
pub unsafe extern "C" fn llm_codec_model_release(
    handle: LlmCodecHandle,
    out_error: *mut LlmCodecError,
) -> i32 {
    boundary(out_error, || {
        let model = models()
            .lock()
            .map_err(|_| Error::new(ErrorCode::Internal, "model registry lock poisoned"))?
            .remove(&handle)
            .ok_or_else(|| {
                Error::new(ErrorCode::InvalidHandle, "unknown or released model handle")
            })?;
        drop(model);
        Ok(())
    })
}

/// Free bytes and zero the same output slot. NULL and zeroed slots are allowed.
/// # Safety
/// The slot must be writable and contain an unmodified allocation returned by
/// this library. Do not copy owning slots or use C free on their pointers.
#[no_mangle]
pub unsafe extern "C" fn llm_codec_bytes_free(bytes: *mut LlmCodecBytes) {
    if bytes.is_null() {
        return;
    }
    let value = ptr::replace(bytes, LlmCodecBytes::default());
    if !value.ptr.is_null() {
        drop(Box::from_raw(ptr::slice_from_raw_parts_mut(
            value.ptr, value.len,
        )));
    }
}

/// Free tokens and zero the same output slot. NULL and zeroed slots are allowed.
/// # Safety
/// The slot must obey the allocation and ownership rules of llm_codec_bytes_free.
#[no_mangle]
pub unsafe extern "C" fn llm_codec_tokens_free(tokens: *mut LlmCodecTokens) {
    if tokens.is_null() {
        return;
    }
    let value = ptr::replace(tokens, LlmCodecTokens::default());
    if !value.ptr.is_null() {
        drop(Box::from_raw(ptr::slice_from_raw_parts_mut(
            value.ptr, value.len,
        )));
    }
}

/// Free the error message and zero the error slot.
/// # Safety
/// error must be NULL or a writable slot returned by this library.
#[no_mangle]
pub unsafe extern "C" fn llm_codec_error_free(error: *mut LlmCodecError) {
    if error.is_null() {
        return;
    }
    llm_codec_bytes_free(&mut (*error).message);
    (*error).code = 0;
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn panic_is_contained_and_error_is_owned() {
        let mut error = LlmCodecError::default();
        unsafe {
            assert_eq!(
                boundary(&mut error, || panic!("fixture panic")),
                ErrorCode::Panic as i32
            );
            assert_eq!(
                std::str::from_utf8(std::slice::from_raw_parts(
                    error.message.ptr,
                    error.message.len
                ))
                .unwrap(),
                "fixture panic"
            );
            llm_codec_error_free(&mut error);
            llm_codec_error_free(&mut error);
            assert_eq!(error.code, 0);
            assert!(error.message.ptr.is_null());
        }
    }
}
