from __future__ import annotations

import unittest

from qnext_intelligence.lab_recommendation import (
    _first_hit,
    build_recommendation,
)
from qnext_intelligence.lab_snapshot import (
    HistoricalLabExample,
    LabCurrentFeatureVector,
)
from qnext_intelligence.learning import (
    LearningDataset,
    LearningExample,
    train_candidate,
)


def recommendation_fixture(count: int = 120):
    learning = []
    historical = []
    for index in range(count):
        up = index % 2 == 0
        actual = "UP" if up else "DOWN"
        signal = 1.0 if up else -1.0
        prediction_time = 1_000_000 + index * 60_000
        return_value = 0.01 if up else -0.01

        learning.append(
            LearningExample(
                prediction_id=f"p-{index}",
                prediction_time_ms=prediction_time,
                feature_snapshot_hash=f"s-{index}",
                feature_set_version="lab-rec-v1",
                features={"indicator.test.signal": signal},
                actual_direction=actual,
                return_value=return_value,
                champion_direction="FLAT",
            )
        )

        if up:
            highs = (0.004, 0.012, 0.020)
            lows = (-0.003, -0.005, -0.004)
            mfe = 0.020
            mae = -0.005
            mfe_bar = 3
            mae_bar = 2
        else:
            highs = (0.003, 0.005, 0.004)
            lows = (-0.004, -0.012, -0.020)
            mfe = 0.005
            mae = -0.020
            mfe_bar = 2
            mae_bar = 3

        historical.append(
            HistoricalLabExample(
                bar_time_ms=prediction_time - 60_000,
                as_of_time_ms=prediction_time,
                features={"indicator.test.signal": signal},
                origin_close=100.0,
                horizon_bars=3,
                return_value=return_value,
                mfe=mfe,
                mae=mae,
                mfe_bar=mfe_bar,
                mae_bar=mae_bar,
                future_high_excursions=highs,
                future_low_excursions=lows,
                direction_actual=actual,
            )
        )

    dataset = LearningDataset(
        examples=tuple(learning),
        feature_names=("indicator.test.signal",),
        feature_set_version="lab-rec-v1",
        dataset_hash="d" * 64,
    )
    return dataset, tuple(historical)


class LabRecommendationTests(unittest.TestCase):
    def test_builds_calibrated_buy_recommendation_with_learned_levels(self):
        dataset, historical = recommendation_fixture()
        candidate = train_candidate(
            dataset,
            model_name="lab-rec-ridge",
            created_at_ms=10_000_000,
            min_samples=60,
            min_test_samples=12,
            min_average_return_improvement=-1.0,
            max_accuracy_regression=1.0,
            max_drawdown_slack=10.0,
        )
        current = LabCurrentFeatureVector(
            bar_time_ms=9_000_000,
            as_of_time_ms=9_060_000,
            features={"indicator.test.signal": 1.0},
            origin_close=100.0,
        )

        recommendation = build_recommendation(
            experiment_id="lab-test",
            dataset=dataset,
            historical_examples=historical,
            current=current,
            selected_model=candidate,
            created_at_ms=10_100_000,
        )

        self.assertTrue(recommendation.probability_calibrated)
        self.assertEqual(recommendation.decision, "BUY")
        self.assertGreater(recommendation.probabilities["BUY"], recommendation.probabilities["SELL"])
        self.assertIsNotNone(recommendation.target_profile)
        self.assertIsNotNone(recommendation.target1_price)
        self.assertIsNotNone(recommendation.target2_price)
        self.assertIsNotNone(recommendation.invalidation_price)
        assert recommendation.target1_price is not None
        assert recommendation.target2_price is not None
        assert recommendation.invalidation_price is not None
        self.assertGreater(recommendation.target1_price, recommendation.entry_price)
        self.assertGreaterEqual(recommendation.target2_price, recommendation.target1_price)
        self.assertLess(recommendation.invalidation_price, recommendation.entry_price)
        self.assertLessEqual(
            recommendation.calibration.validation_log_loss_after,
            recommendation.calibration.validation_log_loss_before + 1e-12,
        )
        self.assertGreaterEqual(recommendation.test_metrics.accuracy, 0.9)
        self.assertRegex(recommendation.recommendation_id, r"^[a-f0-9]{32}$")

        record = recommendation.to_record()
        self.assertEqual(record["schema"], "QNEXT.INTELLIGENCE.LAB.RECOMMENDATION/1")
        self.assertEqual(record["decision"], "BUY")
        self.assertTrue(record["probability_calibrated"])
        self.assertIn("UP", record["target_profiles"])
        self.assertIn("DOWN", record["target_profiles"])

    def test_same_bar_target_and_invalidation_is_conservatively_stopped(self):
        example = HistoricalLabExample(
            bar_time_ms=0,
            as_of_time_ms=60_000,
            features={"x": 1.0},
            origin_close=100.0,
            horizon_bars=1,
            return_value=0.0,
            mfe=0.02,
            mae=-0.02,
            mfe_bar=1,
            mae_bar=1,
            future_high_excursions=(0.02,),
            future_low_excursions=(-0.02,),
            direction_actual="FLAT",
        )

        outcome, bar = _first_hit(
            example,
            "UP",
            target_pct=0.01,
            invalidation_pct=0.01,
        )
        self.assertEqual(outcome, "INVALIDATION")
        self.assertEqual(bar, 1)


if __name__ == "__main__":
    unittest.main()
