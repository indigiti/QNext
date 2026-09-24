from __future__ import annotations

from pathlib import Path
from typing import Any

from .models import ModelManifest
from .storage import AppendOnlyJsonlStore, stable_hash


class ModelLifecycleError(ValueError):
    pass


class ModelRegistry:
    """Append-only model definitions and lifecycle events."""

    def __init__(self, root: str | Path) -> None:
        root = Path(root) / "models"
        self._manifests = AppendOnlyJsonlStore(root / "manifests.jsonl", "model_id")
        self._events = AppendOnlyJsonlStore(root / "lifecycle.jsonl", "event_id")

    def register(self, manifest: ModelManifest) -> ModelManifest:
        self._manifests.append(manifest.to_record())
        self._append_event(manifest.model_id, "REGISTERED", manifest.created_at_ms, {"state": "CANDIDATE"})
        return manifest

    def manifest(self, model_id: str) -> dict[str, Any] | None:
        return self._manifests.get(model_id)

    def state(self, model_id: str) -> str:
        if self.manifest(model_id) is None:
            raise ModelLifecycleError(f"unknown model {model_id!r}")
        state = "CANDIDATE"
        for event in self._events.records():
            if event["model_id"] != model_id:
                continue
            event_type = event["event_type"]
            if event_type == "VALIDATED":
                state = "VALIDATED"
            elif event_type == "PROMOTED":
                state = "PRODUCTION"
            elif event_type == "RETIRED":
                state = "RETIRED"
        return state

    def validate(self, model_id: str, *, validated_at_ms: int, evidence: dict[str, Any]) -> None:
        if self.state(model_id) not in {"CANDIDATE", "VALIDATED"}:
            raise ModelLifecycleError("only candidate models can be validated")
        if not evidence.get("passed"):
            raise ModelLifecycleError("validation evidence must explicitly pass")
        if not evidence.get("dataset_hash") or not evidence.get("metrics"):
            raise ModelLifecycleError("validation evidence requires dataset_hash and metrics")
        manifest = self.manifest(model_id)
        assert manifest is not None
        if evidence["dataset_hash"] != manifest["dataset_hash"]:
            raise ModelLifecycleError("validation dataset hash does not match model manifest")
        self._append_event(model_id, "VALIDATED", validated_at_ms, {"evidence": evidence})

    def promote(self, model_id: str, *, promoted_at_ms: int, approved_by: str) -> None:
        if self.state(model_id) != "VALIDATED":
            raise ModelLifecycleError("model must be VALIDATED before promotion")
        if not approved_by.strip():
            raise ModelLifecycleError("approved_by is required")
        manifest = self.manifest(model_id)
        assert manifest is not None
        family = manifest["model_name"]
        for record in self._manifests.records():
            other_id = str(record["model_id"])
            if other_id != model_id and record["model_name"] == family and self.state(other_id) == "PRODUCTION":
                self._append_event(other_id, "RETIRED", promoted_at_ms, {"reason": f"superseded by {model_id}"})
        self._append_event(model_id, "PROMOTED", promoted_at_ms, {"approved_by": approved_by})

    def retire(self, model_id: str, *, retired_at_ms: int, reason: str) -> None:
        if self.state(model_id) == "RETIRED":
            return
        if not reason.strip():
            raise ModelLifecycleError("retirement reason is required")
        self._append_event(model_id, "RETIRED", retired_at_ms, {"reason": reason})

    def _append_event(self, model_id: str, event_type: str, at_ms: int, detail: dict[str, Any]) -> None:
        if at_ms <= 0:
            raise ModelLifecycleError("lifecycle event timestamp must be positive")
        payload = {"model_id": model_id, "event_type": event_type, "at_ms": at_ms, "detail": detail}
        event = dict(payload)
        event["event_id"] = stable_hash(payload)
        self._events.append(event)
