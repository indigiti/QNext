from __future__ import annotations

from dataclasses import asdict, dataclass, replace
import hashlib
import json
from pathlib import Path
from typing import Mapping


STATES = ("DRAFT", "BACKTESTED", "PAPER", "SHADOW", "APPROVED", "LIVE", "RETIRED")
NEXT_STATE = {
    "DRAFT": "BACKTESTED",
    "BACKTESTED": "PAPER",
    "PAPER": "SHADOW",
    "SHADOW": "APPROVED",
    "APPROVED": "LIVE",
    "LIVE": "RETIRED",
}
REQUIRED_EVIDENCE = {
    "BACKTESTED": "BACKTEST",
    "SHADOW": "PAPER",
    "APPROVED": "SHADOW",
    "LIVE": "HUMAN_APPROVAL",
}
PROTECTED_CHANGE_PREFIXES = (
    "risk.max_daily_loss",
    "risk.max_position",
    "risk.capital_ceiling",
    "risk.kill_switch",
    "permissions",
    "broker_permissions",
)


def _canonical_json(value: object) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


@dataclass(frozen=True, slots=True)
class PromotionEvidence:
    kind: str
    passed: bool
    reference_id: str
    created_at_ms: int
    metrics: Mapping[str, float]

    def canonical(self) -> dict:
        value = asdict(self)
        value["metrics"] = dict(sorted((str(key), float(val)) for key, val in self.metrics.items()))
        return value


@dataclass(frozen=True, slots=True)
class StrategyRevision:
    strategy_name: str
    version: str
    parent_version: str
    definition_hash: str
    dataset_hash: str
    created_at_ms: int
    state: str = "DRAFT"
    changed_paths: tuple[str, ...] = ()
    evidence: tuple[PromotionEvidence, ...] = ()

    def __post_init__(self) -> None:
        if self.state not in STATES:
            raise ValueError(f"unsupported strategy lifecycle state: {self.state}")
        if not self.strategy_name or not self.version or not self.definition_hash:
            raise ValueError("strategy_name, version and definition_hash are required")
        if not self.dataset_hash:
            raise ValueError("dataset_hash is required")

    @property
    def revision_id(self) -> str:
        seed = "|".join(
            [
                self.strategy_name,
                self.version,
                self.parent_version,
                self.definition_hash,
                self.dataset_hash,
            ]
        )
        return hashlib.sha256(seed.encode("utf-8")).hexdigest()[:24]

    @property
    def protected_changes(self) -> tuple[str, ...]:
        return tuple(
            path
            for path in self.changed_paths
            if any(path == prefix or path.startswith(prefix + ".") for prefix in PROTECTED_CHANGE_PREFIXES)
        )

    def canonical(self) -> dict:
        return {
            "revision_id": self.revision_id,
            "strategy_name": self.strategy_name,
            "version": self.version,
            "parent_version": self.parent_version,
            "definition_hash": self.definition_hash,
            "dataset_hash": self.dataset_hash,
            "created_at_ms": self.created_at_ms,
            "state": self.state,
            "changed_paths": list(self.changed_paths),
            "protected_changes": list(self.protected_changes),
            "evidence": [item.canonical() for item in self.evidence],
        }


@dataclass(frozen=True, slots=True)
class PromotionDecision:
    allowed: bool
    from_state: str
    target_state: str
    reasons: tuple[str, ...]


class PromotionGate:
    """Fail-closed promotion rules for self-learning strategy candidates."""

    def add_evidence(
        self,
        revision: StrategyRevision,
        evidence: PromotionEvidence,
    ) -> StrategyRevision:
        if not evidence.kind or not evidence.reference_id:
            raise ValueError("promotion evidence kind and reference_id are required")
        if any(
            item.kind == evidence.kind and item.reference_id == evidence.reference_id
            for item in revision.evidence
        ):
            raise ValueError("duplicate promotion evidence")
        return replace(revision, evidence=(*revision.evidence, evidence))

    def evaluate(
        self,
        revision: StrategyRevision,
        target_state: str,
        *,
        automated: bool = False,
    ) -> PromotionDecision:
        reasons: list[str] = []
        expected = NEXT_STATE.get(revision.state)
        if expected != target_state:
            reasons.append(f"invalid lifecycle transition: {revision.state} -> {target_state}")

        required = REQUIRED_EVIDENCE.get(target_state)
        if required is not None and not any(
            item.kind == required and item.passed for item in revision.evidence
        ):
            reasons.append(f"missing passed {required} evidence")

        if automated and revision.protected_changes:
            reasons.append("automated promotion cannot change protected risk or permission settings")

        if automated and target_state in {"APPROVED", "LIVE"}:
            reasons.append(f"{target_state} promotion requires an explicit controlled action")

        return PromotionDecision(
            allowed=not reasons,
            from_state=revision.state,
            target_state=target_state,
            reasons=tuple(reasons),
        )

    def promote(
        self,
        revision: StrategyRevision,
        target_state: str,
        *,
        automated: bool = False,
    ) -> StrategyRevision:
        decision = self.evaluate(revision, target_state, automated=automated)
        if not decision.allowed:
            raise ValueError("; ".join(decision.reasons))
        return replace(revision, state=target_state)


class ImmutableLifecycleStore:
    """File-backed immutable snapshots for every strategy promotion state."""

    def __init__(self, root: str | Path) -> None:
        self.root = Path(root)

    def write(self, revision: StrategyRevision) -> Path:
        destination = self.root / revision.strategy_name / revision.revision_id / f"{revision.state}.json"
        content = json.dumps(revision.canonical(), sort_keys=True, indent=2) + "\n"
        destination.parent.mkdir(parents=True, exist_ok=True)
        if destination.exists():
            existing = destination.read_text(encoding="utf-8")
            if existing != content:
                raise RuntimeError("immutable lifecycle snapshot collision")
            return destination
        temp = destination.with_suffix(".json.tmp")
        temp.write_text(content, encoding="utf-8")
        temp.replace(destination)
        return destination
