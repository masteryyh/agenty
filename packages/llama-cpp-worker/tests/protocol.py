import json
from pathlib import Path
import subprocess
import sys

binary = Path("bin/llama-cpp-worker" + (".exe" if sys.platform == "win32" else ""))
requests = [dict(version=1, type="request", requestId="1", method="initialize",
                 params=dict(protocolVersion=1, maxFrameBytes=1048576)),
            dict(version=1, type="request", requestId="2", method="shutdown")]
result = subprocess.run([str(binary.resolve())], input="\n".join(map(json.dumps, requests)) + "\n",
                        text=True, capture_output=True, timeout=30, check=True)
frames = list(map(json.loads, result.stdout.splitlines()))
assert [frame["requestId"] for frame in frames] == ["1", "2"], frames
assert frames[0]["result"]["backend"] == "llamaCpp", frames
print("native worker handshake/shutdown passed")
