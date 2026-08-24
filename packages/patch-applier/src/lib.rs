use std::collections::{HashMap, HashSet};
use std::fmt::{self, Display, Formatter};
use std::fs::{self, File, OpenOptions};
use std::io::{self, Write};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};

use serde::Serialize;
use similar::{ChangeTag, TextDiff};

const BEGIN_MARKER: &str = "*** Begin Patch";
const END_MARKER: &str = "*** End Patch";
const UPDATE_MARKER: &str = "*** Update File:";
const DELETE_MARKER: &str = "*** Delete File:";
const ADD_MARKER: &str = "*** Add File:";
const MOVE_MARKER: &str = "*** Move to:";
const END_OF_FILE_MARKER: &str = "*** End of File";

static TRANSACTION_COUNTER: AtomicU64 = AtomicU64::new(0);

#[derive(Debug)]
pub enum PatchError {
    Io(io::Error),
    Invalid(String),
    Conflict(String),
}

impl Display for PatchError {
    fn fmt(&self, formatter: &mut Formatter<'_>) -> fmt::Result {
        match self {
            Self::Io(error) => write!(formatter, "I/O error: {error}"),
            Self::Invalid(message) => write!(formatter, "invalid patch: {message}"),
            Self::Conflict(message) => write!(formatter, "conflicting patch operations: {message}"),
        }
    }
}

impl std::error::Error for PatchError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Self::Io(error) => Some(error),
            Self::Invalid(_) | Self::Conflict(_) => None,
        }
    }
}

