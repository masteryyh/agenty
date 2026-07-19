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
