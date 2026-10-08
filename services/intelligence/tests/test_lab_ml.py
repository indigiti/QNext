from __future__ import annotations

import unittest

from qnext_intelligence.lab_ml import dependency_status, train_ml_challenger
from qnext_intelligence.learning import (
    EvaluationMetrics,
    LearningDataset,
    LearningExample,
)


_STATUS = dependency_status()
_ML_AVAILABLE = bool(
    _STATUS["scikit_learn"]["available"]
    and _STATUS["lightgbm"]["available"]
)


def dataset(count: int = 120) -> LearningDataset:
    examples = []
    for index in range(count):
        up = index % 2 == 0
        direction = "UP" if up else "DOWN"
        x = 1.0 if up else -1.0
        examples.append(
            LearningExample(
                prediction_id=f"p-{index}",
                prediction_time_ms=1_000_000 + index * 60_000,
                feature_snapshot_hash=f"s-{index}",
                feature_set_version="lab-ml-test-v1",
                features={
                    "indicator.test.signal": x,
                    "market.return_1": x * 0.001,
                },
                actual_direction=direction,
                return_value=0.01 if up else -0.01,
                champion_direction="FLAT",
            )
        )
    return LearningDataset(
        examples=tuple(examples),
        feature_names=("indicator.test.signal", "market.return_1"),
        feature_set_version="lab-ml-test-v1",
        dataset_hash="d" * 64,
    )


@unittest.skipUnless(_ML_AVAILABLE, "Lab ML optional dependencies are not installed")
class LabMLTests(unittest.TestCase):
    def test_tournament_trains_serializable_challenger(self):
        baseline = EvaluationMetrics(
            samples=24,
            accuracy=0.0,
            coverage=0.0,
            strategy_return=0.0,
            average_strategy_return=0.0,
            max_drawdown=0.0,
            trades=0,
        )
        candidate = train_ml_challenger(
            dataset(),
            ridge_validation_metrics=baseline,
            ridge_test_metrics=baseline,
            created_at_ms=9_000_000,
            min_test_samples=12,
            min_average_return_improvement=0.0,
            max_accuracy_regression=0.05,
            max_drawdown_slack=0.05,
        )

        self.assertIsNotNone(candidate)
        assert candidate is not None
        self.assertIn(candidate.family, {"scikit-learn", "lightgbm"})
        self.assertTrue(candidate.promotion_gate.passed)
        self.assertGreater(candidate.test_metrics.accuracy, 0.9)
        self.assertGreater(candidate.test_metrics.average_strategy_return, 0.0)
        self.assertFalse(candidate.probability_calibrated)

        record = candidate.to_record()
        self.assertEqual(record["schema"], "QNEXT.INTELLIGENCE.LAB.ML_CANDIDATE/1")
        self.assertRegex(record["model_hash"], r"^[a-f0-9]{64}$")
        self.assertIn(record["model_payload"]["type"], {
            "sklearn-logistic-v1",
            "lightgbm-booster-v1",
        })
        self.assertGreaterEqual(len(record["family_validation"]), 2)


class LabMLDependencyTests(unittest.TestCase):
    def test_dependency_status_is_fail_safe(self):
        status = dependency_status()
        self.assertIn("scikit_learn", status)
        self.assertIn("lightgbm", status)
        for value in status.values():
            self.assertIsInstance(value["available"], bool)
            self.assertIsInstance(value["version"], str)


if __name__ == "__main__":
    unittest.main()
