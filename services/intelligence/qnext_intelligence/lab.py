from __future__ import annotations

from dataclasses import asdict, dataclass, replace
import json
import os
from pathlib import Path
import re
import time
from typing import Any, Mapping, Sequence

from .domain import stable_hash

LAB_SCHEMA = "QNEXT.INTELLIGENCE.LAB/1"
LAB_STATES = {"EXPERIMENT", "BACKTESTED", "SHADOW", "CERTIFIED", "RETIRED"}
_TRANSITIONS = {
    "EXPERIMENT": {"BACKTESTED", "RETIRED"},
    "BACKTESTED": {"SHADOW", "RETIRED"},
    "SHADOW": {"CERTIFIED", "RETIRED"},
    "CERTIFIED": {"RETIRED"},
    "RETIRED": set(),
}
_ID_RE = re.compile(r"^[A-Za-z0-9._:-]{1,96}$")


@dataclass(frozen=True, slots=True)
class LabExperiment:
    experiment_id: str
    name: str
    instrument_id: str
    timeframe: str
    indicator_ids: tuple[str, ...]
    indicator_configuration_hash: str
    feature_schema_version: str
    created_at_ms: int
    lifecycle_state: str = "EXPERIMENT"
    source: str = "MANUAL"
    notes: str = ""

    def to_record(self) -> dict[str, Any]:
        record = asdict(self)
        record["schema"] = LAB_SCHEMA
        record["indicator_ids"] = list(self.indicator_ids)
        return record

    @classmethod
    def from_record(cls, record: Mapping[str, Any]) -> "LabExperiment":
        if str(record.get("schema", "")) != LAB_SCHEMA:
            raise ValueError("unsupported intelligence lab schema")
        state = str(record.get("lifecycle_state", ""))
        if state not in LAB_STATES:
            raise ValueError("invalid intelligence lab lifecycle state")
        return cls(
            experiment_id=str(record["experiment_id"]),
            name=str(record["name"]),
            instrument_id=str(record["instrument_id"]),
            timeframe=str(record["timeframe"]),
            indicator_ids=tuple(str(value) for value in record.get("indicator_ids", [])),
            indicator_configuration_hash=str(record["indicator_configuration_hash"]),
            feature_schema_version=str(record["feature_schema_version"]),
            created_at_ms=int(record["created_at_ms"]),
            lifecycle_state=state,
            source=str(record.get("source", "MANUAL")),
            notes=str(record.get("notes", "")),
        )


@dataclass(frozen=True, slots=True)
class LabEvaluation:
    experiment_id: str
    phase: str
    evaluated_at_ms: int
    dataset_hash: str
    model_spec_hash: str
    gate_passed: bool
    metrics: Mapping[str, float]
    reasons: tuple[str, ...] = ()

    def to_record(self) -> dict[str, Any]:
        return {
            "schema": "QNEXT.INTELLIGENCE.LAB.EVALUATION/1",
            "experiment_id": self.experiment_id,
            "phase": self.phase,
            "evaluated_at_ms": self.evaluated_at_ms,
            "dataset_hash": self.dataset_hash,
            "model_spec_hash": self.model_spec_hash,
            "gate_passed": self.gate_passed,
            "metrics": dict(self.metrics),
            "reasons": list(self.reasons),
        }


