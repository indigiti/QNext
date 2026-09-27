# QNext Identity, Invitation & Access Governance

This workstream introduces invite-only registration, auditable approval provenance, broker/data account gating, and feature entitlement checks for multi-user QNext.

## Access chain

Invite -> registration -> identity verification -> approval -> broker/data account gate -> feature entitlement -> QNext workspace.

## Approval policy

- Registration is invite-only.
- A registration is approved by either one authorized administrator or a configurable quorum of eligible existing users.
- Default user quorum: 3.
- Self-approval is not allowed.
- Each approver counts at most once toward the effective quorum.
- Approval, rejection, revocation, invitation, and activation events are retained as audit history.

## Account gate

An approved user must connect at least one eligible active broker/data account before market-data features become available. The account gate is distinct from feature entitlements.

## Entitlements

Entitlements are evaluated separately from registration approval and account connection. Examples include synthetic charts, 15s/30s chart intervals, custom indicators, Strategy Lab, paper trading, and live trading.

## Persistence

The domain is repository-driven so the current file-backed implementation can later be replaced with MariaDB without changing the access-control workflow.
