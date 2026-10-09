use llm_codec::ffi::*;
use llm_codec::ErrorCode;
use std::ptr;

fn directory() -> String {
    format!("{}/tests/fixtures/basic", env!("CARGO_MANIFEST_DIR"))
}

#[test]
fn handle_and_buffer_lifetimes_and_invalid_inputs() {
    let directory = directory();
    unsafe {
        let mut handle = 0;
        let mut metadata = LlmCodecBytes::default();
        let mut error = LlmCodecError::default();
        assert_eq!(
            llm_codec_model_load(
                directory.as_ptr(),
                directory.len(),
                ptr::null(),
                0,
                &mut handle,
                &mut metadata,
                &mut error
            ),
            0
        );
        let mut tokens = LlmCodecTokens::default();
        assert_eq!(
            llm_codec_tokenize(handle, b"Hello".as_ptr(), 5, 0, &mut tokens, &mut error),
            0
        );
        assert_eq!(llm_codec_model_release(handle, &mut error), 0);
        assert_eq!(std::slice::from_raw_parts(tokens.ptr, tokens.len), [3]);
        assert!(
            serde_json::from_slice::<serde_json::Value>(std::slice::from_raw_parts(
                metadata.ptr,
                metadata.len
            ))
            .is_ok()
        );
        llm_codec_tokens_free(&mut tokens);
        llm_codec_tokens_free(&mut tokens);
        llm_codec_bytes_free(&mut metadata);
        llm_codec_bytes_free(&mut metadata);
        assert_eq!(
            llm_codec_tokenize(handle, b"Hello".as_ptr(), 5, 0, &mut tokens, &mut error),
            ErrorCode::InvalidHandle as i32
        );
        assert!(tokens.ptr.is_null());
        llm_codec_error_free(&mut error);
        assert_eq!(
            llm_codec_tokenize(0, ptr::null(), 0, 2, &mut tokens, ptr::null_mut()),
            ErrorCode::InvalidArgument as i32
        );
        assert_eq!(
            llm_codec_template_resolve(ptr::null(), 5, ptr::null(), 0, &mut metadata, &mut error),
            ErrorCode::InvalidArgument as i32
        );
        llm_codec_error_free(&mut error);
        llm_codec_bytes_free(ptr::null_mut());
        llm_codec_tokens_free(ptr::null_mut());
        llm_codec_error_free(ptr::null_mut());
    }
}

#[test]
fn concurrent_release_and_calls_return_valid_results_or_invalid_handle() {
    let directory = directory();
    let mut handle = 0;
    let mut metadata = LlmCodecBytes::default();
    unsafe {
        assert_eq!(
            llm_codec_model_load(
                directory.as_ptr(),
                directory.len(),
                ptr::null(),
                0,
                &mut handle,
                &mut metadata,
                ptr::null_mut()
            ),
            0
        );
        llm_codec_bytes_free(&mut metadata);
    }
    let barrier = std::sync::Arc::new(std::sync::Barrier::new(5));
    let workers: Vec<_> = (0..4)
        .map(|_| {
            let barrier = barrier.clone();
            std::thread::spawn(move || {
                barrier.wait();
                for _ in 0..100 {
                    let mut tokens = LlmCodecTokens::default();
                    let mut error = LlmCodecError::default();
                    unsafe {
                        let status = llm_codec_tokenize(
                            handle,
                            b"Hello".as_ptr(),
                            5,
                            0,
                            &mut tokens,
                            &mut error,
                        );
                        assert!(status == 0 || status == ErrorCode::InvalidHandle as i32);
                        if status == 0 {
                            assert_eq!(std::slice::from_raw_parts(tokens.ptr, tokens.len), [3]);
                        }
                        llm_codec_tokens_free(&mut tokens);
                        llm_codec_error_free(&mut error);
                    }
                }
            })
        })
        .collect();
    barrier.wait();
    unsafe {
        assert_eq!(llm_codec_model_release(handle, ptr::null_mut()), 0);
    }
    for worker in workers {
        worker.join().unwrap();
    }
}
