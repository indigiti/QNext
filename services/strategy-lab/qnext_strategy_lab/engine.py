from __future__ import annotations

import hashlib
import json
from dataclasses import asdict, dataclass
from typing import Iterable, Protocol

from .execution import DeterministicMarketExecution, ExecutionConfig
from .models import ALLOWED_QUALITY, Bar, EquityPoint, Fill, Order, Signal


class Strategy(Protocol):
    name: str
    version: str
    definition_hash: str

    def on_bar(self, history: list[Bar]) -> Signal | None: ...


@dataclass(frozen=True)
class BacktestResult:
    run_id: str
    instrument_id: str
    timeframe: str
    strategy_name: str
    strategy_version: str
    strategy_definition_hash: str
    execution_model_version: str
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
    result_hash: str

    def summary(self) -> dict:
        value = asdict(self)
        value["fills"] = [asdict(fill) for fill in self.fills]
        value["equity_curve"] = [asdict(point) for point in self.equity_curve]
        return value


class BacktestEngine:
    VERSION = "qnext-backtest-v1"

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
        self.execution = DeterministicMarketExecution(execution_config)
        self.liquidate_on_end = liquidate_on_end

    def run(self, bars: Iterable[Bar], strategy: Strategy) -> BacktestResult:
        series = list(bars)
        self._validate_bars(series)
        dataset_hash = self._dataset_hash(series)
        run_seed = "|".join(
            [
                dataset_hash,
                strategy.definition_hash,
                self.execution.VERSION,
                f"{self.initial_capital:.8f}",
                f"{self.unit_size:.8f}",
            ]
        )
        run_id = hashlib.sha256(run_seed.encode()).hexdigest()[:24]

        cash = self.initial_capital
        position_qty = 0.0
        pending: Order | None = None
        fills: list[Fill] = []
        curve: list[EquityPoint] = []
        history: list[Bar] = []
        order_seq = 0
        fill_seq = 0
        ignored_terminal_signal = False

        for index, bar in enumerate(series):
            if pending is not None:
                fill_seq += 1
                fill = self.execution.fill(pending, bar, f"{run_id}-F{fill_seq:06d}")
                cash -= fill.quantity_delta * fill.price
                cash -= fill.fees
                position_qty += fill.quantity_delta
                fills.append(fill)
                pending = None

            equity = cash + position_qty * bar.close
            curve.append(
                EquityPoint(
                    time_ms=bar.close_time_ms,
                    cash=cash,
                    position_qty=position_qty,
                    mark_price=bar.close,
                    equity=equity,
                )
            )

            history.append(bar)
            signal = strategy.on_bar(history)
            if signal is None:
                continue
            if signal.signal_time_ms != bar.close_time_ms:
                raise ValueError("strategy signal time must equal current final bar close")

            target_qty = signal.target_direction * self.unit_size
            quantity_delta = target_qty - position_qty
            if abs(quantity_delta) < 1e-12:
                continue

            if index == len(series) - 1:
                ignored_terminal_signal = True
                continue

            order_seq += 1
            pending = Order(
                order_id=f"{run_id}-O{order_seq:06d}",
                instrument_id=bar.instrument_id,
                created_at_ms=bar.close_time_ms,
                quantity_delta=quantity_delta,
                signal_time_ms=signal.signal_time_ms,
                reason=signal.reason,
            )

        if self.liquidate_on_end and abs(position_qty) > 1e-12:
            last = series[-1]
            order_seq += 1
            terminal_order = Order(
                order_id=f"{run_id}-O{order_seq:06d}",
                instrument_id=last.instrument_id,
                created_at_ms=last.close_time_ms - 1,
                quantity_delta=-position_qty,
                signal_time_ms=last.close_time_ms - 1,
                reason="declared end-of-run liquidation",
            )
            fill_seq += 1
            # Explicit end-of-run liquidation uses the final close as its reference.
            synthetic = Bar(
                instrument_id=last.instrument_id,
                timeframe=last.timeframe,
                open_time_ms=last.close_time_ms,
                close_time_ms=last.close_time_ms + 1,
                open=last.close,
                high=last.close,
                low=last.close,
                close=last.close,
                final=True,
                quality=last.quality,
            )
            fill = self.execution.fill(terminal_order, synthetic, f"{run_id}-F{fill_seq:06d}")
            cash -= fill.quantity_delta * fill.price
            cash -= fill.fees
            position_qty += fill.quantity_delta
            fills.append(fill)
            curve.append(
                EquityPoint(
                    time_ms=last.close_time_ms,
                    cash=cash,
                    position_qty=position_qty,
                    mark_price=last.close,
                    equity=cash,
                )
            )

        ending_equity = cash + position_qty * series[-1].close
        fees = sum(fill.fees for fill in fills)
        slippage_cost = sum(fill.slippage_cost for fill in fills)
        net_pnl = ending_equity - self.initial_capital
        gross_pnl = net_pnl + fees + slippage_cost
        max_drawdown = self._max_drawdown(curve)

        payload = {
            "run_id": run_id,
            "dataset_hash": dataset_hash,
            "strategy_definition_hash": strategy.definition_hash,
            "execution_model_version": self.execution.VERSION,
            "ending_equity": round(ending_equity, 12),
            "net_pnl": round(net_pnl, 12),
            "fees": round(fees, 12),
            "slippage_cost": round(slippage_cost, 12),
            "fills": [asdict(fill) for fill in fills],
            "curve": [asdict(point) for point in curve],
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
            execution_model_version=self.execution.VERSION,
            dataset_hash=dataset_hash,
            initial_capital=self.initial_capital,
            ending_equity=ending_equity,
            net_pnl=net_pnl,
            gross_pnl_before_costs=gross_pnl,
            fees=fees,
            slippage_cost=slippage_cost,
            max_drawdown=max_drawdown,
            fill_count=len(fills),
            ignored_terminal_signal=ignored_terminal_signal,
            fills=tuple(fills),
            equity_curve=tuple(curve),
            result_hash=result_hash,
        )

    @staticmethod
    def _validate_bars(series: list[Bar]) -> None:
        if len(series) < 2:
            raise ValueError("at least two bars are required")
        instrument = series[0].instrument_id
        timeframe = series[0].timeframe
        previous_open = -1
        for bar in series:
            if not bar.final:
                raise ValueError("backtests require final bars")
            if bar.quality not in ALLOWED_QUALITY:
                raise ValueError(f"bar quality not eligible for Q4 replay: {bar.quality}")
            if bar.instrument_id != instrument or bar.timeframe != timeframe:
                raise ValueError("one run must use one instrument and timeframe")
            if bar.open_time_ms <= previous_open:
                raise ValueError("bars must be strictly ordered with no duplicates")
            if bar.close_time_ms <= bar.open_time_ms:
                raise ValueError("bar close time must be after open time")
            if not (bar.low <= min(bar.open, bar.close) <= max(bar.open, bar.close) <= bar.high):
                raise ValueError("invalid OHLC envelope")
            previous_open = bar.open_time_ms

    @staticmethod
    def _dataset_hash(series: list[Bar]) -> str:
        raw = "\n".join(
            json.dumps(bar.canonical(), sort_keys=True, separators=(",", ":"))
            for bar in series
        ).encode()
        return hashlib.sha256(raw).hexdigest()

    @staticmethod
    def _max_drawdown(curve: list[EquityPoint]) -> float:
        peak = curve[0].equity
        worst = 0.0
        for point in curve:
            peak = max(peak, point.equity)
            if peak > 0:
                worst = max(worst, (peak - point.equity) / peak)
        return worst
