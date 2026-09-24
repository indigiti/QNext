import argparse
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from qnext_intelligence.cli import run
from qnext_intelligence.client import MarketCoreHistoryClient, timeframe_to_milliseconds
from qnext_intelligence.domain import Bar


class FakeResponse:
    def __init__(self, payload):
        self._payload = json.dumps(payload).encode("utf-8")
        self.closed = False

    def read(self):
        return self._payload

    def close(self):
        self.closed = True


class HistoryClientTests(unittest.TestCase):
    def test_timeframe_parser(self):
        self.assertEqual(timeframe_to_milliseconds("15s"), 15_000)
        self.assertEqual(timeframe_to_milliseconds("1m"), 60_000)
        self.assertEqual(timeframe_to_milliseconds("3m"), 180_000)
        self.assertEqual(timeframe_to_milliseconds("1h"), 3_600_000)
        with self.assertRaises(ValueError):
            timeframe_to_milliseconds("0m")

    def test_market_core_response_maps_to_canonical_bars(self):
        payload = {
            "bars": [
                {
                    "time": 60_000,
                    "open": 100,
                    "high": 102,
                    "low": 99,
                    "close": 101,
                    "volume": 12,
                    "final": True,
                    "revision": 0,
                    "quality": "GOOD",
                    "authority_provider": "upstox",
                },
                {
                    "time": 120_000,
                    "open": 101,
                    "high": 103,
                    "low": 100,
                    "close": 102,
                    "volume": 14,
                    "final": True,
                    "revision": 0,
                    "quality": "GOOD",
                    "authority_provider": "upstox",
                },
            ]
        }
        response = FakeResponse(payload)

        def opener(request, timeout):
            self.assertIn("/api/v1/bars?", request.full_url)
            self.assertEqual(timeout, 5.0)
            return response

        client = MarketCoreHistoryClient("http://market-core", timeout=5.0, opener=opener)
        bars = client.fetch_bars(
            instrument_id="QNEXT:NIFTY",
            timeframe="1m",
            from_ms=0,
            to_ms=180_000,
        )
        self.assertEqual(len(bars), 2)
        self.assertEqual(bars[0].close_time_ms, 120_000)
        self.assertEqual(bars[1].close, 102.0)
        self.assertTrue(response.closed)

    def test_duplicate_or_unsorted_bars_fail_closed(self):
        payload = {
            "bars": [
                {"time": 60_000, "open": 100, "high": 101, "low": 99, "close": 100, "final": True},
                {"time": 60_000, "open": 100, "high": 101, "low": 99, "close": 100, "final": True},
            ]
        }
        client = MarketCoreHistoryClient(
            "http://market-core",
            opener=lambda request, timeout: FakeResponse(payload),
        )
        with self.assertRaises(ValueError):
            client.fetch_bars(
                instrument_id="QNEXT:NIFTY",
                timeframe="1m",
                from_ms=0,
                to_ms=180_000,
            )


class CLITests(unittest.TestCase):
    def test_run_persists_feature_and_prediction_records(self):
        source = []
        for i in range(8):
            close = 100.0 + i
            source.append(
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
                )
            )

        fake_client = unittest.mock.Mock()
        fake_client.fetch_bars.return_value = source

        with tempfile.TemporaryDirectory() as tmp, patch(
            "qnext_intelligence.cli.MarketCoreHistoryClient",
            return_value=fake_client,
        ):
            args = argparse.Namespace(
                market_core_url="http://market-core",
                instrument_id="QNEXT:NIFTY",
                timeframe="1m",
                from_ms=0,
                to_ms=600_000,
                storage_root=tmp,
                lookback=5,
                horizon_bars=3,
            )
            record = run(args)
            self.assertEqual(record["instrument_id"], "QNEXT:NIFTY")
            self.assertIn(record["state"], {"IMMUTABLE", "WITHHELD"})
            self.assertTrue((Path(tmp) / "features.jsonl").exists())
            self.assertTrue((Path(tmp) / "predictions.jsonl").exists())


if __name__ == "__main__":
    unittest.main()
