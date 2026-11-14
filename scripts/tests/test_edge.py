import hashlib
import importlib.util
import io
import json
import os
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest import mock
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def module(name):
    spec = importlib.util.spec_from_file_location(name, ROOT / "scripts" / (name + ".py"))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


bundle = module("edge-bundle")
source = module("edge-source")
SHA = "a" * 40
REPOSITORY = "kubebao/openbao-operator-console"


def member(archive, name, data):
    info = tarfile.TarInfo(name)
    info.size = len(data)
    archive.addfile(info, io.BytesIO(data))


def image(path, version, revision=SHA, architectures=("amd64", "arm64")):
    blobs = {}

    def blob(value):
        raw = json.dumps(value).encode()
        digest = "sha256:" + hashlib.sha256(raw).hexdigest()
        blobs["blobs/sha256/" + digest[7:]] = raw
        return digest

    manifests = []
    for architecture in architectures:
        config = blob({"os": "linux", "architecture": architecture, "config": {
            "User": "65532:65532", "Labels": {"org.opencontainers.image.version": version,
                                                 "org.opencontainers.image.revision": revision}}})
        digest = blob({"config": {"digest": config}, "layers": []})
        manifests.append({"platform": {"os": "linux", "architecture": architecture}, "digest": digest})
    digest = blob({"manifests": manifests})
    with tarfile.open(path, "w") as archive:
        member(archive, "index.json", json.dumps({"manifests": [{"digest": digest}]}).encode())
        for name, raw in blobs.items():
            member(archive, name, raw)


