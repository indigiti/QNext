from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from qnext_strategy_lab.syn_plus_runtime import SynPlusPaperRuntime


START = 1_700_000_000_000


def history_path(root: Path) -> Path:
    day = __import__("datetime").datetime.fromtimestamp(START / 1000, tz=__import__("datetime").timezone.utc)
    return (
        root
        / "market"
        / "QNEXT_NIFTY-SYN+"
        / "1m"
        / day.strftime("%Y")
        / day.strftime("%m")
        / (day.strftime("%Y-%m-%d") + ".jsonl")
    )


def write_bar(path: Path, index: int, close: float, *, revision: int = 0) -> None:
    open_time = START + index * 60_000
    payload = {
        "schema": "QNEXT.HISTORY.BAR/2",
        "instrument_id": "QNEXT:NIFTY-SYN+",
        "timeframe": "1m",
        "open_time_ms": open_time,
        "close_time_ms": open_time + 60_000,
        "open": close,
        "high": close,
        "low": close,
        "close": close,
        "volume": 0,
        "final": True,
        "revision": revision,
        "quality": "GOOD",
        "synthetic_version": "nifty-syn-plus-v1",
    }
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(payload) + "\n")


class SynPlusPaperRuntimeTests(unittest.TestCase):
    def test_live_history_is_persisted_and_replayed_after_restart(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            path = history_path(root)
            runtime = SynPlusPaperRuntime(root)
            status = runtime.control(
                {
                    "action": "enable",
                    "at_ms": START,
                    "fast": 1,
                    "slow": 2,
                    "initial_capital": 100_000,
                    "unit_size": 1,
                }
            )
            self.assertTrue(status["enabled"])
            self.assertFalse(status["broker_execution_enabled"])

            write_bar(path, 0, 100.0)
            write_bar(path, 1, 101.0)
            write_bar(path, 2, 102.0)
            self.assertEqual(runtime.poll_once(now_ms=START + 181_000), 3)

            status = runtime.status(poll=False)
            self.assertEqual(status["position_qty"], 1)
            self.assertEqual(len(status["fills"]), 1)
            self.assertGreater(len(status["markers"]), 1)
            session_key = status["session_key"]

            restored = SynPlusPaperRuntime(root)
            restored_status = restored.status(poll=False)
            self.assertTrue(restored_status["enabled"])
            self.assertEqual(restored_status["session_key"], session_key)
            self.assertEqual(restored_status["position_qty"], 1)
            self.assertEqual(len(restored_status["fills"]), 1)

            # A later correction of an already observed candle must not rewrite
            # the forward-paper path or duplicate a fill.
            write_bar(path, 1, 200.0, revision=1)
            self.assertEqual(restored.poll_once(now_ms=START + 182_000), 0)
            corrected_status = restored.status(poll=False)
            self.assertEqual(corrected_status["position_qty"], 1)
            self.assertEqual(len(corrected_status["fills"]), 1)

    def test_disable_then_enable_starts_a_fresh_forward_session(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            runtime = SynPlusPaperRuntime(root)
            first = runtime.control({"action": "enable", "at_ms": START})
            first_key = first["session_key"]
            runtime.control({"action": "disable"})
            second = runtime.control({"action": "enable", "at_ms": START + 600_000})

            self.assertTrue(second["enabled"])
            self.assertNotEqual(second["session_key"], first_key)
            self.assertEqual(second["position_qty"], 0)
            self.assertEqual(second["equity"], second["initial_capital"])

    def test_configuration_is_locked_while_running(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            runtime = SynPlusPaperRuntime(Path(temp))
            runtime.control({"action": "enable", "at_ms": START})
            with self.assertRaisesRegex(ValueError, "disable paper trading"):
                runtime.configure({"timeframe": "5m"})


if __name__ == "__main__":
    unittest.main()
