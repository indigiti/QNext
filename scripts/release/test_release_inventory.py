import tempfile
import unittest
from pathlib import Path
from release_inventory import inventory, compare


class InventoryTests(unittest.TestCase):
    def test_changed_and_removed_components(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "public/admin").mkdir(parents=True)
            (root / "public/index.html").write_text("a")
            (root / "public/admin/index.html").write_text("b")
            before = inventory(root)
            (root / "public/index.html").write_text("aa")
            (root / "public/admin/index.html").unlink()
            after = inventory(root)
            report = compare(before, after)
            self.assertEqual(report["changed"], ["public/index.html"])
            self.assertEqual(report["removed"], ["public/admin/index.html"])
            self.assertEqual(report["changed_components"], ["admin", "workspace"])
            self.assertEqual(report["changed_bytes"], 2)
            self.assertEqual(report["full_bytes"], 2)

    def test_nonruntime_files_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "public").mkdir()
            (root / "public/app.js.map").write_text("not runtime")
            with self.assertRaisesRegex(ValueError, "Non-runtime"):
                inventory(root)

    def test_symlinks_rejected_in_zip(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "public").mkdir()
            (root / "public/alias").symlink_to("/tmp")
            with self.assertRaisesRegex(ValueError, "symlink"):
                inventory(root)

    def test_checksum_detects_same_length_changes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "public").mkdir()
            path = root / "public/index.html"
            path.write_text("a")
            before = inventory(root)
            path.write_text("b")
            self.assertEqual(compare(before, inventory(root))["changed"], ["public/index.html"])
