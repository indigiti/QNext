from __future__ import annotations

import unittest

from qnext_intelligence.domain import Bar
from qnext_intelligence.lab_shadow import (
    ShadowConfig,
    certification_gate,
    evaluate_shadow_observation,
    score_shadow_observation,
    summarize_shadow,
)
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
                feature_set_version="shadow-test-v1",
                features={"indicator.test.signal": 1.0 if up else -1.0},
                actual_direction="UP" if up else "DOWN",
                return_value=0.01 if up else -0.01,
                champion_direction="FLAT",
            )
        )
    dataset = LearningDataset(
        examples=tuple(examples),
        feature_names=("indicator.test.signal",),
        feature_set_version="shadow-test-v1",
        dataset_hash="d" * 64,
    )
    return train_candidate(
        dataset,
        model_name="shadow-ridge",
        created_at_ms=10_000_000,
        min_samples=60,
        min_test_samples=12,
        min_average_return_improvement=-1.0,
        max_accuracy_regression=1.0,
        max_drawdown_slack=10.0,
    )


def bar(index: int, close: float, *, high: float, low: float) -> Bar:
    return Bar(
        instrument_id="QNEXT:NIFTY",
        timeframe="1m",
        open_time_ms=index * 60_000,
        close_time_ms=(index + 1) * 60_000,
        open=close,
        high=high,
        low=low,
        close=close,
        volume=1000,
        final=True,
        quality="GOOD",
    )


class LabShadowTests(unittest.TestCase):
    def test_scores_and_evaluates_shadow_observation(self):
        candidate = candidate_fixture()
        current = LabCurrentFeatureVector(
            bar_time_ms=6_000_000,
            as_of_time_ms=6_060_000,
            features={"indicator.test.signal": 1.0},
            origin_close=100.0,
        )
        policy = {
            "calibration": {"temperature": 1.0},
            "decision_threshold": 0.40,
            "target_profiles": {
                "UP": {
                    "target1_pct": 0.01,
                    "target2_pct": 0.02,
                    "invalidation_pct": 0.01,
                },
                "DOWN": {
                    "target1_pct": 0.01,
                    "target2_pct": 0.02,
                    "invalidation_pct": 0.01,
                },
            },
        }

        observation = score_shadow_observation(
            experiment_id="lab-shadow-test",
            current=current,
            selected_model=candidate,
            recommendation_policy=policy,
            created_at_ms=7_000_000,
        )
        self.assertEqual(observation.decision, "BUY")
        self.assertAlmostEqual(sum(observation.probabilities.values()), 1.0, places=6)
        self.assertRegex(observation.observation_id, r"^[a-f0-9]{32}$")

        outcome = evaluate_shadow_observation(
            observation,
            future_bars=[
                bar(101, 100.5, high=101.2, low=99.6),
                bar(102, 102.0, high=102.2, low=100.2),
            ],
            evaluated_at_ms=8_000_000,
        )
        self.assertTrue(outcome.correct)
        self.assertGreater(outcome.strategy_return, 0.0)
        self.assertTrue(outcome.target1_before_invalidation)
        self.assertTrue(outcome.target2_before_invalidation)
        self.assertFalse(outcome.invalidation_before_target1)

        summary = summarize_shadow((observation,), (outcome,))
        self.assertEqual(summary.completed_samples, 1)
        self.assertEqual(summary.pending_samples, 0)
        self.assertEqual(summary.metrics.trades, 1)
        self.assertGreater(summary.metrics.accuracy, 0.99)
        self.assertGreater(summary.target1_before_invalidation, 0.99)

    def test_same_bar_shadow_target_stop_is_conservative(self):
        candidate = candidate_fixture()
        current = LabCurrentFeatureVector(
            bar_time_ms=6_000_000,
            as_of_time_ms=6_060_000,
            features={"indicator.test.signal": 1.0},
            origin_close=100.0,
        )
        observation = score_shadow_observation(
            experiment_id="lab-shadow-test",
            current=current,
            selected_model=candidate,
            recommendation_policy={
                "calibration": {"temperature": 1.0},
                "decision_threshold": 0.40,
                "target_profiles": {
                    "UP": {
                        "target1_pct": 0.01,
                        "target2_pct": 0.02,
                        "invalidation_pct": 0.01,
                    }
                },
            },
            created_at_ms=7_000_000,
        )
        outcome = evaluate_shadow_observation(
            observation,
            future_bars=[
                bar(101, 100.0, high=102.0, low=98.0),
            ],
            evaluated_at_ms=8_000_000,
        )
        self.assertFalse(outcome.target1_before_invalidation)
        self.assertTrue(outcome.invalidation_before_target1)

    def test_certification_gate_requires_live_evidence_quality(self):
        candidate = candidate_fixture()
        observations = []
        outcomes = []
        policy = {
            "calibration": {"temperature": 1.0},
            "decision_threshold": 0.40,
            "target_profiles": {
                "UP": {
                    "target1_pct": 0.005,
                    "target2_pct": 0.01,
                    "invalidation_pct": 0.02,
                }
            },
        }
        for index in range(12):
            observation = score_shadow_observation(
                experiment_id="lab-shadow-test",
                current=LabCurrentFeatureVector(
                    bar_time_ms=10_000_000 + index * 60_000,
                    as_of_time_ms=10_060_000 + index * 60_000,
                    features={"indicator.test.signal": 1.0},
                    origin_close=100.0,
                ),
                selected_model=candidate,
                recommendation_policy=policy,
                created_at_ms=20_000_000 + index,
            )
            observations.append(observation)
            outcomes.append(
                evaluate_shadow_observation(
                    observation,
                    future_bars=[
                        bar(201 + index, 101.0, high=101.5, low=99.8),
                    ],
                    evaluated_at_ms=30_000_000 + index,
                )
            )

        summary = summarize_shadow(observations, outcomes)
        passed, reasons = certification_gate(
            summary,
            backtest_metrics={
                "accuracy": 0.90,
                "average_strategy_return": 0.005,
                "max_drawdown": 0.01,
            },
            config=ShadowConfig(
                experiment_id="lab-shadow-test",
                started_at_ms=1,
                horizon_bars=1,
                min_samples=12,
                max_accuracy_regression=0.20,
                max_average_return_regression=0.01,
                max_drawdown_slack=0.05,
                max_brier=0.35,
                min_coverage=0.50,
                min_target1_before_invalidation=0.50,
            ),
        )
        self.assertTrue(passed, reasons)
        self.assertEqual(reasons, ())


if __name__ == "__main__":
    unittest.main()