class EdgeBoundaryTests(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory()
        self.addCleanup(self.scratch.cleanup)
        self.directory = Path(self.scratch.name)
        self.expected = bundle.identity(REPOSITORY, SHA, "123", "1", "refs/heads/main")

    def candidate(self):
        version = self.expected["version"]
        image(self.directory / "image.tar", version)
        (self.directory / "LICENSE").write_bytes(b"project license")
        (self.directory / "THIRD_PARTY_LICENSES").write_bytes(b"dependency notices")
        (self.directory / f"openbao-operator-console-{version}.tgz").write_bytes(b"chart")
        for platform in bundle.PLATFORMS:
            stem = f"openbao-operator-console_{version}_{platform}"
            with tarfile.open(self.directory / (stem + ".tar.gz"), "w:gz") as archive:
                member(archive, stem + "/console", b"binary")
                for name in ("LICENSE", "THIRD_PARTY_LICENSES"):
                    member(archive, stem + "/" + name, (self.directory / name).read_bytes())
        info = bundle.image_info(self.directory / "image.tar", version, SHA)
        info["repository"] = "ghcr.io/" + REPOSITORY
        metadata = self.expected | {"image": info, "files": {
            name: bundle.checksum(self.directory / name) for name in bundle.filenames(version)}}
        (self.directory / "metadata.json").write_text(json.dumps(metadata))
        return metadata

    def test_identity_keeps_edge_git_tags_outside_stable_semver(self):
        self.assertEqual(self.expected["version"], "0.0.0-edge.123.1.gaaaaaaaaaaaa")
        self.assertEqual(self.expected["releaseTag"], "edge-123.1.gaaaaaaaaaaaa")
        with self.assertRaises(ValueError):
            bundle.identity(REPOSITORY, "main", "123", "1", "refs/heads/main")
        with self.assertRaises(ValueError):
            bundle.identity(REPOSITORY, SHA, "123\nready=true", "1", "refs/heads/main")

    def test_image_rejects_wrong_revision_and_missing_platform(self):
        for revision, architectures in (("b" * 40, ("amd64", "arm64")), (SHA, ("amd64",))):
            image(self.directory / "image.tar", self.expected["version"], revision, architectures)
            with self.assertRaises(ValueError):
                bundle.image_info(self.directory / "image.tar", self.expected["version"], SHA)

    def test_candidate_accepts_all_four_archives_and_rejects_tampering(self):
        self.candidate()
        bundle.validate(self.directory, self.expected)
        (self.directory / "THIRD_PARTY_LICENSES").write_bytes(b"changed notices")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            bundle.validate(self.directory, self.expected)

    def test_candidate_rejects_unexpected_files_and_another_build(self):
        self.candidate()
        other = self.expected | {"runId": 124}
        with self.assertRaisesRegex(ValueError, "source identity"):
            bundle.validate(self.directory, other)
        (self.directory / "unexpected.sh").write_text("exit 0")
        with self.assertRaisesRegex(ValueError, "file inventory"):
            bundle.validate(self.directory, self.expected)

    def test_chart_defaults_pin_digest_without_changing_other_images(self):
        text = 'image:\n  repository: stable\n  tag: ""\n  digest: ""\nother:\n  repository: keep\n'
        result = bundle.chart_values(text, "ghcr.io/example/console", "sha256:" + "b" * 64)
        self.assertIn('  digest: "sha256:' + "b" * 64 + '"', result)
        self.assertIn("other:\n  repository: keep", result)
        with self.assertRaises(ValueError):
            bundle.chart_values("image:\n  repository: stable\n", "image", "digest")

    def test_unpack_rejects_traversal_symlinks_and_duplicate_paths_before_writing(self):
        for attack in ("traversal", "symlink", "duplicate"):
            with self.subTest(attack=attack):
                archive_path = self.directory / "bundle.tar"
                destination = self.directory / "unpacked"
                with tarfile.open(archive_path, "w") as archive:
                    member(archive, "./metadata.json", b"{}")
                    if attack == "traversal":
                        member(archive, "../outside", b"unsafe")
                    elif attack == "symlink":
                        info = tarfile.TarInfo("link")
                        info.type, info.linkname = tarfile.SYMTYPE, "/tmp/outside"
                        archive.addfile(info)
                    else:
                        member(archive, "./metadata.json", b"replaced")
                with self.assertRaises(ValueError):
                    bundle.unpack(archive_path, destination)
                self.assertFalse(destination.exists())

    def test_handoff_accepts_reused_successful_candidate_and_rejects_future_attempt(self):
        self.candidate()
        with tempfile.TemporaryDirectory() as handoff:
            path = Path(handoff)

            def pack():
                with tarfile.open(path / "candidate.tar", "w") as archive:
                    archive.add(self.directory, arcname=".")

            pack()
            command = [sys.executable, str(ROOT / "scripts/edge-bundle.py"), "unpack", str(path / "dist" / "qualified"),
                       "--bundle", str(path / "candidate.tar"), "--repository", REPOSITORY, "--sha", SHA,
                       "--run-id", "123", "--run-attempt", "2"]
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            metadata = json.loads((self.directory / "metadata.json").read_text())
            metadata["runAttempt"] = 3
            (self.directory / "metadata.json").write_text(json.dumps(metadata))
            pack()
            command[3] = str(path / "future")
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("producer attempt is outside", result.stderr)


class SourceGateTests(unittest.TestCase):
    def test_opted_in_publication_still_requires_qualified_current_main(self):
        run = {"id": 1, "head_sha": SHA, "event": "push", "head_branch": "main", "run_attempt": 1,
               "head_repository": {"full_name": REPOSITORY}, "status": "completed", "conclusion": "success"}
        for main in (SHA, "b" * 40):
            with self.subTest(main=main), tempfile.TemporaryDirectory() as scratch:
                output = Path(scratch) / "output"
                responses = [io.BytesIO(json.dumps(value).encode()) for value in
                             ({"workflow_runs": [run]}, {"workflow_runs": [run]}, {"commit": {"sha": main}})]
                with mock.patch.dict(os.environ, {"CONSOLE_EDGE_ENABLED": "true", "GITHUB_REPOSITORY": REPOSITORY,
                                                  "SOURCE_SHA": SHA, "GH_TOKEN": "fixture", "GITHUB_OUTPUT": str(output)}, clear=True), \
                     mock.patch("urllib.request.urlopen", side_effect=responses) as request:
                    source.main()
                self.assertEqual(request.call_count, 3)
                self.assertIn("ready=" + str(main == SHA).lower() + "\n", output.read_text())

    def test_disabled_publication_does_not_contact_github(self):
        for enabled in ("", "false", "True", "1"):
            with self.subTest(enabled=enabled), tempfile.TemporaryDirectory() as scratch:
                output = Path(scratch) / "output"
                with mock.patch.dict(os.environ, {"CONSOLE_EDGE_ENABLED": enabled,
                                                  "GITHUB_OUTPUT": str(output)}, clear=True), \
                     mock.patch("urllib.request.urlopen") as request:
                    source.main()
                request.assert_not_called()
                self.assertEqual(output.read_text(), "ready=false\n")

    def test_newer_pending_or_failed_run_blocks_an_older_success(self):
        old = {"id": 1, "head_sha": SHA, "event": "push", "head_branch": "main",
               "head_repository": {"full_name": REPOSITORY}, "status": "completed", "conclusion": "success"}
        self.assertEqual(source.latest_success([old], REPOSITORY, SHA), old)
        for status, conclusion in (("in_progress", None), ("completed", "failure")):
            new = old | {"id": 2, "status": status, "conclusion": conclusion}
            self.assertIsNone(source.latest_success([old, new], REPOSITORY, SHA))
        for change in ({"event": "pull_request"}, {"head_branch": "feature"},
                       {"head_sha": "b" * 40}, {"head_repository": {"full_name": "fork/console"}}):
            self.assertIsNone(source.latest_success([old | change], REPOSITORY, SHA))
        self.assertIsNotNone(source.latest_success([old | {"event": "workflow_dispatch"}], REPOSITORY, SHA))


class RegistryFailureTests(unittest.TestCase):
    def test_conflicting_tags_and_lookup_failures_never_import(self):
        with tempfile.TemporaryDirectory() as scratch:
            directory = Path(scratch)
            metadata = {"image": {"repository": "ghcr.io/" + REPOSITORY, "digest": "sha256:" + "a" * 64},
                        "version": "0.0.0-edge.1.1.gaaaaaaaaaaaa", "releaseTag": "edge-1.1.gaaaaaaaaaaaa", "commit": SHA}
            (directory / "metadata.json").write_text(json.dumps(metadata))
            tool = directory / "regctl"
            tool.write_text('#!/bin/sh\nif [ "$2" = digest ]; then\n'
                            '  if [ "$CASE" = conflict ]; then echo "sha256:other"; exit 0; fi\n'
                            '  echo "authentication denied" >&2; exit 1\nfi\n'
                            'touch "$MUTATION"\n')
            tool.chmod(0o755)
            marker = directory / "mutation"
            for case in ("conflict", "authentication"):
                environment = os.environ | {"PATH": str(directory) + os.pathsep + os.environ["PATH"],
                    "GITHUB_REPOSITORY_OWNER": "kubebao", "CASE": case, "MUTATION": str(marker)}
                result = subprocess.run([str(ROOT / "scripts/publish-edge.sh"), "image", str(directory)],
                                        env=environment, capture_output=True, text=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("different content" if case == "conflict" else "registry lookup failed", result.stderr)
                self.assertFalse(marker.exists())

    def test_chart_pull_requires_one_byte_identical_archive(self):
        with tempfile.TemporaryDirectory() as scratch:
            directory = Path(scratch)
            version = "0.0.0-edge.1.1.gaaaaaaaaaaaa"
            (directory / "metadata.json").write_text(json.dumps({
                "image": {"repository": "ghcr.io/" + REPOSITORY, "digest": "sha256:" + "a" * 64},
                "version": version, "releaseTag": "edge-1.1.gaaaaaaaaaaaa", "commit": SHA}))
            (directory / f"openbao-operator-console-{version}.tgz").write_bytes(b"qualified chart")
            regctl = directory / "regctl"
            regctl.write_text('#!/bin/sh\necho "sha256:' + "a" * 64 + '"\n')
            regctl.chmod(0o755)
            helm = directory / "helm"
            helm.write_text('#!/bin/sh\nfor destination; do :; done\n'
                            'case "$CASE" in\n'
                            ' matching) printf "qualified chart" > "$destination/digest-name.tgz";;\n'
                            ' mismatch) printf "changed chart" > "$destination/digest-name.tgz";;\n'
                            ' multiple) touch "$destination/one.tgz" "$destination/two.tgz";;\n'
                            'esac\n')
            helm.chmod(0o755)
            for case in ("matching", "missing", "mismatch", "multiple"):
                environment = os.environ | {"PATH": str(directory) + os.pathsep + os.environ["PATH"],
                    "GITHUB_REPOSITORY_OWNER": "kubebao", "CASE": case, "GITHUB_OUTPUT": str(directory / "output")}
                result = subprocess.run([str(ROOT / "scripts/publish-edge.sh"), "chart", str(directory)],
                                        env=environment, capture_output=True, text=True)
                if case == "matching":
                    self.assertEqual(result.returncode, 0, result.stderr)
                else:
                    self.assertNotEqual(result.returncode, 0)


if __name__ == "__main__":
    unittest.main()
