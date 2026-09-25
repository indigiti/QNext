from __future__ import annotations

from dataclasses import asdict, dataclass
import hashlib
import json
import math
from typing import Iterable, Mapping


ALLOWED_SOURCES = {"HISTORICAL", "LIVE"}
ALLOWED_QUALITY = {"GOOD", "RECOVERED"}


def _canonical_json(value: object) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


@dataclass(frozen=True, slots=True)
class TrainingObservation:
    observation_id: str
    source: str
    instrument_id: str
    timeframe: str
    feature_time_ms: int
    label_time_ms: int
    features: Mapping[str, float]
    label: float
    data_quality: str = "GOOD"
    provenance_hash: str = ""

    def validate(self) -> None:
        if not self.observation_id:
            raise ValueError("observation_id is required")
        if self.source not in ALLOWED_SOURCES:
            raise ValueError(f"unsupported training source: {self.source}")
        if not self.instrument_id or not self.timeframe:
            raise ValueError("instrument_id and timeframe are required")
        if self.label_time_ms <= self.feature_time_ms:
            raise ValueError("label_time_ms must be after feature_time_ms")
        if self.data_quality not in ALLOWED_QUALITY:
            raise ValueError(f"training observation quality not eligible: {self.data_quality}")
        if not self.features:
            raise ValueError("training observation requires at least one feature")
        if not all(math.isfinite(float(value)) for value in self.features.values()):
            raise ValueError("training features must be finite")
        if not math.isfinite(float(self.label)):
            raise ValueError("training label must be finite")

    def canonical(self) -> dict:
        value = asdict(self)
        value["features"] = dict(sorted((str(key), float(val)) for key, val in self.features.items()))
        return value


@dataclass(frozen=True, slots=True)
class HybridTrainingSet:
    train: tuple[TrainingObservation, ...]
    validation: tuple[TrainingObservation, ...]
    test: tuple[TrainingObservation, ...]
    excluded_count: int
    train_end_ms: int
    validation_end_ms: int
    test_end_ms: int
    as_of_ms: int
    dataset_hash: str

    @property
    def included_count(self) -> int:
        return len(self.train) + len(self.validation) + len(self.test)

    def source_counts(self) -> dict[str, int]:
        counts = {"HISTORICAL": 0, "LIVE": 0}
        for observation in (*self.train, *self.validation, *self.test):
            counts[observation.source] += 1
        return counts


class HybridTrainingSetBuilder:
    """Build chronological historical + matured-live datasets without label leakage.

    An observation is admitted only after its label is known. Observations whose
    label crosses a split boundary are purged rather than allowed to leak future
    information into an earlier split.
    """

    def build(
        self,
        observations: Iterable[TrainingObservation],
        *,
        train_end_ms: int,
        validation_end_ms: int,
        test_end_ms: int,
        as_of_ms: int,
    ) -> HybridTrainingSet:
        if not (0 < train_end_ms < validation_end_ms < test_end_ms <= as_of_ms):
            raise ValueError(
                "expected 0 < train_end_ms < validation_end_ms < test_end_ms <= as_of_ms"
            )

        unique: dict[str, TrainingObservation] = {}
        for observation in observations:
            observation.validate()
            if observation.observation_id in unique:
                raise ValueError(f"duplicate training observation: {observation.observation_id}")
            unique[observation.observation_id] = observation

        ordered = sorted(
            unique.values(),
            key=lambda item: (item.feature_time_ms, item.label_time_ms, item.observation_id),
        )
        train: list[TrainingObservation] = []
        validation: list[TrainingObservation] = []
        test: list[TrainingObservation] = []
        excluded = 0

        for observation in ordered:
            if observation.label_time_ms > as_of_ms:
                excluded += 1
                continue

            if observation.feature_time_ms < train_end_ms:
                if observation.label_time_ms <= train_end_ms:
                    train.append(observation)
                else:
                    excluded += 1
                continue

            if observation.feature_time_ms < validation_end_ms:
                if observation.label_time_ms <= validation_end_ms:
                    validation.append(observation)
                else:
                    excluded += 1
                continue

            if observation.feature_time_ms < test_end_ms:
                if observation.label_time_ms <= test_end_ms:
                    test.append(observation)
                else:
                    excluded += 1
                continue

            excluded += 1

        payload = {
            "schema": "qnext-hybrid-training-v1",
            "train_end_ms": train_end_ms,
            "validation_end_ms": validation_end_ms,
            "test_end_ms": test_end_ms,
            "as_of_ms": as_of_ms,
            "train": [item.canonical() for item in train],
            "validation": [item.canonical() for item in validation],
            "test": [item.canonical() for item in test],
        }
        dataset_hash = hashlib.sha256(_canonical_json(payload).encode("utf-8")).hexdigest()

        return HybridTrainingSet(
            train=tuple(train),
            validation=tuple(validation),
            test=tuple(test),
            excluded_count=excluded,
            train_end_ms=train_end_ms,
            validation_end_ms=validation_end_ms,
            test_end_ms=test_end_ms,
            as_of_ms=as_of_ms,
            dataset_hash=dataset_hash,
        )
