from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from qnext_intelligence.learning import LearnedLinearModel, build_learning_dataset, train_candidate
from qnext_intelligence.registry import ModelRegistry


def make_records(count: int = 100):
    features = []
    predictions = []
    outcomes = []
    for i in range(count):
        snapshot = f"snap-{i}"
        prediction_id = f"pred-{i}"
        direction = "UP" if i % 2 == 0 else "DOWN"
        x = 1.0 if direction == "UP" else -1.0
        features.append(
            {
                "snapshot_hash": snapshot,
                "feature_set_version": "qnext-basic-v1",
                "instrument_id": "QNEXT:NIFTY-SYN",
                "timeframe": "1m",
                "as_of_time_ms": 1_000_000 + i * 60_000,
                "features": {
                    "return_1": x,
                    "return_window": x * 0.5,
                    "volatility_window": 0.01,
                    "close_vs_sma": x * 0.25,
                    "range_pct": 0.01,
                    "volume_log1p": 5.0,
                },
                "data_quality": "GOOD",
            }
        )
        predictions.append(
            {
                "prediction_id": prediction_id,
                "prediction_time_ms": 1_000_000 + i * 60_000,
                "feature_snapshot_hash": snapshot,
                "state": "IMMUTABLE",
                "direction": "FLAT",
            }
        )
        outcomes.append(
            {
                "prediction_id": prediction_id,
                "direction_actual": direction,
                "return_value": 0.01 if direction == "UP" else -0.01,
            }
        )
    return features, predictions, outcomes


class LearningWorkflowTests(unittest.TestCase):
    def test_train_candidate_is_time_ordered_and_certified(self):
        feature_records, prediction_records, outcome_records = make_records()
        dataset = build_learning_dataset(feature_records, prediction_records, outcome_records)
        candidate = train_candidate(
            dataset,
            created_at_ms=dataset.end_ms + 1_000,
            min_samples=60,
            min_test_samples=12,
        )
        self.assertEqual(candidate.lifecycle_state, "DRAFT")
        self.assertLess(candidate.training_window_end_ms, candidate.validation_window_start_ms)
        self.assertLess(candidate.validation_window_end_ms, candidate.test_window_start_ms)
        self.assertTrue(candidate.promotion_gate.passed)
        self.assertGreater(candidate.test_metrics.average_strategy_return, 0)
        self.assertGreater(candidate.test_metrics.accuracy, candidate.champion_test_metrics.accuracy)

        model = LearnedLinearModel(candidate)
        values = {
            "return_1": 1,
            "return_window": 0.5,
            "volatility_window": 0.01,
            "close_vs_sma": 0.25,
            "range_pct": 0.01,
            "volume_log1p": 5,
        }
        probabilities = model.probabilities(values)
        self.assertAlmostEqual(sum(probabilities.values()), 1.0)
        self.assertEqual(model.direction(values), "UP")

    def test_registry_requires_explicit_hash_and_supports_rollback(self):
        features, predictions, outcomes = make_records()
        dataset = build_learning_dataset(features, predictions, outcomes)
        first = train_candidate(
            dataset,
            model_name="m1",
            created_at_ms=dataset.end_ms + 1_000,
            min_samples=60,
            min_test_samples=12,
        )
        second = train_candidate(
            dataset,
            model_name="m2",
            created_at_ms=dataset.end_ms + 2_000,
            min_samples=60,
            min_test_samples=12,
        )

        with tempfile.TemporaryDirectory() as tmp:
            registry = ModelRegistry(Path(tmp))
            registry.save_candidate(first)
            registry.save_candidate(second)
            with self.assertRaises(ValueError):
                registry.promote(
                    first.candidate_id,
                    approved_by="ops",
                    expected_model_hash="wrong",
                    promoted_at_ms=dataset.end_ms + 3_000,
                )
            pointer = registry.promote(
                first.candidate_id,
                approved_by="ops",
                expected_model_hash=first.model_hash,
                promoted_at_ms=dataset.end_ms + 3_000,
            )
            self.assertEqual(pointer["candidate_id"], first.candidate_id)
            pointer = registry.promote(
                second.candidate_id,
                approved_by="ops",
                expected_model_hash=second.model_hash,
                promoted_at_ms=dataset.end_ms + 4_000,
            )
            self.assertEqual(pointer["previous_candidate_id"], first.candidate_id)
            pointer = registry.rollback(
                approved_by="ops",
                event_at_ms=dataset.end_ms + 5_000,
            )
            self.assertEqual(pointer["candidate_id"], first.candidate_id)
            self.assertEqual(len(registry.events.read_all()), 3)

    def test_incomplete_join_is_not_used_for_learning(self):
        features, predictions, outcomes = make_records(10)
        outcomes.pop()
        dataset = build_learning_dataset(features, predictions, outcomes)
        self.assertEqual(len(dataset.examples), 9)


if __name__ == "__main__":
    unittest.main()
