<?php

declare(strict_types=1);

namespace QNext\Ops;

use DateTimeImmutable;
use DateTimeZone;
use RuntimeException;

final class AccessGovernance
{
    public function __construct(private readonly InviteApprovalStore $store)
    {
    }

    /** @return array<string,mixed> */
    public function policy(): array
    {
        return $this->store->policy();
    }

    /** @param array<string,mixed> $input @return array<string,mixed> */
    public function updatePolicy(array $input): array
    {
        $current = $this->store->policy();
        $policy = [
            'invite_only' => true,
            'user_quorum' => $this->boundedInt($input['user_quorum'] ?? $current['user_quorum'] ?? 3, 1, 20, 'user_quorum'),
            'admin_override' => $this->boolValue($input['admin_override'] ?? $current['admin_override'] ?? true, 'admin_override'),
            'require_active_account' => $this->boolValue($input['require_active_account'] ?? $current['require_active_account'] ?? true, 'require_active_account'),
            'invite_ttl_hours' => $this->boundedInt($input['invite_ttl_hours'] ?? $current['invite_ttl_hours'] ?? 168, 1, 720, 'invite_ttl_hours'),
        ];
        $this->store->putPolicy($policy);
        return $policy;
    }

    /** @param array<string,mixed> $input @return array<string,mixed> */
    public function issueInvite(array $input, string $actorUserId): array
    {
        $actorUserId = $this->requiredId($actorUserId, 'actor user id');
        $email = strtolower(trim((string) ($input['email'] ?? '')));
        $phone = trim((string) ($input['phone'] ?? ''));
        if ($email === '' && $phone === '') {
            throw new RuntimeException('invite email or phone is required');
        }
        if ($email !== '' && filter_var($email, FILTER_VALIDATE_EMAIL) === false) {
            throw new RuntimeException('invite email is invalid');
        }
        if (strlen($phone) > 32) {
            throw new RuntimeException('invite phone is too long');
        }

        $policy = $this->store->policy();
        $ttlHours = (int) ($policy['invite_ttl_hours'] ?? 168);
        $now = $this->now();
        $expires = $now->modify('+' . $ttlHours . ' hours');
        $token = $this->token();
        $invite = [
            'id' => $this->id('inv'),
            'email' => $email,
            'phone' => $phone,
            'token_hash' => hash('sha256', $token),
            'invited_by_user_id' => $actorUserId,
            'invited_by_role' => AccessPolicy::ROLE_ADMIN,
            'status' => 'OPEN',
            'created_at' => $now->format(DATE_ATOM),
            'expires_at' => $expires->format(DATE_ATOM),
            'used_at' => null,
            'registration_id' => null,
        ];
        $this->store->putInvite($invite);

        $response = $this->sanitizeInvite($invite);
        $response['invite_token'] = $token;
        return $response;
    }

    /** @return array<string,mixed> */
    public function inspectInvite(string $token): array
    {
        $invite = $this->inviteForToken($token);
        $valid = $this->inviteIsUsable($invite);
        return [
            'valid' => $valid,
            'invite_id' => $invite['id'] ?? null,
            'status' => $valid ? 'OPEN' : ($invite['status'] ?? 'INVALID'),
            'expires_at' => $invite['expires_at'] ?? null,
        ];
    }

    /** @param array<string,mixed> $input @return array<string,mixed> */
    public function redeemInvite(array $input): array
    {
        $token = trim((string) ($input['invite_token'] ?? ''));
        $applicantUserId = $this->requiredId((string) ($input['applicant_user_id'] ?? ''), 'applicant user id');
        $invite = $this->inviteForToken($token);
        if (!$this->inviteIsUsable($invite)) {
            throw new RuntimeException('invite is expired, used, or invalid');
        }

        foreach ($this->store->registrations() as $existing) {
            if (($existing['applicant_user_id'] ?? null) === $applicantUserId
                && !in_array((string) ($existing['status'] ?? ''), [AccessPolicy::STATUS_REJECTED], true)) {
                throw new RuntimeException('applicant already has a registration');
            }
        }

        $now = $this->now()->format(DATE_ATOM);
        $registration = [
            'id' => $this->id('reg'),
            'applicant_user_id' => $applicantUserId,
            'invite_id' => $invite['id'],
            'invited_by_user_id' => $invite['invited_by_user_id'] ?? '',
            'status' => AccessPolicy::STATUS_PENDING_APPROVAL,
            'has_active_account' => false,
            'created_at' => $now,
            'updated_at' => $now,
        ];
        $this->store->putRegistration($registration);

        $invite['status'] = 'USED';
        $invite['used_at'] = $now;
        $invite['registration_id'] = $registration['id'];
        $this->store->putInvite($invite);

        return $this->registrationSnapshot($registration['id']);
    }

