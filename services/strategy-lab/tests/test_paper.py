import unittest

from qnext_strategy_lab.engine import BacktestEngine
from qnext_strategy_lab.execution import ExecutionConfig
from qnext_strategy_lab.models import Bar
from qnext_strategy_lab.paper import PaperEngine
from qnext_strategy_lab.strategies import MovingAverageCross


def bars():
    closes = [100, 101, 102, 99, 98, 103, 104]
    result = []
    start = 1_000
    for i, close in enumerate(closes):
        open_price = closes[i - 1] if i else 100
        result.append(
            Bar(
                instrument_id="NSE:NIFTY50",
                timeframe="1m",
                open_time_ms=start + i * 60_000,
                close_time_ms=start + (i + 1) * 60_000,
                open=float(open_price),
                high=float(max(open_price, close) + 1),
                low=float(min(open_price, close) - 1),
                close=float(close),
                final=True,
                quality="GOOD",
            )
        )
    return result


class PaperEngineTest(unittest.TestCase):
    def test_paper_matches_backtest_through_same_final_bar(self):
        config = ExecutionConfig(slippage_bps=2, fee_bps=1)
        strategy = MovingAverageCross(fast=2, slow=3)
        paper = PaperEngine(
            strategy,
            initial_capital=100_000,
            unit_size=10,
            execution_config=config,
        )
        snapshot = None
        for bar in bars():
            snapshot = paper.apply(bar)
        self.assertIsNotNone(snapshot)

        backtest = BacktestEngine(
            initial_capital=100_000,
            unit_size=10,
            execution_config=config,
            liquidate_on_end=False,
        ).run(bars(), strategy)

        self.assertEqual(
            [(f.filled_at_ms, f.quantity_delta, f.price) for f in snapshot.fills],
            [(f.filled_at_ms, f.quantity_delta, f.price) for f in backtest.fills],
        )
        self.assertAlmostEqual(snapshot.equity, backtest.ending_equity, places=9)

    def test_signal_and_fill_markers_are_deterministic(self):
        paper = PaperEngine(MovingAverageCross(fast=2, slow=3))
        snapshot = None
        for bar in bars():
            snapshot = paper.apply(bar)
        self.assertIsNotNone(snapshot)
        self.assertTrue(any(marker.kind == "SIGNAL" for marker in snapshot.markers))
        self.assertTrue(any(marker.kind == "FILL" for marker in snapshot.markers))
        self.assertEqual(len({marker.marker_id for marker in snapshot.markers}), len(snapshot.markers))

    def test_paper_rejects_forming_or_degraded_bars(self):
        paper = PaperEngine(MovingAverageCross())
        first = bars()[0]
        with self.assertRaisesRegex(ValueError, "final bars"):
            paper.apply(Bar(**{**first.canonical(), "final": False}))
        with self.assertRaisesRegex(ValueError, "quality"):
            paper.apply(Bar(**{**first.canonical(), "quality": "DEGRADED"}))


if __name__ == "__main__":
    unittest.main()
