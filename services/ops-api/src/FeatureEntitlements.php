<?php

declare(strict_types=1);

namespace QNext\Ops;

use RuntimeException;

final class FeatureEntitlements
{
    private const INTERVALS = [
        '15s', '30s', '1m', '2m', '3m', '5m', '10m', '15m', '30m', '45m',
        '1h', '2h', '3h', '4h', '1D', '1W', '1M',
    ];

    public function __construct(private readonly InviteApprovalStore $store)
    {
    }

    /** @return array<string,mixed> */
    public function defaults(): array
    {
        $intervals = array_fill_keys(self::INTERVALS, false);
        foreach (['1m', '2m', '3m', '5m', '15m', '30m', '1h', '1D'] as $interval) {
            $intervals[$interval] = true;
        }

        return [
            'charts' => [
                'standard' => true,
                'synthetic' => false,
                'intervals' => $intervals,
            ],
            'custom_indicators' => false,
            'backtesting' => false,
            'strategy_lab' => false,
            'intelligence' => false,
            'paper_trading' => false,
            'live_trading' => false,
        ];
    }

    /** @return array<string,mixed> */
    public function forUser(string $userId): array
    {
        $userId = $this->validateUserId($userId);
        return array_replace_recursive($this->defaults(), $this->store->entitlements($userId));
    }

    /** @param array<string,mixed> $input @return array<string,mixed> */
    public function setForUser(string $userId, array $input, string $actorUserId): array
    {
        $userId = $this->validateUserId($userId);
        $actorUserId = $this->validateUserId($actorUserId);
        $current = $this->forUser($userId);

        $charts = $input['charts'] ?? $current['charts'];
        if (!is_array($charts)) {
            throw new RuntimeException('charts entitlement must be an object');
        }
        $intervals = $charts['intervals'] ?? $current['charts']['intervals'];
        if (!is_array($intervals)) {
            throw new RuntimeException('chart intervals entitlement must be an object');
        }

        $normalizedIntervals = $current['charts']['intervals'];
        foreach ($intervals as $interval => $enabled) {
            if (!in_array($interval, self::INTERVALS, true)) {
                throw new RuntimeException('unsupported chart interval entitlement: ' . $interval);
            }
            if (!is_bool($enabled)) {
                throw new RuntimeException('chart interval entitlement must be boolean: ' . $interval);
            }
            $normalizedIntervals[$interval] = $enabled;
        }

        $normalized = [
            'charts' => [
                'standard' => $this->boolValue($charts['standard'] ?? $current['charts']['standard'], 'charts.standard'),
                'synthetic' => $this->boolValue($charts['synthetic'] ?? $current['charts']['synthetic'], 'charts.synthetic'),
                'intervals' => $normalizedIntervals,
            ],
            'custom_indicators' => $this->boolValue($input['custom_indicators'] ?? $current['custom_indicators'], 'custom_indicators'),
            'backtesting' => $this->boolValue($input['backtesting'] ?? $current['backtesting'], 'backtesting'),
            'strategy_lab' => $this->boolValue($input['strategy_lab'] ?? $current['strategy_lab'], 'strategy_lab'),
            'intelligence' => $this->boolValue($input['intelligence'] ?? $current['intelligence'], 'intelligence'),
            'paper_trading' => $this->boolValue($input['paper_trading'] ?? $current['paper_trading'], 'paper_trading'),
            'live_trading' => $this->boolValue($input['live_trading'] ?? $current['live_trading'], 'live_trading'),
            'updated_at' => gmdate(DATE_ATOM),
            'updated_by_user_id' => $actorUserId,
        ];

        $this->store->putEntitlements($userId, $normalized);
        return $normalized;
    }

    /** @return array<string,mixed> */
    public function effectiveForUser(string $userId): array
    {
        $userId = $this->validateUserId($userId);
        $registration = $this->latestRegistrationForUser($userId);
        $registrationActive = false;
        $registrationStatus = null;

        if ($registration !== null) {
            $events = $this->store->approvalEventsFor((string) $registration['id']);
            $policy = $this->store->policy();
            $registrationStatus = (new AccessPolicy(
                (int) ($policy['user_quorum'] ?? 3),
                (bool) ($policy['admin_override'] ?? true),
                (bool) ($policy['require_active_account'] ?? true),
            ))->evaluate($registration, $events);
            $registrationActive = $registrationStatus === AccessPolicy::STATUS_ACTIVE;
        }

        return [
            'user_id' => $userId,
            'registration_status' => $registrationStatus,
            'access_active' => $registrationActive,
            'entitlements' => $registrationActive ? $this->forUser($userId) : $this->disabled(),
        ];
    }

    /** @return array<string,mixed>|null */
    private function latestRegistrationForUser(string $userId): ?array
    {
        $matches = [];
        foreach ($this->store->registrations() as $registration) {
            if (($registration['applicant_user_id'] ?? null) === $userId) {
                $matches[] = $registration;
            }
        }
        if ($matches === []) {
            return null;
        }
        usort($matches, static fn (array $a, array $b): int => strcmp((string) ($b['created_at'] ?? ''), (string) ($a['created_at'] ?? '')));
        return $matches[0];
    }

    /** @return array<string,mixed> */
    private function disabled(): array
    {
        $disabled = $this->defaults();
        $disabled['charts']['standard'] = false;
        $disabled['charts']['synthetic'] = false;
        foreach ($disabled['charts']['intervals'] as $interval => $_) {
            $disabled['charts']['intervals'][$interval] = false;
        }
        foreach (['custom_indicators', 'backtesting', 'strategy_lab', 'intelligence', 'paper_trading', 'live_trading'] as $feature) {
            $disabled[$feature] = false;
        }
        return $disabled;
    }

    private function validateUserId(string $userId): string
    {
        $userId = trim($userId);
        if ($userId === '' || strlen($userId) > 128 || preg_match('/^[A-Za-z0-9._:@+-]+$/', $userId) !== 1) {
            throw new RuntimeException('user id is invalid');
        }
        return $userId;
    }

    private function boolValue(mixed $value, string $label): bool
    {
        if (!is_bool($value)) {
            throw new RuntimeException($label . ' must be boolean');
        }
        return $value;
    }
}
