from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from qnext_intelligence.lab import (
    IntelligenceLabRegistry,
    LabEvaluation,
    create_experiment,
)


class IntelligenceLabTests(unittest.TestCase):
    def make_experiment(self):
        return create_experiment(
            name="NIFTY 1m EMA + OB + SYN+",
            instrument_id="QNEXT:NIFTY",
            timeframe="1m",
            indicator_ids=["adaptive-ema-qalg", "order-block", "syn-plus"],
            indicator_configuration_hash="a" * 64,
            feature_schema_version="qnext-chart-indicators-v1",
            created_at_ms=1_800_000_000_000,
            source="CHART_CLONE",
        )

    def evaluation(self, experiment_id: str, phase: str, passed: bool = True):
        return LabEvaluation(
            experiment_id=experiment_id,
            phase=phase,
            evaluated_at_ms=1_800_000_100_000,
            dataset_hash="b" * 64,
            model_spec_hash="c" * 64,
            gate_passed=passed,
            metrics={
                "samples": 100.0,
                "accuracy": 0.61,
                "average_strategy_return": 0.0012,
                "max_drawdown": 0.018,
            },
        )

    def test_lab_storage_must_be_separate_from_production(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            production = root / "storage" / "intelligence"
            with self.assertRaisesRegex(ValueError, "isolated"):
                IntelligenceLabRegistry(production, production_root=production)
            with self.assertRaisesRegex(ValueError, "isolated"):
                IntelligenceLabRegistry(production / "lab", production_root=production)

            registry = IntelligenceLabRegistry(
                root / "storage" / "intelligence-lab",
                production_root=production,
            )
            self.assertEqual(registry.root.name, "intelligence-lab")

    def test_experiment_lifecycle_requires_backtest_and_shadow(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            registry = IntelligenceLabRegistry(
                root / "intelligence-lab",
                production_root=root / "intelligence",
            )
            experiment = registry.create(self.make_experiment())
            self.assertEqual(experiment.lifecycle_state, "EXPERIMENT")

            with self.assertRaisesRegex(ValueError, "BACKTESTED"):
                registry.start_shadow(experiment.experiment_id)

            backtested = registry.record_backtest(
                self.evaluation(experiment.experiment_id, "BACKTEST")
            )
            self.assertEqual(backtested.lifecycle_state, "BACKTESTED")

            shadow = registry.start_shadow(experiment.experiment_id)
            self.assertEqual(shadow.lifecycle_state, "SHADOW")

            certified = registry.certify(
                self.evaluation(experiment.experiment_id, "SHADOW")
            )
            self.assertEqual(certified.lifecycle_state, "CERTIFIED")

    def test_failed_gate_does_not_advance_state(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            registry = IntelligenceLabRegistry(root / "lab", production_root=root / "prod")
            experiment = registry.create(self.make_experiment())

            unchanged = registry.record_backtest(
                self.evaluation(experiment.experiment_id, "BACKTEST", passed=False)
            )
            self.assertEqual(unchanged.lifecycle_state, "EXPERIMENT")
            self.assertEqual(
                registry.get(experiment.experiment_id).lifecycle_state,
                "EXPERIMENT",
            )

    def test_duplicate_create_is_idempotent_but_conflicts_fail_closed(self):
        with tempfile.TemporaryDirectory() as tmp:
            registry = IntelligenceLabRegistry(Path(tmp) / "lab")
            experiment = self.make_experiment()
            registry.create(experiment)
            self.assertEqual(registry.create(experiment), experiment)

            changed = create_experiment(
                name=experiment.name,
                instrument_id=experiment.instrument_id,
                timeframe=experiment.timeframe,
                indicator_ids=experiment.indicator_ids,
                indicator_configuration_hash=experiment.indicator_configuration_hash,
                feature_schema_version=experiment.feature_schema_version,
                created_at_ms=experiment.created_at_ms,
                source=experiment.source,
                notes="different immutable content",
            )
            self.assertEqual(changed.experiment_id, experiment.experiment_id)
            with self.assertRaisesRegex(ValueError, "different content"):
                registry.create(changed)


if __name__ == "__main__":
    unittest.main()
