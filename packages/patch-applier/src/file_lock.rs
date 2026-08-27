use std::fs::{self, File, OpenOptions};
use std::io;
use std::path::{Path, PathBuf};

use sha3::{Digest, Sha3_256};

use super::{FileGroup, PatchError};

#[derive(Debug, Clone, Copy)]
pub(super) enum LockMode {
    Shared,
    Exclusive,
}

pub(super) struct FileLocks {
    locks: Vec<FileLock>,
}

impl FileLocks {
    pub(super) fn acquire(
        lock_directory: &Path,
        targets: &[PathBuf],
        mode: LockMode,
    ) -> Result<Self, PatchError> {
        fs::create_dir_all(lock_directory)?;

        let mut targets = targets.to_vec();
        targets.sort();
        targets.dedup();

        let mut locks = Self {
            locks: Vec::with_capacity(targets.len()),
        };
        for target in targets {
            let lock_path = lock_path(lock_directory, &target);
            let lock = OpenOptions::new()
                .read(true)
                .write(true)
                .create(true)
                .truncate(false)
                .open(&lock_path)
                .map_err(|error| lock_error(&target, "open", error))?;
            let result = match mode {
                LockMode::Shared => lock.lock_shared(),
                LockMode::Exclusive => lock.lock(),
            };
            result.map_err(|error| lock_error(&target, "acquire", error))?;
            locks.locks.push(FileLock {
                path: lock_path,
                file: lock,
            });
        }

        Ok(locks)
    }

    pub(super) fn release(self) -> Result<(), PatchError> {
        let mut first_error = None;
        for lock in self.locks.iter().rev() {
            if let Err(error) = lock.file.unlock() {
                first_error.get_or_insert(lock_error(&lock.path, "release", error));
            }
        }
        first_error.map_or(Ok(()), Err)
    }
}

struct FileLock {
    path: PathBuf,
    file: File,
}

fn lock_error(path: &Path, action: &str, error: io::Error) -> PatchError {
    PatchError::Io(io::Error::new(
        error.kind(),
        format!("{action} lock for {}: {error}", path.display()),
    ))
}

pub(super) fn operation_lock_paths(groups: &[FileGroup]) -> Vec<PathBuf> {
    groups
        .iter()
        .flat_map(|group| {
            group.operations.iter().flat_map(|operation| {
                std::iter::once(operation.path.clone()).chain(operation.move_to.clone())
            })
        })
        .collect()
}

pub(super) fn lock_directory() -> Result<PathBuf, PatchError> {
    let data_directory = std::env::var_os("AGENTY_DATA_DIR")
        .filter(|directory| !directory.is_empty())
        .ok_or_else(|| {
            PatchError::Invalid("AGENTY_DATA_DIR is required for apply_patch locks".to_string())
        })?;
    let data_directory = PathBuf::from(data_directory);
    if !data_directory.is_absolute() {
        return Err(PatchError::Invalid(
            "AGENTY_DATA_DIR must be an absolute path for apply_patch locks".to_string(),
        ));
    }
    Ok(data_directory.join("locks"))
}

pub(super) fn lock_path(lock_directory: &Path, target: &Path) -> PathBuf {
    let mut digest = Sha3_256::new();
    digest.update(target.as_os_str().as_encoded_bytes());
    let digest = digest.finalize();
    let mut name = String::with_capacity(digest.len() * 2 + ".lock".len());
    for byte in digest {
        const HEX: &[u8; 16] = b"0123456789abcdef";
        name.push(HEX[(byte >> 4) as usize] as char);
        name.push(HEX[(byte & 0x0f) as usize] as char);
    }
    name.push_str(".lock");
    lock_directory.join(name)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn temp_lock_root(name: &str) -> PathBuf {
        let suffix = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system clock must follow the Unix epoch")
            .as_nanos();
        let root = std::env::temp_dir().join(format!("agenty-patch-lock-{name}-{suffix}"));
        fs::create_dir_all(&root).expect("create lock test directory");
        root
    }

    #[test]
    fn holds_an_exclusive_advisory_lock_in_a_persistent_file() {
        let root = temp_lock_root("exclusive");
        let target = root.join("notes.txt");
        let lock_directory = root.join("locks");
        let locks = FileLocks::acquire(
            &lock_directory,
            std::slice::from_ref(&target),
            LockMode::Exclusive,
        )
        .unwrap();
        let lock = lock_path(&lock_directory, &target);
        assert!(lock.exists());

        let probe = OpenOptions::new()
            .read(true)
            .write(true)
            .open(&lock)
            .unwrap();
        assert!(matches!(
            probe.try_lock(),
            Err(std::fs::TryLockError::WouldBlock)
        ));

        locks.release().unwrap();
        assert!(lock.exists());
        probe.try_lock().unwrap();
        probe.unlock().unwrap();
    }

    #[test]
    fn shared_locks_can_coexist_but_exclude_writers() {
        let root = temp_lock_root("shared");
        let target = root.join("notes.txt");
        let lock_directory = root.join("locks");
        let shared_a = FileLocks::acquire(
            &lock_directory,
            std::slice::from_ref(&target),
            LockMode::Shared,
        )
        .unwrap();
        let shared_b = FileLocks::acquire(
            &lock_directory,
            std::slice::from_ref(&target),
            LockMode::Shared,
        )
        .unwrap();
        let lock = lock_path(&lock_directory, &target);
        let probe = OpenOptions::new()
            .read(true)
            .write(true)
            .open(&lock)
            .unwrap();

        assert!(matches!(
            probe.try_lock(),
            Err(std::fs::TryLockError::WouldBlock)
        ));
        shared_a.release().unwrap();
        shared_b.release().unwrap();

        probe.try_lock().unwrap();
        probe.unlock().unwrap();

        let exclusive = FileLocks::acquire(
            &lock_directory,
            std::slice::from_ref(&target),
            LockMode::Exclusive,
        )
        .unwrap();
        assert!(matches!(
            probe.try_lock_shared(),
            Err(std::fs::TryLockError::WouldBlock)
        ));
        exclusive.release().unwrap();
        probe.try_lock_shared().unwrap();
        probe.unlock().unwrap();
    }
}
