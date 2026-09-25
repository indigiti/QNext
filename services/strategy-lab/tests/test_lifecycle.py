import tempfile
import unittest
from pathlib import Path

from qnext_strategy_lab.lifecycle import (
    ImmutableLifecycleStore,
    PromotionEvidence,
    PromotionGate,
    StrategyRevision,
)


def revision(*, changed_paths=()):
    return StrategyRevision(
        strategy_name="nifty-momentum",
        version="2.0.0-draft",
        parent_version="1.9.0",
        definition_hash="definition-hash",
        dataset_hash="dataset-hash",
        created_at_ms=1_000,
        changed_paths=tuple(changed_paths),
    )


def evidence(kind: str, passed: bool = True):
    return PromotionEvidence(
        kind=kind,
        passed=passed,
        reference_id=f"{kind.lower()}-001",
        created_at_ms=2_000,
        metrics={"score": 1.0},
    )


class PromotionLifecycleTest(unittest.TestCase):
    def setUp(self):
        self.gate = PromotionGate()

    def test_direct_draft_to_live_is_rejected(self):
        decision = self.gate.evaluate(revision(), "LIVE")
        self.assertFalse(decision.allowed)
        self.assertTrue(any("invalid lifecycle transition" in reason for reason in decision.reasons))

    def test_candidate_can_follow_controlled_promotion_sequence(self):
        item = self.gate.add_evidence(revision(), evidence("BACKTEST"))
        item = self.gate.promote(item, "BACKTESTED")
        item = self.gate.promote(item, "PAPER")

        item = self.gate.add_evidence(item, evidence("PAPER"))
        item = self.gate.promote(item, "SHADOW")

        item = self.gate.add_evidence(item, evidence("SHADOW"))
        item = self.gate.promote(item, "APPROVED")

        item = self.gate.add_evidence(item, evidence("HUMAN_APPROVAL"))
        item = self.gate.promote(item, "LIVE")
        self.assertEqual(item.state, "LIVE")

    def test_failed_evidence_does_not_unlock_transition(self):
        item = self.gate.add_evidence(revision(), evidence("BACKTEST", passed=False))
        with self.assertRaisesRegex(ValueError, "missing passed BACKTEST"):
            self.gate.promote(item, "BACKTESTED")

    def test_automated_promotion_blocks_protected_changes(self):
        item = revision(changed_paths=("strategy.fast_period", "risk.max_daily_loss"))
        item = self.gate.add_evidence(item, evidence("BACKTEST"))
        with self.assertRaisesRegex(ValueError, "protected"):
            self.gate.promote(item, "BACKTESTED", automated=True)

    def test_immutable_state_snapshots_are_idempotent(self):
        item = revision()
        with tempfile.TemporaryDirectory() as tmp:
            store = ImmutableLifecycleStore(tmp)
            first = store.write(item)
            second = store.write(item)
            self.assertEqual(first, second)
            self.assertTrue(Path(first).exists())


if __name__ == "__main__":
    unittest.main()
