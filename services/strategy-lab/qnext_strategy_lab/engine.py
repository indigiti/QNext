from __future__ import annotations

import hashlib
import json
from dataclasses import asdict, dataclass
from typing import Iterable

from .execution import ExecutionConfig
from .models import Bar, ChartMarker, EquityPoint, Fill
from .paper import PaperEngine, Strategy


@dataclass(frozen=True)
class BacktestResult:
    run_id: str
    instrument_id: str
    timeframe: str
    strategy_name: str
    strategy_version: str
    strategy_definition_hash: str
    execution_model_version: str
    paper_engine_version: str
    dataset_hash: str
    initial_capital: float
    ending_equity: float
    net_pnl: float
    gross_pnl_before_costs: float
    fees: float
    slippage_cost: float
    max_drawdown: float
    fill_count: int
    ignored_terminal_signal: bool
    fills: tuple[Fill, ...]
    equity_curve: tuple[EquityPoint, ...]
    chart_markers: tuple[ChartMarker, ...]
    result_hash: str

    def summary(self) -> dict:
        value = asdict(self)
        value["fills"] = [asdict(fill) for fill in self.fills]
        value["equity_curve"] = [asdict(point) for point in self.equity_curve]
        value["chart_markers"] = [asdict(marker) for marker in self.chart_markers]
        return value


class BacktestEngine:
    VERSION = "qnext-backtest-v2"

    def __init__(
        self,
        *,
        initial_capital: float = 100_000.0,
        unit_size: float = 1.0,
        execution_config: ExecutionConfig | None = None,
        liquidate_on_end: bool = True,
    ) -> None:
        if initial_capital <= 0 or unit_size <= 0:
            raise ValueError("initial_capital and unit_size must be positive")
        self.initial_capital = float(initial_capital)
        self.unit_size = float(unit_size)
        self.execution_config = execution_config or ExecutionConfig()
        self.liquidate_on_end = liquidate_on_end

    def run(self, bars: Iterable[Bar], strategy: Strategy) -> BacktestResult:
        series = list(bars)
        if len(series) < 2:
            raise ValueError("at least two bars are required")

        dataset_hash = self._dataset_hash(series)
        run_seed = "|".join(
            [
                dataset_hash,
                strategy.definition_hash,
                "qnext-exec-market-v1",
                PaperEngine.VERSION,
                f"{self.initial_capital:.8f}",
                f"{self.unit_size:.8f}",
                f"{self.execution_config.slippage_bps:.8f}",
                f"{self.execution_config.fee_bps:.8f}",
                str(self.liquidate_on_end),
            ]
        )
        run_id = hashlib.sha256(run_seed.encode()).hexdigest()[:24]

        paper = PaperEngine(
            strategy,
            session_id=run_id,
            initial_capital=self.initial_capital,
            unit_size=self.unit_size,
            execution_config=self.execution_config,
        )
        for bar in series:
            paper.process_bar(bar)
        paper_result = paper.finalize(liquidate=self.liquidate_on_end)

        gross_pnl = (
            paper_result.net_pnl
            + paper_result.fees
            + paper_result.slippage_cost
        )
        payload = {
            "run_id": run_id,
            "dataset_hash": dataset_hash,
            "strategy_definition_hash": strategy.definition_hash,
            "execution_model_version": paper.execution.VERSION,
            "paper_engine_version": PaperEngine.VERSION,
            "ending_equity": round(paper_result.ending_equity, 12),
            "net_pnl": round(paper_result.net_pnl, 12),
            "fees": round(paper_result.fees, 12),
            "slippage_cost": round(paper_result.slippage_cost, 12),
            "fills": [asdict(fill) for fill in paper_result.fills],
            "curve": [asdict(point) for point in paper_result.equity_curve],
            "chart_markers": [
                asdict(marker) for marker in paper_result.chart_markers
            ],
        }
        result_hash = hashlib.sha256(
            json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()
        ).hexdigest()

        return BacktestResult(
            run_id=run_id,
            instrument_id=series[0].instrument_id,
            timeframe=series[0].timeframe,
            strategy_name=strategy.name,
            strategy_version=strategy.version,
            strategy_definition_hash=strategy.definition_hash,
            execution_model_version=paper.execution.VERSION,
            paper_engine_version=PaperEngine.VERSION,
            dataset_hash=dataset_hash,
            initial_capital=self.initial_capital,
            ending_equity=paper_result.ending_equity,
            net_pnl=paper_result.net_pnl,
            gross_pnl_before_costs=gross_pnl,
            fees=paper_result.fees,
            slippage_cost=paper_result.slippage_cost,
            max_drawdown=paper_result.max_drawdown,
            fill_count=len(paper_result.fills),
            ignored_terminal_signal=paper_result.ignored_terminal_signal,
            fills=paper_result.fills,
            equity_curve=paper_result.equity_curve,
            chart_markers=paper_result.chart_markers,
            result_hash=result_hash,
        )

    @staticmethod
    def _dataset_hash(series: list[Bar]) -> str:
        raw = "\n".join(
            json.dumps(bar.canonical(), sort_keys=True, separators=(",", ":"))
            for bar in series
        ).encode()
        return hashlib.sha256(raw).hexdigest()
