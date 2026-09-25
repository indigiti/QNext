"""QNext Strategy Lab deterministic replay and paper-execution core."""

from .engine import BacktestEngine, BacktestResult
from .execution import ExecutionConfig
from .lifecycle import (
    ImmutableLifecycleStore,
    PromotionDecision,
    PromotionEvidence,
    PromotionGate,
    StrategyRevision,
)
from .models import Bar, Signal
from .paper import ChartMarker, PaperEngine, PaperSnapshot
from .strategies import MovingAverageCross

__all__ = [
    "BacktestEngine",
    "BacktestResult",
    "ExecutionConfig",
    "Bar",
    "Signal",
    "PaperEngine",
    "PaperSnapshot",
    "ChartMarker",
    "MovingAverageCross",
    "StrategyRevision",
    "PromotionEvidence",
    "PromotionDecision",
    "PromotionGate",
    "ImmutableLifecycleStore",
]
