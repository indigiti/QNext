import unittest

from qnext_intelligence.learning import HybridTrainingSetBuilder, TrainingObservation


def observation(
    observation_id: str,
    source: str,
    feature_time_ms: int,
    label_time_ms: int,
    label: float = 0.01,
):
    return TrainingObservation(
        observation_id=observation_id,
        source=source,
        instrument_id="QNEXT:NIFTY",
        timeframe="1m",
        feature_time_ms=feature_time_ms,
        label_time_ms=label_time_ms,
        features={"return_1": 0.1, "volatility": 0.2},
        label=label,
        provenance_hash=f"source-{observation_id}",
    )


class HybridTrainingSetTest(unittest.TestCase):
    def setUp(self):
        self.builder = HybridTrainingSetBuilder()

    def test_historical_and_mature_live_observations_are_combined(self):
        result = self.builder.build(
            [
                observation("h1", "HISTORICAL", 100, 120),
                observation("h2", "HISTORICAL", 220, 240),
                observation("l1", "LIVE", 320, 340),
            ],
            train_end_ms=200,
            validation_end_ms=300,
            test_end_ms=400,
            as_of_ms=400,
        )
        self.assertEqual([item.observation_id for item in result.train], ["h1"])
        self.assertEqual([item.observation_id for item in result.validation], ["h2"])
        self.assertEqual([item.observation_id for item in result.test], ["l1"])
        self.assertEqual(result.source_counts(), {"HISTORICAL": 2, "LIVE": 1})

    def test_boundary_crossing_label_is_purged(self):
        result = self.builder.build(
            [
                observation("safe", "HISTORICAL", 100, 150),
                observation("leak", "HISTORICAL", 190, 210),
            ],
            train_end_ms=200,
            validation_end_ms=300,
            test_end_ms=400,
            as_of_ms=400,
        )
        self.assertEqual([item.observation_id for item in result.train], ["safe"])
        self.assertEqual(result.excluded_count, 1)

    def test_dataset_hash_is_order_independent_and_deterministic(self):
        items = [
            observation("a", "HISTORICAL", 100, 120),
            observation("b", "LIVE", 320, 340),
        ]
        kwargs = dict(
            train_end_ms=200,
            validation_end_ms=300,
            test_end_ms=400,
            as_of_ms=400,
        )
        first = self.builder.build(items, **kwargs)
        second = self.builder.build(reversed(items), **kwargs)
        self.assertEqual(first.dataset_hash, second.dataset_hash)

    def test_duplicate_observation_is_rejected(self):
        item = observation("dup", "LIVE", 320, 340)
        with self.assertRaisesRegex(ValueError, "duplicate"):
            self.builder.build(
                [item, item],
                train_end_ms=200,
                validation_end_ms=300,
                test_end_ms=400,
                as_of_ms=400,
            )


if __name__ == "__main__":
    unittest.main()
