"""Test the read-only staging acceptance probe without contacting staging."""
import json
import unittest
from pathlib import Path
from unittest.mock import patch

from scripts.diagnostics import staging_audit_probe as probe


class StagingProbeTests(unittest.TestCase):
    def fake_fetch(self, base, path, timeout):
        if path in ("/", "/admin/"):
            return (200, b"<html><body>QNext</body></html>", "text/html")
        if path.startswith("/admin/api/"):
            if "route=%2Fsession" in path:
                return (200, b'{"authenticated":false}', "application/json")
            return (403, b'{"error":"forbidden"}', "application/json")
        if path.startswith("/api/v1/timeframes/"):
            return (200, b'{"timeframes":["1m"]}', "application/json")
        if path.startswith("/api/v1/symbols/"):
            symbols = [
                {"ticker": "NSE:NIFTY", "instrument_id": "NSE:NIFTY50"},
                {"ticker": "QNEXT:NIFTY-SYN", "instrument_id": "QNEXT:NIFTY-SYN"},
                {"ticker": "QNEXT:NIFTY-SYN+", "instrument_id": "QNEXT:NIFTY-SYN+"},
            ]
            return (200, json.dumps({"symbols": symbols}).encode(), "application/json")
        if path.startswith("/api/v1/workspace-settings/"):
            return (200, b'{"chartEngine":"vela"}', "application/json")
        if path.startswith("/api/v1/feed-status/"):
            return (200, b'{"ok":true}', "application/json")
        if path.startswith("/api/v1/paper-markers/"):
            return (200, b'{"available":false,"enabled":false,"markers":[]}', "application/json")
        if path.startswith("/api/v1/bars/"):
            bars = [
                {"time": 1700000000000, "open": 100, "high": 103, "low": 98, "close": 101, "volume": 42},
                {"time": 1700000060000, "open": 101, "high": 104, "low": 100, "close": 103, "volume": 13},
            ]
            return (200, json.dumps({"bars": bars}).encode(), "application/json")
        raise AssertionError("unexpected request: " + path)

    def test_read_only_clean_smoke(self):
        with patch.object(probe, "fetch", side_effect=self.fake_fetch):
            checks = probe.inspect("https://test.example/qnext", 1, public_markers=False)
        self.assertTrue(checks)
        self.assertTrue(all(x["result"] == "PASS" for x in checks), checks)
        self.assertEqual(
            len([x for x in checks if x["check"].startswith("historical ") and x["result"] == "PASS"]),
            3,
        )

    def test_empty_history_is_a_failure_not_a_pass(self):
        def empty_fetch(base, path, timeout):
            if path.startswith("/api/v1/bars/"):
                return (200, b'{"bars":[]}', "application/json")
            return self.fake_fetch(base, path, timeout)
        with patch.object(probe, "fetch", side_effect=empty_fetch):
            checks = probe.inspect("https://test.example/qnext", 1, public_markers=False)
        self.assertEqual(
            len([x for x in checks if x["check"].startswith("historical ") and x["result"] == "FAIL"]),
            3,
        )

    def test_market_candle_validation(self):
        good = [
            {"time": 1700000000000, "open": 100, "high": 101, "low": 99, "close": 100, "volume": 0},
            {"time": 1700000060000, "open": 101, "high": 102, "low": 100, "close": 101, "volume": 10},
        ]
        probe.validate_bars(good, "NIFTY")
        with self.assertRaisesRegex(ValueError, "non-increasing"):
            probe.validate_bars([good[0], good[0]], "NIFTY")
        with self.assertRaisesRegex(ValueError, "OHLC invariant"):
            probe.validate_bars([dict(good[0], high=98)], "NIFTY")
        with self.assertRaisesRegex(ValueError, "volume"):
            probe.validate_bars([dict(good[0], volume=-1)], "NIFTY")
        with self.assertRaisesRegex(ValueError, "volumes unavailable"):
            probe.validate_bars([dict(good[0], volume=None)], "NIFTY")

    def test_synthetic_symbol_classification(self):
        self.assertEqual(probe.classify_nifty("QNEXT:NIFTY-SYN+"), "NIFTY SYN+")
        self.assertEqual(probe.classify_nifty("QNEXT:NIFTY-SYN"), "NIFTY SYN")
        self.assertEqual(probe.classify_nifty("NSE:NIFTY"), "NIFTY")
        self.assertIsNone(probe.classify_nifty("NSE:BANKNIFTY"))

    def test_admin_mutation_helpers_include_browser_csrf_header(self):
        # Prevent recurrence: distinct panel request helpers used to miss this
        # header, making legitimate cookie-authenticated POST operations fail.
        repo = Path(__file__).resolve().parents[2]
        helpers = (
            "apps/ops-console/src/api.ts",
            "apps/ops-console/src/admin-bootstrap.ts",
            "apps/ops-console/src/admin-access.ts",
            "apps/ops-console/src/intelligence-panel.mts",
            "apps/ops-console/src/intelligence-lab-panel.mts",
            "apps/ops-console/src/syn-plus-panel.ts",
        )
        for name in helpers:
            with self.subTest(name=name):
                contents = (repo / name).read_text(encoding="utf-8")
                self.assertIn("X-QNext-Admin-Request", contents)

    def test_execution_marker_exposure_is_rejected(self):
        def leaking_fetch(base, path, timeout):
            if path.startswith("/api/v1/paper-markers/"):
                return (200, json.dumps({"available": True, "enabled": True,
                                         "markers": [{"side": "BUY", "price": 250}]}).encode(), "application/json")
            return self.fake_fetch(base, path, timeout)
        with patch.object(probe, "fetch", side_effect=leaking_fetch):
            checks = probe.inspect("https://test.example/qnext", 1, public_markers=False)
        self.assertTrue(any(x["check"] == "no public paper executions" and x["result"] == "FAIL"
                            for x in checks))


if __name__ == "__main__":
    unittest.main()
