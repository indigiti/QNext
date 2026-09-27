from __future__ import annotations

import unittest

from qnext_strategy_lab import Bar
from qnext_strategy_lab.syn_plus_paper import SynPlusPaperConfig, SynPlusPaperSession


def bar(
    i: int,
    close: float,
    *,
    instrument: str = "QNEXT:NIFTY-SYN+",
    timeframe: str = "1m",
    quality: str = "GOOD",
    final: bool = True,
) -> Bar:
    open_time = i * 60_000
    return Bar(
        instrument_id=instrument,
        timeframe=timeframe,
        open_time_ms=open_time,
        close_time_ms=open_time + 60_000,
        open=close,
        high=close,
        low=close,
        close=close,
        final=final,
        quality=quality,
    )


class SynPlusPaperSessionTests(unittest.TestCase):
    def test_is_paper_only_and_uses_next_bar_fill(self) -> None:
        session = SynPlusPaperSession(
            SynPlusPaperConfig(initial_capital=100_000, unit_size=1, fast=1, slow=2)
        )

        first = session.apply_bar(bar(0, 100.0))
        self.assertEqual(first.position_qty, 0)
        self.assertIsNone(first.pending_order)

        second = session.apply_bar(bar(1, 101.0))
        self.assertEqual(second.position_qty, 0)
        self.assertIsNotNone(second.pending_order)
        self.assertEqual(len(second.fills), 0)

        third = session.apply_bar(bar(2, 102.0))
        self.assertEqual(third.position_qty, 1)
        self.assertEqual(len(third.fills), 1)
        self.assertFalse(session.broker_execution_enabled)

    def test_rejects_wrong_symbol_or_timeframe(self) -> None:
        session = SynPlusPaperSession()

        with self.assertRaisesRegex(ValueError, "wrong instrument"):
            session.apply_bar(bar(0, 100.0, instrument="QNEXT:NIFTY-SYN"))

        with self.assertRaisesRegex(ValueError, "wrong timeframe"):
            session.apply_bar(bar(0, 100.0, timeframe="5m"))

    def test_inherits_q4_quality_gate(self) -> None:
        session = SynPlusPaperSession()

        with self.assertRaisesRegex(ValueError, "quality"):
            session.apply_bar(bar(0, 100.0, quality="DEGRADED"))

        with self.assertRaisesRegex(ValueError, "final bars"):
            session.apply_bar(bar(0, 100.0, final=False))


if __name__ == "__main__":
    unittest.main()
