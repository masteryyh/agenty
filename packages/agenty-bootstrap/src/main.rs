mod progress;

use std::fs::File;
use std::path::Path;

use agenty_bootstrap::{
    artifact_paths, check_artifact_integrity, install_artifact, read_footer, reuse_artifact,
    ArtifactIntegrity, BootstrapError, PayloadSpec, Result,
};
use progress::ProgressLog;

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
    let progress = ProgressLog::new();
    progress.parent(format!("starting agenty {}...", agenty_version()));

    let exe = std::env::current_exe()?;
    let mut file = File::open(&exe)?;
    let footer = read_footer(&mut file)?;

    let home = dirs::home_dir().ok_or_else(|| {
        BootstrapError::Invalid("cannot locate the current user's home directory".to_string())
    })?;
    let (cli, runtime) = artifact_paths(&home);

    if !cli.is_file() && !runtime.is_file() {
        progress.parent("local binary not found, extracting...");
        install_artifact(&mut file, &footer.cli, &cli)?;
        install_artifact(&mut file, &footer.runtime, &runtime)?;
    } else {
        progress.parent("checking local binary integrity...");
        ensure_with_progress(&mut file, &footer.cli, &cli, "cli", &progress)?;
        ensure_with_progress(&mut file, &footer.runtime, &runtime, "runtime", &progress)?;
    }

    progress.finish();
    launch(&cli)
}

fn agenty_version() -> &'static str {
    option_env!("AGENTY_VERSION")
        .filter(|version| !version.trim().is_empty())
        .unwrap_or("dev")
}

fn ensure_with_progress(
    packed: &mut File,
    spec: &PayloadSpec,
    target: &Path,
    name: &str,
    progress: &ProgressLog,
) -> Result<()> {
    progress.child(format!("checking {name} binary integrity"));
    match check_artifact_integrity(spec, target)? {
        ArtifactIntegrity::Valid => {
            reuse_artifact(target)?;
            progress.child(format!(
                "{name} integrity check passed, skipping extraction."
            ));
        }
        ArtifactIntegrity::Missing | ArtifactIntegrity::Invalid => {
            progress.child(format!("{name} integrity check failed, extracting..."));
            install_artifact(packed, spec, target)?;
        }
    }
    Ok(())
}

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
