#!/usr/bin/env python3
"""Verify a checksummed console archive and exercise its native executable."""

import argparse
import hashlib
import json
import platform
import re
import socket
import subprocess
import tarfile
import tempfile
import time
import urllib.error
import urllib.request
from pathlib import Path

PLATFORMS = ("linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64")


def read_archive(path, version, target, digest):
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?", version) or target not in PLATFORMS:
        raise ValueError("invalid archive identity")
    if not re.fullmatch(r"[0-9a-f]{64}", digest) or path.is_symlink() or path.stat().st_size > 256 * 1024 * 1024:
        raise ValueError("invalid archive checksum or file")
    if hashlib.sha256(path.read_bytes()).hexdigest() != digest:
        raise ValueError("archive checksum mismatch")
    stem = f"openbao-operator-console_{version}_{target}"
    expected = {stem + "/" + name for name in ("console", "LICENSE", "THIRD_PARTY_LICENSES")}
    contents, seen = {}, set()
    with tarfile.open(path, "r:gz") as archive:
        for member in archive:
            if member.name in seen or len(seen) >= 4:
                raise ValueError("duplicate or excessive archive entries")
            seen.add(member.name)
            if member.isdir() and member.name.rstrip("/") == stem:
                continue
            if member.name not in expected or not member.isfile() or not 0 < member.size <= 128 * 1024 * 1024:
                raise ValueError("unexpected archive entry")
            contents[member.name.rsplit("/", 1)[1]] = archive.extractfile(member).read()
    if set(contents) != {"console", "LICENSE", "THIRD_PARTY_LICENSES"}:
        raise ValueError("incomplete binary archive")
    return contents


def smoke(contents, version, commit):
    with tempfile.TemporaryDirectory(prefix="console-archive-") as scratch:
        root = Path(scratch)
        executable = root / "console"
        executable.write_bytes(contents["console"])
        executable.chmod(0o755)
        result = subprocess.run([str(executable), "--version"], capture_output=True, text=True, check=True, timeout=15)
        if result.stderr or result.stdout.strip() != f"openbao-operator-console {version} (commit {commit})":
            raise ValueError("archive executable has the wrong source identity")
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        address = f"http://127.0.0.1:{port}"
        with (root / "console.log").open("w") as log:
            process = subprocess.Popen([str(executable), "--mode=demo", f"--listen=127.0.0.1:{port}"],
                                       cwd=root, stdout=log, stderr=log)
            try:
                deadline = time.monotonic() + 30
                while time.monotonic() < deadline:
                    if process.poll() is not None:
                        raise ValueError("archive executable stopped before readiness")
                    try:
                        with urllib.request.urlopen(address + "/readyz", timeout=1) as response:
                            if response.status == 204:
                                break
                    except (OSError, urllib.error.URLError):
                        pass
                    time.sleep(0.1)
                else:
                    raise ValueError("archive executable did not become ready")
                with urllib.request.urlopen(address + "/", timeout=5) as response:
                    if not response.headers.get("Content-Security-Policy") or b"<html" not in response.read():
                        raise ValueError("archive is missing its embedded UI or content security policy")
                with urllib.request.urlopen(address + "/api/v1/snapshot", timeout=5) as response:
                    if json.load(response).get("mode") != "demo":
                        raise ValueError("archive smoke test did not use fixture mode")
            finally:
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
        print("Verified native archive identity, embedded UI, CSP, and fixture HTTP startup.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--version", required=True)
    parser.add_argument("--commit", required=True)
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9a-f]{40}", args.commit):
        raise ValueError("expected a full source commit")
    target = platform.system().lower() + "_" + {"aarch64": "arm64", "arm64": "arm64", "x86_64": "amd64"}[platform.machine()]
    checksums = {}
    for line in (args.directory / "checksums.txt").read_text().splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9_.-]+)", line)
        if not match or match[2] in checksums:
            raise ValueError("invalid or duplicate checksum entry")
        checksums[match[2]] = match[1]
    expected = {f"openbao-operator-console_{args.version}_{value}.tar.gz" for value in PLATFORMS}
    if set(checksums) != expected:
        raise ValueError("unexpected archive inventory")
    name = f"openbao-operator-console_{args.version}_{target}.tar.gz"
    contents = read_archive(args.directory / name, args.version, target, checksums[name])
    source = Path(__file__).resolve().parent.parent
    for name in ("LICENSE", "THIRD_PARTY_LICENSES"):
        if contents[name] != (source / name).read_bytes():
            raise ValueError("archive license notices differ from the source checkout")
    smoke(contents, args.version, args.commit)


if __name__ == "__main__":
    main()
