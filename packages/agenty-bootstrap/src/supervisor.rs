use std::ffi::OsString;
use std::fs::{self, File};
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::time::{Duration, Instant};

use agenty_bootstrap::{
    artifact_paths_for_version, check_artifact_integrity, install_artifact, read_footer,
    reuse_artifact, ArtifactIntegrity, BootstrapError, PayloadSpec, Result,
};
use bytes::Bytes;
use http_body_util::{BodyExt, Full};
use hyper::client::conn::http2;
use hyper::{Method, Request, StatusCode};
use hyper_util::rt::{TokioExecutor, TokioIo};
use sha3::{Digest, Sha3_256};
use tokio::time::{sleep, timeout};

use crate::progress::ProgressLog;

const IPC_VERSION: &str = "1";
const API_CONTRACT: &str = "v1";
const START_TIMEOUT: Duration = Duration::from_secs(30);
const STOP_TIMEOUT: Duration = Duration::from_secs(15);

pub fn run() -> i32 {
    let runtime = match tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
    {
        Ok(runtime) => runtime,
        Err(error) => {
            eprintln!("agenty: failed to initialize async runtime: {error}");
            return 1;
        }
    };
    match runtime.block_on(bootstrap()) {
        Ok(code) => code,
        Err(error) => {
            eprintln!("agenty: {error}");
            1
        }
    }
}

async fn bootstrap() -> Result<i32> {
    let progress = ProgressLog::new();
    progress.parent(format!("starting agenty {}...", agenty_version()));
    let executable = std::env::current_exe()?;
    let mut packed = File::open(&executable)?;
    let footer = read_footer(&mut packed)?;
    let home = dirs::home_dir().ok_or_else(|| {
        BootstrapError::Invalid("cannot locate the current user's home directory".to_string())
    })?;
    let mut combined = Sha3_256::new();
    combined.update(footer.cli.sha3_256);
    combined.update(footer.core.sha3_256);
    combined.update(footer.file_editor.sha3_256);
    let version: [u8; 32] = combined.finalize().into();
    let artifacts = artifact_paths_for_version(&home, &version);
    fs::create_dir_all(artifacts.cli.parent().unwrap_or(&home))?;
    progress.parent("checking local binary integrity...");
    ensure_payload(&mut packed, &footer.cli, &artifacts.cli, "cli", &progress)?;
    ensure_payload(
        &mut packed,
        &footer.core,
        &artifacts.core,
        "core",
        &progress,
    )?;
    ensure_payload(
        &mut packed,
        &footer.file_editor,
        &artifacts.file_editor,
        "fileedit",
        &progress,
    )?;
    progress.finish();

    let (args, data_dir_arg) = parse_args(std::env::args_os().skip(1))?;
    let core_path = std::env::var_os("AGENTY_CORE_BIN")
        .map(PathBuf::from)
        .unwrap_or_else(|| artifacts.core.clone());
    supervise(
        &artifacts.cli,
        &core_path,
        &artifacts.file_editor,
        &home,
        args,
        data_dir_arg,
    )
    .await
}

async fn supervise(
    cli_path: &Path,
    core_path: &Path,
    file_editor_path: &Path,
    home: &Path,
    args: Vec<OsString>,
    data_dir_arg: Option<OsString>,
) -> Result<i32> {
    let data_dir = resolve_data_dir(&home, data_dir_arg)?;
    let (transport, address) = address_for_data_dir(&data_dir);
    if !core_path.is_file() {
        return Err(BootstrapError::Invalid(format!(
            "core binary does not exist: {}",
            core_path.display()
        )));
    }
    let executable_path = prepare_path(core_path, file_editor_path)?;

    let mut job = ChildJob::new()?;
    let mut core = Some(spawn_core(
        core_path,
        &data_dir,
        &transport,
        &address,
        &executable_path,
        &mut job,
    )?);
    let started_at = Instant::now();
    let owns_core = loop {
        if let Some(child) = core.as_mut() {
            if let Some(status) = child.try_wait()? {
                core = None;
                if status.code() != Some(3) {
                    return Err(BootstrapError::Invalid(format!(
                        "core exited before becoming ready with status {status}"
                    )));
                }
            }
        }
        if let Ok(Ok((status, body))) = timeout(
            Duration::from_millis(500),
            h2_request(&address, Method::GET, "/v1/system"),
        )
        .await
        {
            if status == StatusCode::OK {
                let response: APIResponse<SystemInfo> =
                    serde_json::from_slice(&body).map_err(|error| {
                        BootstrapError::Invalid(format!(
                            "core returned invalid system info: {error}"
                        ))
                    })?;
                if response.code != StatusCode::OK.as_u16() || response.message != "ok" {
                    return Err(BootstrapError::Invalid(
                        "core returned an unsuccessful system response".to_string(),
                    ));
                }
                let info = response.data;
                if !info.matches(&data_dir) {
                    return Err(BootstrapError::Invalid(
                        "an incompatible core is already listening at the resolved data directory address".to_string(),
                    ));
                }
                let Some(child) = core.as_ref() else {
                    break false;
                };
                if info.process_id == child.id() {
                    break true;
                }
                let mut contender = core.take().expect("core child exists");
                match wait_child(&mut contender, Duration::from_secs(5)).await? {
                    true if contender.wait()?.code() == Some(3) => break false,
                    true => {
                        return Err(BootstrapError::Invalid(
                            "a competing core exited without reporting an existing data-directory owner".to_string(),
                        ));
                    }
                    false => {
                        let _ = contender.kill();
                        let _ = contender.wait();
                        return Err(BootstrapError::Invalid(
                            "a competing core did not release its startup lock".to_string(),
                        ));
                    }
                }
            }
        }
        if started_at.elapsed() >= START_TIMEOUT {
            if let Some(mut child) = core.take() {
                let _ = child.kill();
                let _ = child.wait();
            }
            return Err(BootstrapError::Invalid(
                "timed out waiting for the local core to become ready".to_string(),
            ));
        }
        sleep(Duration::from_millis(100)).await;
    };

    let mut cli = match spawn_cli(
        cli_path,
        &args,
        &data_dir,
        &transport,
        &address,
        &executable_path,
        &mut job,
    ) {
        Ok(child) => child,
        Err(error) => {
            stop_owned_core(&address, &mut core).await;
            return Err(error);
        }
    };
    let cli_status = loop {
        if let Some(status) = cli.try_wait()? {
            break status;
        }
        if let Some(child) = core.as_mut() {
            if let Some(status) = child.try_wait()? {
                let _ = cli.kill();
                let _ = cli.wait();
                return Err(BootstrapError::Invalid(format!(
                    "owned core exited while the CLI was running: {status}"
                )));
            }
        }
        sleep(Duration::from_millis(100)).await;
    };
    let code = cli_status.code().unwrap_or(1);
    if owns_core {
        stop_owned_core(&address, &mut core).await;
    }
    Ok(code)
}

