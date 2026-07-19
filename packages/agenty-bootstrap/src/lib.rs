use std::error::Error;
use std::fmt;
use std::fs::{self, File};
use std::io::{self, Read, Seek, SeekFrom, Write};
use std::path::{Path, PathBuf};

use liblzma::read::XzDecoder;
use sha3::{Digest, Sha3_256};

pub const MAGIC: [u8; 8] = [0xca, 0xfe, 0xba, 0xbe, 0x10, 0x13, 0x66, 0x66];

pub const FORMAT_VERSION: u32 = 1;

pub const FOOTER_SIZE: usize = 108;

const COPY_BUFFER_SIZE: usize = 64 * 1024;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PayloadSpec {
    pub offset: u64,
    pub len: u64,
    pub sha3_256: [u8; 32],
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Footer {
    pub cli: PayloadSpec,
    pub runtime: PayloadSpec,
}

impl Footer {
    pub fn encode(&self) -> [u8; FOOTER_SIZE] {
        let mut out = [0u8; FOOTER_SIZE];
        out[0..8].copy_from_slice(&self.cli.offset.to_le_bytes());
        out[8..16].copy_from_slice(&self.cli.len.to_le_bytes());
        out[16..48].copy_from_slice(&self.cli.sha3_256);
        out[48..56].copy_from_slice(&self.runtime.offset.to_le_bytes());
        out[56..64].copy_from_slice(&self.runtime.len.to_le_bytes());
        out[64..96].copy_from_slice(&self.runtime.sha3_256);
        out[96..100].copy_from_slice(&FORMAT_VERSION.to_le_bytes());
        out[100..108].copy_from_slice(&MAGIC);
        out
    }

    pub fn decode(bytes: &[u8; FOOTER_SIZE]) -> Result<Self> {
        if bytes[100..108] != MAGIC {
            return Err(BootstrapError::CorruptFooter(
                "magic trailer not found; this binary carries no payloads".to_string(),
            ));
        }

        let mut version = [0u8; 4];
        version.copy_from_slice(&bytes[96..100]);
        if u32::from_le_bytes(version) != FORMAT_VERSION {
            return Err(BootstrapError::CorruptFooter(format!(
                "unsupported footer format version {}",
                u32::from_le_bytes(version)
            )));
        }

        let read_u64 = |at: usize| -> u64 {
            let mut v = [0u8; 8];
            v.copy_from_slice(&bytes[at..at + 8]);
            u64::from_le_bytes(v)
        };
        let mut cli_sha = [0u8; 32];
        cli_sha.copy_from_slice(&bytes[16..48]);
        let mut runtime_sha = [0u8; 32];
        runtime_sha.copy_from_slice(&bytes[64..96]);

        Ok(Footer {
            cli: PayloadSpec {
                offset: read_u64(0),
                len: read_u64(8),
                sha3_256: cli_sha,
            },
            runtime: PayloadSpec {
                offset: read_u64(48),
                len: read_u64(56),
                sha3_256: runtime_sha,
            },
        })
    }
}

pub fn read_footer(file: &mut File) -> Result<Footer> {
    let file_len = file.metadata()?.len();
    if file_len < FOOTER_SIZE as u64 {
        return Err(BootstrapError::CorruptFooter(
            "file is smaller than the payload footer".to_string(),
        ));
    }

    file.seek(SeekFrom::End(-(FOOTER_SIZE as i64)))?;
    let mut buf = [0u8; FOOTER_SIZE];
    file.read_exact(&mut buf)?;
    Footer::decode(&buf)
}

pub fn hash_file(path: &Path) -> Result<[u8; 32]> {
    let mut file = File::open(path)?;
    let mut hasher = Sha3_256::new();
    let mut buf = [0u8; COPY_BUFFER_SIZE];

    loop {
        let n = file.read(&mut buf)?;
        if n == 0 {
            break;
        }
        hasher.update(&buf[..n]);
    }
    Ok(hasher.finalize().into())
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EnsureOutcome {
    Reused,
    Installed,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ArtifactIntegrity {
    Missing,
    Valid,
    Invalid,
}

pub fn check_artifact_integrity(spec: &PayloadSpec, target: &Path) -> Result<ArtifactIntegrity> {
    if !target.is_file() {
        return Ok(ArtifactIntegrity::Missing);
    }
    if hash_file(target)? == spec.sha3_256 {
        Ok(ArtifactIntegrity::Valid)
    } else {
        Ok(ArtifactIntegrity::Invalid)
    }
}

pub fn reuse_artifact(target: &Path) -> Result<()> {
    set_executable(target)
}

pub fn ensure_artifact<R: Read + Seek>(
    packed: &mut R,
    spec: &PayloadSpec,
    target: &Path,
) -> Result<EnsureOutcome> {
    if check_artifact_integrity(spec, target)? == ArtifactIntegrity::Valid {
        reuse_artifact(target)?;
        return Ok(EnsureOutcome::Reused);
    }

    install_artifact(packed, spec, target)?;
    Ok(EnsureOutcome::Installed)
}

pub fn install_artifact<R: Read + Seek>(
    packed: &mut R,
    spec: &PayloadSpec,
    target: &Path,
) -> Result<()> {
    packed.seek(SeekFrom::Start(spec.offset))?;
    let section = packed.take(spec.len);
    let mut decoder = XzDecoder::new(section);

    let dir = target.parent().ok_or_else(|| {
        BootstrapError::Invalid(format!("target path has no parent: {}", target.display()))
    })?;
    fs::create_dir_all(dir)?;

    let temp = temp_path_for(target);
    let install = (|| -> Result<()> {
        let mut out = File::create(&temp)?;
        let mut hasher = Sha3_256::new();
        let mut buf = [0u8; COPY_BUFFER_SIZE];

        loop {
            let n = decoder.read(&mut buf).map_err(|e| {
                BootstrapError::Invalid(format!(
                    "failed to decompress embedded payload for {}: {e}",
                    target.display()
                ))
            })?;
            if n == 0 {
                break;
            }
            hasher.update(&buf[..n]);
            out.write_all(&buf[..n])?;
        }
        out.flush()?;

        let actual: [u8; 32] = hasher.finalize().into();
        if actual != spec.sha3_256 {
            return Err(BootstrapError::HashMismatch {
                artifact: target.display().to_string(),
                expected: hex(&spec.sha3_256),
                actual: hex(&actual),
            });
        }

        set_executable(&temp)?;
        fs::rename(&temp, target)?;
        Ok(())
    })();

    if install.is_err() {
        let _ = fs::remove_file(&temp);
    }
    install
}

pub fn managed_bin_dir(home: &Path) -> PathBuf {
    home.join(".agenty").join("bin")
}

pub fn artifact_paths(home: &Path) -> (PathBuf, PathBuf) {
    let dir = managed_bin_dir(home);
    let ext = if cfg!(windows) { ".exe" } else { "" };
    (
        dir.join(format!("cli{ext}")),
        dir.join(format!("runtime{ext}")),
    )
}

fn temp_path_for(target: &Path) -> PathBuf {
    let mut name = target.as_os_str().to_owned();
    name.push(format!(".{}.tmp", std::process::id()));
    PathBuf::from(name)
}

fn hex(bytes: &[u8]) -> String {
    let mut out = String::with_capacity(bytes.len() * 2);
    for b in bytes {
        out.push_str(&format!("{b:02x}"));
    }
    out
}

#[cfg(unix)]
fn set_executable(path: &Path) -> Result<()> {
    use std::os::unix::fs::PermissionsExt;
    fs::set_permissions(path, fs::Permissions::from_mode(0o755))?;
    Ok(())
}

#[cfg(not(unix))]
fn set_executable(_path: &Path) -> Result<()> {
    Ok(())
}

#[derive(Debug)]
pub enum BootstrapError {
    Io(io::Error),
    CorruptFooter(String),
    HashMismatch {
        artifact: String,
        expected: String,
        actual: String,
    },
    Invalid(String),
}

impl fmt::Display for BootstrapError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            BootstrapError::Io(e) => write!(f, "{e}"),
            BootstrapError::CorruptFooter(msg) => write!(f, "corrupt payload footer: {msg}"),
            BootstrapError::HashMismatch {
                artifact,
                expected,
                actual,
            } => write!(
                f,
                "embedded payload digest mismatch for {artifact}: expected {expected}, got {actual}"
            ),
            BootstrapError::Invalid(msg) => write!(f, "{msg}"),
        }
    }
}

