import tempfile
import unittest
from pathlib import Path
from performance_gate import check


class PerformanceGateTests(unittest.TestCase):
    def test_budget_and_lazy_chunk(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            assets = root / "public/assets"
            assets.mkdir(parents=True)
            (assets / "index-test.js").write_text("main")
            browser = assets / "symbol-browser-test.js"
            browser.write_text("browser")
            budget = {"components": {"workspace": {"max_bytes": 100}},
                      "max_total_bytes": 100, "workspace_js_gzip_max_bytes": 100,
                      "symbol_browser_js_gzip_max_bytes": 50}
            self.assertEqual(check(root, budget)["failures"], [])
            budget["components"]["workspace"]["max_bytes"] = 2
            self.assertTrue(any("workspace:" in x for x in check(root, budget)["failures"]))
            browser.unlink()
            self.assertTrue(any("lazy" in x for x in check(root, budget)["failures"]))
