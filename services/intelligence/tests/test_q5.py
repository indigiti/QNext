import tempfile
from dataclasses import replace
import unittest
from pathlib import Path

from qnext_intelligence.domain import Bar, ModelManifest
from qnext_intelligence.features import build_feature_vector
from qnext_intelligence.outcomes import evaluate_outcome
from qnext_intelligence.prediction import predict
from qnext_intelligence.shadow import evaluate_shadow, run_shadow
from qnext_intelligence.store import ImmutableJSONLStore


def bars(count=8, start=100.0, quality="GOOD"):
    result = []
    for i in range(count):
        close = start + i
        result.append(
            Bar(
                instrument_id="QNEXT:NIFTY",
                timeframe="1m",
                open_time_ms=i * 60_000,
                close_time_ms=(i + 1) * 60_000,
                open=close - 0.25,
                high=close + 0.5,
                low=close - 0.5,
                close=close,
                volume=1000 + i,
                quality=quality,
            )
        )
    return result


def manifest_for(features):
    return ModelManifest(
        model_name="linear-demo",
        model_version="1",
        lifecycle_state="PRODUCTION",
        feature_set_version=features.feature_set_version,
        dataset_hash="dataset",
        model_hash="model",
        algorithm="deterministic-test",
        training_window_start_ms=0,
        training_window_end_ms=features.as_of_time_ms - 1,
        created_at_ms=features.as_of_time_ms - 1,
    )


