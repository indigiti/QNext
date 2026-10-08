from __future__ import annotations

from dataclasses import dataclass
import math
from typing import Any, Mapping, Sequence

from .domain import Bar, stable_hash
from .lab import LabExperiment, create_experiment

LAB_CHART_SNAPSHOT_SCHEMA = "QNEXT.INTELLIGENCE.LAB.CHART_SNAPSHOT/1"
USABLE_QUALITY = {"GOOD", "RECOVERED"}


@dataclass(frozen=True, slots=True)
class LabFeatureRow:
    bar_time_ms: int
    features: Mapping[str, float]


@dataclass(frozen=True, slots=True)
class ImportedChartSnapshot:
    instrument_id: str
    timeframe: str
    created_at_ms: int
    feature_schema_version: str
    indicator_configuration_hash: str
    indicator_ids: tuple[str, ...]
    feature_rows: tuple[LabFeatureRow, ...]
    snapshot_hash: str

    def experiment(self, *, name: str | None = None) -> LabExperiment:
        label = name or f"{self.instrument_id} {self.timeframe} chart indicators"
        return create_experiment(
            name=label,
            instrument_id=self.instrument_id,
            timeframe=self.timeframe,
            indicator_ids=self.indicator_ids,
            indicator_configuration_hash=self.indicator_configuration_hash,
            feature_schema_version=self.feature_schema_version,
            created_at_ms=self.created_at_ms,
            source="CHART_CLONE",
        )


@dataclass(frozen=True, slots=True)
class HistoricalLabExample:
    bar_time_ms: int
    as_of_time_ms: int
    features: Mapping[str, float]
    origin_close: float
    horizon_bars: int
    return_value: float
    mfe: float
    mae: float
    direction_actual: str

    def to_record(self) -> dict[str, Any]:
        return {
            "bar_time_ms": self.bar_time_ms,
            "as_of_time_ms": self.as_of_time_ms,
            "features": dict(self.features),
            "origin_close": self.origin_close,
            "horizon_bars": self.horizon_bars,
            "return_value": self.return_value,
            "mfe": self.mfe,
            "mae": self.mae,
            "direction_actual": self.direction_actual,
        }


def import_chart_snapshot(payload: Mapping[str, Any]) -> ImportedChartSnapshot:
    if str(payload.get("schema", "")) != LAB_CHART_SNAPSHOT_SCHEMA:
        raise ValueError("unsupported chart snapshot schema")

    instrument_id = _required_text(payload, "instrument_id")
    timeframe = _required_text(payload, "timeframe")
    feature_schema_version = _required_text(payload, "feature_schema_version")
    configuration_hash = _required_hash(payload, "indicator_configuration_hash")
    created_at_ms = int(payload.get("created_at_ms", 0))
    if created_at_ms <= 0:
        raise ValueError("created_at_ms must be positive")

    raw_indicators = payload.get("indicators")
    if not isinstance(raw_indicators, list) or not raw_indicators:
        raise ValueError("chart snapshot must contain enabled indicators")

    indicator_ids: list[str] = []
    for raw in raw_indicators:
        if not isinstance(raw, Mapping):
            raise ValueError("invalid indicator descriptor")
        instance_id = _required_text(raw, "instance_id")
        _required_text(raw, "title")
        _required_hash(raw, "source_hash")
        _required_hash(raw, "configuration_hash")
        if instance_id in indicator_ids:
            raise ValueError("duplicate indicator instance id")
        indicator_ids.append(instance_id)

    raw_rows = payload.get("feature_rows")
    if not isinstance(raw_rows, list) or not raw_rows:
        raise ValueError("chart snapshot has no historical feature rows")

    rows: list[LabFeatureRow] = []
    previous_time = -1
    for raw in raw_rows:
        if not isinstance(raw, Mapping):
            raise ValueError("invalid chart feature row")
        bar_time_ms = int(raw.get("bar_time_ms", 0))
        if bar_time_ms <= previous_time:
            raise ValueError("chart feature rows must be strictly increasing")
        features = _feature_record(raw.get("features"))
        if not features:
            raise ValueError("chart feature row must contain at least one feature")
        rows.append(LabFeatureRow(bar_time_ms=bar_time_ms, features=features))
        previous_time = bar_time_ms

    normalized_material = {
        "schema": LAB_CHART_SNAPSHOT_SCHEMA,
        "feature_schema_version": feature_schema_version,
        "created_at_ms": created_at_ms,
        "instrument_id": instrument_id,
        "timeframe": timeframe,
        "indicator_configuration_hash": configuration_hash,
        "indicator_ids": indicator_ids,
        "feature_rows": [
            {"bar_time_ms": row.bar_time_ms, "features": dict(sorted(row.features.items()))}
            for row in rows
        ],
    }

    return ImportedChartSnapshot(
        instrument_id=instrument_id,
        timeframe=timeframe,
        created_at_ms=created_at_ms,
        feature_schema_version=feature_schema_version,
        indicator_configuration_hash=configuration_hash,
        indicator_ids=tuple(indicator_ids),
        feature_rows=tuple(rows),
        snapshot_hash=stable_hash(normalized_material),
    )


