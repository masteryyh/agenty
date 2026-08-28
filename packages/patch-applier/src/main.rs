use std::io::{self, Read, Write};
use std::path::PathBuf;

use patch_applier::{apply_patch, FileResult, PatchResult};
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
                eprintln!("apply_patch: write result: {error}");
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
                eprintln!("apply_patch: write error result: {write_error}");
            }
            eprintln!("apply_patch: {error}");
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
    let mut patch = String::new();
    io::stdin().read_to_string(&mut patch)?;
    let cwd: PathBuf = std::env::current_dir()?;
    Ok(apply_patch(&cwd, &patch)?)
}