def create_experiment(
    *,
    name: str,
    instrument_id: str,
    timeframe: str,
    indicator_ids: Sequence[str],
    indicator_configuration_hash: str,
    feature_schema_version: str,
    created_at_ms: int | None = None,
    source: str = "MANUAL",
    notes: str = "",
) -> LabExperiment:
    normalized_ids = tuple(sorted({value.strip() for value in indicator_ids if value.strip()}))
    if not normalized_ids:
        raise ValueError("at least one enabled chart indicator is required")
    if not name.strip() or not instrument_id.strip() or not timeframe.strip():
        raise ValueError("name, instrument_id and timeframe are required")
    if not re.fullmatch(r"[a-f0-9]{64}", indicator_configuration_hash):
        raise ValueError("indicator_configuration_hash must be a SHA-256 hash")
    if not feature_schema_version.strip():
        raise ValueError("feature_schema_version is required")

    created = int(time.time() * 1000) if created_at_ms is None else int(created_at_ms)
    material = {
        "name": name.strip(),
        "instrument_id": instrument_id.strip(),
        "timeframe": timeframe.strip(),
        "indicator_ids": normalized_ids,
        "indicator_configuration_hash": indicator_configuration_hash,
        "feature_schema_version": feature_schema_version.strip(),
        "created_at_ms": created,
        "source": source.strip() or "MANUAL",
    }
    experiment_id = "lab-" + stable_hash(material)[:24]
    return LabExperiment(
        experiment_id=experiment_id,
        name=name.strip(),
        instrument_id=instrument_id.strip(),
        timeframe=timeframe.strip(),
        indicator_ids=normalized_ids,
        indicator_configuration_hash=indicator_configuration_hash,
        feature_schema_version=feature_schema_version.strip(),
        created_at_ms=created,
        source=source.strip() or "MANUAL",
        notes=notes.strip(),
    )


