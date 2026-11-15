import hashlib
import importlib.util
import io
from pathlib import Path
import tarfile
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("check_archive", Path(__file__).resolve().parents[1] / "check-archive.py")
archive_check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(archive_check)


class ArchiveTests(unittest.TestCase):
    def archive(self, root, extra=None):
        path = root / "archive.tar.gz"
        with tarfile.open(path, "w:gz") as output:
            stem = "openbao-operator-console_0.1.0_linux_amd64/"
            for name in ("console", "LICENSE", "THIRD_PARTY_LICENSES"):
                member = tarfile.TarInfo(stem + name)
                member.size = 4
                output.addfile(member, io.BytesIO(b"test"))
            if extra:
                output.addfile(extra)
        return path, hashlib.sha256(path.read_bytes()).hexdigest()

    def test_regular_archive(self):
        with tempfile.TemporaryDirectory() as scratch:
            path, digest = self.archive(Path(scratch))
            self.assertEqual(archive_check.read_archive(path, "0.1.0", "linux_amd64", digest)["console"], b"test")

    def test_tampered_archive(self):
        with tempfile.TemporaryDirectory() as scratch:
            path, _ = self.archive(Path(scratch))
            with self.assertRaisesRegex(ValueError, "checksum mismatch"):
                archive_check.read_archive(path, "0.1.0", "linux_amd64", "0" * 64)

    def test_links_and_escaping_paths(self):
        for name, kind in (("../escape", tarfile.REGTYPE), ("link", tarfile.SYMTYPE)):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as scratch:
                member = tarfile.TarInfo(name)
                member.type = kind
                member.linkname = "/tmp/escape"
                path, digest = self.archive(Path(scratch), member)
                with self.assertRaisesRegex(ValueError, "unexpected archive entry"):
                    archive_check.read_archive(path, "0.1.0", "linux_amd64", digest)


if __name__ == "__main__":
    unittest.main()