impl Error for BootstrapError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        match self {
            BootstrapError::Io(e) => Some(e),
            _ => None,
        }
    }
}

impl From<io::Error> for BootstrapError {
    fn from(e: io::Error) -> Self {
        BootstrapError::Io(e)
    }
}

pub type Result<T> = std::result::Result<T, BootstrapError>;

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Cursor;

    const GOLDEN_FOOTER_HEX: &str =
        "88776655443322110807060504030201000102030405060708090a0b0c0d0e0f\
        101112131415161718191a1b1c1d1e1f1122334455667788010203040506070820212223242526\
        2728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f01000000cafebabe10136666";

    const INTEROP_XZ_HEX: &str = "fd377a585a000004e6d6b4460200210116000000742fe5a3e0087f00355d003099c8db4efc244eb58cf58f4699c115ba2fbad7ad9c231199c49368b315728a5421d1340068b4b68fd6e65bef9dbfedfd1f52190000000000154019c351bd7ee70001518011000000e78fc45db1c467fb020000000004595a";
    const INTEROP_RAW_SHA3: &str =
        "9c1c4966ec1caebac758ba46d39a4a97934b039ff518675baf367f99a070816e";
    const INTEROP_RAW_LEN: usize = 2176;

    fn unhex(s: &str) -> Vec<u8> {
        let s: String = s.chars().filter(|c| !c.is_whitespace()).collect();
        assert!(s.len() % 2 == 0, "odd-length hex string");
        (0..s.len())
            .step_by(2)
            .map(|i| u8::from_str_radix(&s[i..i + 2], 16).expect("invalid hex"))
            .collect()
    }

    fn hex(bytes: &[u8]) -> String {
        bytes.iter().map(|b| format!("{b:02x}")).collect()
    }

    fn golden_footer() -> Footer {
        let mut cli_sha = [0u8; 32];
        for (i, b) in cli_sha.iter_mut().enumerate() {
            *b = i as u8;
        }
        let mut runtime_sha = [0u8; 32];
        for (i, b) in runtime_sha.iter_mut().enumerate() {
            *b = 0x20 + i as u8;
        }
        Footer {
            cli: PayloadSpec {
                offset: 0x1122334455667788,
                len: 0x0102030405060708,
                sha3_256: cli_sha,
            },
            runtime: PayloadSpec {
                offset: 0x8877665544332211,
                len: 0x0807060504030201,
                sha3_256: runtime_sha,
            },
        }
    }

    fn temp_dir(tag: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!(
            "agenty-bootstrap-test-{}-{tag}",
            std::process::id()
        ));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();
        dir
    }

    /// Builds a synthetic packed file: stub bytes + xz(payload) + footer,
    /// returning the file and the spec pointing at the payload section.
    fn synthetic_packed(dir: &Path, payload: &[u8]) -> (File, PayloadSpec) {
        use liblzma::read::XzEncoder;

        let stub = b"fake bootstrap stub bytes";
        let mut compressed = Vec::new();
        XzEncoder::new(Cursor::new(payload), 6)
            .read_to_end(&mut compressed)
            .unwrap();

        let mut sha = Sha3_256::new();
        sha.update(payload);
        let spec = PayloadSpec {
            offset: stub.len() as u64,
            len: compressed.len() as u64,
            sha3_256: sha.finalize().into(),
        };
        let footer = Footer {
            cli: spec.clone(),
            runtime: spec.clone(),
        };

        let path = dir.join("packed");
        let mut file = File::create(&path).unwrap();
        file.write_all(stub).unwrap();
        file.write_all(&compressed).unwrap();
        file.write_all(&footer.encode()).unwrap();
        drop(file);
        (File::open(&path).unwrap(), spec)
    }

    #[test]
    fn footer_roundtrip() {
        let footer = golden_footer();
        assert_eq!(Footer::decode(&footer.encode()).unwrap(), footer);
    }

    #[test]
    fn footer_golden_bytes() {
        let encoded = golden_footer().encode();
        assert_eq!(
            hex(&encoded),
            GOLDEN_FOOTER_HEX.replace([' ', '\t', '\n'], "")
        );
        let golden = unhex(GOLDEN_FOOTER_HEX);
        assert_eq!(golden.len(), FOOTER_SIZE);
        let mut buf = [0u8; FOOTER_SIZE];
        buf.copy_from_slice(&golden);
        assert_eq!(Footer::decode(&buf).unwrap(), golden_footer());
    }

    #[test]
    fn footer_rejects_bad_magic() {
        let mut bytes = golden_footer().encode();
        bytes[107] ^= 0xff;
        let err = Footer::decode(&bytes).unwrap_err();
        assert!(matches!(err, BootstrapError::CorruptFooter(_)));
    }

    #[test]
    fn footer_rejects_unknown_version() {
        let mut bytes = golden_footer().encode();
        bytes[96] = 0x7f;
        let err = Footer::decode(&bytes).unwrap_err();
        assert!(matches!(err, BootstrapError::CorruptFooter(_)));
    }

    #[test]
    fn sha3_256_known_vector() {
        let mut hasher = Sha3_256::new();
        hasher.update(b"abc");
        let digest: [u8; 32] = hasher.finalize().into();
        assert_eq!(
            hex(&digest),
            "3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532"
        );
    }

    #[test]
    fn napi_rs_xz_interop() {
        let compressed = unhex(INTEROP_XZ_HEX);
        let mut decoded = Vec::new();
        XzDecoder::new(Cursor::new(compressed))
            .read_to_end(&mut decoded)
            .unwrap();

        assert_eq!(decoded.len(), INTEROP_RAW_LEN);
        let mut sha = Sha3_256::new();
        sha.update(&decoded);
        let digest: [u8; 32] = sha.finalize().into();
        assert_eq!(hex(&digest), INTEROP_RAW_SHA3);
    }

    #[test]
    fn ensure_installs_missing_artifact() {
        let dir = temp_dir("install");
        let payload = b"fake cli binary contents".repeat(500);
        let (mut packed, spec) = synthetic_packed(&dir, &payload);

        let target = dir.join("bin").join("cli");
        let outcome = ensure_artifact(&mut packed, &spec, &target).unwrap();
        assert_eq!(outcome, EnsureOutcome::Installed);
        assert_eq!(fs::read(&target).unwrap(), payload);
    }

    #[test]
    fn integrity_distinguishes_missing_valid_and_invalid_artifacts() {
        let dir = temp_dir("integrity-states");
        let payload = b"integrity payload".repeat(100);
        let (mut packed, spec) = synthetic_packed(&dir, &payload);
        let target = dir.join("cli");

        assert_eq!(
            check_artifact_integrity(&spec, &target).unwrap(),
            ArtifactIntegrity::Missing
        );
        install_artifact(&mut packed, &spec, &target).unwrap();
        assert_eq!(
            check_artifact_integrity(&spec, &target).unwrap(),
            ArtifactIntegrity::Valid
        );
        fs::write(&target, b"stale contents").unwrap();
        assert_eq!(
            check_artifact_integrity(&spec, &target).unwrap(),
            ArtifactIntegrity::Invalid
        );
    }

    #[cfg(unix)]
    #[test]
    fn reusing_artifact_refreshes_executable_permissions() {
        use std::os::unix::fs::PermissionsExt;

        let dir = temp_dir("reuse-permissions");
        let target = dir.join("cli");
        fs::write(&target, b"verified payload").unwrap();
        fs::set_permissions(&target, fs::Permissions::from_mode(0o644)).unwrap();

        reuse_artifact(&target).unwrap();
        assert_eq!(
            fs::metadata(&target).unwrap().permissions().mode() & 0o111,
            0o111
        );
    }

    #[test]
    fn ensure_reuses_matching_artifact() {
        let dir = temp_dir("reuse");
        let payload = b"same bytes".repeat(200);
        let (mut packed, spec) = synthetic_packed(&dir, &payload);

        let target = dir.join("cli");
        ensure_artifact(&mut packed, &spec, &target).unwrap();
        let before = fs::metadata(&target).unwrap().modified().unwrap();

        let outcome = ensure_artifact(&mut packed, &spec, &target).unwrap();
        assert_eq!(outcome, EnsureOutcome::Reused);
        let after = fs::metadata(&target).unwrap().modified().unwrap();
        assert_eq!(before, after);
        assert_eq!(fs::read(&target).unwrap(), payload);
    }

    #[test]
    fn ensure_replaces_mismatched_artifact() {
        let dir = temp_dir("replace");
        let payload = b"fresh payload".repeat(100);
        let (mut packed, spec) = synthetic_packed(&dir, &payload);

        let target = dir.join("cli");
        fs::write(&target, b"stale contents").unwrap();

        let outcome = ensure_artifact(&mut packed, &spec, &target).unwrap();
        assert_eq!(outcome, EnsureOutcome::Installed);
        assert_eq!(fs::read(&target).unwrap(), payload);
    }

    #[test]
    fn ensure_rejects_corrupt_payload_without_touching_target() {
        let dir = temp_dir("corrupt");
        let payload = b"payload".repeat(100);
        let (mut packed, mut spec) = synthetic_packed(&dir, &payload);
        spec.sha3_256[0] ^= 0xff;

        let target = dir.join("bin").join("cli");
        let err = ensure_artifact(&mut packed, &spec, &target).unwrap_err();
        assert!(matches!(err, BootstrapError::HashMismatch { .. }));
        assert!(!target.exists());
        let leftovers: Vec<_> = fs::read_dir(target.parent().unwrap())
            .map(|rd| rd.filter_map(|e| e.ok()).collect())
            .unwrap_or_default();
        assert!(leftovers.is_empty(), "temporary file was not cleaned up");
    }

    #[test]
    fn read_footer_from_packed_file() {
        let dir = temp_dir("footer");
        let payload = b"content".repeat(50);
        let (mut packed, spec) = synthetic_packed(&dir, &payload);

        let footer = read_footer(&mut packed).unwrap();
        assert_eq!(footer.cli, spec);
        assert_eq!(footer.runtime, spec);
    }

    #[test]
    fn artifact_paths_use_agenty_bin_dir() {
        let home = Path::new("/home/tester");
        let (cli, runtime) = artifact_paths(home);
        let ext = if cfg!(windows) { ".exe" } else { "" };
        assert_eq!(cli, home.join(".agenty/bin").join(format!("cli{ext}")));
        assert_eq!(
            runtime,
            home.join(".agenty/bin").join(format!("runtime{ext}"))
        );
    }
}
