import concurrent.futures
import hashlib
import importlib.metadata
import json
import os
from pathlib import Path
import sys
import threading
import uuid

MAX_FRAME = 16 * 1024 * 1024
MAX_TOKENS = 131072


class Worker:
    def __init__(self, output):
        self.output = output
        self.write_lock = threading.Lock()
        self.state_lock = threading.Lock()
        self.pool = concurrent.futures.ThreadPoolExecutor(max_workers=1)
        self.active = None
        self.cancelled = threading.Event()
        self.model = None
        self.handle = None
        self.resource = None
        self.config = None
        self.initialized = False
        self.limit = MAX_FRAME

    def send(self, request_id, kind="response", **fields):
        data = json.dumps(dict(version=1, type=kind, requestId=request_id, **fields),
                          ensure_ascii=False, separators=(",", ":")) + "\n"
        if len(data.encode("utf-8")) > self.limit:
            raise ValueError("worker output frame limit exceeded")
        with self.write_lock:
            self.output.write(data)
            self.output.flush()

    def probe(self, resource):
        from mlx_lm.utils import _get_classes
        if resource["backend"] != "mlx" or resource["format"] != "safetensors":
            raise ValueError("MLX worker requires a Safetensors resource")
        directory = Path(resource["weightsPath"])
        config = json.loads((directory / "config.json").read_text())
        _get_classes(config)
        files = list(directory.glob("model*.safetensors"))
        if not files:
            raise ValueError("no model*.safetensors weights found")
        index = directory / "model.safetensors.index.json"
        if index.exists():
            weight_map = json.loads(index.read_text())["weight_map"]
            for name in set(weight_map.values()):
                if Path(name).name != name or not (directory / name).is_file():
                    raise ValueError("missing or invalid weight shard: " + name)
        for file in files:
            with file.open("rb") as weights:
                header_size = int.from_bytes(weights.read(8), "little")
                size = file.stat().st_size
                if not 0 < header_size <= min(size - 8, 100 * 1024 * 1024):
                    raise ValueError("invalid Safetensors header: " + file.name)
                header = json.loads(weights.read(header_size))
                for name, tensor in header.items():
                    if name == "__metadata__":
                        continue
                    start, end = tensor["data_offsets"]
                    if not 0 <= start <= end <= size - 8 - header_size:
                        raise ValueError("truncated Safetensors tensor: " + name)
        codec_dir = Path(resource["codecDirectory"])
        digest = hashlib.sha256((codec_dir / "tokenizer.json").read_bytes()).hexdigest()
        if digest != resource["tokenizerSha256"]:
            raise ValueError("tokenizer fingerprint mismatch")
        # Raw token IDs require the checkpoint's tokenizer to match the codec.
        checkpoint_digest = hashlib.sha256((directory / "tokenizer.json").read_bytes()).hexdigest()
        if checkpoint_digest != digest:
            raise ValueError("weight directory and codec tokenizer differ")
        return config

    def run_operation(self, request_id, method, params):
        sequence = 0
        try:
            if method == "probeModel":
                try:
                    config = self.probe(params)
                    result = dict(compatible=True, modelType=config["model_type"])
                except (ValueError, OSError, KeyError) as error:
                    result = dict(compatible=False, reason=str(error))
            elif method == "loadModel":
                self.config = self.probe(params)
                from mlx_lm.utils import load_model
                import mlx.core as mx
                self.model, self.config = load_model(Path(params["weightsPath"]), lazy=False, strict=True)
                text_config = self.config.get("text_config", self.config)
                # Force a minimal evaluation before announcing model readiness.
                mx.eval(self.model(mx.array([[0]])))
                self.handle = str(uuid.uuid4())
                self.resource = params
                result = dict(modelHandle=self.handle,
                              contextWindow=text_config["max_position_embeddings"],
                              vocabSize=text_config["vocab_size"],
                              tokenizerSha256=params["tokenizerSha256"])
            elif method == "unloadModel":
                if params["modelHandle"] != self.handle:
                    raise ValueError("unknown model handle")
                self.unload()
                result = {}
            elif method == "generate":
                if self.model is None or params["modelHandle"] != self.handle:
                    raise ValueError("unknown model handle")
                import mlx.core as mx
                from mlx_lm.generate import generate_step
                from mlx_lm.sample_utils import make_sampler
                ids = params["tokenIds"]
                budget = params["maxOutputTokens"]
                config = self.config.get("text_config", self.config)
                if not ids or len(ids) > MAX_TOKENS or not 0 < budget <= MAX_TOKENS:
                    raise ValueError("invalid prompt or output budget")
                if len(ids) + budget > config["max_position_embeddings"]:
                    raise ValueError("context capacity exceeded")
                if any(type(token) is not int or token < 0 or token >= config["vocab_size"] for token in ids):
                    raise ValueError("input token out of vocabulary range")
                stops = set(params["stopTokenIds"])
                eos = self.config.get("eos_token_id", config.get("eos_token_id", []))
                stops.update(eos if isinstance(eos, list) else [eos])

                def progress(done, total):
                    if self.cancelled.is_set():
                        raise InterruptedError("generation cancelled during prefill")

                generator = generate_step(mx.array(ids), self.model, max_tokens=budget,
                                          sampler=make_sampler(temp=0.7, top_p=0.95),
                                          prefill_step_size=256, prompt_progress_callback=progress)
                count = 0
                reason = "maxTokens"
                try:
                    if self.cancelled.is_set():
                        reason = "cancelled"
                    else:
                        for token, _ in generator:
                            if self.cancelled.is_set():
                                reason = "cancelled"
                                break
                            sequence += 1
                            count += 1
                            self.send(request_id, "event", sequence=sequence, event="tokens",
                                      data=dict(tokenIds=[int(token)]))
                            if int(token) in stops:
                                reason = "eos"
                                break
                except InterruptedError:
                    reason = "cancelled"
                finally:
                    generator.close()
                    mx.synchronize()
                result = dict(finishReason=reason,
                              usage=dict(inputTokens=len(ids), outputTokens=count))
            else:
                raise ValueError("unknown operation: " + method)
            fields = dict(result=result)
            if method == "generate":
                fields["sequence"] = sequence + 1
            with self.state_lock:
                if self.active == request_id:
                    self.active = None
            self.send(request_id, **fields)
        except Exception as error:
            fields = dict(error=dict(code="worker_error", message=str(error)))
            if method == "generate":
                fields["sequence"] = sequence + 1
            if method == "loadModel":
                self.unload()
            with self.state_lock:
                if self.active == request_id:
                    self.active = None
            self.send(request_id, **fields)
        finally:
            with self.state_lock:
                if self.active == request_id:
                    self.active = None

    def unload(self):
        self.model = None
        self.handle = None
        self.resource = None
        import mlx.core as mx
        mx.clear_cache()

    def serve(self, source):
        try:
            while True:
                line = source.readline(self.limit + 1)
                if not line:
                    break
                if len(line) > self.limit or not line.endswith(b"\n"):
                    raise ValueError("invalid or oversized NDJSON frame")
                frame = json.loads(line)
                if frame.get("version") != 1 or frame.get("type") != "request" or not frame.get("requestId"):
                    raise ValueError("invalid request envelope")
                request_id = frame["requestId"]
                method, params = frame["method"], frame.get("params", {})
                if method == "initialize":
                    if self.initialized or params.get("protocolVersion") != 1:
                        raise ValueError("invalid initialization")
                    self.limit = min(MAX_FRAME, params["maxFrameBytes"])
                    if self.limit <= 0:
                        raise ValueError("invalid frame limit")
                    import mlx.core  # Verify Metal access before declaring ready.
                    self.initialized = True
                    self.send(request_id, result=dict(protocolVersion=1, backend="mlx",
                              engineVersion=importlib.metadata.version("mlx-lm"), maxFrameBytes=self.limit,
                              maxInputTokens=MAX_TOKENS, maxOutputTokens=MAX_TOKENS,
                              maxConcurrentGenerations=1, supportsJsonSchema=False))
                elif not self.initialized:
                    raise ValueError("initialize must be called first")
                elif method == "cancel":
                    with self.state_lock:
                        accepted = params["requestId"] == self.active
                        if accepted:
                            self.cancelled.set()
                    self.send(request_id, result=dict(accepted=accepted))
                elif method == "shutdown":
                    self.cancelled.set()
                    self.send(request_id, result={})
                    break
                else:
                    with self.state_lock:
                        if self.active is not None:
                            self.send(request_id, error=dict(code="worker_busy", message="one operation at a time"))
                            continue
                        self.active = request_id
                        self.cancelled.clear()
                    self.pool.submit(self.run_operation, request_id, method, params)
        finally:
            self.cancelled.set()
            self.pool.shutdown(wait=True, cancel_futures=True)
            if self.model is not None:
                self.unload()


def main():
    # Preserve the protocol descriptor before redirecting native/library stdout.
    output = os.fdopen(os.dup(sys.stdout.fileno()), "w", encoding="utf-8", buffering=1)
    os.dup2(sys.stderr.fileno(), sys.stdout.fileno())
    try:
        Worker(output).serve(sys.stdin.buffer)
    except Exception as error:
        print("mlx-worker: " + str(error), file=sys.stderr)
        return 1
    finally:
        output.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