async fn stop_owned_core(address: &str, core: &mut Option<Child>) {
    if let Some(child) = core.as_mut() {
        let _ = timeout(
            Duration::from_secs(2),
            h2_request(address, Method::POST, "/v1/system/shutdown"),
        )
        .await;
        match wait_child(child, STOP_TIMEOUT).await {
            Ok(true) => *core = None,
            _ => {
                let _ = child.kill();
                let _ = child.wait();
                *core = None;
            }
        }
    }
}

#[derive(serde::Deserialize)]
struct APIResponse<T> {
    code: u16,
    message: String,
    data: T,
}

#[derive(serde::Deserialize)]
#[serde(rename_all = "camelCase")]
struct SystemInfo {
    data_dir: String,
    ipc_version: String,
    api_contract: String,
    process_id: u32,
}

impl SystemInfo {
    fn matches(&self, expected: &str) -> bool {
        let same_dir = if cfg!(windows) {
            self.data_dir.eq_ignore_ascii_case(expected)
        } else {
            self.data_dir == expected
        };
        same_dir && self.ipc_version == IPC_VERSION && self.api_contract == API_CONTRACT
    }
}

async fn wait_child(child: &mut Child, limit: Duration) -> Result<bool> {
    let start = Instant::now();
    loop {
        if child.try_wait()?.is_some() {
            return Ok(true);
        }
        if start.elapsed() >= limit {
            return Ok(false);
        }
        sleep(Duration::from_millis(100)).await;
    }
}

fn parse_args(args: impl Iterator<Item = OsString>) -> Result<(Vec<OsString>, Option<OsString>)> {
    let mut retained = Vec::new();
    let mut data_dir = None;
    let mut args = args.peekable();
    while let Some(arg) = args.next() {
        let text = arg.to_string_lossy();
        if text == "--data-dir" {
            data_dir = Some(args.next().ok_or_else(|| {
                BootstrapError::Invalid("--data-dir requires a path".to_string())
            })?);
        } else if let Some(value) = text.strip_prefix("--data-dir=") {
            if value.is_empty() {
                return Err(BootstrapError::Invalid(
                    "--data-dir requires a path".to_string(),
                ));
            }
            data_dir = Some(OsString::from(value));
        } else {
            retained.push(arg);
        }
    }
    Ok((retained, data_dir))
}

fn resolve_data_dir(home: &Path, arg: Option<OsString>) -> Result<String> {
    let path = arg
        .or_else(|| std::env::var_os("AGENTY_DATA_DIR"))
        .map(PathBuf::from)
        .unwrap_or_else(|| home.join(".agenty"));
    fs::create_dir_all(&path)?;
    Ok(normalize_data_dir_path(
        &fs::canonicalize(path)?.to_string_lossy(),
    ))
}

fn normalize_data_dir_path(path: &str) -> String {
    if !cfg!(windows) {
        return path.to_string();
    }
    normalize_windows_path(path)
}

