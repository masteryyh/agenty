use std::io::{self, Read, Write};
use std::path::PathBuf;

use file_editor::{apply_patch, text_editor, FileResult, PatchResult, TextEditorInput};
use serde::Serialize;

#[derive(Debug, Serialize)]
struct ErrorResult {
    success: bool,
    cwd: String,
    files: Vec<FileResult>,
    error: String,
}

fn main() {
    std::process::exit(exit_code());
}

fn exit_code() -> i32 {
    let result = run();
    match result {
        Ok(output) => {
            if let Err(error) = write_json(&output) {
                eprintln!("fileedit: write result: {error}");
                return 1;
            }
            0
        }
        Err(error) => {
            let output = ErrorResult {
                success: false,
                cwd: std::env::current_dir()
                    .map(|path| path.display().to_string())
                    .unwrap_or_default(),
                files: Vec::new(),
                error: error.to_string(),
            };
            if let Err(write_error) = write_json(&output) {
                eprintln!("fileedit: write error result: {write_error}");
            }
            eprintln!("fileedit: {error}");
            1
        }
    }
}

fn write_json<T: Serialize>(value: &T) -> Result<(), Box<dyn std::error::Error>> {
    let stdout = io::stdout();
    let mut output = stdout.lock();
    serde_json::to_writer(&mut output, value)?;
    output.write_all(b"\n")?;
    output.flush()?;
    Ok(())
}

fn run() -> Result<PatchResult, Box<dyn std::error::Error>> {
    let mode = std::env::args()
        .nth(1)
        .ok_or("usage: fileedit <apply_patch|text_editor>")?;
    let mut input = String::new();
    io::stdin().read_to_string(&mut input)?;
    let cwd: PathBuf = std::env::current_dir()?;
    match mode.as_str() {
        "apply_patch" => Ok(apply_patch(&cwd, &input)?),
        "text_editor" => {
            let request = serde_json::from_str::<TextEditorInput>(&input)?;
            Ok(text_editor(&cwd, request)?)
        }
        _ => Err(format!("unknown fileedit mode: {mode}").into()),
    }
}