class IntelligenceLabRegistry:
    """File-backed experimental registry isolated from production intelligence state."""

    def __init__(self, root: Path, *, production_root: Path | None = None) -> None:
        self.root = root.resolve()
        self.production_root = production_root.resolve() if production_root else None
        if self.production_root is not None:
            if self.root == self.production_root or self.production_root in self.root.parents:
                raise ValueError("intelligence lab storage must be isolated from production storage")
        self.experiments_root = self.root / "experiments"
        self.events_path = self.root / "events.jsonl"

    def create(self, experiment: LabExperiment) -> LabExperiment:
        self._validate_id(experiment.experiment_id)
        path = self._manifest_path(experiment.experiment_id)
        if path.exists():
            existing = self.get(experiment.experiment_id)
            if existing == experiment:
                return existing
            raise ValueError("experiment id already exists with different content")
        self._write_json(path, experiment.to_record(), overwrite=False)
        self._append_event(experiment, "CREATE", {"state": experiment.lifecycle_state})
        return experiment

    def get(self, experiment_id: str) -> LabExperiment:
        self._validate_id(experiment_id)
        path = self._manifest_path(experiment_id)
        if not path.is_file():
            raise ValueError("intelligence lab experiment not found")
        with path.open("r", encoding="utf-8") as handle:
            decoded = json.load(handle)
        if not isinstance(decoded, dict):
            raise ValueError("invalid intelligence lab experiment record")
        return LabExperiment.from_record(decoded)

    def list(self) -> tuple[LabExperiment, ...]:
        if not self.experiments_root.is_dir():
            return ()
        values: list[LabExperiment] = []
        for path in sorted(self.experiments_root.glob("*/manifest.json")):
            with path.open("r", encoding="utf-8") as handle:
                decoded = json.load(handle)
            if isinstance(decoded, dict):
                values.append(LabExperiment.from_record(decoded))
        return tuple(sorted(values, key=lambda value: (value.created_at_ms, value.experiment_id)))

    def record_backtest(self, evaluation: LabEvaluation) -> LabExperiment:
        if evaluation.phase != "BACKTEST":
            raise ValueError("backtest evaluation phase must be BACKTEST")
        experiment = self.get(evaluation.experiment_id)
        if experiment.lifecycle_state != "EXPERIMENT":
            raise ValueError("backtest can only be recorded for an EXPERIMENT")

        evaluation_record = evaluation.to_record()
        evaluation_id = stable_hash(evaluation_record)[:32]
        self._write_evaluation(evaluation, f"backtests/{evaluation_id}.json")
        self._write_json(
            self._experiment_dir(evaluation.experiment_id) / "results" / "backtest.json",
            evaluation_record,
            overwrite=True,
        )

        if not evaluation.gate_passed:
            self._append_event(experiment, "BACKTEST_FAIL", {
                "evaluation_id": evaluation_id,
                **evaluation_record,
            })
            return experiment
        return self._transition(experiment, "BACKTESTED", "BACKTEST_PASS", {
            "evaluation_id": evaluation_id,
            **evaluation_record,
        })

    def start_shadow(self, experiment_id: str) -> LabExperiment:
        experiment = self.get(experiment_id)
        if experiment.lifecycle_state != "BACKTESTED":
            raise ValueError("shadow can only start from BACKTESTED")
        return self._transition(experiment, "SHADOW", "SHADOW_START", {})

    def certify(self, evaluation: LabEvaluation) -> LabExperiment:
        if evaluation.phase != "SHADOW":
            raise ValueError("certification evaluation phase must be SHADOW")
        experiment = self.get(evaluation.experiment_id)
        if experiment.lifecycle_state != "SHADOW":
            raise ValueError("certification requires SHADOW state")

        evaluation_record = evaluation.to_record()
        evaluation_id = stable_hash(evaluation_record)[:32]
        self._write_evaluation(evaluation, f"shadow-evaluations/{evaluation_id}.json")
        self._write_json(
            self._experiment_dir(evaluation.experiment_id) / "results" / "shadow.json",
            evaluation_record,
            overwrite=True,
        )
        if not evaluation.gate_passed:
            self._append_event(experiment, "SHADOW_FAIL", {
                "evaluation_id": evaluation_id,
                **evaluation_record,
            })
            return experiment
        return self._transition(experiment, "CERTIFIED", "CERTIFY", {
            "evaluation_id": evaluation_id,
            **evaluation_record,
        })

    def retire(self, experiment_id: str, reason: str = "") -> LabExperiment:
        experiment = self.get(experiment_id)
        if experiment.lifecycle_state == "RETIRED":
            return experiment
        return self._transition(experiment, "RETIRED", "RETIRE", {"reason": reason.strip()})

    def read_evaluation(self, experiment_id: str, phase: str) -> dict[str, Any] | None:
        self._validate_id(experiment_id)
        filename = {"BACKTEST": "backtest.json", "SHADOW": "shadow.json"}.get(phase.upper())
        if filename is None:
            raise ValueError("unsupported lab evaluation phase")
        path = self._experiment_dir(experiment_id) / "results" / filename
        if not path.is_file():
            return None
        with path.open("r", encoding="utf-8") as handle:
            decoded = json.load(handle)
        return decoded if isinstance(decoded, dict) else None

    def save_chart_snapshot(
        self,
        experiment_id: str,
        payload: Mapping[str, Any],
        *,
        snapshot_hash: str,
    ) -> Path:
        self.get(experiment_id)
        if not re.fullmatch(r"[a-f0-9]{64}", snapshot_hash):
            raise ValueError("snapshot_hash must be a SHA-256 hash")
        path = self._experiment_dir(experiment_id) / "sources" / "chart-snapshot.json"
        material = {
            "snapshot_hash": snapshot_hash,
            "payload": dict(payload),
        }
        if path.exists():
            with path.open("r", encoding="utf-8") as handle:
                existing = json.load(handle)
            if existing == material:
                return path
            raise ValueError("chart snapshot already exists with different content")
        self._write_json(path, material, overwrite=False)
        return path

    def load_chart_snapshot(self, experiment_id: str) -> dict[str, Any]:
        self.get(experiment_id)
        path = self._experiment_dir(experiment_id) / "sources" / "chart-snapshot.json"
        if not path.is_file():
            raise ValueError("chart snapshot is not stored for this experiment")
        with path.open("r", encoding="utf-8") as handle:
            decoded = json.load(handle)
        if not isinstance(decoded, dict) or not isinstance(decoded.get("payload"), dict):
            raise ValueError("stored chart snapshot is invalid")
        return decoded

    def save_backtest_selection(
        self,
        experiment_id: str,
        selection: Mapping[str, Any],
    ) -> str:
        self.get(experiment_id)
        material = dict(selection)
        selection_id = stable_hash(material)[:32]
        immutable_path = (
            self._experiment_dir(experiment_id)
            / "results"
            / "selections"
            / f"{selection_id}.json"
        )
        if immutable_path.exists():
            with immutable_path.open("r", encoding="utf-8") as handle:
                existing = json.load(handle)
            if existing != material:
                raise ValueError("selection id already exists with different content")
        else:
            self._write_json(immutable_path, material, overwrite=False)

        self._write_json(
            self._experiment_dir(experiment_id) / "results" / "selection.json",
            {"selection_id": selection_id, **material},
            overwrite=True,
        )
        return selection_id

    def save_recommendation_artifact(
        self,
        experiment_id: str,
        recommendation: Mapping[str, Any],
    ) -> str:
        self.get(experiment_id)
        recommendation_id = str(recommendation.get("recommendation_id", ""))
        self._validate_id(recommendation_id)
        material = dict(recommendation)
        immutable_path = (
            self._experiment_dir(experiment_id)
            / "results"
            / "recommendations"
            / f"{recommendation_id}.json"
        )
        if immutable_path.exists():
            with immutable_path.open("r", encoding="utf-8") as handle:
                existing = json.load(handle)
            if existing != material:
                raise ValueError("recommendation id already exists with different content")
        else:
            self._write_json(immutable_path, material, overwrite=False)

        self._write_json(
            self._experiment_dir(experiment_id) / "results" / "recommendation.json",
            material,
            overwrite=True,
        )
        return recommendation_id

    def save_shadow_config(
        self,
        experiment_id: str,
        config: Mapping[str, Any],
    ) -> Path:
        experiment = self.get(experiment_id)
        if experiment.lifecycle_state != "SHADOW":
            raise ValueError("shadow config requires SHADOW state")
        path = self._experiment_dir(experiment_id) / "shadow" / "config.json"
        material = dict(config)
        if path.exists():
            with path.open("r", encoding="utf-8") as handle:
                existing = json.load(handle)
            if existing == material:
                return path
            raise ValueError("shadow config already exists with different content")
        self._write_json(path, material, overwrite=False)
        return path

    def read_shadow_config(self, experiment_id: str) -> dict[str, Any] | None:
        self.get(experiment_id)
        path = self._experiment_dir(experiment_id) / "shadow" / "config.json"
        if not path.is_file():
            return None
        with path.open("r", encoding="utf-8") as handle:
            decoded = json.load(handle)
        return decoded if isinstance(decoded, dict) else None

    def save_shadow_observation(
        self,
        experiment_id: str,
        observation: Mapping[str, Any],
    ) -> Path:
        experiment = self.get(experiment_id)
        if experiment.lifecycle_state != "SHADOW":
            raise ValueError("shadow observation requires SHADOW state")
        observation_id = str(observation.get("observation_id", ""))
        self._validate_id(observation_id)
        path = (
            self._experiment_dir(experiment_id)
            / "shadow"
            / "observations"
            / f"{observation_id}.json"
        )
        material = dict(observation)
        if path.exists():
            with path.open("r", encoding="utf-8") as handle:
                existing = json.load(handle)
            if isinstance(existing, dict):
                existing_compare = dict(existing)
                material_compare = dict(material)
                existing_compare.pop("created_at_ms", None)
                material_compare.pop("created_at_ms", None)
                if existing_compare == material_compare:
                    return path
            raise ValueError("shadow observation id already exists with different content")
        self._write_json(path, material, overwrite=False)
        self._write_json(
            self._experiment_dir(experiment_id) / "shadow" / "latest-observation.json",
            material,
            overwrite=True,
        )
        return path

    def save_shadow_outcome(
        self,
        experiment_id: str,
        outcome: Mapping[str, Any],
    ) -> Path:
        self.get(experiment_id)
        observation_id = str(outcome.get("observation_id", ""))
        self._validate_id(observation_id)
        path = (
            self._experiment_dir(experiment_id)
            / "shadow"
            / "outcomes"
            / f"{observation_id}.json"
        )
        material = dict(outcome)
        if path.exists():
            with path.open("r", encoding="utf-8") as handle:
                existing = json.load(handle)
            if existing == material:
                return path
            raise ValueError("shadow outcome already exists with different content")
        self._write_json(path, material, overwrite=False)
        return path

    def save_shadow_summary(
        self,
        experiment_id: str,
        summary: Mapping[str, Any],
    ) -> Path:
        self.get(experiment_id)
        path = self._experiment_dir(experiment_id) / "shadow" / "summary.json"
        self._write_json(path, dict(summary), overwrite=True)
        return path

    def read_shadow_records(
        self,
        experiment_id: str,
        kind: str,
    ) -> tuple[dict[str, Any], ...]:
        self.get(experiment_id)
        if kind not in {"observations", "outcomes"}:
            raise ValueError("unsupported shadow record kind")
        root = self._experiment_dir(experiment_id) / "shadow" / kind
        if not root.is_dir():
            return ()
        records: list[dict[str, Any]] = []
        for path in sorted(root.glob("*.json")):
            with path.open("r", encoding="utf-8") as handle:
                decoded = json.load(handle)
            if isinstance(decoded, dict):
                records.append(decoded)
        return tuple(records)

    def read_result_record(
        self,
        experiment_id: str,
        filename: str,
    ) -> dict[str, Any] | None:
        self.get(experiment_id)
        if not re.fullmatch(r"[A-Za-z0-9._-]{1,96}", filename):
            raise ValueError("invalid result filename")
        path = self._experiment_dir(experiment_id) / "results" / filename
        if not path.is_file():
            return None
        with path.open("r", encoding="utf-8") as handle:
            decoded = json.load(handle)
        return decoded if isinstance(decoded, dict) else None

    def save_certified_advisory(
        self,
        experiment_id: str,
        advisory: Mapping[str, Any],
    ) -> Path:
        experiment = self.get(experiment_id)
        if experiment.lifecycle_state != "CERTIFIED":
            raise ValueError("certified advisory requires CERTIFIED state")
        advisory_id = str(advisory.get("advisory_id", ""))
        self._validate_id(advisory_id)
        material = dict(advisory)
        path = (
            self._experiment_dir(experiment_id)
            / "advisory"
            / "observations"
            / f"{advisory_id}.json"
        )
        if path.exists():
            with path.open("r", encoding="utf-8") as handle:
                existing = json.load(handle)
            existing_compare = dict(existing) if isinstance(existing, dict) else {}
            material_compare = dict(material)
            existing_compare.pop("created_at_ms", None)
            material_compare.pop("created_at_ms", None)
            if existing_compare != material_compare:
                raise ValueError("certified advisory id already exists with different content")
        else:
            self._write_json(path, material, overwrite=False)

        self._write_json(
            self._experiment_dir(experiment_id) / "advisory" / "latest.json",
            material,
            overwrite=True,
        )
        return path

    def read_certified_advisory(
        self,
        experiment_id: str,
    ) -> dict[str, Any] | None:
        self.get(experiment_id)
        path = self._experiment_dir(experiment_id) / "advisory" / "latest.json"
        if not path.is_file():
            return None
        with path.open("r", encoding="utf-8") as handle:
            decoded = json.load(handle)
        return decoded if isinstance(decoded, dict) else None

    def save_candidate_artifact(
        self,
        experiment_id: str,
        candidate: Mapping[str, Any],
    ) -> Path:
        self.get(experiment_id)
        candidate_id = str(candidate.get("candidate_id", ""))
        self._validate_id(candidate_id)
        path = self._experiment_dir(experiment_id) / "models" / f"{candidate_id}.json"
        if path.exists():
            with path.open("r", encoding="utf-8") as handle:
                existing = json.load(handle)
            if existing == dict(candidate):
                return path
            raise ValueError("candidate artifact already exists with different content")
        self._write_json(path, dict(candidate), overwrite=False)
        return path

    def read_candidate_artifacts(self, experiment_id: str) -> tuple[dict[str, Any], ...]:
        self.get(experiment_id)
        root = self._experiment_dir(experiment_id) / "models"
        if not root.is_dir():
            return ()
        records: list[dict[str, Any]] = []
        for path in sorted(root.glob("*.json")):
            with path.open("r", encoding="utf-8") as handle:
                decoded = json.load(handle)
            if isinstance(decoded, dict):
                records.append(decoded)
        return tuple(records)

    def _transition(
        self,
        experiment: LabExperiment,
        next_state: str,
        action: str,
        payload: Mapping[str, Any],
    ) -> LabExperiment:
        if next_state not in _TRANSITIONS[experiment.lifecycle_state]:
            raise ValueError(f"invalid lab transition {experiment.lifecycle_state} -> {next_state}")
        updated = replace(experiment, lifecycle_state=next_state)
        self._write_json(self._manifest_path(experiment.experiment_id), updated.to_record(), overwrite=True)
        self._append_event(updated, action, payload)
        return updated

    def _write_evaluation(self, evaluation: LabEvaluation, filename: str) -> None:
        if evaluation.experiment_id != self.get(evaluation.experiment_id).experiment_id:
            raise ValueError("evaluation experiment mismatch")
        if not re.fullmatch(r"[a-f0-9]{64}", evaluation.dataset_hash):
            raise ValueError("dataset_hash must be a SHA-256 hash")
        if not re.fullmatch(r"[a-f0-9]{64}", evaluation.model_spec_hash):
            raise ValueError("model_spec_hash must be a SHA-256 hash")
        if not evaluation.metrics:
            raise ValueError("evaluation metrics are required")
        if any(not isinstance(value, (int, float)) for value in evaluation.metrics.values()):
            raise ValueError("evaluation metrics must be numeric")
        path = self._experiment_dir(evaluation.experiment_id) / "results" / filename
        record = evaluation.to_record()
        if path.exists():
            with path.open("r", encoding="utf-8") as handle:
                existing = json.load(handle)
            if existing == record:
                return
            raise ValueError("immutable lab evaluation already exists with different content")
        self._write_json(path, record, overwrite=False)

    def _append_event(
        self,
        experiment: LabExperiment,
        action: str,
        payload: Mapping[str, Any],
    ) -> None:
        self.root.mkdir(parents=True, exist_ok=True)
        event = {
            "schema": "QNEXT.INTELLIGENCE.LAB.EVENT/1",
            "event_id": stable_hash({
                "experiment_id": experiment.experiment_id,
                "action": action,
                "state": experiment.lifecycle_state,
                "payload": payload,
            })[:32],
            "event_at_ms": int(time.time() * 1000),
            "experiment_id": experiment.experiment_id,
            "action": action,
            "state": experiment.lifecycle_state,
            "payload": dict(payload),
        }
        with self.events_path.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(event, sort_keys=True, separators=(",", ":")) + "\n")
            handle.flush()
            os.fsync(handle.fileno())

    def _manifest_path(self, experiment_id: str) -> Path:
        return self._experiment_dir(experiment_id) / "manifest.json"

    def _experiment_dir(self, experiment_id: str) -> Path:
        self._validate_id(experiment_id)
        return self.experiments_root / experiment_id

    @staticmethod
    def _validate_id(value: str) -> None:
        if not _ID_RE.fullmatch(value):
            raise ValueError("invalid intelligence lab experiment id")

    @staticmethod
    def _write_json(path: Path, value: Mapping[str, Any], *, overwrite: bool) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        if path.exists() and not overwrite:
            raise ValueError(f"immutable lab artifact already exists: {path.name}")
        temp = path.with_suffix(path.suffix + ".tmp")
        payload = json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n"
        fd = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o640)
        try:
            os.write(fd, payload.encode("utf-8"))
            os.fsync(fd)
        finally:
            os.close(fd)
        os.replace(temp, path)
