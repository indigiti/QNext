from __future__ import annotations

import tempfile
import unittest
from dataclasses import replace
from pathlib import Path

from qnext_intelligence.domain import Bar
from qnext_intelligence.engine import IntelligenceEngine
from qnext_intelligence.features import build_feature_vector
from qnext_intelligence.model import predict
from qnext_intelligence.outcomes import evaluate_outcome


def make_bars(count: int = 30, *, quality: str = "GOOD") -> list[Bar]:
    result = []
    price = 25000.0
    for i in range(count):
        close = price + (i * 4.0) + (2.0 if i % 2 else -1.0)
        result.append(
            Bar(
                instrument_id="QNEXT:NIFTY",
                timeframe="1m",
                time_ms=1_790_000_000_000 + i * 60_000,
                open=close - 1.0,
                high=close + 3.0,
                low=close - 3.0,
                close=close,
                volume=1000.0 + i * 10.0,
                final=True,
                revision=0,
                quality=quality,
                authority_provider="upstox",
            )
        )
    return result


class Q3CertificationTests(unittest.TestCase):
    def test_features_are_deterministic_and_leakage_safe(self) -> None:
        bars = make_bars()
        a = build_feature_vector(bars[:25], lookback=20)
        b = build_feature_vector(bars[:25], lookback=20)
        self.assertEqual(a.snapshot_hash, b.snapshot_hash)
        self.assertEqual(a.as_of_time_ms, bars[24].time_ms)
        with self.assertRaises(ValueError):
            build_feature_vector([*bars[:24], replace(bars[24], final=False)], lookback=20)

    def test_prediction_identity_is_stable(self) -> None:
        vector = build_feature_vector(make_bars()[:25], lookback=20)
        a = predict(vector, horizon_bars=3)
        b = predict(vector, horizon_bars=3)
        self.assertEqual(a.prediction_id, b.prediction_id)
        self.assertEqual(a.decision_context_hash, b.decision_context_hash)
        self.assertAlmostEqual(sum(a.probabilities.values()), 1.0)

    def test_degraded_quality_withholds_prediction(self) -> None:
        bars = make_bars(quality="DEGRADED")
        vector = build_feature_vector(bars[:25], lookback=20)
        prediction = predict(vector)
        self.assertEqual(vector.data_quality, "DEGRADED")
        self.assertEqual(prediction.state, "WITHHELD")

    def test_outcome_uses_only_future_horizon(self) -> None:
        bars = make_bars()
        vector = build_feature_vector(bars[:25], lookback=20)
        prediction = predict(vector, horizon_bars=3)
        outcome = evaluate_outcome(prediction, bars)
        self.assertEqual(outcome.evaluated_at_ms, bars[27].time_ms)
        self.assertGreater(outcome.mfe, outcome.mae)

    def test_append_only_persistence_rejects_mutation_by_duplicate_id(self) -> None:
        bars = make_bars()
        with tempfile.TemporaryDirectory() as tmp:
            engine = IntelligenceEngine(Path(tmp))
            _, prediction = engine.infer(bars[:25], lookback=20, horizon_bars=3)
            outcome = engine.attribute(prediction, bars)
            self.assertEqual(len(engine.prediction_store.read_all()), 1)
            self.assertEqual(len(engine.outcome_store.read_all()), 1)
            with self.assertRaises(ValueError):
                engine.infer(bars[:25], lookback=20, horizon_bars=3)
            with self.assertRaises(ValueError):
                engine.attribute(prediction, bars)
            self.assertEqual(outcome.prediction_id, prediction.prediction_id)


if __name__ == "__main__":
    unittest.main()