impl From<io::Error> for PatchError {
    fn from(error: io::Error) -> Self {
        Self::Io(error)
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
enum OperationKind {
    Add,
    Update,
    Delete,
}

#[derive(Debug, Clone)]
struct Operation {
    kind: OperationKind,
    path: PathBuf,
    move_to: Option<PathBuf>,
    diff: String,
    line: usize,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct FileSnapshot {
    kind: EntryKind,
    data: Vec<u8>,
    mode: u32,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum EntryKind {
    Missing,
    Regular,
    Symlink,
    Directory,
    Other,
}

#[derive(Debug, Clone)]
struct VirtualFile {
    kind: EntryKind,
    data: Vec<u8>,
    mode: u32,
}

impl VirtualFile {
    fn missing() -> Self {
        Self {
            kind: EntryKind::Missing,
            data: Vec::new(),
            mode: 0,
        }
    }

    fn regular(data: Vec<u8>, mode: u32) -> Self {
        Self {
            kind: EntryKind::Regular,
            data,
            mode,
        }
    }
}

#[derive(Debug, Clone)]
struct FileGroup {
    first_index: usize,
    operations: Vec<Operation>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PatchResult {
    pub success: bool,
    pub cwd: String,
    pub files: Vec<FileResult>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FileResult {
    pub path: String,
    pub diff: String,
    pub added_lines: usize,
    pub removed_lines: usize,
}

pub fn apply_patch(cwd: &Path, patch: &str) -> Result<PatchResult, PatchError> {
    let operations = parse_envelope(cwd, patch)?;
    let groups = classify_operations(operations)?;
    let mut transaction = Transaction::new(cwd.to_path_buf());

    for group in groups {
        for operation in group.operations {
            transaction.apply(operation)?;
        }
    }

    let results = transaction.prepare_results()?;
    transaction.commit()?;

    Ok(PatchResult {
        success: true,
        cwd: cwd.display().to_string(),
        files: results,
    })
}

fn parse_envelope(cwd: &Path, patch: &str) -> Result<Vec<Operation>, PatchError> {
    let lines = normalized_lines(patch);
    if lines.len() < 3 || lines.first().map(String::as_str) != Some(BEGIN_MARKER) {
        return Err(PatchError::Invalid(format!(
            "patch must start with {BEGIN_MARKER:?}"
        )));
    }
    if lines.last().map(String::as_str) != Some(END_MARKER) {
        return Err(PatchError::Invalid(format!(
            "patch must end with {END_MARKER:?}"
        )));
    }

    let mut operations = Vec::new();
    let mut index = 1;
    while index < lines.len() - 1 {
        let (operation, next_index) = parse_operation(cwd, &lines, index)?;
        operations.push(operation);
        index = next_index;
    }
    if operations.is_empty() {
        return Err(PatchError::Invalid(
            "patch contains no file operations".to_string(),
        ));
    }
    Ok(operations)
}

fn normalized_lines(patch: &str) -> Vec<String> {
    let normalized = patch.replace("\r\n", "\n");
    let mut lines: Vec<String> = normalized
        .split('\n')
        .map(|line| line.strip_suffix('\r').unwrap_or(line).to_string())
        .collect();
    if lines.last().is_some_and(String::is_empty) {
        lines.pop();
    }
    lines
}

fn parse_operation(
    cwd: &Path,
    lines: &[String],
    start: usize,
) -> Result<(Operation, usize), PatchError> {
    let header = &lines[start];
    let (kind, marker) = if header.starts_with(UPDATE_MARKER) {
        (OperationKind::Update, UPDATE_MARKER)
    } else if header.starts_with(DELETE_MARKER) {
        (OperationKind::Delete, DELETE_MARKER)
    } else if header.starts_with(ADD_MARKER) {
        (OperationKind::Add, ADD_MARKER)
    } else {
        return Err(PatchError::Invalid(format!(
            "invalid patch header at line {}: {header}",
            start + 1
        )));
    };

    let raw_path = header.strip_prefix(marker).unwrap_or_default().trim();
    if raw_path.is_empty() {
        return Err(PatchError::Invalid(format!(
            "operation at line {} has an empty path",
            start + 1
        )));
    }
    let path = resolve_path(cwd, raw_path)?;

    let mut index = start + 1;
    let mut move_to = None;
    if kind == OperationKind::Update
        && index < lines.len() - 1
        && lines[index].starts_with(MOVE_MARKER)
    {
        let raw_move_to = lines[index]
            .strip_prefix(MOVE_MARKER)
            .unwrap_or_default()
            .trim();
        if raw_move_to.is_empty() {
            return Err(PatchError::Invalid(format!(
                "move at line {} has an empty path",
                index + 1
            )));
        }
        move_to = Some(resolve_path(cwd, raw_move_to)?);
        index += 1;
    }

    let body_start = index;
    while index < lines.len() - 1 && !is_operation_header(&lines[index]) {
        index += 1;
    }
    let body = lines[body_start..index].join("\n");
    if kind == OperationKind::Delete && !body.is_empty() {
        return Err(PatchError::Invalid(format!(
            "delete operation for {raw_path:?} must not contain a diff body"
        )));
    }

    Ok((
        Operation {
            kind,
            path,
            move_to,
            diff: body,
            line: start + 1,
        },
        index,
    ))
}

fn is_operation_header(line: &str) -> bool {
    line.starts_with(UPDATE_MARKER)
        || line.starts_with(DELETE_MARKER)
        || line.starts_with(ADD_MARKER)
}

fn resolve_path(cwd: &Path, raw: &str) -> Result<PathBuf, PatchError> {
    let path = Path::new(raw);
    let resolved = if path.is_absolute() {
        path.to_path_buf()
    } else {
        cwd.join(path)
    };
    lexical_normalize(&resolved)
}

fn lexical_normalize(path: &Path) -> Result<PathBuf, PatchError> {
    let mut normalized = PathBuf::new();
    for component in path.components() {
        match component {
            std::path::Component::Prefix(prefix) => {
                normalized.push(prefix.as_os_str());
            }
            std::path::Component::RootDir => {
                normalized.push(Path::new(std::path::MAIN_SEPARATOR_STR));
            }
            std::path::Component::CurDir => {}
            std::path::Component::ParentDir => {
                let rooted = normalized.has_root();
                if !normalized.pop() || (rooted && !normalized.has_root()) {
                    return Err(PatchError::Invalid(format!(
                        "path escapes its root: {}",
                        path.display()
                    )));
                }
            }
            std::path::Component::Normal(component) => normalized.push(component),
        }
    }
    Ok(normalized)
}

fn classify_operations(operations: Vec<Operation>) -> Result<Vec<FileGroup>, PatchError> {
    let mut groups = Vec::<FileGroup>::new();
    let mut aliases = HashMap::<PathBuf, usize>::new();

    for (index, operation) in operations.into_iter().enumerate() {
        let group_index = if let Some(group_index) = aliases.get(&operation.path) {
            *group_index
        } else {
            groups.push(FileGroup {
                first_index: index,
                operations: Vec::new(),
            });
            groups.len() - 1
        };

        aliases.insert(operation.path.clone(), group_index);
        if let Some(destination) = &operation.move_to {
            if let Some(existing) = aliases.get(destination) {
                if *existing != group_index {
                    return Err(PatchError::Conflict(format!(
                        "operation at line {} moves {} onto a path owned by another file",
                        operation.line,
                        destination.display()
                    )));
                }
            }
            aliases.insert(destination.clone(), group_index);
        }
        groups[group_index].operations.push(operation);
    }

    groups.sort_by_key(|group| group.first_index);
    Ok(groups)
}

struct Transaction {
    cwd: PathBuf,
    original: HashMap<PathBuf, FileSnapshot>,
    state: HashMap<PathBuf, VirtualFile>,
    touched_order: Vec<PathBuf>,
    deleted_paths: HashSet<PathBuf>,
}

impl Transaction {
    fn new(cwd: PathBuf) -> Self {
        Self {
            cwd,
            original: HashMap::new(),
            state: HashMap::new(),
            touched_order: Vec::new(),
            deleted_paths: HashSet::new(),
        }
    }

    fn apply(&mut self, operation: Operation) -> Result<(), PatchError> {
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
                let data = apply_update_diff(&current.data, &operation.diff)?;
                self.state.insert(
                    operation.path.clone(),
                    VirtualFile::regular(data, current.mode),
                );
                if let Some(destination) = operation.move_to {
                    self.move_file(&operation.path, &destination, operation.line)?;
                }
            }
            OperationKind::Delete => {
                if current.kind != EntryKind::Regular {
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

    fn prepare_results(&self) -> Result<Vec<FileResult>, PatchError> {
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

    fn commit(&self) -> Result<(), PatchError> {
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
        let mut backups = Vec::new();
        let mut created_dirs = Vec::new();

        let result = (|| -> Result<(), PatchError> {
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

            for change in &changes {
                if path_exists(&change.path)? {
                    let backup = backup_path(&change.path, &transaction_id)?;
                    fs::rename(&change.path, &backup)?;
                    backups.push((change.path.clone(), backup));
                }
            }

            for (path, temp) in &staged {
                fs::rename(temp, path)?;
            }
            sync_parent_directories(&changes)?;
            Ok(())
        })();

        if result.is_err() {
            for (path, _) in staged.iter().rev() {
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

fn read_snapshot(path: &Path) -> Result<FileSnapshot, PatchError> {
    let metadata = match fs::symlink_metadata(path) {
        Ok(metadata) => metadata,
        Err(error) if error.kind() == io::ErrorKind::NotFound => {
            return Ok(FileSnapshot {
                kind: EntryKind::Missing,
                data: Vec::new(),
                mode: 0,
            });
        }
        Err(error) => return Err(error.into()),
    };
    let mode = metadata.mode();
    if metadata.file_type().is_symlink() {
        return Ok(FileSnapshot {
            kind: EntryKind::Symlink,
            data: Vec::new(),
            mode,
        });
    }
    if metadata.is_dir() {
        return Ok(FileSnapshot {
            kind: EntryKind::Directory,
            data: Vec::new(),
            mode,
        });
    }
    if !metadata.is_file() {
        return Ok(FileSnapshot {
            kind: EntryKind::Other,
            data: Vec::new(),
            mode,
        });
    }
    let data = fs::read(path)?;
    Ok(FileSnapshot {
        kind: EntryKind::Regular,
        data,
        mode,
    })
}

fn parse_create_diff(diff: &str) -> Result<Vec<u8>, PatchError> {
    let lines = normalized_lines(diff);
    let mut output = Vec::with_capacity(diff.len());
    for line in lines {
        if !line.starts_with('+') {
            return Err(PatchError::Invalid(format!(
                "invalid add file line: {line}"
            )));
        }
        output.extend_from_slice(&line.as_bytes()[1..]);
        output.push(b'\n');
    }
    if !output.is_empty() {
        output.pop();
    }
    Ok(output)
}

fn apply_update_diff(input: &[u8], diff: &str) -> Result<Vec<u8>, PatchError> {
    let input = std::str::from_utf8(input)
        .map_err(|_| PatchError::Invalid("update target is not valid UTF-8".to_string()))?;
    let lines = normalized_lines(diff);
    let source: Vec<String> = input.split('\n').map(ToOwned::to_owned).collect();
    let mut chunks = Vec::<DiffChunk>::new();
    let mut index = 0;
    let mut cursor = 0;

    while index < lines.len() {
        let mut anchors = Vec::new();
        let mut anchor_count = 0;
        while index < lines.len() {
            if let Some(anchor) = lines[index].strip_prefix("@@ ") {
                anchor_count += 1;
                if !anchor.trim().is_empty() {
                    anchors.push(anchor.to_string());
                }
                index += 1;
            } else if lines[index] == "@@" {
                anchor_count += 1;
                index += 1;
            } else {
                break;
            }
        }
        if anchor_count == 0 && cursor != 0 {
            return Err(PatchError::Invalid(format!(
                "invalid line: {}",
                lines.get(index).cloned().unwrap_or_default()
            )));
        }
        let require_anchor = anchor_count > 1;
        for (anchor_index, anchor) in anchors.iter().enumerate() {
            if let Some(found) = advance_to_anchor(&source, anchor, cursor, false, anchor_index > 0)
            {
                cursor = found;
            } else if let Some(found) =
                advance_to_anchor(&source, anchor, cursor, true, anchor_index > 0)
            {
                cursor = found;
            } else if require_anchor {
                return Err(PatchError::Invalid(format!(
                    "invalid anchor {cursor}: {anchor}"
                )));
            }
        }

        let (context, section_chunks, next_index, eof) = read_diff_section(&lines, index)?;
        let new_index = find_context(&source, &context, cursor, eof).ok_or_else(|| {
            let label = if eof { "EOF context" } else { "context" };
            PatchError::Invalid(format!("invalid {label} {cursor}: {}", context.join("\n")))
        })?;
        for mut chunk in section_chunks {
            chunk.original_index += new_index;
            chunks.push(chunk);
        }
        cursor = new_index + context.len();
        index = next_index;
    }

    let mut output = Vec::with_capacity(source.len());
    let mut original_index = 0;
    for chunk in chunks {
        if chunk.original_index < original_index || chunk.original_index > source.len() {
            return Err(PatchError::Conflict(format!(
                "overlapping diff chunk at {}",
                chunk.original_index
            )));
        }
        output.extend(source[original_index..chunk.original_index].iter().cloned());
        output.extend(chunk.inserted_lines);
        original_index = chunk.original_index + chunk.deleted_lines.len();
    }
    output.extend(source[original_index..].iter().cloned());
    Ok(output.join("\n").into_bytes())
}

#[derive(Debug)]
struct DiffChunk {
    original_index: usize,
    deleted_lines: Vec<String>,
    inserted_lines: Vec<String>,
}

fn read_diff_section(
    lines: &[String],
    start: usize,
) -> Result<(Vec<String>, Vec<DiffChunk>, usize, bool), PatchError> {
    let mut context = Vec::new();
    let mut deleted = Vec::new();
    let mut inserted = Vec::new();
    let mut chunks = Vec::new();
    let mut mode = ' ';
    let mut index = start;

    let flush = |context: &Vec<String>,
                 deleted: &mut Vec<String>,
                 inserted: &mut Vec<String>,
                 chunks: &mut Vec<DiffChunk>| {
        if deleted.is_empty() && inserted.is_empty() {
            return;
        }
        chunks.push(DiffChunk {
            original_index: context.len() - deleted.len(),
            deleted_lines: std::mem::take(deleted),
            inserted_lines: std::mem::take(inserted),
        });
    };

    while index < lines.len() {
        let raw = &lines[index];
        if raw.starts_with("@@")
            || raw.starts_with(END_MARKER)
            || is_operation_header(raw)
            || raw == END_OF_FILE_MARKER
            || raw == "***"
        {
            break;
        }
        if raw.starts_with("***") {
            return Err(PatchError::Invalid(format!("invalid line: {raw}")));
        }
        index += 1;
        let line = if raw.is_empty() {
            " ".to_string()
        } else {
            raw.clone()
        };
        let next_mode = line.chars().next().unwrap_or(' ');
        if !matches!(next_mode, '+' | '-' | ' ') {
            return Err(PatchError::Invalid(format!("invalid line: {raw}")));
        }
        if next_mode == ' ' && mode != ' ' {
            flush(&context, &mut deleted, &mut inserted, &mut chunks);
        }
        mode = next_mode;
        let value = line[1..].to_string();
        match mode {
            '-' => {
                deleted.push(value.clone());
                context.push(value);
            }
            '+' => inserted.push(value),
            ' ' => context.push(value),
            _ => unreachable!(),
        }
    }
    flush(&context, &mut deleted, &mut inserted, &mut chunks);
    let eof = lines.get(index).map(String::as_str) == Some(END_OF_FILE_MARKER);
    if eof {
        index += 1;
    }
    if index == start {
        return Err(PatchError::Invalid(format!(
            "nothing in diff section at index {index}"
        )));
    }
    Ok((context, chunks, index, eof))
}

fn advance_to_anchor(
    lines: &[String],
    target: &str,
    cursor: usize,
    trimmed: bool,
    force_forward: bool,
) -> Option<usize> {
    if !force_forward
        && lines
            .iter()
            .take(cursor.min(lines.len()))
            .any(|line| line_equal(line, target, trimmed))
    {
        return Some(cursor);
    }
    lines
        .iter()
        .enumerate()
        .skip(cursor)
        .find_map(|(index, line)| line_equal(line, target, trimmed).then_some(index + 1))
}

fn line_equal(left: &str, right: &str, trimmed: bool) -> bool {
    if trimmed {
        left.trim() == right.trim()
    } else {
        left == right
    }
}

fn find_context(lines: &[String], context: &[String], start: usize, eof: bool) -> Option<usize> {
    if context.is_empty() {
        return Some(start.min(lines.len()));
    }
    let preferred_start = if eof {
        lines.len().saturating_sub(context.len())
    } else {
        start
    };
    find_context_variants(lines, context, preferred_start).or_else(|| {
        eof.then(|| find_context_variants(lines, context, start))
            .flatten()
    })
}

fn find_context_variants(lines: &[String], context: &[String], start: usize) -> Option<usize> {
    find_context_with(lines, context, start, |line| line.to_string())
        .or_else(|| find_context_with(lines, context, start, |line| line.trim_end().to_string()))
        .or_else(|| find_context_with(lines, context, start, |line| line.trim().to_string()))
}

fn find_context_with<F>(lines: &[String], context: &[String], start: usize, map: F) -> Option<usize>
where
    F: Fn(&str) -> String,
{
    if context.len() > lines.len() {
        return None;
    }
    (start..=lines.len() - context.len()).find(|&index| {
        context
            .iter()
            .enumerate()
            .all(|(offset, target)| map(&lines[index + offset]) == map(target))
    })
}

fn text_for_diff(file: &FileSnapshot) -> Result<String, PatchError> {
    if file.kind == EntryKind::Missing {
        return Ok(String::new());
    }
    if file.kind != EntryKind::Regular {
        return Err(PatchError::Conflict(
            "diff result contains a non-regular file".to_string(),
        ));
    }
    String::from_utf8(file.data.clone())
        .map_err(|_| PatchError::Invalid("file is not valid UTF-8".to_string()))
}

fn text_for_diff_virtual(file: &VirtualFile) -> Result<String, PatchError> {
    if file.kind == EntryKind::Missing {
        return Ok(String::new());
    }
    if file.kind != EntryKind::Regular {
        return Err(PatchError::Conflict(
            "diff result contains a non-regular file".to_string(),
        ));
    }
    String::from_utf8(file.data.clone())
        .map_err(|_| PatchError::Invalid("file is not valid UTF-8".to_string()))
}

fn unified_diff(
    old: &str,
    new: &str,
    old_header: &str,
    new_header: &str,
) -> (String, usize, usize) {
    let diff = TextDiff::from_lines(old, new);
    let mut unified = diff.unified_diff();
    unified.header(old_header, new_header);
    let rendered = unified.to_string();
    let mut added = 0;
    let mut removed = 0;
    for change in diff.iter_all_changes() {
        match change.tag() {
            ChangeTag::Insert => added += 1,
            ChangeTag::Delete => removed += 1,
            ChangeTag::Equal => {}
        }
    }
    (rendered, added, removed)
}

fn relative_display(cwd: &Path, path: &Path) -> String {
    path.strip_prefix(cwd).unwrap_or(path).display().to_string()
}

fn path_exists(path: &Path) -> Result<bool, PatchError> {
    match fs::symlink_metadata(path) {
        Ok(_) => Ok(true),
        Err(error) if error.kind() == io::ErrorKind::NotFound => Ok(false),
        Err(error) => Err(error.into()),
    }
}

fn unique_path(candidate: &Path) -> Result<PathBuf, PatchError> {
    if !path_exists(candidate)? {
        return Ok(candidate.to_path_buf());
    }
    for suffix in 1..1000 {
        let mut path = candidate.to_path_buf();
        path.set_extension(format!("stage-{suffix}"));
        if !path_exists(&path)? {
            return Ok(path);
        }
    }
    Err(PatchError::Conflict(format!(
        "cannot allocate temporary path near {}",
        candidate.display()
    )))
}

fn backup_path(path: &Path, transaction_id: &str) -> Result<PathBuf, PatchError> {
    let parent = path.parent().unwrap_or_else(|| Path::new("."));
    let file_name = path
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or("file");
    unique_path(&parent.join(format!(".{file_name}.{transaction_id}.backup")))
}

fn create_missing_dirs(path: &Path) -> Result<Vec<PathBuf>, PatchError> {
    let mut missing = Vec::new();
    let mut current = path.to_path_buf();
    while !path_exists(&current)? {
        missing.push(current.clone());
        if !current.pop() {
            break;
        }
    }
    fs::create_dir_all(path)?;
    Ok(missing)
}

#[cfg(unix)]
fn set_mode(file: &File, mode: u32) -> Result<(), PatchError> {
    use std::os::unix::fs::PermissionsExt;
    file.set_permissions(fs::Permissions::from_mode(mode & 0o7777))?;
    Ok(())
}

#[cfg(not(unix))]
fn set_mode(_file: &File, _mode: u32) -> Result<(), PatchError> {
    Ok(())
}

#[cfg(unix)]
trait MetadataMode {
    fn mode(&self) -> u32;
}

#[cfg(unix)]
impl MetadataMode for fs::Metadata {
    fn mode(&self) -> u32 {
        use std::os::unix::fs::PermissionsExt;
        self.permissions().mode()
    }
}

#[cfg(not(unix))]
trait MetadataMode {
    fn mode(&self) -> u32;
}

#[cfg(not(unix))]
impl MetadataMode for fs::Metadata {
    fn mode(&self) -> u32 {
        0o644
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn temp_dir(name: &str) -> PathBuf {
        let suffix = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        let path = std::env::temp_dir().join(format!("agenty-patch-applier-{name}-{suffix}"));
        fs::create_dir_all(&path).unwrap();
        path
    }

    #[test]
    fn applies_repeated_operations_in_file_order_and_reports_diff() {
        let cwd = temp_dir("ordered");
        let patch = "*** Begin Patch\n*** Add File: notes.txt\n+one\n*** Update File: notes.txt\n@@\n-one\n+two\n*** Update File: notes.txt\n@@\n-two\n+three\n*** End Patch";
        let result = apply_patch(&cwd, patch).unwrap();
        assert_eq!(fs::read_to_string(cwd.join("notes.txt")).unwrap(), "three");
        assert_eq!(result.files.len(), 1);
        assert_eq!(result.files[0].added_lines, 1);
        assert_eq!(result.files[0].removed_lines, 0);
        assert!(result.files[0].diff.contains("+three"));
    }

    #[test]
    fn rejects_conflict_without_writing_any_file() {
        let cwd = temp_dir("conflict");
        let patch = "*** Begin Patch\n*** Add File: created.txt\n+created\n*** Update File: missing.txt\n@@\n-missing\n+updated\n*** End Patch";
        let error = apply_patch(&cwd, patch).unwrap_err().to_string();
        assert!(error.contains("updates a non-existent"));
        assert!(!cwd.join("created.txt").exists());
    }

    #[test]
    fn moves_then_updates_destination_as_one_file_group() {
        let cwd = temp_dir("move");
        fs::write(cwd.join("old.txt"), "one\n").unwrap();
        let patch = "*** Begin Patch\n*** Update File: old.txt\n*** Move to: new.txt\n@@\n-one\n+two\n*** Update File: new.txt\n@@\n-two\n+three\n*** End Patch";
        apply_patch(&cwd, patch).unwrap();
        assert!(!cwd.join("old.txt").exists());
        assert_eq!(fs::read_to_string(cwd.join("new.txt")).unwrap(), "three\n");
    }

    #[test]
    fn malformed_patch_has_no_side_effect() {
        let cwd = temp_dir("malformed");
        let error = apply_patch(&cwd, "*** Begin Patch\n*** Add File: x.txt\n+x").unwrap_err();
        assert!(error.to_string().contains("must end"));
        assert!(!cwd.join("x.txt").exists());
    }

    #[test]
    fn accepts_trailing_newline_and_reports_update_counts() {
        let cwd = temp_dir("trailing-newline");
        fs::write(cwd.join("notes.txt"), "one\ntwo\nthree\n").unwrap();
        let patch = "*** Begin Patch\n*** Update File: notes.txt\n@@\n-one\n-two\n+alpha\n three\n*** End Patch\n";
        let result = apply_patch(&cwd, patch).unwrap();
        assert_eq!(
            fs::read_to_string(cwd.join("notes.txt")).unwrap(),
            "alpha\nthree\n"
        );
        assert_eq!(result.files[0].added_lines, 1);
        assert_eq!(result.files[0].removed_lines, 2);
        assert!(result.files[0].diff.contains("-one"));
        assert!(result.files[0].diff.contains("-two"));
        assert!(result.files[0].diff.contains("+alpha"));
    }

    #[test]
    fn applies_multiple_hunks_with_reused_anchor() {
        let cwd = temp_dir("multi-hunk");
        fs::write(cwd.join("notes.txt"), "header\none\nmiddle\ntwo\nfooter\n").unwrap();
        let patch = "*** Begin Patch\n*** Update File: notes.txt\n@@ header\n-one\n+first\n@@ middle\n-two\n+second\n*** End Patch";
        apply_patch(&cwd, patch).unwrap();
        assert_eq!(
            fs::read_to_string(cwd.join("notes.txt")).unwrap(),
            "header\nfirst\nmiddle\nsecond\nfooter\n"
        );
    }

    #[test]
    fn applies_end_of_file_section() {
        let cwd = temp_dir("eof");
        fs::write(cwd.join("notes.txt"), "one\ntwo\nthree").unwrap();
        let patch = "*** Begin Patch\n*** Update File: notes.txt\n@@\n two\n-three\n+final\n*** End of File\n*** End Patch";
        apply_patch(&cwd, patch).unwrap();
        assert_eq!(
            fs::read_to_string(cwd.join("notes.txt")).unwrap(),
            "one\ntwo\nfinal"
        );
    }

    #[test]
    fn detects_cross_file_move_conflict_before_writing() {
        let cwd = temp_dir("move-conflict");
        let patch = "*** Begin Patch\n*** Add File: first.txt\n+first\n*** Add File: second.txt\n+second\n*** Update File: first.txt\n*** Move to: second.txt\n@@\n-first\n+updated\n*** End Patch";
        let error = apply_patch(&cwd, patch).unwrap_err().to_string();
        assert!(error.contains("owned by another file"));
        assert!(!cwd.join("first.txt").exists());
        assert!(!cwd.join("second.txt").exists());
    }

    #[test]
    fn rejects_duplicate_create_before_writing_other_files() {
        let cwd = temp_dir("duplicate-create");
        let patch = "*** Begin Patch\n*** Add File: first.txt\n+first\n*** Add File: second.txt\n+second\n*** Add File: first.txt\n+again\n*** End Patch";
        let error = apply_patch(&cwd, patch).unwrap_err().to_string();
        assert!(error.contains("creates an existing"));
        assert!(!cwd.join("first.txt").exists());
        assert!(!cwd.join("second.txt").exists());
    }

    #[test]
    fn refuses_to_overwrite_file_changed_during_preparation() {
        let cwd = temp_dir("concurrent-change");
        fs::write(cwd.join("notes.txt"), "one").unwrap();
        let patch = "*** Begin Patch\n*** Update File: notes.txt\n@@\n-one\n+two\n*** End Patch";
        let operations = parse_envelope(&cwd, patch).unwrap();
        let mut transaction = Transaction::new(cwd.clone());
        for operation in operations {
            transaction.apply(operation).unwrap();
        }

        fs::write(cwd.join("notes.txt"), "external").unwrap();
        let error = transaction.commit().unwrap_err().to_string();
        assert!(error.contains("changed while the patch was being prepared"));
        assert_eq!(
            fs::read_to_string(cwd.join("notes.txt")).unwrap(),
            "external"
        );
    }

    #[test]
    fn preserves_v4a_diff_compatibility() {
        let create_cases = [
            ("+hello\n+world\n+", "hello\nworld\n"),
            ("+hello\r\n+\r\n+world\r\n", "hello\n\nworld"),
            ("", ""),
        ];
        for (diff, expected) in create_cases {
            assert_eq!(
                String::from_utf8(parse_create_diff(diff).unwrap()).unwrap(),
                expected
            );
        }

        let update_cases = [
            ("", "@@\n+hello\n+world", "hello\nworld\n"),
            (
                "- Milk\n- Bread\n- Eggs\n- Apples\n- Coffee",
                "@@\n - Milk\n - Bread\n - Eggs\n-- Apples\n-- Coffee\n+- [x] Apples\n+- [x] Coffee",
                "- Milk\n- Bread\n- Eggs\n- [x] Apples\n- [x] Coffee",
            ),
            (
                "class First\n    def target():\n        pass\n\nclass Second\n    def target():\n        pass\n",
                "@@ class Second\n@@     def target():\n-        pass\n+        return 1",
                "class First\n    def target():\n        pass\n\nclass Second\n    def target():\n        return 1\n",
            ),
            (
                "  class Target  \n    def first():\n        pass\n\n    def second():\n        pass\n",
                "@@ class Target\n@@     def first():\n-        pass\n+        return 1\n@@ class Target\n@@     def second():\n-        pass\n+        return 2",
                "  class Target  \n    def first():\n        return 1\n\n    def second():\n        return 2\n",
            ),
            ("one   \ntwo\n", " one\n-two\n+second", "one   \nsecond\n"),
            ("one\ntwo\n", " one\r\n-two\r\n+second\r\n", "one\nsecond\n"),
            (
                "target\nmiddle\nend",
                " target\n+after\n*** End of File",
                "target\nafter\nmiddle\nend",
            ),
        ];
        for (input, diff, expected) in update_cases {
            assert_eq!(
                String::from_utf8(apply_update_diff(input.as_bytes(), diff).unwrap()).unwrap(),
                expected
            );
        }
    }

    #[test]
    fn rejects_invalid_v4a_diff_sections() {
        let cases = [
            ("one\ntwo\n", " missing\n-two\n+second", "invalid context"),
            (
                "class Wrong\n    def desired():\n        pass\n",
                "@@ class Target\n@@     def desired():\n-        pass\n+        return 1",
                "invalid anchor",
            ),
            ("one\n", "one", "invalid line"),
            ("one\n", "*** Unknown Directive", "invalid line"),
            ("one\n", "@@", "nothing in diff section"),
            (
                "one\ntwo",
                " missing\n*** End of File",
                "invalid EOF context",
            ),
        ];
        for (input, diff, expected) in cases {
            let error = apply_update_diff(input.as_bytes(), diff)
                .unwrap_err()
                .to_string();
            assert!(
                error.contains(expected),
                "error {error:?} does not contain {expected:?}"
            );
        }
        let error = parse_create_diff("+valid\ninvalid")
            .unwrap_err()
            .to_string();
        assert!(error.contains("invalid add file line"));
    }

    #[test]
    fn lexical_normalization_preserves_absolute_roots() {
        let root = Path::new(std::path::MAIN_SEPARATOR_STR);
        let nested = root.join("tmp").join("..").join("file.txt");
        assert_eq!(lexical_normalize(&nested).unwrap(), root.join("file.txt"));

        let escaping = root.join("..").join("file.txt");
        assert!(lexical_normalize(&escaping).is_err());
    }
}
