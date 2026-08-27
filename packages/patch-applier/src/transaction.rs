use std::collections::{HashMap, HashSet};
use std::fs::{self, File, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};

use super::{
    apply_update_diff, backup_path, create_missing_dirs, is_binary_data, is_binary_snapshot,
    is_binary_virtual, parse_create_diff, path_exists, read_snapshot, relative_display, set_mode,
    text_for_diff, text_for_diff_virtual, unified_diff, unique_path, EntryKind, FileResult,
    FileSnapshot, Operation, OperationKind, PatchError, VirtualFile,
};

static TRANSACTION_COUNTER: AtomicU64 = AtomicU64::new(0);

pub(super) struct Transaction {
    cwd: PathBuf,
    original: HashMap<PathBuf, FileSnapshot>,
    state: HashMap<PathBuf, VirtualFile>,
    touched_order: Vec<PathBuf>,
    deleted_paths: HashSet<PathBuf>,
}

impl Transaction {
    pub(super) fn new(cwd: PathBuf) -> Self {
        Self {
            cwd,
            original: HashMap::new(),
            state: HashMap::new(),
            touched_order: Vec::new(),
            deleted_paths: HashSet::new(),
        }
    }

    pub(super) fn apply(&mut self, operation: Operation) -> Result<(), PatchError> {
        self.touch(&operation.path)?;
        let current = self.current(&operation.path)?.clone();
        match operation.kind {
            OperationKind::Add => {
                if current.kind != EntryKind::Missing
                    || self.deleted_paths.contains(&operation.path)
                {
                    return Err(PatchError::Conflict(format!(
                        "operation at line {} creates an existing or previously deleted file {}",
                        operation.line,
                        operation.path.display()
                    )));
                }
                let data = parse_create_diff(&operation.diff)?;
                self.state
                    .insert(operation.path.clone(), VirtualFile::regular(data, 0o644));
            }
            OperationKind::Update => {
                if current.kind != EntryKind::Regular {
                    return Err(PatchError::Conflict(format!(
                        "operation at line {} updates a non-existent or non-regular file {}",
                        operation.line,
                        operation.path.display()
                    )));
                }
                let data = if is_binary_data(&current.data) {
                    if operation.diff.is_empty() {
                        current.data.clone()
                    } else {
                        return Err(PatchError::Invalid(
                            "binary file updates must not contain a text diff".to_string(),
                        ));
                    }
                } else {
                    apply_update_diff(&current.data, &operation.diff)?
                };
                self.state.insert(
                    operation.path.clone(),
                    VirtualFile::regular(data, current.mode),
                );
                if let Some(destination) = operation.move_to {
                    self.move_file(&operation.path, &destination, operation.line)?;
                }
            }
            OperationKind::Delete => {
                if !matches!(current.kind, EntryKind::Regular | EntryKind::Symlink) {
                    return Err(PatchError::Conflict(format!(
                        "operation at line {} deletes a missing or non-regular file {}",
                        operation.line,
                        operation.path.display()
                    )));
                }
                self.state
                    .insert(operation.path.clone(), VirtualFile::missing());
                self.deleted_paths.insert(operation.path);
            }
        }
        Ok(())
    }

    fn touch(&mut self, path: &Path) -> Result<(), PatchError> {
        if self.original.contains_key(path) {
            return Ok(());
        }
        let snapshot = read_snapshot(path)?;
        let state = VirtualFile {
            kind: snapshot.kind,
            data: snapshot.data.clone(),
            mode: snapshot.mode,
        };
        self.original.insert(path.to_path_buf(), snapshot);
        self.state.insert(path.to_path_buf(), state);
        self.touched_order.push(path.to_path_buf());
        Ok(())
    }

    fn current(&mut self, path: &Path) -> Result<&VirtualFile, PatchError> {
        self.touch(path)?;
        self.state
            .get(path)
            .ok_or_else(|| PatchError::Invalid(format!("missing state for {}", path.display())))
    }

    fn move_file(
        &mut self,
        source: &Path,
        destination: &Path,
        line: usize,
    ) -> Result<(), PatchError> {
        if source == destination {
            return Ok(());
        }
        self.touch(destination)?;
        let destination_state = self.current(destination)?.clone();
        if destination_state.kind != EntryKind::Missing || self.deleted_paths.contains(destination)
        {
            return Err(PatchError::Conflict(format!(
                "operation at line {line} moves {} onto an occupied path {}",
                source.display(),
                destination.display()
            )));
        }
        let source_state = self.current(source)?.clone();
        self.state.insert(destination.to_path_buf(), source_state);
        self.state
            .insert(source.to_path_buf(), VirtualFile::missing());
        self.deleted_paths.insert(source.to_path_buf());
        Ok(())
    }

