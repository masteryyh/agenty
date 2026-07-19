// Copyright © 2026 masteryyh <yyh991013@163.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//! agenty launcher entrypoint.
//!
//! Reads the payload footer appended to the current executable, verifies and
//! extracts the bundled CLI and runtime into `~/.agenty/bin`, then hands over
//! to the CLI with all arguments forwarded.

use std::fs::File;
use std::path::Path;

use agenty_bootstrap::{artifact_paths, ensure_artifact, read_footer, BootstrapError, Result};

fn main() {
    std::process::exit(run());
}

fn run() -> i32 {
    match bootstrap() {
        Ok(code) => code,
        Err(err) => {
            eprintln!("agenty: {err}");
            1
        }
    }
}

fn bootstrap() -> Result<i32> {
    let exe = std::env::current_exe()?;
    let mut file = File::open(&exe)?;
    let footer = read_footer(&mut file)?;

    let home = dirs::home_dir().ok_or_else(|| {
        BootstrapError::Invalid("cannot locate the current user's home directory".to_string())
    })?;
    let (cli, runtime) = artifact_paths(&home);

    ensure_artifact(&mut file, &footer.cli, &cli)?;
    ensure_artifact(&mut file, &footer.runtime, &runtime)?;

    launch(&cli)
}

/// Replaces the current process with the CLI on Unix so signals, exit codes
/// and the controlling terminal behave exactly as if the CLI was started
/// directly; on Windows spawns the CLI and forwards its exit code.
#[cfg(unix)]
fn launch(cli: &Path) -> Result<i32> {
    use std::os::unix::process::CommandExt;

    let err = std::process::Command::new(cli)
        .args(std::env::args_os().skip(1))
        .exec();
    Err(BootstrapError::Invalid(format!(
        "failed to start {}: {err}",
        cli.display()
    )))
}

#[cfg(windows)]
fn launch(cli: &Path) -> Result<i32> {
    let status = std::process::Command::new(cli)
        .args(std::env::args_os().skip(1))
        .status()?;
    Ok(status.code().unwrap_or(1))
}
