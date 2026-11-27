import tempfile
import unittest
from pathlib import Path

from qnext_intelligence.store import ImmutableJSONLStore
from qnext_intelligence.domain import Bar
from qnext_intelligence.features import build_feature_vector


class ResearchPerformanceParityTests(unittest.TestCase):
    def test_streaming_duplicate_validation_preserves_corrupt_tail_priority(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root) / "records.jsonl"
            store = ImmutableJSONLStore(path, identity_field="id")
            record = {"id": "a", "value": 1}
            store.append(record)
            with self.assertRaisesRegex(ValueError, "immutable record already exists"):
                store.append(record)
            with self.assertRaisesRegex(ValueError, "identity collision"):
                store.append({"id": "a", "value": 2})
            with path.open("a", encoding="utf-8") as handle:
                handle.write("\nnot json\n")
            before = path.read_bytes()
            for candidate in (record, {"id": "b"}):
                with self.assertRaisesRegex(ValueError, "corrupt JSONL at line 3"):
                    store.append(candidate)
            self.assertEqual(path.read_bytes(), before)

    def test_feature_selection_preserves_stable_ties_and_excludes_future_forming(self):
        def bar(close_time, close, final=True):
            return Bar("NSE:NIFTY50", "1m", close_time - 1, close_time,
                       close, close + 1, close - 1, close, final=final)
        data = [bar(3, 103), bar(2, 101), bar(1, 100), bar(2, 102),
                bar(5, 999), bar(3, 777, False)]
        expected = [data[1], data[3], data[0]]
        actual = build_feature_vector(data, as_of_time_ms=3, lookback=3)
        self.assertEqual(actual, build_feature_vector(expected, as_of_time_ms=3, lookback=3))


if __name__ == "__main__":
    unittest.main()