fn normalize_windows_path(path: &str) -> String {
    let path = path.to_lowercase();
    if let Some(unc_path) = path.strip_prefix(r"\\?\unc\") {
        return format!(r"\\{unc_path}");
    }
    path.strip_prefix(r"\\?\").unwrap_or(&path).to_string()
}

fn address_for_data_dir(data_dir: &str) -> (&'static str, String) {
    let digest = Sha3_256::digest(data_dir.as_bytes());
    let key = hex(&digest[..16]);
    if cfg!(windows) {
        ("named_pipe", format!(r"\\.\pipe\agenty-core-{key}"))
    } else {
        (
            "uds",
            std::env::temp_dir()
                .join(format!("agenty-{key}.sock"))
                .to_string_lossy()
                .into_owned(),
        )
    }
}

fn spawn_core(
    binary: &Path,
    data_dir: &str,
    transport: &str,
    address: &str,
    executable_path: &std::ffi::OsStr,
    job: &mut ChildJob,
) -> Result<Child> {
    let mut child = Command::new(binary)
        .env("AGENTY_TRANSPORT", transport)
        .env("AGENTY_CORE_ADDR", address)
        .env("AGENTY_DATA_DIR", data_dir)
        .env("AGENTY_IPC_VERSION", IPC_VERSION)
        .env("PATH", executable_path)
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::inherit())
        .spawn()?;
    if let Err(error) = job.assign(&child) {
        let _ = child.kill();
        let _ = child.wait();
        return Err(error);
    }
    Ok(child)
}

fn spawn_cli(
    binary: &Path,
    args: &[OsString],
    data_dir: &str,
    transport: &str,
    address: &str,
    executable_path: &std::ffi::OsStr,
    job: &mut ChildJob,
) -> Result<Child> {
    let mut child = Command::new(binary)
        .args(args)
        .env("AGENTY_TRANSPORT", transport)
        .env("AGENTY_CORE_ADDR", address)
        .env("AGENTY_DATA_DIR", data_dir)
        .env("AGENTY_IPC_VERSION", IPC_VERSION)
        .env("PATH", executable_path)
        .stdin(Stdio::inherit())
        .stdout(Stdio::inherit())
        .stderr(Stdio::inherit())
        .spawn()?;
    if let Err(error) = job.assign(&child) {
        let _ = child.kill();
        let _ = child.wait();
        return Err(error);
    }
    Ok(child)
}

fn prepare_path(core: &Path, file_editor: &Path) -> Result<OsString> {
    let mut paths = vec![
        core.parent().unwrap_or(Path::new("")).to_path_buf(),
        file_editor.parent().unwrap_or(Path::new("")).to_path_buf(),
    ];
    if let Some(existing) = std::env::var_os("PATH") {
        paths.extend(std::env::split_paths(&existing));
    }
    let path = std::env::join_paths(paths).map_err(|error| {
        BootstrapError::Invalid(format!("could not prepare helper executable PATH: {error}"))
    })?;
    Ok(path)
}

async fn h2_request(address: &str, method: Method, path: &str) -> Result<(StatusCode, Vec<u8>)> {
    let io = local_stream(address).await?;
    let (mut sender, connection) =
        http2::handshake::<_, _, Full<Bytes>>(TokioExecutor::new(), TokioIo::new(io))
            .await
            .map_err(protocol_error)?;
    let task = tokio::spawn(async move {
        let _ = connection.await;
    });
    let request = Request::builder()
        .method(method)
        .uri(format!("http://agenty.local{path}"))
        .header("content-length", "0")
        .body(Full::new(Bytes::new()))
        .map_err(protocol_error)?;
    let response = sender.send_request(request).await.map_err(protocol_error)?;
    let status = response.status();
    let body = response
        .into_body()
        .collect()
        .await
        .map_err(protocol_error)?
        .to_bytes()
        .to_vec();
    task.abort();
    Ok((status, body))
}

#[cfg(unix)]
async fn local_stream(address: &str) -> Result<tokio::net::UnixStream> {
    tokio::net::UnixStream::connect(address)
        .await
        .map_err(protocol_error)
}

#[cfg(windows)]
async fn local_stream(address: &str) -> Result<tokio::net::windows::named_pipe::NamedPipeClient> {
    tokio::net::windows::named_pipe::ClientOptions::new()
        .open(address)
        .map_err(protocol_error)
}

fn protocol_error(error: impl std::fmt::Display) -> BootstrapError {
    BootstrapError::Invalid(format!("local HTTP/2 request failed: {error}"))
}

fn ensure_payload(
    packed: &mut File,
    spec: &PayloadSpec,
    target: &Path,
    name: &str,
    progress: &ProgressLog,
) -> Result<()> {
    progress.child(format!("checking {name} binary integrity..."));
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

fn agenty_version() -> &'static str {
    option_env!("AGENTY_VERSION")
        .filter(|version| !version.trim().is_empty())
        .unwrap_or("dev")
}

fn hex(bytes: &[u8]) -> String {
    let mut output = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        output.push_str(&format!("{byte:02x}"));
    }
    output
}

#[cfg(unix)]
struct ChildJob;

#[cfg(unix)]
impl ChildJob {
    fn new() -> Result<Self> {
        Ok(Self)
    }

    fn assign(&mut self, _child: &Child) -> Result<()> {
        Ok(())
    }
}

