import json
import tempfile
import unittest
import zipfile
from pathlib import Path

from release_inventory import NAME, digest, inventory
from release_delta import create, reconstruct


class DeltaTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.base = self.root / "base"
        self.target = self.root / "target"
        self.output = self.root / "delta.zip"
        self.restored = self.root / "restored"
        for root, commit in ((self.base, "a" * 40), (self.target, "b" * 40)):
            (root / "public").mkdir(parents=True)
            (root / "release-manifest.json").write_text(json.dumps({
                "schema": "QNEXT.RELEASE/1", "artifact": "digiops-release", "commit": commit,
            }))
            (root / "public/unchanged.js").write_text("stable" * 500)
        (self.base / "public/app.js").write_text("before")
        (self.base / "public/removed.js").write_text("obsolete")
        (self.target / "public/app.js").write_text("after")
        (self.target / "public/new.js").write_text("new")
        for root in (self.base, self.target):
            (root / NAME).write_text(json.dumps(inventory(root), sort_keys=True))

    def restore(self, expected="b" * 40, checksum=None):
        return reconstruct(self.base, self.output, self.restored, "a" * 40,
                           expected, checksum or digest(self.target / NAME))

    def test_round_trip_and_deletion(self):
        report = create(self.base, self.target, self.output)
        self.assertLess(report["changed_bytes"], report["full_bytes"])
        self.restore()
        self.assertEqual(inventory(self.restored), inventory(self.target))
        self.assertFalse((self.restored / "public/removed.js").exists())
        self.assertTrue((self.base / "public/removed.js").exists())
        self.assertEqual((self.restored / NAME).read_bytes(), (self.target / NAME).read_bytes())

    def test_wrong_base_or_approval(self):
        create(self.base, self.target, self.output)
        with self.assertRaises(ValueError):
            self.restore(expected="c" * 40)
        with self.assertRaises(ValueError):
            self.restore(checksum="0" * 64)
        (self.base / "public/unchanged.js").write_text("corrupt")
        with self.assertRaises(ValueError):
            self.restore()
        self.assertFalse(self.restored.exists())

    def test_unexpected_entry_and_traversal(self):
        create(self.base, self.target, self.output)
        with zipfile.ZipFile(self.output, "a") as z:
            z.writestr("../escape", "bad")
        with self.assertRaises(ValueError):
            self.restore()
        self.assertFalse(self.restored.exists())
        self.assertFalse((self.root / "escape").exists())

    def test_tampered_changed_file(self):
        create(self.base, self.target, self.output)
        other = self.root / "tampered.zip"
        with zipfile.ZipFile(self.output) as source, zipfile.ZipFile(other, "w") as dest:
            for name in source.namelist():
                data = b"wrong" if name == "files/public/app.js" else source.read(name)
                dest.writestr(name, data)
        self.output = other
        with self.assertRaises(ValueError):
            self.restore()
        self.assertFalse(self.restored.exists())


if __name__ == "__main__":
    unittest.main()
