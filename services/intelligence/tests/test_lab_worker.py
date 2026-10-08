from __future__ import annotations

import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from qnext_intelligence.domain import Bar
from qnext_intelligence.lab_worker import process_request


def chart_snapshot(rows: int = 90):
    feature_rows = []
    for index in range(rows):
        feature_rows.append(
            {
                "bar_time_ms": index * 60_000,
                "features": {
                    "indicator.ema_1.plot.ema": 100.0 + index * 0.1,
                    "indicator.ema_1.var.trend": 1.0 if index % 2 == 0 else -1.0,
                },
            }
        )
    return {
        "schema": "QNEXT.INTELLIGENCE.LAB.CHART_SNAPSHOT/1",
        "feature_schema_version": "qnext-chart-indicators-v1",
        "created_at_ms": 1_700_000_000_000,
        "instrument_id": "QNEXT:NIFTY",
        "timeframe": "1m",
        "indicator_configuration_hash": "a" * 64,
        "indicators": [
            {
                "instance_id": "ema-1",
                "title": "Adaptive EMA",
                "kind": "native",
                "source_hash": "b" * 64,
                "configuration_hash": "c" * 64,
                "inputs": {"emaLength": 20},
            }
        ],
        "feature_rows": feature_rows,
        "current_features": feature_rows[-1]["features"],
    }


def canonical_bars(count: int = 95):
    closes = [100.0]
    for index in range(1, count):
        previous_index = index - 1
        step = 1.0 if previous_index % 2 == 0 else -0.5
        closes.append(closes[-1] + step)

    bars = []
    for index, close in enumerate(closes):
        bars.append(
            Bar(
                instrument_id="QNEXT:NIFTY",
                timeframe="1m",
                open_time_ms=index * 60_000,
                close_time_ms=(index + 1) * 60_000,
                open=close,
                high=close + 0.4,
                low=close - 0.4,
                close=close,
                volume=1000 + index,
                final=True,
                quality="GOOD",
            )
        )
    return bars


class FakeHistoryClient:
    def __init__(self, _base_url: str):
        pass

    def fetch_bars(self, **_kwargs):
        return canonical_bars()


class IntelligenceLabWorkerTests(unittest.TestCase):
    def request(self, action: str, payload: dict) -> dict:
        return {
            "schema": "QNEXT.INTELLIGENCE.LAB.REQUEST/1",
            "request_id": "lab-request-1",
            "action": action,
            "requested_at_ms": 1,
            "payload": payload,
        }

    def test_import_then_backtest_stays_in_lab_storage(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            lab_root = root / "storage" / "intelligence-lab"
            production_root = root / "storage" / "intelligence"

            imported = process_request(
                self.request("import", {"snapshot": chart_snapshot()}),
                storage_root=lab_root,
                production_root=production_root,
                market_core_url="http://market-core",
            )
            experiment_id = imported["result"]["experiment"]["experiment_id"]
            self.assertTrue(
                (lab_root / "experiments" / experiment_id / "sources" / "chart-snapshot.json").is_file()
            )
            self.assertFalse(production_root.exists())

            with patch(
                "qnext_intelligence.lab_worker.MarketCoreHistoryClient",
                FakeHistoryClient,
            ):
                result = process_request(
                    self.request(
                        "backtest",
                        {
                            "experimentId": experiment_id,
                            "horizonBars": 1,
                            "minSamples": 60,
                            "minTestSamples": 12,
                            "minAverageReturnImprovement": -1.0,
                            "maxAccuracyRegression": 1.0,
                            "maxDrawdownSlack": 10.0,
                        },
                    ),
                    storage_root=lab_root,
                    production_root=production_root,
                    market_core_url="http://market-core",
                )

            self.assertEqual(result["state"], "SUCCESS")
            self.assertEqual(result["action"], "backtest")
            evaluation = result["result"]["evaluation"]
            self.assertGreaterEqual(evaluation["metrics"]["historical_examples"], 60)
            self.assertGreaterEqual(evaluation["metrics"]["feature_count"], 1)
            self.assertRegex(evaluation["dataset_hash"], r"^[a-f0-9]{64}$")
            self.assertTrue(
                (lab_root / "experiments" / experiment_id / "results" / "backtest.json").is_file()
            )
            self.assertFalse((production_root / "models" / "production.json").exists())

    def test_rejects_unknown_action_and_schema(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            with self.assertRaisesRegex(ValueError, "unsupported intelligence lab action"):
                process_request(
                    self.request("promote", {}),
                    storage_root=root / "lab",
                    production_root=root / "prod",
                    market_core_url="http://market-core",
                )

            bad = self.request("import", {"snapshot": chart_snapshot()})
            bad["schema"] = "OTHER"
            with self.assertRaisesRegex(ValueError, "unsupported intelligence lab request schema"):
                process_request(
                    bad,
                    storage_root=root / "lab",
                    production_root=root / "prod",
                    market_core_url="http://market-core",
                )


if __name__ == "__main__":
    unittest.main()
