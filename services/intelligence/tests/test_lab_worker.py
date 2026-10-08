from __future__ import annotations

import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from qnext_intelligence.domain import Bar
from qnext_intelligence.lab_worker import _selected_feature_names, process_request


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
            recommendation_path = (
                lab_root
                / "experiments"
                / experiment_id
                / "results"
                / "recommendation.json"
            )
            self.assertTrue(recommendation_path.is_file())
            recommendation = result["result"]["recommendation"]
            self.assertIsInstance(recommendation, dict)
            self.assertTrue(recommendation["probability_calibrated"])
            self.assertIn(recommendation["decision"], {"BUY", "SELL", "NO_TRADE"})
            self.assertAlmostEqual(
                sum(recommendation["probabilities"].values()),
                1.0,
                places=6,
            )
            self.assertFalse((production_root / "models" / "production.json").exists())

    def test_shadow_live_observation_and_retryable_certification(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            lab_root = root / "storage" / "intelligence-lab"
            production_root = root / "storage" / "intelligence"
            snapshot = chart_snapshot()

            imported = process_request(
                self.request("import", {"snapshot": snapshot}),
                storage_root=lab_root,
                production_root=production_root,
                market_core_url="http://market-core",
            )
            experiment_id = imported["result"]["experiment"]["experiment_id"]

            with patch(
                "qnext_intelligence.lab_worker.MarketCoreHistoryClient",
                FakeHistoryClient,
            ):
                backtest = process_request(
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
            self.assertEqual(
                backtest["result"]["experiment"]["lifecycle_state"],
                "BACKTESTED",
            )

            with patch("qnext_intelligence.lab_worker.time.time", return_value=5_000.0):
                shadow = process_request(
                    self.request(
                        "start-shadow",
                        {
                            "experimentId": experiment_id,
                            "horizonBars": 1,
                            "minSamples": 12,
                        },
                    ),
                    storage_root=lab_root,
                    production_root=production_root,
                    market_core_url="http://market-core",
                )
            self.assertEqual(
                shadow["result"]["experiment"]["lifecycle_state"],
                "SHADOW",
            )

            final_rows = snapshot["feature_rows"][-30:]
            with patch(
                "qnext_intelligence.lab_worker.MarketCoreHistoryClient",
                FakeHistoryClient,
            ):
                observed = process_request(
                    self.request(
                        "shadow-observation",
                        {
                            "experimentId": experiment_id,
                            "indicatorConfigurationHash": "a" * 64,
                            "featureSchemaVersion": "qnext-chart-indicators-v1",
                            "barTimeMs": final_rows[-1]["bar_time_ms"],
                            "createdAtMs": 5_500_000,
                            "featureRows": final_rows,
                            "currentFeatures": final_rows[-1]["features"],
                        },
                    ),
                    storage_root=lab_root,
                    production_root=production_root,
                    market_core_url="http://market-core",
                )

            self.assertEqual(observed["state"], "SUCCESS")
            self.assertIn("observation", observed["result"])
            self.assertEqual(
                observed["result"]["summary"]["completed_samples"],
                1,
            )
            shadow_root = (
                lab_root
                / "experiments"
                / experiment_id
                / "shadow"
            )
            self.assertEqual(
                len(list((shadow_root / "observations").glob("*.json"))),
                1,
            )
            self.assertEqual(
                len(list((shadow_root / "outcomes").glob("*.json"))),
                1,
            )

            with patch(
                "qnext_intelligence.lab_worker.MarketCoreHistoryClient",
                FakeHistoryClient,
            ):
                first_certification = process_request(
                    self.request(
                        "certify-shadow",
                        {"experimentId": experiment_id},
                    ),
                    storage_root=lab_root,
                    production_root=production_root,
                    market_core_url="http://market-core",
                )
                second_certification = process_request(
                    self.request(
                        "certify-shadow",
                        {"experimentId": experiment_id},
                    ),
                    storage_root=lab_root,
                    production_root=production_root,
                    market_core_url="http://market-core",
                )

            self.assertEqual(
                first_certification["result"]["experiment"]["lifecycle_state"],
                "SHADOW",
            )
            self.assertEqual(
                second_certification["result"]["experiment"]["lifecycle_state"],
                "SHADOW",
            )
            self.assertIn(
                "insufficient_shadow_samples",
                second_certification["result"]["evaluation"]["reasons"],
            )
            self.assertGreaterEqual(
                len(
                    list(
                        (
                            lab_root
                            / "experiments"
                            / experiment_id
                            / "results"
                            / "shadow-evaluations"
                        ).glob("*.json")
                    )
                ),
                1,
            )
            self.assertFalse(
                (production_root / "models" / "production.json").exists()
            )

    def test_missing_enabled_indicator_features_block_backtested_state(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            lab_root = root / "storage" / "intelligence-lab"
            production_root = root / "storage" / "intelligence"

            snapshot = chart_snapshot()
            snapshot["indicators"].append(
                {
                    "instance_id": "order-block-1",
                    "title": "Order Block",
                    "kind": "script",
                    "source_hash": "e" * 64,
                    "configuration_hash": "f" * 64,
                    "inputs": {},
                    "historical_feature_names": [],
                    "current_feature_names": [],
                }
            )

            imported = process_request(
                self.request("import", {"snapshot": snapshot}),
                storage_root=lab_root,
                production_root=production_root,
                market_core_url="http://market-core",
            )
            experiment_id = imported["result"]["experiment"]["experiment_id"]

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

            self.assertEqual(
                result["result"]["experiment"]["lifecycle_state"],
                "EXPERIMENT",
            )
            self.assertIn(
                "order-block-1",
                result["result"]["missing_indicators"],
            )
            self.assertIn(
                "missing_historical_indicator_features:order-block-1",
                result["result"]["evaluation"]["reasons"],
            )
            self.assertLess(
                result["result"]["evaluation"]["metrics"]["indicator_coverage_ratio"],
                1.0,
            )

    def test_feature_cap_reserves_one_feature_per_enabled_indicator(self):
        examples = []
        from qnext_intelligence.lab_snapshot import HistoricalLabExample

        for index in range(100):
            features = {
                f"indicator.alpha.plot.feature_{feature}": float(index + feature)
                for feature in range(110)
            }
            features["indicator.omega.plot.signal"] = float(index % 7)
            examples.append(
                HistoricalLabExample(
                    bar_time_ms=index * 60_000,
                    as_of_time_ms=(index + 1) * 60_000,
                    features=features,
                    origin_close=100.0 + index,
                    horizon_bars=1,
                    return_value=0.001,
                    mfe=0.002,
                    mae=-0.001,
                    mfe_bar=1,
                    mae_bar=1,
                    future_high_excursions=(0.002,),
                    future_low_excursions=(-0.001,),
                    direction_actual="UP",
                )
            )

        selected = _selected_feature_names(
            examples,
            indicator_ids=("alpha", "omega"),
            max_features=20,
        )
        self.assertEqual(len(selected), 20)
        self.assertTrue(any(name.startswith("indicator.alpha.") for name in selected))
        self.assertTrue(any(name.startswith("indicator.omega.") for name in selected))

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
