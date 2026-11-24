import json
import tempfile
import subprocess
import os
import unittest
import zipfile
from pathlib import Path

from release_inventory import NAME, digest, inventory
from release_delta import DEPLOY_EXECUTABLES, create, reconstruct, materialize_stage


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


    def test_verified_delta_can_stage_without_activation(self):
        for root, commit in ((self.base, "a" * 40), (self.target, "b" * 40)):
            manifest = json.loads((root / "release-manifest.json").read_text())
            manifest["version"] = commit[:12]
            manifest["component_identity"] = {}
            manifest["services"] = {}
            bins = root / "private/bin"
            bins.mkdir(parents=True)
            for key in ("market_core", "data_collector"):
                binary = "qnext-" + key.replace("_", "-")
                data = root / "temporary-binary"
                data.write_text(binary + commit)
                sha = digest(data)
                target = binary + "-" + sha
                data.rename(bins / target)
                (bins / (binary + ".current")).write_text(target + "\n")
                manifest["component_identity"][key] = {"sha256": sha}
                manifest["services"][key] = target
            (root / "release-manifest.json").write_text(json.dumps(manifest))
            for path in ("public/index.html", "public/admin/index.html", "public/admin/api/index.php",
                         "public/api/v1/bars/index.php", "public/qnext-release.json",
                         "private/ops-api/public/index.php"):
                file = root / path
                file.parent.mkdir(parents=True, exist_ok=True)
                file.write_text("fixture")
            (root / "private/deploy").mkdir()
            for name in DEPLOY_EXECUTABLES:
                (root / "private/deploy" / name).write_text("#!/bin/sh\n")
            (root / NAME).write_text(json.dumps(inventory(root), sort_keys=True))
        create(self.base, self.target, self.output)
        self.restore()
        staged_payload = self.root / "posix"
        materialize_stage(self.restored, staged_payload, "b" * 40, digest(self.target / NAME))
        installer_root = self.root / "private-runtime"
        installer_root.mkdir()
        marker = installer_root / "existing-state"
        marker.write_text("keep")
        installer = Path(__file__).resolve().parents[2] / "deploy/qnext-stage-release"
        subprocess.run(["bash", str(installer), str(staged_payload), str(installer_root), "b" * 40],
                       check=True, capture_output=True, text=True)
        destination = installer_root / "releases" / ("b" * 12)
        for binary in ("qnext-market-core", "qnext-data-collector"):
            link = destination / "private/bin" / binary
            self.assertTrue(link.is_symlink())
            self.assertTrue(os.access(link, os.X_OK))
        self.assertFalse((installer_root / "current").exists())
        self.assertEqual(marker.read_text(), "keep")

if __name__ == "__main__":
    unittest.main()
