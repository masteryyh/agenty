# llama-cpp-worker

Native C++17 worker for Agenty's local inference provider. CMake pins llama.cpp
and nlohmann/json to commit archives with SHA-256 checksums. It statically links
engine dependencies, embeds the Metal library on macOS, and uses a portable CPU
build on Linux/Windows. macOS uses Metal; supported platforms still require their
normal system libraries. Current inference context capacity is at most 8192.

```bash
pnpm llama-worker:build
pnpm llama-worker:test
pnpm worker:bundle llamaCpp
```

A C/C++ compiler, CMake 3.24+ (or uv for the pinned CMake fallback), and Python 3
for the protocol smoke test are needed. Turbo retains the executable and dependency
licenses in `bin`; ZIP packaging reuses those outputs.
Building downloads pinned source archives; runtime never downloads models.
The output ZIP contains the worker executable and `worker.json`.

GGUF probing checks metadata, architecture, shard availability and tensor extents
without allocating tensor weights. Loading validates all BPE token IDs and added
tokens against the external `tokenizer.json`, then performs a small evaluation.
Non-BPE tokenizer profiles are currently rejected.
