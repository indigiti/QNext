"""Unit tests for the staging-only read-only baseline collector."""
import importlib.util
from pathlib import Path
import unittest


SOURCE = Path(__file__).with_name("qnext_phase4_baseline.py")
SPEC = importlib.util.spec_from_file_location("qnext_baseline", SOURCE)
baseline = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(baseline)

LATENCY = ("tick_handle_latency", "publish_latency",
           "publish_to_ws_write_latency", "ws_write_latency")


def feed(observed=30, subscribers=1, samples=100, persistence_errors=0):
    return {"live_configured": True, "telemetry": {
        "providers": {"upstox": {"observed": observed}},
        "instruments": {"NSE:NIFTY50": {"last_processed_time_ms": 1_760_000_000_000}},
        "runtime": {
            "broker": {
                "streams": 29, "active_streams": 1, "subscribers": subscribers,
                "websocket_connections": 1, "slow_disconnects": 0,
                "coalesced_forming": 10, "dropped_forming": 0,
                **{key: {"samples": samples, "p50_us": 8,
                         "p95_us": 64, "p99_us": 128} for key in LATENCY},
                "details": [{"instrument_id": "SECRET:FOO", "stream_id": "XYZ"}],
            },
            "history": {"persistence": {"queued": 3, "pending": 0,
                                        "errors": persistence_errors,
                                        "last_error": "SECRET TOKEN"}},
            "capture": {"dropped": 0, "errors": 0},
        },
    }}


class BaselineTests(unittest.TestCase):
    def test_loopback_only_and_no_credentials(self):
        for url in ("http://127.0.0.1:18080", "http://localhost:18080",
                    "http://[::1]:18080"):
            self.assertEqual(baseline.require_loopback(url), url)
        for url in ("https://stage.digiti.in", "http://0.0.0.0:18080",
                    "http://localhost:18080/?secret=token",
                    "http://name:secret@localhost:18080"):
            with self.assertRaises(ValueError):
                baseline.require_loopback(url)

    def test_only_nonsecret_operational_fields_are_retained(self):
        sample = baseline.sanitize_feed(feed(), 1_760_000_000.01)
        self.assertEqual(sample["broker"]["subscribers"], 1)
        self.assertEqual(sample["latencies"]["tick_handle_latency"]["samples"], 100)
        self.assertEqual(sample["provider_observations"], 30)
        self.assertEqual(sample["persistence"]["errors"], 0)
        self.assertNotIn("instrument_id", str(sample))
        self.assertNotIn("SECRET", str(sample))
        self.assertNotIn("details", str(sample))

    def test_window_deltas_and_provisional_percentiles(self):
        first = baseline.sanitize_feed(feed(), 1_760_000_000)
        last = baseline.sanitize_feed(feed(observed=62, samples=116), 1_760_000_010)
        report = baseline.build_report({"version": "dev", "commit": "d74b82f100f2abcd"},
                                       [first, last], "d74b82f100f2")
        self.assertEqual(report["provider_observations_delta"], 32)
        self.assertEqual(report["latencies"]["publish_latency"]["new_samples"], 16)
        self.assertEqual(report["counter_deltas"]["slow_disconnects_delta"], 0)
        self.assertTrue(report["expected_commit_matched"])
        self.assertTrue(any("provisional" in w for w in report["warnings"]))
        self.assertTrue(any("cumulative" in n for n in report["notes"]))

    def test_commit_mismatch_must_fail(self):
        a = baseline.sanitize_feed(feed(), 1_760_000_000)
        with self.assertRaisesRegex(ValueError, "Staging commit mismatch"):
            baseline.build_report({"commit": "wrong"}, [a, a], "d74b82f100f2")

    def test_missing_phase4_metric_fails_closed(self):
        f = feed()
        del f["telemetry"]["runtime"]["broker"]["publish_to_ws_write_latency"]
        with self.assertRaisesRegex(ValueError, "Phase 4 metrics missing"):
            baseline.sanitize_feed(f, 1_760_000_000)

    def test_capture_and_persistence_error_increments_warn(self):
        start = baseline.sanitize_feed(feed(), 1_760_000_000)
        end = baseline.sanitize_feed(feed(observed=90, samples=240,
                                          persistence_errors=1), 1_760_000_010)
        report = baseline.build_report({"commit": "d74b82f100f2"},
                                       [start, end], "d74b82f100f2")
        self.assertTrue(any("persistence.errors" in w for w in report["warnings"]))


if __name__ == "__main__":
    unittest.main()
