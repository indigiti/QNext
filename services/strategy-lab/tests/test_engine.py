import tempfile
import unittest
from pathlib import Path

from qnext_strategy_lab.engine import BacktestEngine
from qnext_strategy_lab.execution import ExecutionConfig
from qnext_strategy_lab.models import Bar
from qnext_strategy_lab.paper import PaperEngine
from qnext_strategy_lab.storage import ImmutableRunStore
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


class BacktestEngineTest(unittest.TestCase):
    def test_deterministic_run_is_byte_stable(self):
        engine = BacktestEngine(
            initial_capital=100_000,
            unit_size=10,
            execution_config=ExecutionConfig(slippage_bps=2, fee_bps=1),
        )
        strategy = MovingAverageCross(fast=2, slow=3)
        first = engine.run(bars(), strategy)
        second = engine.run(bars(), strategy)
        self.assertEqual(first.run_id, second.run_id)
        self.assertEqual(first.dataset_hash, second.dataset_hash)
        self.assertEqual(first.result_hash, second.result_hash)
        self.assertEqual(first.summary(), second.summary())

    def test_signal_never_fills_on_same_bar(self):
        engine = BacktestEngine(unit_size=1)
        result = engine.run(bars(), MovingAverageCross(fast=2, slow=3))
        self.assertGreater(result.fill_count, 0)
        first_fill = result.fills[0]
        self.assertGreaterEqual(first_fill.filled_at_ms, bars()[3].open_time_ms)

    def test_non_final_bar_is_rejected(self):
        series = bars()
        series[2] = Bar(**{**series[2].canonical(), "final": False})
        with self.assertRaisesRegex(ValueError, "final bars"):
            BacktestEngine().run(series, MovingAverageCross())

    def test_degraded_bar_is_rejected(self):
        series = bars()
        series[2] = Bar(**{**series[2].canonical(), "quality": "DEGRADED"})
        with self.assertRaisesRegex(ValueError, "quality"):
            BacktestEngine().run(series, MovingAverageCross())

    def test_immutable_store_reuses_identical_run(self):
        result = BacktestEngine().run(bars(), MovingAverageCross())
        with tempfile.TemporaryDirectory() as tmp:
            store = ImmutableRunStore(tmp)
            first = store.write(result)
            second = store.write(result)
            self.assertEqual(first, second)
            self.assertTrue(Path(first).exists())

    def test_chart_markers_are_emitted_for_every_fill(self):
        result = BacktestEngine().run(bars(), MovingAverageCross())
        self.assertEqual(len(result.chart_markers), result.fill_count)
        self.assertTrue(result.chart_markers)
        for marker in result.chart_markers:
            self.assertIn(marker.action, {"BUY", "SELL"})
            self.assertGreater(marker.price, 0)
            self.assertTrue(marker.label)

    def test_replay_paper_parity(self):
        strategy = MovingAverageCross(fast=2, slow=3)
        execution = ExecutionConfig(slippage_bps=2, fee_bps=1)
        replay = BacktestEngine(
            initial_capital=100_000,
            unit_size=10,
            execution_config=execution,
        ).run(bars(), strategy)

        paper = PaperEngine(
            strategy,
            session_id=replay.run_id,
            initial_capital=100_000,
            unit_size=10,
            execution_config=execution,
        )
        for bar in bars():
            paper.process_bar(bar)
        paper_result = paper.finalize(liquidate=True)

        self.assertEqual(paper_result.fills, replay.fills)
        self.assertEqual(paper_result.equity_curve, replay.equity_curve)
        self.assertEqual(paper_result.chart_markers, replay.chart_markers)
        self.assertEqual(paper_result.ending_equity, replay.ending_equity)
        self.assertEqual(paper_result.net_pnl, replay.net_pnl)
        self.assertEqual(paper_result.fees, replay.fees)
        self.assertEqual(paper_result.slippage_cost, replay.slippage_cost)
        self.assertEqual(paper_result.max_drawdown, replay.max_drawdown)

    def test_paper_engine_is_incremental(self):
        strategy = MovingAverageCross(fast=2, slow=3)
        paper = PaperEngine(strategy, session_id="paper-test")
        series = bars()

        paper.process_bar(series[0])
        paper.process_bar(series[1])
        self.assertEqual(len(paper.fills), 0)

        paper.process_bar(series[2])
        self.assertIsNotNone(paper.pending)
        self.assertEqual(len(paper.fills), 0)

        paper.process_bar(series[3])
        self.assertGreater(len(paper.fills), 0)
        self.assertGreater(len(paper.markers), 0)


if __name__ == "__main__":
    unittest.main()
