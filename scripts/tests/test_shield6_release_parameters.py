import hashlib
import importlib.util
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location(
    "shield6_parameters", Path(__file__).resolve().parents[1] / "shield6-release-parameters.py"
)
parameters = importlib.util.module_from_spec(spec)
spec.loader.exec_module(parameters)


class ParameterDownloadTest(unittest.TestCase):
    def metadata(self):
        payload = b"public parameters"
        return {
            "relationVersion": 2, "chainId": 8979,
            "sourceURL": "https://example.invalid/parameters",
            "files": {name: {"size": len(payload), "sha256": hashlib.sha256(payload).hexdigest()}
                      for name in parameters.NAMES},
        }

    def response(self, payload):
        response = io.BytesIO(payload)
        response.url = "https://example.invalid/parameters/file"
        return response

    def test_authenticated_download_and_cache_tamper(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            with patch.object(parameters.urllib.request, "urlopen",
                              side_effect=lambda *args, **kwargs: self.response(b"public parameters")):
                parameters.fetch(self.metadata(), output)
            with patch.object(parameters.urllib.request, "urlopen", side_effect=AssertionError("cache fetched")):
                parameters.fetch(self.metadata(), output)
            (output / "recipient.pk").write_bytes(b"tampered")
            with self.assertRaisesRegex(ValueError, "differs from pin"):
                parameters.fetch(self.metadata(), output)

    def test_invalid_download_never_published(self):
        for payload in [b"public parameterX", b"short", b"public parameters extra"]:
            with self.subTest(payload=payload), tempfile.TemporaryDirectory() as directory:
                with patch.object(parameters.urllib.request, "urlopen",
                                  side_effect=lambda *args, **kwargs: self.response(payload)):
                    with self.assertRaises(ValueError):
                        parameters.fetch(self.metadata(), Path(directory))
                self.assertEqual(list(Path(directory).iterdir()), [])


if __name__ == "__main__":
    unittest.main()
