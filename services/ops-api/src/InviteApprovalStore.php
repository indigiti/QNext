<?php

declare(strict_types=1);

namespace QNext\Ops;

use JsonException;
use RuntimeException;

final class InviteApprovalStore
{
    public function __construct(private readonly string $path)
    {
    }

    /** @return array<string,mixed> */
    public function load(): array
    {
        if (!is_file($this->path)) {
            return $this->defaults();
        }

        $raw = file_get_contents($this->path);
        if ($raw === false || trim($raw) === '') {
            throw new RuntimeException('failed to read access store');
        }

        try {
            $payload = json_decode($raw, true, 64, JSON_THROW_ON_ERROR);
        } catch (JsonException $e) {
            throw new RuntimeException('access store contains invalid JSON', 0, $e);
        }

        if (!is_array($payload) || ($payload['schema'] ?? null) !== 'QNEXT.ACCESS/1') {
            throw new RuntimeException('unsupported access store schema');
        }

        return array_replace_recursive($this->defaults(), $payload);
    }

    /** @return array<string,mixed> */
    public function policy(): array
    {
        return (array) ($this->load()['policy'] ?? []);
    }

    /** @param array<string,mixed> $policy */
    public function putPolicy(array $policy): void
    {
        $this->mutate(static function (array &$state) use ($policy): void {
            $state['policy'] = $policy;
        });
    }

    /** @param array<string,mixed> $invite */
    public function putInvite(array $invite): void
    {
        $id = trim((string) ($invite['id'] ?? ''));
        if ($id === '') {
            throw new RuntimeException('invite id is required');
        }

        $this->mutate(static function (array &$state) use ($id, $invite): void {
            foreach ($state['invites'] as $index => $existing) {
                if (($existing['id'] ?? null) === $id) {
                    $state['invites'][$index] = $invite;
                    return;
                }
            }
            $state['invites'][] = $invite;
        });
    }

    /** @return array<string,mixed>|null */
    public function inviteById(string $id): ?array
    {
        foreach ($this->load()['invites'] ?? [] as $invite) {
            if (($invite['id'] ?? null) === $id) {
                return is_array($invite) ? $invite : null;
            }
        }
        return null;
    }

    /** @return array<string,mixed>|null */
    public function inviteByTokenHash(string $tokenHash): ?array
    {
        foreach ($this->load()['invites'] ?? [] as $invite) {
            $stored = (string) ($invite['token_hash'] ?? '');
            if ($stored !== '' && hash_equals($stored, $tokenHash)) {
                return is_array($invite) ? $invite : null;
            }
        }
        return null;
    }

    /** @param array<string,mixed> $registration */
    public function putRegistration(array $registration): void
    {
        $id = trim((string) ($registration['id'] ?? ''));
        if ($id === '') {
            throw new RuntimeException('registration id is required');
        }

        $this->mutate(static function (array &$state) use ($id, $registration): void {
            $state['registrations'][$id] = $registration;
        });
    }

    /** @return array<string,mixed>|null */
    public function registration(string $id): ?array
    {
        $registration = $this->load()['registrations'][$id] ?? null;
        return is_array($registration) ? $registration : null;
    }

    /** @return list<array<string,mixed>> */
    public function registrations(): array
    {
        return array_values(array_filter(
            $this->load()['registrations'] ?? [],
            static fn (mixed $row): bool => is_array($row),
        ));
    }

    /** @param array<string,mixed> $approver */
    public function putApprover(array $approver): void
    {
        $userId = trim((string) ($approver['user_id'] ?? ''));
        if ($userId === '') {
            throw new RuntimeException('approver user id is required');
        }

        $this->mutate(static function (array &$state) use ($userId, $approver): void {
            $state['approvers'][$userId] = $approver;
        });
    }

    /** @return array<string,mixed>|null */
    public function approver(string $userId): ?array
    {
        $approver = $this->load()['approvers'][$userId] ?? null;
        return is_array($approver) ? $approver : null;
    }

    /** @return list<array<string,mixed>> */
    public function approvers(): array
    {
        return array_values(array_filter(
            $this->load()['approvers'] ?? [],
            static fn (mixed $row): bool => is_array($row),
        ));
    }

    /** @param array<string,mixed> $entitlements */
    public function putEntitlements(string $userId, array $entitlements): void
    {
        $userId = trim($userId);
        if ($userId === '') {
            throw new RuntimeException('entitlement user id is required');
        }
        $this->mutate(static function (array &$state) use ($userId, $entitlements): void {
            $state['entitlements'][$userId] = $entitlements;
        });
    }

    /** @return array<string,mixed> */
    public function entitlements(string $userId): array
    {
        $row = $this->load()['entitlements'][$userId] ?? [];
        return is_array($row) ? $row : [];
    }

    /** @return array<string,array<string,mixed>> */
    public function allEntitlements(): array
    {
        $rows = $this->load()['entitlements'] ?? [];
        return is_array($rows) ? $rows : [];
    }

    /** @param array<string,mixed> $event */
    public function appendApprovalEvent(array $event): void
    {
        foreach (['id', 'registration_id', 'applicant_user_id', 'approver_user_id', 'approver_role', 'decision', 'created_at'] as $field) {
            if (trim((string) ($event[$field] ?? '')) === '') {
                throw new RuntimeException("approval event field {$field} is required");
            }
        }

        $this->mutate(static function (array &$state) use ($event): void {
            foreach ($state['approval_events'] as $existing) {
                if (($existing['id'] ?? null) === $event['id']) {
                    throw new RuntimeException('duplicate approval event id');
                }
            }
            $state['approval_events'][] = $event;
        });
    }

    /** @return list<array<string,mixed>> */
    public function approvalEventsFor(string $registrationId): array
    {
        $events = [];
        foreach ($this->load()['approval_events'] ?? [] as $event) {
            if (is_array($event) && ($event['registration_id'] ?? null) === $registrationId) {
                $events[] = $event;
            }
        }
        return $events;
    }

    /** @return array<string,mixed> */
    private function defaults(): array
    {
        return [
            'schema' => 'QNEXT.ACCESS/1',
            'policy' => [
                'invite_only' => true,
                'user_quorum' => 3,
                'admin_override' => true,
                'require_active_account' => true,
                'invite_ttl_hours' => 168,
            ],
            'invites' => [],
            'registrations' => [],
            'approvers' => [],
            'entitlements' => [],
            'approval_events' => [],
        ];
    }

    /** @param callable(array<string,mixed>&):void $callback */
    private function mutate(callable $callback): void
    {
        $state = $this->load();
        $callback($state);
        AtomicFile::writeJson($this->path, $state, 0600);
    }
}
