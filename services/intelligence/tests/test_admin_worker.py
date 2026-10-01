from __future__ import annotations

import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from qnext_intelligence import admin_worker


class AdminWorkerTests(unittest.TestCase):
    def request(self, action: str, payload: dict | None = None) -> dict:
        return {
            "schema": "QNEXT.INTELLIGENCE.REQUEST/1",
            "request_id": "request-1",
            "action": action,
            "requested_at_ms": 1,
            "payload": payload or {},
        }

    def test_dispatches_only_allowlisted_actions(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            with patch.object(admin_worker, "_train", return_value={"candidate_id": "c1"}) as train:
                result = admin_worker.process_request(self.request("train"), root)
                self.assertEqual(result["state"], "SUCCESS")
                self.assertEqual(result["result"]["candidate_id"], "c1")
                train.assert_called_once()

            with patch.object(admin_worker, "_promote", return_value={"candidate_id": "c1"}) as promote:
                result = admin_worker.process_request(
                    self.request("promote", {"candidateId": "c1"}), root
                )
                self.assertEqual(result["action"], "promote")
                promote.assert_called_once()

            with patch.object(admin_worker, "_rollback", return_value={"candidate_id": "c0"}) as rollback:
                result = admin_worker.process_request(
                    self.request("rollback", {"candidateId": "c0"}), root
                )
                self.assertEqual(result["action"], "rollback")
                rollback.assert_called_once()

            with self.assertRaisesRegex(ValueError, "unsupported action"):
                admin_worker.process_request(self.request("shell"), root)

    def test_rejects_unknown_schema_and_malformed_payload(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            request = self.request("train")
            request["schema"] = "OTHER"
            with self.assertRaisesRegex(ValueError, "unsupported request schema"):
                admin_worker.process_request(request, root)

            request = self.request("train")
            request["payload"] = "not-an-object"
            with self.assertRaisesRegex(ValueError, "invalid request"):
                admin_worker.process_request(request, root)


if __name__ == "__main__":
    unittest.main()
