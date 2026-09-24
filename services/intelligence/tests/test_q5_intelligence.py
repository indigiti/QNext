from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from qnext_intelligence import (
    FeaturePoint, FeatureStore, ModelLifecycleError, ModelManifest, ModelRegistry,
    OutcomeError, OutcomeService, PredictionError, PredictionService,
)


class Q5IntelligenceTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.registry = ModelRegistry(self.root)
        self.manifest = ModelManifest(
            model_name="nifty-direction", model_version="1.0.0", feature_set_version="nifty-basic/1",
            dataset_hash="dataset-abc", model_hash="model-def", algorithm="fixture-linear",
            training_window_start_ms=1_700_000_000_000, training_window_end_ms=1_700_100_000_000,
            created_at_ms=1_700_200_000_000,
        )
        self.registry.register(self.manifest)

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def _snapshot(self):
        return FeatureStore(self.root).create_snapshot(
            feature_set_version="nifty-basic/1", instrument_id="NSE:NIFTY50", timeframe="1m",
            as_of_time_ms=1_700_300_000_000,
            features=[
                FeaturePoint("return_1", 0.001, 1_700_299_940_000),
                FeaturePoint("ema_gap", 12.5, 1_700_300_000_000),
            ],
        )

    def _promote(self) -> None:
        self.registry.validate(
            self.manifest.model_id, validated_at_ms=1_700_210_000_000,
            evidence={"passed": True, "dataset_hash": "dataset-abc", "metrics": {"accuracy": 0.61}},
        )
        self.registry.promote(
            self.manifest.model_id, promoted_at_ms=1_700_220_000_000, approved_by="q5-certification",
        )

    def test_feature_snapshot_rejects_future_information(self) -> None:
        with self.assertRaisesRegex(ValueError, "future information"):
            FeatureStore(self.root).create_snapshot(
                feature_set_version="nifty-basic/1", instrument_id="NSE:NIFTY50", timeframe="1m",
                as_of_time_ms=1_000, features=[FeaturePoint("leak", 1.0, 1_001)],
            )

    def test_feature_snapshot_hash_is_deterministic(self) -> None:
        first = self._snapshot()
        second = self._snapshot()
        self.assertEqual(first.snapshot_hash, second.snapshot_hash)
        self.assertIsNotNone(FeatureStore(self.root).get(first.snapshot_hash))

    def test_model_requires_validation_before_promotion(self) -> None:
        with self.assertRaisesRegex(ModelLifecycleError, "VALIDATED"):
            self.registry.promote(self.manifest.model_id, promoted_at_ms=1_700_220_000_000, approved_by="qa")
        self._promote()
        self.assertEqual(self.registry.state(self.manifest.model_id), "PRODUCTION")

    def test_prediction_requires_production_model_and_is_version_bound(self) -> None:
        snapshot = self._snapshot()
        service = PredictionService(self.root, self.registry)
        with self.assertRaisesRegex(PredictionError, "production model"):
            service.create(
                snapshot=snapshot, model_id=self.manifest.model_id, prediction_time_ms=snapshot.as_of_time_ms,
                direction="UP", probabilities={"UP": 0.6, "DOWN": 0.3, "FLAT": 0.1}, horizon_bars=5,
            )
        self._promote()
        prediction = service.create(
            snapshot=snapshot, model_id=self.manifest.model_id, prediction_time_ms=snapshot.as_of_time_ms,
            direction="UP", probabilities={"UP": 0.6, "DOWN": 0.3, "FLAT": 0.1}, horizon_bars=5, regime="TREND",
        )
        self.assertEqual(prediction.model_version, "1.0.0")
        self.assertEqual(prediction.feature_snapshot_hash, snapshot.snapshot_hash)
        self.assertEqual(prediction.state, "FINAL")

    def test_outcome_waits_for_horizon_and_attributes_result(self) -> None:
        self._promote()
        snapshot = self._snapshot()
        prediction = PredictionService(self.root, self.registry).create(
            snapshot=snapshot, model_id=self.manifest.model_id, prediction_time_ms=snapshot.as_of_time_ms,
            direction="UP", probabilities={"UP": 0.7, "DOWN": 0.2, "FLAT": 0.1}, horizon_bars=2,
        )
        outcomes = OutcomeService(self.root)
        with self.assertRaisesRegex(OutcomeError, "horizon"):
            outcomes.evaluate(
                prediction=prediction, entry_price=25_000, exit_price=25_050,
                evaluated_at_ms=prediction.prediction_time_ms + 60_000,
            )
        outcome = outcomes.evaluate(
            prediction=prediction, entry_price=25_000, exit_price=25_050,
            evaluated_at_ms=prediction.prediction_time_ms + 120_000, mfe=0.003, mae=-0.001,
        )
        self.assertTrue(outcome.correct)
        self.assertEqual(outcome.direction_actual, "UP")
        self.assertAlmostEqual(outcome.return_value, 0.002)


if __name__ == "__main__":
    unittest.main()
