from __future__ import annotations

from dataclasses import dataclass

from .models import Bar, Fill, Order


@dataclass(frozen=True)
class ExecutionConfig:
    slippage_bps: float = 1.0
    fee_bps: float = 0.5

    def __post_init__(self) -> None:
        if self.slippage_bps < 0 or self.fee_bps < 0:
            raise ValueError("execution costs cannot be negative")


class DeterministicMarketExecution:
    """Deterministic next-bar-open fill model.

    The model deliberately does not use intrabar high/low to choose a favorable
    price. Orders produced after bar T closes are eligible only at bar T+1 open.
    """

    VERSION = "qnext-exec-market-v1"

    def __init__(self, config: ExecutionConfig | None = None) -> None:
        self.config = config or ExecutionConfig()

    def fill(self, order: Order, bar: Bar, fill_id: str) -> Fill:
        if order.created_at_ms >= bar.open_time_ms:
            raise ValueError("look-ahead violation: order must execute on a later bar")
        direction = 1.0 if order.quantity_delta > 0 else -1.0
        slip = self.config.slippage_bps / 10_000.0
        reference = bar.open
        price = reference * (1.0 + direction * slip)
        notional = abs(order.quantity_delta * price)
        fees = notional * self.config.fee_bps / 10_000.0
        slippage_cost = abs(order.quantity_delta) * abs(price - reference)
        return Fill(
            fill_id=fill_id,
            order_id=order.order_id,
            filled_at_ms=bar.open_time_ms,
            quantity_delta=order.quantity_delta,
            reference_price=reference,
            price=price,
            fees=fees,
            slippage_cost=slippage_cost,
        )
