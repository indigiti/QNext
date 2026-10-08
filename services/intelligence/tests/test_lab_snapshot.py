from __future__ import annotations

import copy
import unittest

from qnext_intelligence.domain import Bar
from qnext_intelligence.lab_snapshot import (
    build_historical_examples,
    historical_dataset_hash,
    import_chart_snapshot,
)


def snapshot_payload():
    return {
        "schema": "QNEXT.INTELLIGENCE.LAB.CHART_SNAPSHOT/1",
        "feature_schema_version": "qnext-chart-indicators-v1",
        "created_at_ms": 1_800_000_120_000,
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
            },
            {
                "instance_id": "ob-1",
                "title": "Order Block",
                "kind": "script",
                "source_hash": "d" * 64,
                "configuration_hash": "e" * 64,
                "inputs": {},
            },
        ],
        "feature_rows": [
            {
                "bar_time_ms": 0,
                "features": {
                    "indicator.ema_1.plot.ema": 100.0,
                    "indicator.ob_1.var.trend": 1.0,
                },
            },
            {
                "bar_time_ms": 60_000,
                "features": {
                    "indicator.ema_1.plot.ema": 101.0,
                    "indicator.ob_1.var.trend": 1.0,
                },
            },
            {
                "bar_time_ms": 120_000,
                "features": {
                    "indicator.ema_1.plot.ema": 102.0,
                    "indicator.ob_1.var.trend": -1.0,
                },
            },
        ],
        "current_features": {
            "indicator.ema_1.plot.ema": 102.0,
            "indicator.ob_1.var.trend": -1.0,
        },
    }


def canonical_bars():
    closes = [100.0, 101.0, 99.0, 102.0, 104.0]
    result = []
    for index, close in enumerate(closes):
        result.append(
            Bar(
                instrument_id="QNEXT:NIFTY",
                timeframe="1m",
                open_time_ms=index * 60_000,
                close_time_ms=(index + 1) * 60_000,
                open=close,
                high=close + 1.0,
                low=close - 1.0,
                close=close,
                volume=1000 + index,
                final=True,
                quality="GOOD",
            )
        )
    return result


class IntelligenceLabSnapshotTests(unittest.TestCase):
    def test_import_builds_chart_clone_experiment(self):
        payload = snapshot_payload()
        payload["feature_rows"][0]["bar_time_ms"] = 1
        imported = import_chart_snapshot(payload)
        experiment = imported.experiment()

        self.assertEqual(imported.indicator_ids, ("ema-1", "ob-1"))
        self.assertEqual(experiment.source, "CHART_CLONE")
        self.assertEqual(experiment.instrument_id, "QNEXT:NIFTY")
        self.assertEqual(experiment.timeframe, "1m")
        self.assertEqual(experiment.lifecycle_state, "EXPERIMENT")
        self.assertRegex(imported.snapshot_hash, r"^[a-f0-9]{64}$")

    def test_rejects_duplicate_or_unsorted_feature_rows(self):
        payload = snapshot_payload()
        payload["feature_rows"][1]["bar_time_ms"] = payload["feature_rows"][0]["bar_time_ms"]
        with self.assertRaisesRegex(ValueError, "strictly increasing"):
            import_chart_snapshot(payload)

    def test_rejects_non_finite_features(self):
        payload = snapshot_payload()
        payload["feature_rows"][0]["bar_time_ms"] = 1
        payload["feature_rows"][0]["features"]["bad"] = float("nan")
        with self.assertRaisesRegex(ValueError, "finite"):
            import_chart_snapshot(payload)

    def test_historical_examples_use_only_future_bars_after_final_origin(self):
        payload = snapshot_payload()
        imported = import_chart_snapshot(payload)
        examples = build_historical_examples(imported, canonical_bars(), horizon_bars=2)

        self.assertEqual(len(examples), 3)
        first = examples[0]
        self.assertEqual(first.bar_time_ms, 0)
        self.assertEqual(first.as_of_time_ms, 60_000)
        self.assertEqual(first.origin_close, 100.0)
        self.assertAlmostEqual(first.return_value, -0.01)
        self.assertEqual(first.direction_actual, "DOWN")
        self.assertAlmostEqual(first.mfe, 0.02)
        self.assertAlmostEqual(first.mae, -0.02)
        self.assertEqual(first.mfe_bar, 1)
        self.assertEqual(first.mae_bar, 2)
        self.assertEqual(len(first.future_high_excursions), 2)
        self.assertEqual(len(first.future_low_excursions), 2)
        self.assertAlmostEqual(first.future_high_excursions[0], 0.02)
        self.assertAlmostEqual(first.future_high_excursions[1], 0.0)
        self.assertAlmostEqual(first.future_low_excursions[0], 0.0)
        self.assertAlmostEqual(first.future_low_excursions[1], -0.02)

        digest = historical_dataset_hash(imported, examples)
        self.assertRegex(digest, r"^[a-f0-9]{64}$")

    def test_forming_or_degraded_origin_is_not_used(self):
        imported = import_chart_snapshot(snapshot_payload())
        bars = canonical_bars()
        bars[0] = Bar(
            instrument_id=bars[0].instrument_id,
            timeframe=bars[0].timeframe,
            open_time_ms=bars[0].open_time_ms,
            close_time_ms=bars[0].close_time_ms,
            open=bars[0].open,
            high=bars[0].high,
            low=bars[0].low,
            close=bars[0].close,
            volume=bars[0].volume,
            final=False,
            quality="GOOD",
        )
        bars[1] = Bar(
            instrument_id=bars[1].instrument_id,
            timeframe=bars[1].timeframe,
            open_time_ms=bars[1].open_time_ms,
            close_time_ms=bars[1].close_time_ms,
            open=bars[1].open,
            high=bars[1].high,
            low=bars[1].low,
            close=bars[1].close,
            volume=bars[1].volume,
            final=True,
            quality="DEGRADED",
        )

        examples = build_historical_examples(imported, bars, horizon_bars=1)
        self.assertTrue(all(example.bar_time_ms >= 120_000 for example in examples))


if __name__ == "__main__":
    unittest.main()
