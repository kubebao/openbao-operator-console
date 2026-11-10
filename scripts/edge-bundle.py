#!/usr/bin/env python3
"""Package and validate the console's edge publication boundary."""

import argparse
import hashlib
import json
import re
import shutil
import subprocess
import tarfile
import tempfile
from pathlib import Path

PLATFORMS = ("linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64")


def identity(repository, sha, run_id, attempt, source_ref):
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/openbao-operator-console", repository):
        raise ValueError("unexpected source repository")
    if not re.fullmatch(r"[0-9a-f]{40}", sha):
        raise ValueError("commit must be a full lowercase Git SHA")
    if not all(re.fullmatch(r"[1-9][0-9]{0,19}", str(value)) for value in (run_id, attempt)):
        raise ValueError("run ID and attempt must be positive integers")
    suffix = f"{run_id}.{attempt}.g{sha[:12]}"
    return {
        "schemaVersion": 1, "repository": repository, "commit": sha, "sourceRef": source_ref,
        "runId": int(run_id), "runAttempt": int(attempt), "version": f"0.0.0-edge.{suffix}",
        # The non-SemVer Git tag keeps edge downloads outside Release Please's tag discovery.
        "releaseTag": f"edge-{suffix}",
    }


def checksum(path):
    with path.open("rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def image_info(path, version, sha):
    with tarfile.open(path) as archive:
        def document(name, digest=None):
            member = archive.getmember(name)
            if not member.isfile() or member.size > 1024 * 1024:
                raise ValueError("invalid OCI metadata member")
            raw = archive.extractfile(member).read()
            if digest and hashlib.sha256(raw).hexdigest() != digest.removeprefix("sha256:"):
                raise ValueError("OCI metadata digest mismatch")
            return json.loads(raw)

        def blob(digest):
            if not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
                raise ValueError("invalid OCI digest")
            return document("blobs/sha256/" + digest[7:], digest)

        index = document("index.json")
        if len(index["manifests"]) != 1:
            raise ValueError("OCI archive must contain one multi-platform image")
        digest = index["manifests"][0]["digest"]
        manifests = blob(digest)["manifests"]
        platforms = {}
        for manifest in manifests:
            platform = manifest["platform"]
            key = platform["os"] + "/" + platform["architecture"]
            if key in platforms or key not in {"linux/amd64", "linux/arm64"}:
                raise ValueError("unexpected or duplicate image platform")
            image = blob(manifest["digest"])
            config = blob(image["config"]["digest"])
            labels = config["config"]["Labels"]
            if (config["os"] != "linux" or config["architecture"] != platform["architecture"]
                    or config["config"]["User"] != "65532:65532"
                    or labels["org.opencontainers.image.version"] != version
                    or labels["org.opencontainers.image.revision"] != sha):
                raise ValueError("image configuration does not match the candidate")
            platforms[key] = manifest["digest"]
        if set(platforms) != {"linux/amd64", "linux/arm64"}:
            raise ValueError("both image platforms are required")
        return {"digest": digest, "platforms": platforms}


def chart_values(text, image, digest):
    match = re.search(r"^image:\n(.*?)(?=^\S|\Z)", text, re.MULTILINE | re.DOTALL)
    if not match:
        raise ValueError("chart image defaults were not found")
    block = match.group(0)
    for name, value in (("repository", image), ("tag", ""), ("digest", digest)):
        block, count = re.subn(r"^  " + name + r":.*$", f'  {name}: "{value}"', block, flags=re.MULTILINE)
        if count != 1:
            raise ValueError(f"expected one image.{name} default")
    return text[:match.start()] + block + text[match.end():]


def package_chart(directory, metadata, image):
    with tempfile.TemporaryDirectory(prefix="console-edge-chart-") as scratch:
        chart = Path(scratch) / "openbao-operator-console"
        shutil.copytree("charts/openbao-operator-console", chart)
        chart_file = chart / "Chart.yaml"
        text = chart_file.read_text()
        # Edge is a separate, unregistered OCI repository. Omit stable Artifact Hub metadata.
        text = re.sub(r"^annotations:\n(?:[ \t].*\n|\n)*", "", text, flags=re.MULTILINE)
        if "artifacthub.io/" in text:
            raise ValueError("unexpected Artifact Hub metadata in edge chart")
        chart_file.write_text(text)
        values = chart / "values.yaml"
        values.write_text(chart_values(values.read_text(), image["repository"], image["digest"]))
        subprocess.run(["helm", "package", str(chart), "--version", metadata["version"],
                        "--app-version", metadata["version"], "--destination", str(directory)], check=True)


def filenames(version):
    return {"image.tar", "LICENSE", "THIRD_PARTY_LICENSES",
            f"openbao-operator-console-{version}.tgz"} | {
                f"openbao-operator-console_{version}_{platform}.tar.gz" for platform in PLATFORMS}


def validate(directory, expected):
    metadata = json.loads((directory / "metadata.json").read_text())
    if set(metadata) != set(expected) | {"image", "files"}:
        raise ValueError("unexpected edge metadata fields")
    if any(metadata.get(key) != value for key, value in expected.items()):
        raise ValueError("edge source identity does not match the triggering build")
    names = filenames(expected["version"])
    if set(metadata["files"]) != names or {p.name for p in directory.iterdir()} != names | {"metadata.json"}:
        raise ValueError("candidate file inventory does not match")
    for name in names:
        path = directory / name
        if path.is_symlink() or not path.is_file() or checksum(path) != metadata["files"][name]:
            raise ValueError(f"candidate checksum mismatch: {name}")
    image = image_info(directory / "image.tar", expected["version"], expected["commit"])
    image["repository"] = "ghcr.io/" + expected["repository"].lower()
    if metadata["image"] != image:
        raise ValueError("candidate image identity does not match")
    for platform in PLATFORMS:
        stem = f'openbao-operator-console_{expected["version"]}_{platform}'
        with tarfile.open(directory / (stem + ".tar.gz")) as archive:
            entries = archive.getmembers()
            members = {member.name: member for member in entries if not member.isdir()}
            if len({member.name for member in entries}) != len(entries) or any(
                    member.isdir() and member.name.rstrip("/") != stem for member in entries):
                raise ValueError("unexpected or duplicate binary archive paths")
            if set(members) != {stem + "/" + name for name in ("console", "LICENSE", "THIRD_PARTY_LICENSES")}:
                raise ValueError("unexpected binary archive contents")
            if not all(member.isfile() for member in members.values()):
                raise ValueError("binary archives must contain regular files")
            for name in ("LICENSE", "THIRD_PARTY_LICENSES"):
                if archive.extractfile(stem + "/" + name).read() != (directory / name).read_bytes():
                    raise ValueError("binary license notices do not match")
    return metadata


def unpack(source, directory):
    with tarfile.open(source) as archive:
        members = []
        names = set()
        total = 0
        for member in archive.getmembers():
            name = member.name.removeprefix("./")
            if member.name in {".", "./"} and member.isdir():
                continue
            total += member.size
            if (not member.isfile() or name != Path(name).name or name.startswith(".")
                    or "\\" in name or name in names or total > 1024 * 1024 * 1024):
                raise ValueError("unsafe edge bundle member")
            names.add(name)
            members.append((member, name))
        directory.mkdir(parents=True)
        for member, name in members:
            with (directory / name).open("xb") as output:
                shutil.copyfileobj(archive.extractfile(member), output)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("identity", "create", "validate", "unpack"))
    parser.add_argument("directory", type=Path)
    parser.add_argument("--bundle", type=Path)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--sha", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--run-attempt", required=True)
    parser.add_argument("--source-ref", default="refs/heads/main")
    args = parser.parse_args()
    expected = identity(args.repository, args.sha, args.run_id, args.run_attempt, args.source_ref)
    if args.command == "identity":
        print("version=" + expected["version"])
        print("release_tag=" + expected["releaseTag"])
        return
    if args.command == "unpack":
        if args.bundle is None:
            raise ValueError("--bundle is required")
        unpack(args.bundle, args.directory)
        # A failed-job rerun can reuse an image job that passed in an earlier attempt.
        # Its original artifacts remain valid when the same commit passes the remaining gates.
        producer_attempt = json.loads((args.directory / "metadata.json").read_text())["runAttempt"]
        if type(producer_attempt) is not int or not 1 <= producer_attempt <= int(args.run_attempt):
            raise ValueError("candidate producer attempt is outside the selected run")
        expected = identity(args.repository, args.sha, args.run_id, producer_attempt, args.source_ref)
    if args.command == "create":
        image = image_info(args.directory / "image.tar", expected["version"], expected["commit"])
        image["repository"] = "ghcr.io/" + expected["repository"].lower()
        package_chart(args.directory, expected, image)
        metadata = expected | {"image": image, "files": {
            name: checksum(args.directory / name) for name in sorted(filenames(expected["version"]))}}
        (args.directory / "metadata.json").write_text(json.dumps(metadata, indent=2) + "\n")
    metadata = validate(args.directory, expected)
    print(json.dumps(metadata, indent=2))


if __name__ == "__main__":
    main()
