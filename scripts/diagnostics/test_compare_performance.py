import copy
import unittest
from qnext_compare_performance import compare
from qnext_phase4_baseline import LATENCIES


def report():
    broker = {"subscribers": 2, "active_streams": 3, "websocket_connections": 1}
    return {"schema": "qnext-phase4-staging-baseline-v1", "expected_commit_matched": True,
            "elapsed_seconds": 60, "provider_observations_delta": 6000,
            "workload": {"first_broker": broker, "last_broker": broker},
            "latencies": {name: {"new_samples": 150, "latest_p95_us": 64} for name in LATENCIES},
            "process": {"cpu_percent_one_core": 10, "rss_bytes_peak_sampled": 100000},
            "counter_deltas": {"slow_disconnects_delta": 0},
            "error_deltas": {"capture.errors": 0, "capture.dropped": 0, "persistence.errors": 0}}


class ComparisonTests(unittest.TestCase):
    def test_equivalent_and_regression(self):
        old = report()
        new = copy.deepcopy(old)
        self.assertEqual(compare(old, new)["status"], "PASS")
        new["process"]["rss_bytes_peak_sampled"] *= 2
        self.assertEqual(compare(old, new)["status"], "FAIL")

    def test_insufficient_traffic(self):
        new = report()
        new["provider_observations_delta"] = 0
        self.assertEqual(compare(report(), new)["status"], "INCONCLUSIVE")

    def test_different_workload(self):
        new = report()
        new["workload"]["last_broker"]["subscribers"] = 4
        self.assertEqual(compare(report(), new)["status"], "INCONCLUSIVE")

    def test_capture_loss(self):
        new = report()
        new["error_deltas"]["capture.dropped"] = 1
        self.assertEqual(compare(report(), new)["status"], "FAIL")