    /** @return array<string,mixed> */
    public function setApprover(string $userId, bool $eligible, string $role, string $actorUserId): array
    {
        $userId = $this->requiredId($userId, 'approver user id');
        $actorUserId = $this->requiredId($actorUserId, 'actor user id');
        $role = strtoupper(trim($role));
        if (!in_array($role, [AccessPolicy::ROLE_USER, AccessPolicy::ROLE_ADMIN], true)) {
            throw new RuntimeException('approver role must be USER or ADMIN');
        }

        $record = [
            'user_id' => $userId,
            'role' => $role,
            'eligible' => $eligible,
            'updated_at' => $this->now()->format(DATE_ATOM),
            'updated_by_user_id' => $actorUserId,
        ];
        $this->store->putApprover($record);
        return $record;
    }

    /**
     * This method requires a trusted caller. USER decisions must come from a verified
     * QNext user session; ADMIN decisions are expected to be protected by ops admin auth.
     *
     * @return array<string,mixed>
     */
    public function recordDecision(string $registrationId, string $approverUserId, string $role, string $decision, string $comment = ''): array
    {
        $registration = $this->store->registration($registrationId);
        if ($registration === null) {
            throw new RuntimeException('registration not found');
        }

        $approverUserId = $this->requiredId($approverUserId, 'approver user id');
        $role = strtoupper(trim($role));
        $decision = strtoupper(trim($decision));

        if ($role === AccessPolicy::ROLE_USER) {
            $approver = $this->store->approver($approverUserId);
            if ($approver === null || !((bool) ($approver['eligible'] ?? false)) || ($approver['role'] ?? null) !== AccessPolicy::ROLE_USER) {
                throw new RuntimeException('user is not an eligible approver');
            }
        } elseif ($role !== AccessPolicy::ROLE_ADMIN) {
            throw new RuntimeException('invalid approver role');
        }

        $event = [
            'id' => $this->id('apr'),
            'registration_id' => $registrationId,
            'applicant_user_id' => $registration['applicant_user_id'],
            'approver_user_id' => $approverUserId,
            'approver_role' => $role,
            'decision' => $decision,
            'comment' => mb_substr(trim($comment), 0, 500),
            'created_at' => $this->now()->format(DATE_ATOM),
        ];

        $policy = $this->evaluator();
        $policy->validateEvent($registration, $event);
        $this->store->appendApprovalEvent($event);
        $this->refreshRegistration($registrationId);
        return $this->registrationSnapshot($registrationId);
    }

    /** @return array<string,mixed> */
    public function setAccountGate(string $registrationId, bool $active, string $actorUserId): array
    {
        $registration = $this->store->registration($registrationId);
        if ($registration === null) {
            throw new RuntimeException('registration not found');
        }
        $this->requiredId($actorUserId, 'actor user id');

        $registration['has_active_account'] = $active;
        $registration['account_gate_updated_at'] = $this->now()->format(DATE_ATOM);
        $registration['account_gate_updated_by_user_id'] = $actorUserId;
        $this->store->putRegistration($registration);
        $this->refreshRegistration($registrationId);
        return $this->registrationSnapshot($registrationId);
    }

    /** @return array<string,mixed> */
    public function registrationSnapshot(string $registrationId): array
    {
        $registration = $this->store->registration($registrationId);
        if ($registration === null) {
            throw new RuntimeException('registration not found');
        }
        $events = $this->store->approvalEventsFor($registrationId);
        $status = $this->evaluator()->evaluate($registration, $events);

        return [
            'registration' => array_replace($registration, ['status' => $status]),
            'approval_events' => $events,
            'approval_summary' => $this->approvalSummary($events),
        ];
    }

    /** @return array<string,mixed> */
    public function adminSnapshot(): array
    {
        $state = $this->store->load();
        $invites = [];
        foreach ($state['invites'] ?? [] as $invite) {
            if (is_array($invite)) {
                $invites[] = $this->sanitizeInvite($invite);
            }
        }
        $registrations = [];
        foreach ($this->store->registrations() as $registration) {
            $registrations[] = $this->registrationSnapshot((string) $registration['id']);
        }

        return [
            'policy' => $this->store->policy(),
            'invites' => $invites,
            'registrations' => $registrations,
            'approvers' => $this->store->approvers(),
        ];
    }

