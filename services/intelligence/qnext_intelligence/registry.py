from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any, Mapping

from .domain import canonical_json, stable_hash
from .learning import CandidateModel
from .store import ImmutableJSONLStore


class ModelRegistry:
    """File-backed candidate registry with explicit promotion and rollback."""

    def __init__(self, root: str | Path) -> None:
        self.root = Path(root)
        self.candidates = ImmutableJSONLStore(
            self.root / "candidates.jsonl", identity_field="candidate_id"
        )
        self.events = ImmutableJSONLStore(
            self.root / "promotion_events.jsonl", identity_field="event_id"
        )
        self.production_path = self.root / "production.json"

    def save_candidate(self, candidate: CandidateModel) -> None:
        self.candidates.append(candidate.to_record())

    def list_candidates(self) -> list[dict[str, Any]]:
        return self.candidates.read_all()

    def get_candidate(self, candidate_id: str) -> CandidateModel:
        for record in self.candidates.read_all():
            if record.get("candidate_id") == candidate_id:
                return CandidateModel.from_record(record)
        raise KeyError(f"unknown candidate: {candidate_id}")

    def production(self) -> dict[str, Any] | None:
        if not self.production_path.exists():
            return None
        with self.production_path.open("r", encoding="utf-8") as handle:
            value = json.load(handle)
        if not isinstance(value, dict):
            raise ValueError("production model pointer is corrupt")
        return value

    def promote(
        self,
        candidate_id: str,
        *,
        approved_by: str,
        expected_model_hash: str,
        promoted_at_ms: int,
        note: str = "",
    ) -> dict[str, Any]:
        approved_by = approved_by.strip()
        if not approved_by:
            raise ValueError("approved_by is required")
        if promoted_at_ms <= 0:
            raise ValueError("promoted_at_ms must be positive")

        candidate = self.get_candidate(candidate_id)
        if not candidate.promotion_gate.passed:
            raise ValueError("candidate failed promotion gates")
        if candidate.model_hash != expected_model_hash:
            raise ValueError("expected model hash does not match candidate")

        previous = self.production()
        previous_candidate_id = (
            str(previous.get("candidate_id", "")) if previous is not None else ""
        )
        pointer = {
            "candidate_id": candidate.candidate_id,
            "model_name": candidate.model_name,
            "model_version": candidate.model_version,
            "model_hash": candidate.model_hash,
            "dataset_hash": candidate.dataset_hash,
            "feature_set_version": candidate.feature_set_version,
            "promoted_at_ms": promoted_at_ms,
            "approved_by": approved_by,
            "previous_candidate_id": previous_candidate_id,
        }
        event = self._event(
            action="PROMOTE",
            candidate=candidate,
            approved_by=approved_by,
            event_at_ms=promoted_at_ms,
            previous_candidate_id=previous_candidate_id,
            note=note,
        )
        self.events.append(event)
        self._write_pointer(pointer)
        return pointer

    def rollback(
        self,
        *,
        approved_by: str,
        event_at_ms: int,
        candidate_id: str | None = None,
        note: str = "",
    ) -> dict[str, Any]:
        approved_by = approved_by.strip()
        if not approved_by:
            raise ValueError("approved_by is required")
        if event_at_ms <= 0:
            raise ValueError("event_at_ms must be positive")

        current = self.production()
        if current is None:
            raise ValueError("no production candidate exists")
        target_id = (candidate_id or str(current.get("previous_candidate_id", ""))).strip()
        if not target_id:
            raise ValueError("no rollback target is available")
        target = self.get_candidate(target_id)
        if not target.promotion_gate.passed:
            raise ValueError("rollback target did not pass promotion gates")

        current_id = str(current.get("candidate_id", ""))
        pointer = {
            "candidate_id": target.candidate_id,
            "model_name": target.model_name,
            "model_version": target.model_version,
            "model_hash": target.model_hash,
            "dataset_hash": target.dataset_hash,
            "feature_set_version": target.feature_set_version,
            "promoted_at_ms": event_at_ms,
            "approved_by": approved_by,
            "previous_candidate_id": current_id,
        }
        event = self._event(
            action="ROLLBACK",
            candidate=target,
            approved_by=approved_by,
            event_at_ms=event_at_ms,
            previous_candidate_id=current_id,
            note=note,
        )
        self.events.append(event)
        self._write_pointer(pointer)
        return pointer

    def _event(
        self,
        *,
        action: str,
        candidate: CandidateModel,
        approved_by: str,
        event_at_ms: int,
        previous_candidate_id: str,
        note: str,
    ) -> dict[str, Any]:
        material = {
            "action": action,
            "candidate_id": candidate.candidate_id,
            "model_hash": candidate.model_hash,
            "approved_by": approved_by,
            "event_at_ms": event_at_ms,
            "previous_candidate_id": previous_candidate_id,
            "note": note,
        }
        return {"event_id": stable_hash(material)[:32], **material}

    def _write_pointer(self, pointer: Mapping[str, Any]) -> None:
        self.root.mkdir(parents=True, exist_ok=True)
        temp = self.production_path.with_suffix(".json.tmp")
        payload = canonical_json(dict(pointer)) + "\n"
        fd = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o640)
        try:
            os.write(fd, payload.encode("utf-8"))
            os.fsync(fd)
        finally:
            os.close(fd)
        os.replace(temp, self.production_path)
