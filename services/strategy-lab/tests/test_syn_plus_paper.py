from qnext_strategy_lab import Bar
from qnext_strategy_lab.syn_plus_paper import SynPlusPaperConfig, SynPlusPaperSession


def bar(i: int, close: float, *, instrument: str = "QNEXT:NIFTY-SYN+", timeframe: str = "1m", quality: str = "GOOD", final: bool = True) -> Bar:
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


def test_syn_plus_session_is_paper_only_and_uses_next_bar_fill() -> None:
    session = SynPlusPaperSession(
        SynPlusPaperConfig(initial_capital=100_000, unit_size=1, fast=1, slow=2)
    )

    first = session.apply_bar(bar(0, 100.0))
    assert first.position_qty == 0
    assert first.pending_order is None

    second = session.apply_bar(bar(1, 101.0))
    assert second.position_qty == 0
    assert second.pending_order is not None
    assert len(second.fills) == 0

    third = session.apply_bar(bar(2, 102.0))
    assert third.position_qty == 1
    assert len(third.fills) == 1
    assert session.broker_execution_enabled is False


def test_syn_plus_session_rejects_wrong_symbol_or_timeframe() -> None:
    session = SynPlusPaperSession()

    try:
        session.apply_bar(bar(0, 100.0, instrument="QNEXT:NIFTY-SYN"))
        assert False, "expected wrong instrument to fail"
    except ValueError as exc:
        assert "wrong instrument" in str(exc)

    try:
        session.apply_bar(bar(0, 100.0, timeframe="5m"))
        assert False, "expected wrong timeframe to fail"
    except ValueError as exc:
        assert "wrong timeframe" in str(exc)


def test_syn_plus_session_inherits_q4_quality_gate() -> None:
    session = SynPlusPaperSession()

    try:
        session.apply_bar(bar(0, 100.0, quality="DEGRADED"))
        assert False, "expected degraded bar to fail"
    except ValueError as exc:
        assert "quality" in str(exc)

    try:
        session.apply_bar(bar(0, 100.0, final=False))
        assert False, "expected non-final bar to fail"
    except ValueError as exc:
        assert "final bars" in str(exc)