#[cfg(windows)]
mod windows_job {
    use super::*;
    use std::io;
    use std::os::windows::io::AsRawHandle;
    use std::ptr::null_mut;

    type Handle = *mut std::ffi::c_void;
    const EXTENDED_LIMIT_INFORMATION: i32 = 9;
    const KILL_ON_JOB_CLOSE: u32 = 0x2000;

    #[repr(C)]
    struct BasicLimitInformation {
        per_process_user_time_limit: i64,
        per_job_user_time_limit: i64,
        limit_flags: u32,
        minimum_working_set_size: usize,
        maximum_working_set_size: usize,
        active_process_limit: u32,
        affinity: usize,
        priority_class: u32,
        scheduling_class: u32,
    }

    #[repr(C)]
    struct IoCounters {
        read_operation_count: u64,
        write_operation_count: u64,
        other_operation_count: u64,
        read_transfer_count: u64,
        write_transfer_count: u64,
        other_transfer_count: u64,
    }

    #[repr(C)]
    struct ExtendedLimitInformation {
        basic: BasicLimitInformation,
        io: IoCounters,
        process_memory_limit: usize,
        job_memory_limit: usize,
        peak_process_memory_used: usize,
        peak_job_memory_used: usize,
    }

    #[link(name = "kernel32")]
    unsafe extern "system" {
        fn CreateJobObjectW(attributes: *mut std::ffi::c_void, name: *const u16) -> Handle;
        fn SetInformationJobObject(
            job: Handle,
            class: i32,
            info: *const std::ffi::c_void,
            size: u32,
        ) -> i32;
        fn AssignProcessToJobObject(job: Handle, process: Handle) -> i32;
        fn CloseHandle(handle: Handle) -> i32;
    }

    pub struct ChildJob {
        handle: Handle,
    }

    impl ChildJob {
        pub fn new() -> Result<Self> {
            let handle = unsafe { CreateJobObjectW(null_mut(), null_mut()) };
            if handle.is_null() {
                return Err(BootstrapError::Invalid(
                    io::Error::last_os_error().to_string(),
                ));
            }
            let mut info = ExtendedLimitInformation {
                basic: BasicLimitInformation {
                    per_process_user_time_limit: 0,
                    per_job_user_time_limit: 0,
                    limit_flags: KILL_ON_JOB_CLOSE,
                    minimum_working_set_size: 0,
                    maximum_working_set_size: 0,
                    active_process_limit: 0,
                    affinity: 0,
                    priority_class: 0,
                    scheduling_class: 0,
                },
                io: IoCounters {
                    read_operation_count: 0,
                    write_operation_count: 0,
                    other_operation_count: 0,
                    read_transfer_count: 0,
                    write_transfer_count: 0,
                    other_transfer_count: 0,
                },
                process_memory_limit: 0,
                job_memory_limit: 0,
                peak_process_memory_used: 0,
                peak_job_memory_used: 0,
            };
            if unsafe {
                SetInformationJobObject(
                    handle,
                    EXTENDED_LIMIT_INFORMATION,
                    &mut info as *mut _ as *const std::ffi::c_void,
                    std::mem::size_of::<ExtendedLimitInformation>() as u32,
                )
            } == 0
            {
                let error = io::Error::last_os_error();
                unsafe { CloseHandle(handle) };
                return Err(BootstrapError::Invalid(error.to_string()));
            }
            Ok(Self { handle })
        }

        pub fn assign(&mut self, child: &Child) -> Result<()> {
            if unsafe { AssignProcessToJobObject(self.handle, child.as_raw_handle() as Handle) }
                == 0
            {
                return Err(BootstrapError::Invalid(format!(
                    "could not assign child process to bootstrap job: {}",
                    io::Error::last_os_error()
                )));
            }
            Ok(())
        }
    }

    impl Drop for ChildJob {
        fn drop(&mut self) {
            if !self.handle.is_null() {
                unsafe { CloseHandle(self.handle) };
            }
        }
    }
}

