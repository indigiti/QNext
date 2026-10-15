"""Test the read-only staging acceptance probe without contacting staging."""
import json
import unittest
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
            return (200, b'{"symbols":[{"ticker":"NSE:NIFTY","instrument_id":"NSE:NIFTY50"}]}', "application/json")
        if path.startswith("/api/v1/workspace-settings/"):
            return (200, b'{"chartEngine":"vela"}', "application/json")
        if path.startswith("/api/v1/feed-status/"):
            return (200, b'{"ok":true}', "application/json")
        if path.startswith("/api/v1/paper-markers/"):
            return (200, b'{"available":false,"enabled":false,"markers":[]}', "application/json")
        if path.startswith("/api/v1/bars/"):
            return (200, b'{"bars":[]}', "application/json")
        raise AssertionError("unexpected request: " + path)

    def test_read_only_clean_smoke(self):
        with patch.object(probe, "fetch", side_effect=self.fake_fetch):
            checks = probe.inspect("https://test.example/qnext", 1, public_markers=False)
        self.assertTrue(checks)
        self.assertTrue(all(x["result"] == "PASS" for x in checks), checks)
        self.assertTrue(any(x["check"] == "historical NIFTY candles response" for x in checks))

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
