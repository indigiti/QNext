from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol

from .execution import DeterministicMarketExecution, ExecutionConfig
from .models import (
    ALLOWED_QUALITY,
    Bar,
    ChartMarker,
    EquityPoint,
    Fill,
    Order,
    Signal,
)


class Strategy(Protocol):
    name: str
    version: str
    definition_hash: str

    def on_bar(self, history: list[Bar]) -> Signal | None: ...


@dataclass(frozen=True)
class PaperResult:
    session_id: str
    ending_equity: float
    net_pnl: float
    fees: float
    slippage_cost: float
    max_drawdown: float
    ignored_terminal_signal: bool
    fills: tuple[Fill, ...]
    equity_curve: tuple[EquityPoint, ...]
    chart_markers: tuple[ChartMarker, ...]


class PaperEngine:
    """Incremental paper engine sharing Q4 execution semantics with replay."""

    VERSION = "qnext-paper-v1"

    def __init__(
        self,
        strategy: Strategy,
        *,
        session_id: str,
        initial_capital: float = 100_000.0,
        unit_size: float = 1.0,
        execution_config: ExecutionConfig | None = None,
    ) -> None:
        if not session_id:
            raise ValueError("session_id is required")
        if initial_capital <= 0 or unit_size <= 0:
            raise ValueError("initial_capital and unit_size must be positive")
        self.strategy = strategy
        self.session_id = session_id
        self.initial_capital = float(initial_capital)
        self.unit_size = float(unit_size)
        self.execution = DeterministicMarketExecution(execution_config)
        self.cash = self.initial_capital
        self.position_qty = 0.0
        self.pending: Order | None = None
        self.history: list[Bar] = []
        self.fills: list[Fill] = []
        self.curve: list[EquityPoint] = []
        self.markers: list[ChartMarker] = []
        self.order_seq = 0
        self.fill_seq = 0
        self.closed = False

    def process_bar(self, bar: Bar) -> Signal | None:
        if self.closed:
            raise RuntimeError("paper session is finalized")
        self._validate_bar(bar)

        if self.pending is not None:
            self._execute_pending(bar)

        self._mark(bar)

        self.history.append(bar)
        signal = self.strategy.on_bar(self.history)
        if signal is None:
            return None
        if signal.signal_time_ms != bar.close_time_ms:
            raise ValueError("strategy signal time must equal current final bar close")

        target_qty = signal.target_direction * self.unit_size
        quantity_delta = target_qty - self.position_qty
        if abs(quantity_delta) >= 1e-12:
            self.order_seq += 1
            self.pending = Order(
                order_id=f"{self.session_id}-O{self.order_seq:06d}",
                instrument_id=bar.instrument_id,
                created_at_ms=bar.close_time_ms,
                quantity_delta=quantity_delta,
                signal_time_ms=signal.signal_time_ms,
                reason=signal.reason,
            )
        return signal

    def finalize(self, *, liquidate: bool = True) -> PaperResult:
        if self.closed:
            raise RuntimeError("paper session is already finalized")
        if not self.history:
            raise ValueError("cannot finalize an empty paper session")

        ignored_terminal_signal = self.pending is not None
        self.pending = None

        last = self.history[-1]
        if liquidate and abs(self.position_qty) > 1e-12:
            self.order_seq += 1
            terminal_order = Order(
                order_id=f"{self.session_id}-O{self.order_seq:06d}",
                instrument_id=last.instrument_id,
                created_at_ms=last.close_time_ms - 1,
                quantity_delta=-self.position_qty,
                signal_time_ms=last.close_time_ms - 1,
                reason="declared end-of-run liquidation",
            )
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
            self._execute_order(terminal_order, synthetic)
            self._mark(last)

        self.closed = True
        ending_equity = self.cash + self.position_qty * last.close
        fees = sum(fill.fees for fill in self.fills)
        slippage_cost = sum(fill.slippage_cost for fill in self.fills)
        return PaperResult(
            session_id=self.session_id,
            ending_equity=ending_equity,
            net_pnl=ending_equity - self.initial_capital,
            fees=fees,
            slippage_cost=slippage_cost,
            max_drawdown=self._max_drawdown(self.curve),
            ignored_terminal_signal=ignored_terminal_signal,
            fills=tuple(self.fills),
            equity_curve=tuple(self.curve),
            chart_markers=tuple(self.markers),
        )

    def _execute_pending(self, bar: Bar) -> None:
        order = self.pending
        if order is None:
            return
        self.pending = None
        self._execute_order(order, bar)

    def _execute_order(self, order: Order, bar: Bar) -> None:
        self.fill_seq += 1
        fill = self.execution.fill(
            order, bar, f"{self.session_id}-F{self.fill_seq:06d}"
        )
        self.cash -= fill.quantity_delta * fill.price
        self.cash -= fill.fees
        self.position_qty += fill.quantity_delta
        self.fills.append(fill)
        action = "BUY" if fill.quantity_delta > 0 else "SELL"
        self.markers.append(
            ChartMarker(
                marker_id=f"{self.session_id}-M{self.fill_seq:06d}",
                instrument_id=bar.instrument_id,
                time_ms=fill.filled_at_ms,
                action=action,
                price=fill.price,
                quantity_delta=fill.quantity_delta,
                label=f"{action} {abs(fill.quantity_delta):g} @ {fill.price:.4f}",
            )
        )

    def _mark(self, bar: Bar) -> None:
        equity = self.cash + self.position_qty * bar.close
        point = EquityPoint(
            time_ms=bar.close_time_ms,
            cash=self.cash,
            position_qty=self.position_qty,
            mark_price=bar.close,
            equity=equity,
        )
        if self.curve and self.curve[-1].time_ms == point.time_ms:
            self.curve[-1] = point
        else:
            self.curve.append(point)

    def _validate_bar(self, bar: Bar) -> None:
        if not bar.final:
            raise ValueError("paper engine requires final bars")
        if bar.quality not in ALLOWED_QUALITY:
            raise ValueError(f"bar quality not eligible for Q4 paper: {bar.quality}")
        if bar.close_time_ms <= bar.open_time_ms:
            raise ValueError("bar close time must be after open time")
        if not (
            bar.low
            <= min(bar.open, bar.close)
            <= max(bar.open, bar.close)
            <= bar.high
        ):
            raise ValueError("invalid OHLC envelope")

        if self.history:
            previous = self.history[-1]
            if bar.instrument_id != previous.instrument_id:
                raise ValueError("paper session cannot change instrument")
            if bar.timeframe != previous.timeframe:
                raise ValueError("paper session cannot change timeframe")
            if bar.open_time_ms <= previous.open_time_ms:
                raise ValueError("bars must be strictly ordered with no duplicates")

    @staticmethod
    def _max_drawdown(curve: list[EquityPoint]) -> float:
        if not curve:
            return 0.0
        peak = curve[0].equity
        worst = 0.0
        for point in curve:
            peak = max(peak, point.equity)
            if peak > 0:
                worst = max(worst, (peak - point.equity) / peak)
        return worst
