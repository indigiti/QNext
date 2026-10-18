from __future__ import annotations

import unittest

from qnext_intelligence.lab import LabExperiment
from qnext_intelligence.lab_advisory import build_certified_advisory
from qnext_intelligence.lab_snapshot import LabCurrentFeatureVector
from qnext_intelligence.learning import (
    LearningDataset,
    LearningExample,
    train_candidate,
)


def candidate_fixture():
    examples = []
    for index in range(120):
        up = index % 2 == 0
        examples.append(
            LearningExample(
                prediction_id=f"p-{index}",
                prediction_time_ms=1_000_000 + index * 60_000,
                feature_snapshot_hash=f"s-{index}",
                feature_set_version="advisory-test-v1",
                features={"indicator.test.signal": 1.0 if up else -1.0},
                actual_direction="UP" if up else "DOWN",
                return_value=0.01 if up else -0.01,
                champion_direction="FLAT",
            )
        )
    dataset = LearningDataset(
        examples=tuple(examples),
        feature_names=("indicator.test.signal",),
        feature_set_version="advisory-test-v1",
        dataset_hash="d" * 64,
    )
    return train_candidate(
        dataset,
        model_name="advisory-ridge",
        created_at_ms=10_000_000,
        min_samples=60,
        min_test_samples=12,
        min_average_return_improvement=-1.0,
        max_accuracy_regression=1.0,
        max_drawdown_slack=10.0,
    )


class CertifiedAdvisoryTests(unittest.TestCase):
    def test_builds_sanitized_certified_buy_advisory(self):
        candidate = candidate_fixture()
        experiment = LabExperiment(
            experiment_id="lab-certified-test",
            name="Certified test",
            instrument_id="NSE:NIFTY50",
            timeframe="1m",
            indicator_ids=("ema-1",),
            indicator_configuration_hash="a" * 64,
            feature_schema_version="qnext-chart-indicators-v2",
            created_at_ms=1_000_000,
            lifecycle_state="CERTIFIED",
        )
        current = LabCurrentFeatureVector(
            bar_time_ms=20_000_000,
            as_of_time_ms=20_060_000,
            features={"indicator.test.signal": 1.0},
            origin_close=100.0,
        )
        policy = {
            "recommendation_id": "rec-certified-1",
            "probability_calibrated": True,
            "calibration": {"temperature": 1.0},
            "decision_threshold": 0.40,
            "target_profiles": {
                "UP": {
                    "target1_pct": 0.01,
                    "target2_pct": 0.02,
                    "invalidation_pct": 0.01,
                    "expected_side_return": 0.012,
                    "expected_horizon_bars": 3.0,
                },
                "DOWN": {
                    "target1_pct": 0.01,
                    "target2_pct": 0.02,
                    "invalidation_pct": 0.01,
                    "expected_side_return": 0.011,
                    "expected_horizon_bars": 4.0,
                },
            },
        }

        advisory = build_certified_advisory(
            experiment=experiment,
            current=current,
            selected_model=candidate,
            recommendation_policy=policy,
            certified_at_ms=19_000_000,
            created_at_ms=20_070_000,
        )

        self.assertEqual(advisory.decision, "BUY")
        self.assertAlmostEqual(sum(advisory.probabilities.values()), 1.0, places=6)
        self.assertAlmostEqual(advisory.entry_price, 100.0)
        self.assertAlmostEqual(advisory.target1_price or 0.0, 101.0)
        self.assertAlmostEqual(advisory.target2_price or 0.0, 102.0)
        self.assertAlmostEqual(advisory.invalidation_price or 0.0, 99.0)
        self.assertAlmostEqual(advisory.expected_return_pct or 0.0, 0.012)
        self.assertEqual(advisory.expected_horizon_bars, 3.0)
        self.assertRegex(advisory.advisory_id, r"^[a-f0-9]{32}$")

        record = advisory.to_record()
        self.assertEqual(
            record["schema"],
            "QNEXT.INTELLIGENCE.CERTIFIED_ADVISORY/1",
        )
        self.assertNotIn("features", record)
        self.assertNotIn("model_payload", record)
        self.assertNotIn("coefficients", record)
        self.assertNotIn("order", record)
        self.assertNotIn("quantity", record)

    def test_rejects_non_certified_experiment(self):
        candidate = candidate_fixture()
        experiment = LabExperiment(
            experiment_id="lab-shadow-test",
            name="Shadow test",
            instrument_id="NSE:NIFTY50",
            timeframe="1m",
            indicator_ids=("ema-1",),
            indicator_configuration_hash="a" * 64,
            feature_schema_version="qnext-chart-indicators-v2",
            created_at_ms=1_000_000,
            lifecycle_state="SHADOW",
        )
        with self.assertRaisesRegex(ValueError, "CERTIFIED"):
            build_certified_advisory(
                experiment=experiment,
                current=LabCurrentFeatureVector(
                    bar_time_ms=20_000_000,
                    as_of_time_ms=20_060_000,
                    features={"indicator.test.signal": 1.0},
                    origin_close=100.0,
                ),
                selected_model=candidate,
                recommendation_policy={
                    "recommendation_id": "rec-1",
                    "probability_calibrated": True,
                    "calibration": {"temperature": 1.0},
                    "decision_threshold": 0.4,
                    "target_profiles": {},
                },
                certified_at_ms=19_000_000,
                created_at_ms=20_070_000,
            )


if __name__ == "__main__":
    unittest.main()
