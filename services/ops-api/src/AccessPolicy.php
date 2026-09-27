<?php

declare(strict_types=1);

namespace QNext\Ops;

use RuntimeException;

final class AccessPolicy
{
    public const STATUS_PENDING_APPROVAL = 'PENDING_APPROVAL';
    public const STATUS_ACCOUNT_REQUIRED = 'ACCOUNT_REQUIRED';
    public const STATUS_ACTIVE = 'ACTIVE';
    public const STATUS_REJECTED = 'REJECTED';

    public const DECISION_APPROVED = 'APPROVED';
    public const DECISION_REJECTED = 'REJECTED';
    public const DECISION_REVOKED = 'REVOKED';

    public const ROLE_USER = 'USER';
    public const ROLE_ADMIN = 'ADMIN';

    public function __construct(
        private readonly int $userQuorum = 3,
        private readonly bool $adminOverride = true,
        private readonly bool $requireActiveAccount = true,
    ) {
        if ($this->userQuorum < 1) {
            throw new RuntimeException('user approval quorum must be at least 1');
        }
    }

    /**
     * @param array<string,mixed> $registration
     * @param list<array<string,mixed>> $events
     */
    public function evaluate(array $registration, array $events): string
    {
        $latest = $this->latestByApprover($events);
        $adminApproved = false;
        $adminRejected = false;
        $userApprovals = 0;

        foreach ($latest as $event) {
            $role = (string) ($event['approver_role'] ?? '');
            $decision = (string) ($event['decision'] ?? '');

            if ($role === self::ROLE_ADMIN && $decision === self::DECISION_REJECTED) {
                $adminRejected = true;
            }
            if ($decision !== self::DECISION_APPROVED) {
                continue;
            }
            if ($role === self::ROLE_ADMIN) {
                $adminApproved = true;
            } elseif ($role === self::ROLE_USER) {
                $userApprovals++;
            }
        }

        if ($adminRejected) {
            return self::STATUS_REJECTED;
        }

        $approved = ($this->adminOverride && $adminApproved) || $userApprovals >= $this->userQuorum;
        if (!$approved) {
            return self::STATUS_PENDING_APPROVAL;
        }

        if ($this->requireActiveAccount && !((bool) ($registration['has_active_account'] ?? false))) {
            return self::STATUS_ACCOUNT_REQUIRED;
        }

        return self::STATUS_ACTIVE;
    }

    /** @param array<string,mixed> $registration @param array<string,mixed> $event */
    public function validateEvent(array $registration, array $event): void
    {
        $applicant = (string) ($registration['applicant_user_id'] ?? '');
        $approver = (string) ($event['approver_user_id'] ?? '');
        if ($applicant !== '' && hash_equals($applicant, $approver)) {
            throw new RuntimeException('applicant cannot approve own registration');
        }

        if (!in_array((string) ($event['approver_role'] ?? ''), [self::ROLE_USER, self::ROLE_ADMIN], true)) {
            throw new RuntimeException('invalid approver role');
        }
        if (!in_array((string) ($event['decision'] ?? ''), [self::DECISION_APPROVED, self::DECISION_REJECTED, self::DECISION_REVOKED], true)) {
            throw new RuntimeException('invalid approval decision');
        }
    }

    /**
     * @param list<array<string,mixed>> $events
     * @return array<string,array<string,mixed>>
     */
    private function latestByApprover(array $events): array
    {
        $latest = [];
        foreach ($events as $index => $event) {
            $approver = (string) ($event['approver_user_id'] ?? '');
            if ($approver === '') {
                continue;
            }
            $timestamp = (string) ($event['created_at'] ?? '');
            $order = [$timestamp, $index];
            if (!isset($latest[$approver]) || $order > $latest[$approver]['_order']) {
                $event['_order'] = $order;
                $latest[$approver] = $event;
            }
        }
        foreach ($latest as &$event) {
            unset($event['_order']);
        }
        unset($event);
        return $latest;
    }
}