#[cfg(windows)]
use windows_job::ChildJob;

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicU64, Ordering};
    use tokio::task::LocalSet;

    static NEXT_TEST_DIR: AtomicU64 = AtomicU64::new(0);

    struct TestDirectory(PathBuf);

    impl Drop for TestDirectory {
        fn drop(&mut self) {
            let Ok(root) = fs::canonicalize(&self.0) else {
                return;
            };
            let Ok(temp_root) = fs::canonicalize(std::env::temp_dir()) else {
                return;
            };
            if root.starts_with(temp_root) {
                let _ = fs::remove_dir_all(root);
            }
        }
    }

    fn test_binaries() -> (PathBuf, PathBuf, PathBuf) {
        let core =
            PathBuf::from(std::env::var_os("AGENTY_TEST_CORE_BIN").expect("AGENTY_TEST_CORE_BIN"));
        let file_editor = PathBuf::from(
            std::env::var_os("AGENTY_TEST_FILEEDIT_BIN").expect("AGENTY_TEST_FILEEDIT_BIN"),
        );
        let shell_root =
            std::env::var_os("SystemRoot").unwrap_or_else(|| OsString::from(r"C:\Windows"));
        let powershell = PathBuf::from(shell_root)
            .join("System32")
            .join("WindowsPowerShell")
            .join("v1.0")
            .join("powershell.exe");
        assert!(
            core.is_file(),
            "core binary does not exist: {}",
            core.display()
        );
        assert!(
            file_editor.is_file(),
            "fileedit binary does not exist: {}",
            file_editor.display()
        );
        assert!(
            powershell.is_file(),
            "Windows PowerShell does not exist: {}",
            powershell.display()
        );
        (core, file_editor, powershell)
    }

    fn supervisor_args(data_dir: &Path, script: &Path) -> (Vec<OsString>, Option<OsString>) {
        let (args, data_dir_arg) = parse_args(
            [
                OsString::from("--data-dir"),
                data_dir.as_os_str().to_owned(),
                OsString::from("-NoProfile"),
                OsString::from("-NonInteractive"),
                OsString::from("-File"),
                script.as_os_str().to_owned(),
            ]
            .into_iter(),
        )
        .expect("parse supervisor test arguments");
        (args, data_dir_arg)
    }

    fn call_supervisor<'a>(
        cli: &'a Path,
        core: &'a Path,
        file_editor: &'a Path,
        root: &'a Path,
        data_dir: &'a Path,
        script: &'a Path,
    ) -> impl std::future::Future<Output = Result<i32>> + 'a {
        let (args, data_dir_arg) = supervisor_args(data_dir, script);
        supervise(cli, core, file_editor, root, args, data_dir_arg)
    }

    async fn call_supervisor_owned(
        cli: PathBuf,
        core: PathBuf,
        file_editor: PathBuf,
        root: PathBuf,
        data_dir: PathBuf,
        script: PathBuf,
    ) -> Result<i32> {
        let (args, data_dir_arg) = supervisor_args(&data_dir, &script);
        supervise(&cli, &core, &file_editor, &root, args, data_dir_arg).await
    }

    fn test_directory() -> TestDirectory {
        TestDirectory(std::env::temp_dir().join(format!(
            "agenty-bootstrap-supervisor-{}-{}",
            std::process::id(),
            NEXT_TEST_DIR.fetch_add(1, Ordering::Relaxed),
        )))
    }

    fn write_script(root: &Path, name: &str, source: &str) -> PathBuf {
        let path = root.join(name);
        fs::create_dir_all(root).expect("create test directory");
        fs::write(&path, source).expect("write PowerShell script");
        path
    }

    fn ps_literal(path: &Path) -> String {
        path.to_string_lossy().replace('\'', "''")
    }

    fn waiting_script(started: &Path, release: &Path, pid_file: Option<&Path>) -> String {
        let mut script = format!(
            "Set-Content -LiteralPath '{}' -Value 'ready'\n",
            ps_literal(started)
        );
        if let Some(pid_file) = pid_file {
            script.push_str(&format!(
                "Set-Content -LiteralPath '{}' -Value ([string]$PID)\n",
                ps_literal(pid_file)
            ));
        }
        script.push_str(&format!(
            "while (-not (Test-Path -LiteralPath '{}')) {{ Start-Sleep -Milliseconds 30 }}\nexit 0\n",
            ps_literal(release)
        ));
        script
    }

    fn runtime() -> tokio::runtime::Runtime {
        tokio::runtime::Builder::new_multi_thread()
            .enable_all()
            .build()
            .expect("create test runtime")
    }

    async fn wait_for_path(path: &Path) {
        let deadline = Instant::now() + Duration::from_secs(15);
        while Instant::now() < deadline {
            if path.exists() {
                return;
            }
            sleep(Duration::from_millis(25)).await;
        }
        panic!("timed out waiting for {}", path.display());
    }

    async fn wait_for_server_to_stop(address: &str) {
        let deadline = Instant::now() + Duration::from_secs(10);
        while Instant::now() < deadline {
            if h2_request(address, Method::GET, "/v1/system")
                .await
                .is_err()
            {
                return;
            }
            sleep(Duration::from_millis(50)).await;
        }
        panic!("core still answered HTTP/2 requests at {address}");
    }

    fn powershell_process_running(powershell: &Path, pid: u32) -> bool {
        let check = format!(
            "if (Get-Process -Id {pid} -ErrorAction SilentlyContinue) {{ exit 0 }} else {{ exit 1 }}"
        );
        Command::new(powershell)
            .args(["-NoProfile", "-NonInteractive", "-Command", &check])
            .status()
            .is_ok_and(|status| status.success())
    }

    async fn wait_for_process_exit(powershell: &Path, pid: u32) {
        let deadline = Instant::now() + Duration::from_secs(10);
        while Instant::now() < deadline {
            if !powershell_process_running(powershell, pid) {
                return;
            }
            sleep(Duration::from_millis(50)).await;
        }
        panic!("child process {pid} remained alive after bootstrap cleanup");
    }

    async fn wait_for_process_to_exit(process: &mut Child) {
        let deadline = Instant::now() + Duration::from_secs(15);
        while Instant::now() < deadline {
            if process.try_wait().expect("poll process").is_some() {
                return;
            }
            sleep(Duration::from_millis(50)).await;
        }
        panic!("process {} did not exit", process.id());
    }

    fn run_taskkill(pid: u32) {
        let status = Command::new("taskkill")
            .args(["/F", "/PID", &pid.to_string()])
            .status()
            .expect("run taskkill");
        assert!(status.success(), "taskkill failed for PID {pid}: {status}");
    }

    async fn read_system_info(address: &str) -> SystemInfo {
        let (_, body) = h2_request(address, Method::GET, "/v1/system")
            .await
            .expect("read core system information");
        let response: APIResponse<SystemInfo> =
            serde_json::from_slice(&body).expect("decode core system information");
        assert_eq!(response.code, 200);
        assert_eq!(response.message, "ok");
        response.data
    }

    #[test]
    fn data_directory_option_is_removed_before_cli_launch() {
        let (args, data_dir) = parse_args(
            ["--quiet", "--data-dir", "C:\\data", "mcp", "list"]
                .into_iter()
                .map(OsString::from),
        )
        .unwrap();
        assert_eq!(args, ["--quiet", "mcp", "list"].map(OsString::from));
        assert_eq!(data_dir, Some(OsString::from("C:\\data")));
    }

    #[test]
    fn equals_form_of_data_directory_option_is_removed() {
        let (args, data_dir) = parse_args(
            ["--data-dir=/tmp/agenty", "help"]
                .into_iter()
                .map(OsString::from),
        )
        .unwrap();
        assert_eq!(args, [OsString::from("help")]);
        assert_eq!(data_dir, Some(OsString::from("/tmp/agenty")));
    }

    #[test]
    fn windows_canonical_paths_drop_the_verbatim_prefix() {
        assert_eq!(
            normalize_windows_path(r"\\?\C:\Users\Master\Agenty"),
            r"c:\users\master\agenty"
        );
        assert_eq!(
            normalize_windows_path(r"\\?\UNC\server\share\agenty"),
            r"\\server\share\agenty"
        );
    }

    #[test]
    #[ignore = "requires real Windows core and CLI binaries; run with AGENTY_TEST_* paths"]
    fn real_core_cli_lifecycle_uses_http2_and_stops_only_its_owned_core() {
        let core =
            PathBuf::from(std::env::var_os("AGENTY_TEST_CORE_BIN").expect("AGENTY_TEST_CORE_BIN"));
        let cli =
            PathBuf::from(std::env::var_os("AGENTY_TEST_CLI_BIN").expect("AGENTY_TEST_CLI_BIN"));
        let file_editor = PathBuf::from(
            std::env::var_os("AGENTY_TEST_FILEEDIT_BIN").expect("AGENTY_TEST_FILEEDIT_BIN"),
        );
        assert!(
            core.is_file(),
            "core binary does not exist: {}",
            core.display()
        );
        assert!(
            cli.is_file(),
            "CLI binary does not exist: {}",
            cli.display()
        );
        assert!(
            file_editor.is_file(),
            "fileedit binary does not exist: {}",
            file_editor.display()
        );

        let test_root = TestDirectory(std::env::temp_dir().join(format!(
            "agenty-bootstrap-supervisor-{}-{}",
            std::process::id(),
            NEXT_TEST_DIR.fetch_add(1, Ordering::Relaxed),
        )));
        let data_dir = test_root.0.join("data");
        let (args, data_dir_arg) = parse_args(
            [
                OsString::from("--data-dir"),
                data_dir.as_os_str().to_owned(),
                OsString::from("provider"),
                OsString::from("list"),
                OsString::from("--json"),
            ]
            .into_iter(),
        )
        .expect("parse test arguments");
        let runtime = tokio::runtime::Builder::new_multi_thread()
            .enable_all()
            .build()
            .expect("create test runtime");
        let result = runtime.block_on(supervise(
            &cli,
            &core,
            &file_editor,
            &test_root.0,
            args,
            data_dir_arg,
        ));
        assert_eq!(result.expect("supervisor lifecycle"), 0);
    }

    #[test]
    #[ignore = "requires real Windows core binary; run with AGENTY_TEST_CORE_BIN and AGENTY_TEST_FILEEDIT_BIN"]
    fn supervisor_lifecycle_matrix_owns_and_cleans_up_processes() {
        let (core, file_editor, powershell) = test_binaries();
        let local = LocalSet::new();
        let runtime = runtime();
        runtime.block_on(local.run_until(async {
            // A same-directory bootstrap attaches to the existing core and may
            // only manage its own CLI process.
            let owner_root = test_directory();
            let owner_data = owner_root.0.join("data");
            fs::create_dir_all(&owner_data).expect("create owner data directory");
            let owner_started = owner_root.0.join("owner-started");
            let owner_release = owner_root.0.join("owner-release");
            let owner_script = write_script(
                &owner_root.0,
                "owner.ps1",
                &waiting_script(&owner_started, &owner_release, None),
            );
            let owner = tokio::task::spawn_local(call_supervisor_owned(
                powershell.clone(),
                core.clone(),
                file_editor.clone(),
                owner_root.0.clone(),
                owner_data.clone(),
                owner_script.clone(),
            ));
            wait_for_path(&owner_started).await;
            let canonical_owner =
                resolve_data_dir(&owner_root.0, Some(owner_data.as_os_str().to_owned()))
                    .expect("resolve owner data directory");
            let (_, owner_address) = address_for_data_dir(&canonical_owner);
            let first_info = read_system_info(&owner_address).await;

            let attach_script = write_script(&owner_root.0, "attach.ps1", "exit 0\n");
            let attach_result = call_supervisor(
                &powershell,
                &core,
                &file_editor,
                &owner_root.0,
                &owner_data,
                &attach_script,
            )
            .await
            .expect("same-directory attaching bootstrap");
            assert_eq!(attach_result, 0);
            let after_attach = read_system_info(&owner_address).await;
            assert_eq!(after_attach.process_id, first_info.process_id);
            fs::write(&owner_release, "release").expect("release owner CLI");
            assert_eq!(
                owner
                    .await
                    .expect("join owner bootstrap")
                    .expect("owner lifecycle"),
                0
            );
            wait_for_server_to_stop(&owner_address).await;

            // Separate canonical directories receive distinct cores and can
            // execute concurrently without sharing ownership.
            let left_root = test_directory();
            let right_root = test_directory();
            let left_data = left_root.0.join("data");
            let right_data = right_root.0.join("data");
            fs::create_dir_all(&left_data).expect("create left data directory");
            fs::create_dir_all(&right_data).expect("create right data directory");
            let left_started = left_root.0.join("cli-started");
            let right_started = right_root.0.join("cli-started");
            let left_release = left_root.0.join("cli-release");
            let right_release = right_root.0.join("cli-release");
            let left_script = write_script(
                &left_root.0,
                "wait.ps1",
                &waiting_script(&left_started, &left_release, None),
            );
            let right_script = write_script(
                &right_root.0,
                "wait.ps1",
                &waiting_script(&right_started, &right_release, None),
            );
            let left = tokio::task::spawn_local(call_supervisor_owned(
                powershell.clone(),
                core.clone(),
                file_editor.clone(),
                left_root.0.clone(),
                left_data.clone(),
                left_script.clone(),
            ));
            let right = tokio::task::spawn_local(call_supervisor_owned(
                powershell.clone(),
                core.clone(),
                file_editor.clone(),
                right_root.0.clone(),
                right_data.clone(),
                right_script.clone(),
            ));
            wait_for_path(&left_started).await;
            wait_for_path(&right_started).await;
            let canonical_left =
                resolve_data_dir(&left_root.0, Some(left_data.as_os_str().to_owned()))
                    .expect("resolve left data directory");
            let canonical_right =
                resolve_data_dir(&right_root.0, Some(right_data.as_os_str().to_owned()))
                    .expect("resolve right data directory");
            let (_, left_address) = address_for_data_dir(&canonical_left);
            let (_, right_address) = address_for_data_dir(&canonical_right);
            let left_info = read_system_info(&left_address).await;
            let right_info = read_system_info(&right_address).await;
            assert_ne!(left_info.process_id, right_info.process_id);
            assert_eq!(left_info.data_dir, canonical_left);
            assert_eq!(right_info.data_dir, canonical_right);
            fs::write(&left_release, "release").expect("release left CLI");
            fs::write(&right_release, "release").expect("release right CLI");
            assert_eq!(
                left.await
                    .expect("join left bootstrap")
                    .expect("left lifecycle"),
                0
            );
            assert_eq!(
                right
                    .await
                    .expect("join right bootstrap")
                    .expect("right lifecycle"),
                0
            );
            wait_for_server_to_stop(&left_address).await;
            wait_for_server_to_stop(&right_address).await;

            // An abnormal CLI exit still performs bounded graceful core shutdown.
            let failed_root = test_directory();
            let failed_data = failed_root.0.join("data");
            let failed_script = write_script(&failed_root.0, "fail.ps1", "exit 23\n");
            let (failed_args, failed_data_arg) = supervisor_args(&failed_data, &failed_script);
            assert_eq!(
                supervise(
                    &powershell,
                    &core,
                    &file_editor,
                    &failed_root.0,
                    failed_args,
                    failed_data_arg,
                )
                .await
                .expect("nonzero CLI exit cleanup"),
                23
            );
            let canonical_failed =
                resolve_data_dir(&failed_root.0, Some(failed_data.as_os_str().to_owned()))
                    .expect("resolve failed data directory");
            let (_, failed_address) = address_for_data_dir(&canonical_failed);
            wait_for_server_to_stop(&failed_address).await;

            // If core fails while the CLI is active, the bootstrap must end
            // the CLI rather than leave an orphan attached to a dead core.
            let crash_root = test_directory();
            let crash_data = crash_root.0.join("data");
            fs::create_dir_all(&crash_data).expect("create crash data directory");
            let crash_started = crash_root.0.join("cli-started");
            let crash_release = crash_root.0.join("cli-release");
            let cli_pid_file = crash_root.0.join("cli-pid");
            let crash_script = write_script(
                &crash_root.0,
                "wait.ps1",
                &waiting_script(&crash_started, &crash_release, Some(&cli_pid_file)),
            );
            let crash = tokio::task::spawn_local(call_supervisor_owned(
                powershell.clone(),
                core.clone(),
                file_editor.clone(),
                crash_root.0.clone(),
                crash_data.clone(),
                crash_script.clone(),
            ));
            wait_for_path(&crash_started).await;
            wait_for_path(&cli_pid_file).await;
            let canonical_crash =
                resolve_data_dir(&crash_root.0, Some(crash_data.as_os_str().to_owned()))
                    .expect("resolve crash data directory");
            let (_, crash_address) = address_for_data_dir(&canonical_crash);
            let crash_info = read_system_info(&crash_address).await;
            let cli_pid: u32 = fs::read_to_string(&cli_pid_file)
                .expect("read CLI process id")
                .trim()
                .parse()
                .expect("parse CLI process id");
            run_taskkill(crash_info.process_id);
            assert!(crash.await.expect("join crash bootstrap").is_err());
            wait_for_process_exit(&powershell, cli_pid).await;
            wait_for_server_to_stop(&crash_address).await;
        }));
    }

    #[test]
    #[ignore = "helper for supervisor_lifecycle_matrix_bootstrap_crash_cleans_children"]
    fn abrupt_supervisor_helper() {
        let core = PathBuf::from(std::env::var_os("AGENTY_TEST_CORE_BIN").expect("core path"));
        let file_editor =
            PathBuf::from(std::env::var_os("AGENTY_TEST_FILEEDIT_BIN").expect("file editor path"));
        let cli =
            PathBuf::from(std::env::var_os("AGENTY_TEST_POWERSHELL_BIN").expect("PowerShell path"));
        let root = PathBuf::from(std::env::var_os("AGENTY_TEST_ROOT").expect("test root"));
        let data_dir = PathBuf::from(std::env::var_os("AGENTY_TEST_DATA_DIR").expect("data dir"));
        let script = PathBuf::from(std::env::var_os("AGENTY_TEST_SCRIPT").expect("script"));
        let started =
            PathBuf::from(std::env::var_os("AGENTY_TEST_STARTED").expect("started marker"));
        let result = runtime().block_on(call_supervisor(
            &cli,
            &core,
            &file_editor,
            &root,
            &data_dir,
            &script,
        ));
        if started.exists() {
            std::process::abort();
        }
        panic!("supervisor returned before CLI marker: {result:?}");
    }

    #[test]
    #[ignore = "requires real Windows core binary; run with AGENTY_TEST_CORE_BIN and AGENTY_TEST_FILEEDIT_BIN"]
    fn supervisor_lifecycle_matrix_bootstrap_crash_cleans_children() {
        let (core, file_editor, powershell) = test_binaries();
        let root = test_directory();
        let data_dir = root.0.join("data");
        fs::create_dir_all(&data_dir).expect("create data directory");
        let started = root.0.join("started");
        let release = root.0.join("release");
        let pid_file = root.0.join("cli-pid");
        let script = write_script(
            &root.0,
            "wait.ps1",
            &waiting_script(&started, &release, Some(&pid_file)),
        );
        let child = Command::new(std::env::current_exe().expect("current test executable"))
            .args([
                "--exact",
                "supervisor::tests::abrupt_supervisor_helper",
                "--ignored",
                "--nocapture",
            ])
            .env("AGENTY_TEST_CORE_BIN", &core)
            .env("AGENTY_TEST_FILEEDIT_BIN", &file_editor)
            .env("AGENTY_TEST_POWERSHELL_BIN", &powershell)
            .env("AGENTY_TEST_ROOT", &root.0)
            .env("AGENTY_TEST_DATA_DIR", &data_dir)
            .env("AGENTY_TEST_SCRIPT", &script)
            .env("AGENTY_TEST_STARTED", &started)
            .spawn()
            .expect("start crash helper");
        let mut child = child;
        let runtime = runtime();
        runtime.block_on(wait_for_path(&started));
        runtime.block_on(wait_for_path(&pid_file));
        let cli_pid: u32 = fs::read_to_string(&pid_file)
            .expect("read CLI process id")
            .trim()
            .parse()
            .expect("parse CLI process id");
        let canonical_data = resolve_data_dir(&root.0, Some(data_dir.as_os_str().to_owned()))
            .expect("resolve data directory");
        let (_, address) = address_for_data_dir(&canonical_data);
        let info = runtime.block_on(read_system_info(&address));
        run_taskkill(child.id());
        runtime.block_on(wait_for_process_to_exit(&mut child));
        runtime.block_on(wait_for_process_exit(&powershell, cli_pid));
        runtime.block_on(wait_for_server_to_stop(&address));
        assert!(info.process_id > 0);
    }
}
