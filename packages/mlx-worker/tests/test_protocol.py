import io
import json
import sys
import types
import unittest
from unittest.mock import patch

from mlx_worker.__main__ import Worker


class ProtocolTests(unittest.TestCase):
    def test_initialization_and_shutdown_are_ndjson(self):
        output = io.StringIO()
        requests = [dict(version=1, type="request", requestId="a", method="initialize",
                         params=dict(protocolVersion=1, maxFrameBytes=1024)),
                    dict(version=1, type="request", requestId="b", method="shutdown")]
        modules = {"mlx": types.ModuleType("mlx"), "mlx.core": types.ModuleType("mlx.core")}
        with patch.dict(sys.modules, modules), patch("importlib.metadata.version", return_value="test"):
            Worker(output).serve(io.BytesIO(("\n".join(map(json.dumps, requests)) + "\n").encode()))
        frames = list(map(json.loads, output.getvalue().splitlines()))
        self.assertEqual([frame["requestId"] for frame in frames], ["a", "b"])
        self.assertEqual(frames[0]["result"]["backend"], "mlx")

    def test_rejects_partial_and_foreign_frames(self):
        for data in [b'{"version":1}', b'{"version":2,"type":"request","requestId":"a"}\n']:
            with self.assertRaises(ValueError):
                Worker(io.StringIO()).serve(io.BytesIO(data))


if __name__ == "__main__":
    unittest.main()
