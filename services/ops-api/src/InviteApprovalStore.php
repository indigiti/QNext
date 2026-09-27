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
            return [
                'schema' => 'QNEXT.ACCESS/1',
                'policy' => [
                    'invite_only' => true,
                    'user_quorum' => 3,
                    'admin_override' => true,
                    'require_active_account' => true,
                ],
                'invites' => [],
                'registrations' => [],
                'approval_events' => [],
            ];
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

        return $payload;
    }

    /** @param array<string,mixed> $invite */
    public function appendInvite(array $invite): void
    {
        $this->mutate(function (array &$state) use ($invite): void {
            $state['invites'][] = $invite;
        });
    }

    /** @param array<string,mixed> $registration */
    public function putRegistration(array $registration): void
    {
        $id = trim((string) ($registration['id'] ?? ''));
        if ($id === '') {
            throw new RuntimeException('registration id is required');
        }

        $this->mutate(function (array &$state) use ($id, $registration): void {
            $state['registrations'][$id] = $registration;
        });
    }

    /** @param array<string,mixed> $event */
    public function appendApprovalEvent(array $event): void
    {
        foreach (['id', 'registration_id', 'applicant_user_id', 'approver_user_id', 'approver_role', 'decision', 'created_at'] as $field) {
            if (trim((string) ($event[$field] ?? '')) === '') {
                throw new RuntimeException("approval event field {$field} is required");
            }
        }

        $this->mutate(function (array &$state) use ($event): void {
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
        $state = $this->load();
        $events = [];
        foreach ($state['approval_events'] ?? [] as $event) {
            if (($event['registration_id'] ?? null) === $registrationId) {
                $events[] = $event;
            }
        }
        return $events;
    }

    /** @param callable(array<string,mixed>&):void $callback */
    private function mutate(callable $callback): void
    {
        $state = $this->load();
        $callback($state);
        AtomicFile::writeJson($this->path, $state, 0600);
    }
}