    pub(super) fn prepare_results(&self) -> Result<Vec<FileResult>, PatchError> {
        let mut results = Vec::new();
        for path in &self.touched_order {
            let before = self.original.get(path).ok_or_else(|| {
                PatchError::Invalid(format!("missing original snapshot for {}", path.display()))
            })?;
            let after = self.state.get(path).ok_or_else(|| {
                PatchError::Invalid(format!("missing final state for {}", path.display()))
            })?;
            if before.kind == after.kind && before.data == after.data {
                continue;
            }
            if is_binary_snapshot(before) || is_binary_virtual(after) {
                results.push(FileResult {
                    path: relative_display(&self.cwd, path),
                    diff: String::new(),
                    added_lines: 0,
                    removed_lines: 0,
                });
                continue;
            }
            let old_text = text_for_diff(before)?;
            let new_text = text_for_diff_virtual(after)?;
            let relative = relative_display(&self.cwd, path);
            let old_header = if before.kind == EntryKind::Missing {
                "/dev/null".to_string()
            } else {
                format!("a/{relative}")
            };
            let new_header = if after.kind == EntryKind::Missing {
                "/dev/null".to_string()
            } else {
                format!("b/{relative}")
            };
            let (diff, added_lines, removed_lines) =
                unified_diff(&old_text, &new_text, &old_header, &new_header);
            results.push(FileResult {
                path: relative,
                diff,
                added_lines,
                removed_lines,
            });
        }
        Ok(results)
    }

    pub(super) fn lock_paths(&self) -> Result<Vec<PathBuf>, PatchError> {
        Ok(self
            .changes()?
            .into_iter()
            .map(|change| change.path)
            .collect())
    }

    pub(super) fn commit(&self) -> Result<(), PatchError> {
        let changes = self.changes()?;
        if changes.is_empty() {
            return Ok(());
        }

        for change in &changes {
            let expected = self.original.get(&change.path).ok_or_else(|| {
                PatchError::Invalid(format!(
                    "missing original snapshot for {}",
                    change.path.display()
                ))
            })?;
            if &read_snapshot(&change.path)? != expected {
                return Err(PatchError::Conflict(format!(
                    "file changed while the patch was being prepared: {}",
                    change.path.display()
                )));
            }
        }

        let transaction_id = format!(
            ".agenty-apply-patch-{}-{}",
            std::process::id(),
            TRANSACTION_COUNTER.fetch_add(1, Ordering::Relaxed)
        );
        let mut staged = Vec::new();
        let mut installed = Vec::new();
        let mut backups = Vec::new();
        let mut created_dirs = Vec::new();

        let result = (|| -> Result<(), PatchError> {
            for change in &changes {
                if path_exists(&change.path)? {
                    let backup = backup_path(&change.path, &transaction_id)?;
                    fs::rename(&change.path, &backup)?;
                    backups.push((change.path.clone(), backup));
                }
            }

            for change in &changes {
                if let Some(file) = &change.after {
                    let parent = change.path.parent().unwrap_or(&self.cwd);
                    created_dirs.extend(create_missing_dirs(parent)?);
                    let temp = parent.join(format!(".{transaction_id}.stage"));
                    let temp = unique_path(&temp)?;
                    let mut output = OpenOptions::new()
                        .write(true)
                        .create_new(true)
                        .open(&temp)?;
                    output.write_all(&file.data)?;
                    output.flush()?;
                    set_mode(&output, file.mode)?;
                    output.sync_all()?;
                    staged.push((change.path.clone(), temp));
                }
            }

            for (path, temp) in &staged {
                fs::rename(temp, path)?;
                installed.push(path.clone());
            }
            sync_parent_directories(&changes)?;
            Ok(())
        })();

        if result.is_err() {
            for path in installed.iter().rev() {
                let _ = fs::remove_file(path);
            }
            for (path, backup) in backups.iter().rev() {
                let _ = fs::rename(backup, path);
            }
            for temp in staged.iter().map(|(_, temp)| temp) {
                let _ = fs::remove_file(temp);
            }
            for directory in &created_dirs {
                let _ = fs::remove_dir(directory);
            }
            let _ = sync_parent_directories(&changes);
        } else {
            for (_, backup) in backups {
                let _ = fs::remove_file(backup);
            }
        }
        result
    }

    fn changes(&self) -> Result<Vec<Change>, PatchError> {
        let mut changes = Vec::new();
        for path in &self.touched_order {
            let before = self.original.get(path).ok_or_else(|| {
                PatchError::Invalid(format!("missing original snapshot for {}", path.display()))
            })?;
            let after = self.state.get(path).ok_or_else(|| {
                PatchError::Invalid(format!("missing final state for {}", path.display()))
            })?;
            let before_exists = before.kind != EntryKind::Missing;
            let after_file = if after.kind == EntryKind::Regular {
                Some(after.clone())
            } else {
                None
            };
            if before_exists && after.kind == EntryKind::Missing {
                changes.push(Change {
                    path: path.clone(),
                    after: None,
                });
            } else if (!before_exists && after_file.is_some())
                || (before.kind == EntryKind::Regular
                    && after.kind == EntryKind::Regular
                    && (before.data != after.data || before.mode != after.mode))
            {
                changes.push(Change {
                    path: path.clone(),
                    after: after_file,
                });
            } else if before.kind != after.kind {
                return Err(PatchError::Conflict(format!(
                    "unsupported final state transition for {}",
                    path.display()
                )));
            }
        }
        Ok(changes)
    }
}

struct Change {
    path: PathBuf,
    after: Option<VirtualFile>,
}

#[cfg(unix)]
fn sync_parent_directories(changes: &[Change]) -> Result<(), PatchError> {
    let mut parents = HashSet::new();
    for change in changes {
        if let Some(parent) = change.path.parent() {
            parents.insert(parent);
        }
    }
    for parent in parents {
        File::open(parent)?.sync_all()?;
    }
    Ok(())
}

#[cfg(not(unix))]
fn sync_parent_directories(_changes: &[Change]) -> Result<(), PatchError> {
    Ok(())
}