def build_historical_examples(
    snapshot: ImportedChartSnapshot,
    bars: Sequence[Bar],
    *,
    horizon_bars: int,
) -> tuple[HistoricalLabExample, ...]:
    if horizon_bars < 1:
        raise ValueError("horizon_bars must be >= 1")

    ordered = sorted(bars, key=lambda bar: bar.open_time_ms)
    if not ordered:
        raise ValueError("historical evaluation requires canonical bars")

    previous_open = -1
    bars_by_open: dict[int, Bar] = {}
    for bar in ordered:
        bar.validate()
        if bar.instrument_id != snapshot.instrument_id or bar.timeframe != snapshot.timeframe:
            raise ValueError("historical bars do not match chart snapshot market")
        if bar.open_time_ms <= previous_open:
            raise ValueError("historical bars must be strictly increasing")
        bars_by_open[bar.open_time_ms] = bar
        previous_open = bar.open_time_ms

    row_by_open = {row.bar_time_ms: row for row in snapshot.feature_rows}
    examples: list[HistoricalLabExample] = []

    for index, origin in enumerate(ordered):
        row = row_by_open.get(origin.open_time_ms)
        if row is None:
            continue
        if not origin.final or origin.quality.upper() not in USABLE_QUALITY:
            continue
        future = [
            bar
            for bar in ordered[index + 1:index + 1 + horizon_bars]
            if bar.final and bar.quality.upper() in USABLE_QUALITY
        ]
        if len(future) != horizon_bars:
            continue

        # Feature rows are keyed to the origin bar. The decision becomes eligible
        # only after that canonical bar has finalized; no future bar is included.
        as_of_time_ms = origin.close_time_ms
        terminal = future[-1].close
        return_value = (terminal / origin.close) - 1.0
        mfe = max((bar.high / origin.close) - 1.0 for bar in future)
        mae = min((bar.low / origin.close) - 1.0 for bar in future)
        direction = "UP" if terminal > origin.close else "DOWN" if terminal < origin.close else "FLAT"

        examples.append(
            HistoricalLabExample(
                bar_time_ms=origin.open_time_ms,
                as_of_time_ms=as_of_time_ms,
                features=dict(sorted(row.features.items())),
                origin_close=origin.close,
                horizon_bars=horizon_bars,
                return_value=return_value,
                mfe=mfe,
                mae=mae,
                direction_actual=direction,
            )
        )

    if not examples:
        raise ValueError("no complete leakage-safe historical examples are available")
    return tuple(examples)


def historical_dataset_hash(
    snapshot: ImportedChartSnapshot,
    examples: Sequence[HistoricalLabExample],
) -> str:
    return stable_hash({
        "snapshot_hash": snapshot.snapshot_hash,
        "instrument_id": snapshot.instrument_id,
        "timeframe": snapshot.timeframe,
        "indicator_configuration_hash": snapshot.indicator_configuration_hash,
        "feature_schema_version": snapshot.feature_schema_version,
        "examples": [example.to_record() for example in examples],
    })


def _feature_record(value: Any) -> dict[str, float]:
    if not isinstance(value, Mapping):
        raise ValueError("features must be an object")
    result: dict[str, float] = {}
    for raw_name, raw_value in value.items():
        name = str(raw_name).strip()
        if not name or len(name) > 240:
            raise ValueError("invalid feature name")
        if isinstance(raw_value, bool) or not isinstance(raw_value, (int, float)):
            raise ValueError("feature values must be finite numbers")
        number = float(raw_value)
        if not math.isfinite(number):
            raise ValueError("feature values must be finite numbers")
        result[name] = number
    return dict(sorted(result.items()))


def _required_text(record: Mapping[str, Any], key: str) -> str:
    value = record.get(key)
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{key} is required")
    return value.strip()


def _required_hash(record: Mapping[str, Any], key: str) -> str:
    value = _required_text(record, key)
    if len(value) != 64 or any(char not in "0123456789abcdef" for char in value):
        raise ValueError(f"{key} must be a SHA-256 hash")
    return value
