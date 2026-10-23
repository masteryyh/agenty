# mlx-worker

Private Python worker for Agenty's local inference provider. Apple Silicon only;
text generation uses `mlx-lm==0.32.2`. `requirements.lock` pins all runtime
dependencies and hashes. Version 0.31.0 is intentionally excluded because its
upstream release was withdrawn.

```bash
pnpm mlx-worker:build
pnpm mlx-worker:test
pnpm worker:bundle mlx
```

On macOS arm64 the Turbo build prepares standalone CPython 3.12.13, installs
hash-locked dependencies, and writes `bin/mlx-worker` plus its private runtime.
Other targets skip this build. Run `bin/mlx-worker` with stdin/stdout connected
to a protocol client. The launcher resolves its own directory, so the complete
`bin` directory can be relocated. A GUI sandbox without Metal access cannot start
the engine. Runtime imports never download dependencies or models.

ZIP packaging reuses `bin`. To build a ZIP with a different standalone CPython
3.12 distribution, supply `--python-runtime` (not a venv). It contains no model weights.

```bash
pnpm worker:bundle mlx --python-runtime /absolute/path/standalone-cpython
```
