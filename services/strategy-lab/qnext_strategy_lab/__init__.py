"""QNext Strategy Lab deterministic replay and paper-execution core."""

from .engine import BacktestEngine, BacktestResult
from .execution import ExecutionConfig
from .models import Bar, Signal
from .paper import ChartMarker, PaperEngine, PaperSnapshot
from .strategies import MovingAverageCross
from .syn_plus_paper import SynPlusPaperConfig, SynPlusPaperSession

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
    "SynPlusPaperConfig",
    "SynPlusPaperSession",
]
