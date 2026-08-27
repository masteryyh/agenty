# patch-applier

`patch-applier` builds the `apply_patch` executable bundled with Agenty. It reads one
complete V4A patch envelope from stdin and resolves relative paths from its working
directory.

Before writing, it parses every operation, groups operations by logical file in first
appearance order, applies each group's operations in source order to an in-memory
snapshot, and rejects incompatible state transitions or path ownership conflicts. The
commit phase stages all new contents before replacing or deleting targets and rolls back
completed replacements if a later filesystem operation fails.

Filesystem coordination uses persistent lock files under `AGENTY_DATA_DIR/locks`. Each patch
holds shared advisory locks while reading and preparing its snapshot, then acquires exclusive
locks for the paths it will change before revalidating and committing. Lock files are coordination
artifacts and are intentionally retained after the process exits. Paths are acquired in sorted
order, and conflicting acquisitions wait until the existing holder releases its lock. The
operating system releases the advisory lock when the owning `File` handle closes.

Success and failure both write one JSON object to stdout. A successful result includes
the final unified diff and added/removed line counts for every changed path:

```json
{
  "success": true,
  "cwd": "/workspace",
  "files": [
    {
      "path": "src/main.rs",
      "diff": "--- a/src/main.rs\n+++ b/src/main.rs\n...",
      "addedLines": 2,
      "removedLines": 1
    }
  ]
}
```

Run `cargo test`, `cargo fmt --all -- --check`, and
`cargo clippy --all-targets --all-features -- -D warnings` before release.