class Q5CertificationTests(unittest.TestCase):
    def test_features_never_consume_future_or_forming_bars(self):
        source = bars()
        future = Bar(
            instrument_id="QNEXT:NIFTY",
            timeframe="1m",
            open_time_ms=999_000,
            close_time_ms=1_059_000,
            open=500,
            high=510,
            low=490,
            close=505,
            volume=99_999,
            final=True,
        )
        forming = Bar(
            instrument_id="QNEXT:NIFTY",
            timeframe="1m",
            open_time_ms=480_000,
            close_time_ms=540_000,
            open=900,
            high=910,
            low=890,
            close=905,
            final=False,
        )
        as_of = source[-1].close_time_ms
        baseline = build_feature_vector(source, as_of_time_ms=as_of, lookback=5)
        contaminated = build_feature_vector(source + [future, forming], as_of_time_ms=as_of, lookback=5)
        self.assertEqual(baseline.snapshot_hash, contaminated.snapshot_hash)
        self.assertEqual(baseline.features, contaminated.features)

    def test_model_training_window_must_precede_inference(self):
        source = bars()
        features = build_feature_vector(source, as_of_time_ms=source[-1].close_time_ms, lookback=5)
        manifest = ModelManifest(
            model_name="linear-demo",
            model_version="1",
            lifecycle_state="CANDIDATE",
            feature_set_version=features.feature_set_version,
            dataset_hash="dataset",
            model_hash="model",
            algorithm="deterministic-test",
            training_window_start_ms=0,
            training_window_end_ms=features.as_of_time_ms,
            created_at_ms=features.as_of_time_ms,
        )
        with self.assertRaises(ValueError):
            predict(features, manifest, lambda _: {"UP": 1}, horizon_bars=2)

    def test_degraded_quality_is_withheld_without_running_model(self):
        source = bars(quality="DEGRADED")
        features = build_feature_vector(source, as_of_time_ms=source[-1].close_time_ms, lookback=5)

        def must_not_run(_):
            raise AssertionError("model should not run on degraded data")

        prediction = predict(features, manifest_for(features), must_not_run, horizon_bars=2)
        self.assertEqual(prediction.state, "WITHHELD")
        self.assertEqual(prediction.direction, "NONE")
        self.assertEqual(prediction.probabilities, {})
        with self.assertRaises(ValueError):
            evaluate_outcome(prediction, origin_close=source[-1].close, future_bars=[])

    def test_prediction_is_reproducible_and_outcome_is_horizon_bounded(self):
        source = bars()
        features = build_feature_vector(source, as_of_time_ms=source[-1].close_time_ms, lookback=5)
        manifest = manifest_for(features)
        model = lambda f: {"UP": 0.7, "DOWN": 0.2, "FLAT": 0.1}
        first = predict(features, manifest, model, horizon_bars=2, regime="TREND")
        second = predict(features, manifest, model, horizon_bars=2, regime="TREND")
        self.assertEqual(first.prediction_id, second.prediction_id)
        self.assertEqual(first.decision_context_hash, second.decision_context_hash)

        future = [
            Bar("QNEXT:NIFTY", "1m", 480_000, 540_000, 108, 110, 107, 109, 1000),
            Bar("QNEXT:NIFTY", "1m", 540_000, 600_000, 109, 112, 108, 111, 1000),
            Bar("QNEXT:NIFTY", "1m", 600_000, 660_000, 111, 200, 50, 150, 1000),
        ]
        outcome = evaluate_outcome(first, origin_close=source[-1].close, future_bars=future)
        self.assertEqual(outcome.evaluated_at_ms, 600_000)
        self.assertEqual(outcome.direction_actual, "UP")
        self.assertTrue(outcome.correct)
        self.assertLess(outcome.mfe, 0.10)

    def test_prediction_store_is_append_only(self):
        with tempfile.TemporaryDirectory() as tmp:
            store = ImmutableJSONLStore(Path(tmp) / "predictions.jsonl", identity_field="prediction_id")
            record = {"prediction_id": "p1", "state": "IMMUTABLE"}
            store.append(record)
            with self.assertRaises(ValueError):
                store.append(record)
            with self.assertRaises(ValueError):
                store.append({"prediction_id": "p1", "state": "MUTATED"})
            self.assertEqual(store.read_all(), [record])


    def test_decision_context_hash_binds_market_operating_context(self):
        source = bars()
        source[-1] = replace(source[-1], authority_provider="upstox")
        as_of = source[-1].close_time_ms
        first_features = build_feature_vector(
            source,
            as_of_time_ms=as_of,
            lookback=5,
            calendar_version="nse-2026-v1",
            session="regular",
            configuration_hash="config-a",
        )
        second_features = build_feature_vector(
            source,
            as_of_time_ms=as_of,
            lookback=5,
            calendar_version="nse-2026-v2",
            session="regular",
            configuration_hash="config-a",
        )
        first = predict(
            first_features,
            manifest_for(first_features),
            lambda _: {"UP": 1},
            horizon_bars=2,
            strategy_version="strategy-v1",
        )
        second = predict(
            second_features,
            manifest_for(second_features),
            lambda _: {"UP": 1},
            horizon_bars=2,
            strategy_version="strategy-v1",
        )
        self.assertNotEqual(first.decision_context_hash, second.decision_context_hash)
        self.assertEqual(first.authority_provider, "upstox")
        self.assertEqual(first.calendar_version, "nse-2026-v1")
        self.assertEqual(first.configuration_hash, "config-a")

    def test_candidate_runs_in_shadow_without_promotion(self):
        source = bars()
        features = build_feature_vector(
            source,
            as_of_time_ms=source[-1].close_time_ms,
            lookback=5,
            calendar_version="nse-2026-v1",
            session="regular",
            configuration_hash="config-a",
        )
        production = manifest_for(features)
        candidate = ModelManifest(
            model_name="candidate-demo",
            model_version="2",
            lifecycle_state="CANDIDATE",
            feature_set_version=features.feature_set_version,
            dataset_hash="candidate-dataset",
            model_hash="candidate-model",
            algorithm="deterministic-test",
            training_window_start_ms=0,
            training_window_end_ms=features.as_of_time_ms - 1,
            created_at_ms=features.as_of_time_ms - 1,
        )
        pair = run_shadow(
            features,
            production_manifest=production,
            production_model=lambda _: {"UP": 0.7, "DOWN": 0.2, "FLAT": 0.1},
            candidate_manifest=candidate,
            candidate_model=lambda _: {"DOWN": 0.7, "UP": 0.2, "FLAT": 0.1},
            horizon_bars=2,
            strategy_version="strategy-v1",
        )
        self.assertNotEqual(pair.production.prediction_id, pair.candidate.prediction_id)
        self.assertEqual(pair.candidate.model_version, "2")

        future = [
            Bar("QNEXT:NIFTY", "1m", 480_000, 540_000, 108, 110, 107, 109, 1000),
            Bar("QNEXT:NIFTY", "1m", 540_000, 600_000, 109, 112, 108, 111, 1000),
        ]
        comparison = evaluate_shadow(
            pair,
            origin_close=source[-1].close,
            future_bars=future,
        )
        self.assertTrue(comparison.production_correct)
        self.assertFalse(comparison.candidate_correct)
        self.assertEqual(comparison.candidate_correctness_delta, -1)


if __name__ == "__main__":
    unittest.main()
