"""Build relocatable worker ZIPs and release-index entries (no model weights)."""
import argparse
from contextlib import nullcontext
import hashlib
import json
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parent.parent


def prepare_mlx(stage, runtime):
    prefix = runtime.resolve()
    if (prefix / "pyvenv.cfg").exists() or not (prefix / "lib" / "python3.12").is_dir():
        raise ValueError("provide a standalone CPython distribution, not a venv")
    shutil.copytree(prefix, stage / "python", symlinks=False,
                    ignore=shutil.ignore_patterns("site-packages", "__pycache__"))
    python = stage / "python" / "bin" / "python3.12"
    subprocess.run([str(python), "-I", "-c", "import sys; assert sys.version_info[:2] == (3,12)"], check=True)
    subprocess.run(["uv", "pip", "install", "--python", str(python), "--prefix", str(stage / "python"), "--require-hashes",
                    "-r", str(ROOT / "packages/mlx-worker/requirements.lock")], check=True)
    shutil.copytree(ROOT / "packages/mlx-worker/mlx_worker", stage / "mlx_worker",
                    ignore=shutil.ignore_patterns("__pycache__"))
    launcher = stage / "mlx-worker"
    launcher.write_text('#!/bin/sh\nworker_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)\nexec "$worker_dir/python/bin/python3.12" -I "$worker_dir/mlx_worker/__main__.py" "$@"\n')
    launcher.chmod(0o755)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("backend", choices=["mlx", "llamaCpp"])
    parser.add_argument("--python-runtime", type=Path)
    parser.add_argument("--directory", type=Path, help="prepare an unpacked MLX runtime for local builds")
    parser.add_argument("--version", default="0.1.0")
    parser.add_argument("--output", type=Path, default=ROOT / "dist" / "workers")
    args = parser.parse_args()
    if not args.version or any(ch not in "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-" for ch in args.version):
        raise ValueError("version must contain only letters, digits, dot and hyphen")
    os_name = {"Darwin": "darwin", "Linux": "linux", "Windows": "windows"}[platform.system()]
    arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "AMD64": "amd64"}[platform.machine()]
    args.output.mkdir(parents=True, exist_ok=True)
    if args.directory is not None and args.backend != "mlx":
        raise ValueError("--directory is only supported for MLX runtime preparation")
    if args.directory is not None:
        args.directory.mkdir(parents=True, exist_ok=True)
    workspace = nullcontext(args.directory) if args.directory is not None else tempfile.TemporaryDirectory(prefix="worker-bundle-")
    with workspace as temporary:
        stage = Path(temporary)
        shutil.copy2(ROOT / "LICENSE", stage / "LICENSE")
        if args.backend == "mlx":
            if os_name != "darwin" or arch != "arm64":
                raise ValueError("MLX bundles require macOS arm64")
            if args.python_runtime is None:
                built = ROOT / "packages/mlx-worker/bin"
                if not (built / "mlx-worker").is_file():
                    raise ValueError("build mlx-worker first or supply --python-runtime")
                shutil.copytree(built, stage, dirs_exist_ok=True)
            else:
                prepare_mlx(stage, args.python_runtime)
            command = "mlx-worker"
            arguments = []
            worker_id = "mlx"
        else:
            extension = ".exe" if os_name == "windows" else ""
            binary = ROOT / "packages/llama-cpp-worker/bin" / ("llama-cpp-worker" + extension)
            if not binary.is_file():
                raise ValueError("build llama-cpp-worker first")
            (stage / "bin").mkdir()
            shutil.copy2(binary, stage / "bin" / binary.name)
            command = "bin/" + binary.name
            arguments = []
            worker_id = "llama-cpp"
            shutil.copytree(binary.parent / "licenses", stage / "licenses")
        manifest = dict(schemaVersion=1, id=worker_id, backend=args.backend, version=args.version,
                        os=os_name, arch=arch, command=command, arguments=arguments)
        (stage / "worker.json").write_text(json.dumps(manifest, indent=2) + "\n")
        if args.directory is not None:
            print(stage)
            return
        filename = f"{args.backend}-{os_name}-{arch}.zip"
        destination = args.output / filename
        with zipfile.ZipFile(destination, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=6) as archive:
            for file in sorted(stage.rglob("*")):
                if file.is_file():
                    archive.write(file, file.relative_to(stage).as_posix())
        entry = dict(name=worker_id, backend=args.backend, version=args.version, os=os_name, arch=arch,
                     url=f"https://github.com/masteryyh/agenty/releases/download/workers-v{args.version}/{filename}",
                     sha256=hashlib.sha256(destination.read_bytes()).hexdigest())
        (args.output / filename.replace(".zip", ".json")).write_text(json.dumps(entry, indent=2) + "\n")
        print(destination)


if __name__ == "__main__":
    main()
