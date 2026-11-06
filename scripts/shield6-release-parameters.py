#!/usr/bin/env python3
"""Fetch public Shield6 parameters, authenticating every byte before publication."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import tempfile
import urllib.request


NAMES = {
    "recipient.r1cs", "recipient.pk", "recipient.vk", "manifest.json",
    "registration.r1cs", "registration.pk", "registration.vk",
    "registration-manifest.json", "recipient-provenance.json",
    "registration-provenance.json",
}


def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def fetch(metadata, output):
    if metadata["relationVersion"] != 2 or metadata["chainId"] != 8979:
        raise ValueError("unexpected parameter relation or network")
    files = metadata["files"]
    if set(files) != NAMES:
        raise ValueError("parameter file allowlist mismatch")
    source = metadata["sourceURL"].rstrip("/")
    if not source.startswith("https://"):
        raise ValueError("parameter source must use HTTPS")
    output.mkdir(parents=True, exist_ok=True)
    for name, pin in files.items():
        if not re.fullmatch(r"[0-9a-f]{64}", pin["sha256"]):
            raise ValueError("invalid SHA-256 pin")
        if type(pin["size"]) is not int or not 0 < pin["size"] <= 1024**3:
            raise ValueError("invalid artifact size")
        target = output / name
        if target.exists():
            if target.stat().st_size != pin["size"] or digest(target) != pin["sha256"]:
                raise ValueError("existing artifact differs from pin: " + name)
            continue
        temporary = None
        try:
            with tempfile.NamedTemporaryFile(dir=output, delete=False) as stream:
                temporary = Path(stream.name)
                h, size = hashlib.sha256(), 0
                with urllib.request.urlopen(source + "/" + name, timeout=120) as response:
                    if not response.url.startswith("https://"):
                        raise ValueError("insecure parameter redirect")
                    for chunk in iter(lambda: response.read(1024 * 1024), b""):
                        size += len(chunk)
                        if size > pin["size"]:
                            raise ValueError("oversized artifact: " + name)
                        h.update(chunk)
                        stream.write(chunk)
                if size != pin["size"] or h.hexdigest() != pin["sha256"]:
                    raise ValueError("parameter authentication failed: " + name)
                stream.flush()
                os.fsync(stream.fileno())
            os.replace(temporary, target)
        finally:
            if temporary is not None:
                temporary.unlink(missing_ok=True)
        print("verified " + name, flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--metadata", type=Path, default=Path("params/shield6_production.json"))
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    fetch(json.loads(args.metadata.read_text()), args.output)


if __name__ == "__main__":
    main()