    private function refreshRegistration(string $registrationId): void
    {
        $registration = $this->store->registration($registrationId);
        if ($registration === null) {
            throw new RuntimeException('registration not found');
        }
        $registration['status'] = $this->evaluator()->evaluate($registration, $this->store->approvalEventsFor($registrationId));
        $registration['updated_at'] = $this->now()->format(DATE_ATOM);
        $this->store->putRegistration($registration);
    }

    private function evaluator(): AccessPolicy
    {
        $policy = $this->store->policy();
        return new AccessPolicy(
            (int) ($policy['user_quorum'] ?? 3),
            (bool) ($policy['admin_override'] ?? true),
            (bool) ($policy['require_active_account'] ?? true),
        );
    }

    /** @param list<array<string,mixed>> $events @return array<string,mixed> */
    private function approvalSummary(array $events): array
    {
        $latest = [];
        foreach ($events as $index => $event) {
            $id = (string) ($event['approver_user_id'] ?? '');
            if ($id === '') {
                continue;
            }
            $order = [(string) ($event['created_at'] ?? ''), $index];
            if (!isset($latest[$id]) || $order > $latest[$id]['order']) {
                $latest[$id] = ['order' => $order, 'event' => $event];
            }
        }

        $userApprovals = 0;
        $adminApprovals = 0;
        $rejections = 0;
        foreach ($latest as $row) {
            $event = $row['event'];
            if (($event['decision'] ?? null) === AccessPolicy::DECISION_APPROVED) {
                if (($event['approver_role'] ?? null) === AccessPolicy::ROLE_ADMIN) {
                    $adminApprovals++;
                } elseif (($event['approver_role'] ?? null) === AccessPolicy::ROLE_USER) {
                    $userApprovals++;
                }
            } elseif (($event['decision'] ?? null) === AccessPolicy::DECISION_REJECTED) {
                $rejections++;
            }
        }

        return [
            'user_approvals' => $userApprovals,
            'admin_approvals' => $adminApprovals,
            'rejections' => $rejections,
            'required_user_quorum' => (int) ($this->store->policy()['user_quorum'] ?? 3),
        ];
    }

    /** @return array<string,mixed> */
    private function inviteForToken(string $token): array
    {
        $token = trim($token);
        if (strlen($token) < 32 || strlen($token) > 256) {
            throw new RuntimeException('invite token is invalid');
        }
        $invite = $this->store->inviteByTokenHash(hash('sha256', $token));
        if ($invite === null) {
            throw new RuntimeException('invite token is invalid');
        }
        return $invite;
    }

    /** @param array<string,mixed> $invite */
    private function inviteIsUsable(array $invite): bool
    {
        if (($invite['status'] ?? null) !== 'OPEN') {
            return false;
        }
        $expiresAt = (string) ($invite['expires_at'] ?? '');
        if ($expiresAt === '') {
            return false;
        }
        try {
            return new DateTimeImmutable($expiresAt) > $this->now();
        } catch (\Throwable) {
            return false;
        }
    }

    /** @param array<string,mixed> $invite @return array<string,mixed> */
    private function sanitizeInvite(array $invite): array
    {
        unset($invite['token_hash']);
        return $invite;
    }

    private function now(): DateTimeImmutable
    {
        return new DateTimeImmutable('now', new DateTimeZone('UTC'));
    }

    private function id(string $prefix): string
    {
        return $prefix . '_' . bin2hex(random_bytes(12));
    }

    private function token(): string
    {
        return rtrim(strtr(base64_encode(random_bytes(32)), '+/', '-_'), '=');
    }

    private function requiredId(string $value, string $label): string
    {
        $value = trim($value);
        if ($value === '' || strlen($value) > 128 || preg_match('/^[A-Za-z0-9._:@+-]+$/', $value) !== 1) {
            throw new RuntimeException($label . ' is invalid');
        }
        return $value;
    }

    private function boundedInt(mixed $value, int $min, int $max, string $label): int
    {
        if (filter_var($value, FILTER_VALIDATE_INT) === false) {
            throw new RuntimeException($label . ' must be an integer');
        }
        $value = (int) $value;
        if ($value < $min || $value > $max) {
            throw new RuntimeException($label . " must be between {$min} and {$max}");
        }
        return $value;
    }

    private function boolValue(mixed $value, string $label): bool
    {
        if (!is_bool($value)) {
            throw new RuntimeException($label . ' must be boolean');
        }
        return $value;
    }
}
