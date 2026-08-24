use std::fs;
use std::io::Write;
use std::process::{Command, Stdio};
use std::time::{SystemTime, UNIX_EPOCH};

use serde_json::Value;

fn temp_dir(name: &str) -> std::path::PathBuf {
    let suffix = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("system clock must follow the Unix epoch")
        .as_nanos();
    let path = std::env::temp_dir().join(format!("agenty-patch-cli-{name}-{suffix}"));
    fs::create_dir_all(&path).expect("create test directory");
    path
}

#[test]
fn prints_complete_success_json() {
    let cwd = temp_dir("success");
    let mut child = Command::new(env!("CARGO_BIN_EXE_apply_patch"))
        .current_dir(&cwd)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .expect("start apply_patch");
    child
        .stdin
        .take()
        .expect("capture stdin")
        .write_all(b"*** Begin Patch\n*** Add File: notes.txt\n+hello\n*** End Patch\n")
        .expect("write patch");
    let output = child.wait_with_output().expect("wait for apply_patch");
    assert!(
        output.status.success(),
        "stderr: {}",
        String::from_utf8_lossy(&output.stderr)
    );

    let result: Value = serde_json::from_slice(&output.stdout).expect("decode stdout JSON");
    assert_eq!(result["success"], true);
    assert_eq!(
        result["cwd"],
        cwd.canonicalize().unwrap().display().to_string()
    );
    assert_eq!(result["files"][0]["path"], "notes.txt");
    assert_eq!(result["files"][0]["addedLines"], 1);
    assert_eq!(result["files"][0]["removedLines"], 0);
    assert!(result["files"][0]["diff"]
        .as_str()
        .unwrap()
        .contains("+hello"));
}

#[test]
fn prints_complete_error_json() {
    let cwd = temp_dir("error");
    let mut child = Command::new(env!("CARGO_BIN_EXE_apply_patch"))
        .current_dir(&cwd)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .expect("start apply_patch");
    child
        .stdin
        .take()
        .expect("capture stdin")
        .write_all(b"malformed")
        .expect("write patch");
    let output = child.wait_with_output().expect("wait for apply_patch");
    assert!(!output.status.success());

    let result: Value = serde_json::from_slice(&output.stdout).expect("decode stdout JSON");
    assert_eq!(result["success"], false);
    assert_eq!(
        result["cwd"],
        cwd.canonicalize().unwrap().display().to_string()
    );
    assert_eq!(result["files"], Value::Array(Vec::new()));
    assert!(result["error"].as_str().unwrap().contains("must start"));
}
