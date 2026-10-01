<?php

declare(strict_types=1);

namespace QNext\Ops;

use JsonException;
use RuntimeException;

final class Auth
{
    public function __construct(
        private readonly string $authPath,
        private readonly string $legacyExpectedToken = '',
    ) {
    }

    public function initialized(): bool
    {
        return $this->legacyExpectedToken !== '' || is_file($this->authPath);
    }

    public function initialize(string $token): void
    {
        $token = trim($token);
        if ($this->initialized()) {
            throw new RuntimeException('admin token is already initialized');
        }
        if (strlen($token) < 16) {
            throw new RuntimeException('admin token must be at least 16 characters');
        }
        if (strlen($token) > 512) {
            throw new RuntimeException('admin token is too long');
        }
        if (str_contains($token, "\n") || str_contains($token, "\r") || str_contains($token, "\0")) {
            throw new RuntimeException('admin token contains invalid control characters');
        }

        $hash = password_hash($token, PASSWORD_DEFAULT);
        if (!is_string($hash) || $hash === '') {
            throw new RuntimeException('failed to hash admin token');
        }

        AtomicFile::writeJson($this->authPath, [
            'schema' => 'QNEXT.AUTH/1',
            'initialized' => true,
            'token_hash' => $hash,
            'created_at' => gmdate(DATE_ATOM),
        ], 0600);
    }

    public function authorized(?string $providedToken): bool
    {
        if ($providedToken === null || $providedToken === '') {
            return false;
        }

        // Prefer the persisted password hash when it exists. Older Cloudways
        // installs may still expose QNEXT_OPS_ADMIN_TOKEN while a newer
        // first-time setup has already written ops-auth.json. The previous
        // implementation returned immediately on the legacy token and made
        // the persisted credential unusable, which manifested as every
        // restricted Admin card returning HTTP 403 while public telemetry
        // remained healthy.
        if ($this->authorizedByPersistedHash($providedToken)) {
            return true;
        }

        // Keep the environment token as a bounded migration fallback. Hosts
        // should remove QNEXT_OPS_ADMIN_TOKEN after confirming the persisted
        // credential, but accepting either prevents an accidental lockout.
        return $this->legacyExpectedToken !== ''
            && hash_equals($this->legacyExpectedToken, $providedToken);
    }

    private function authorizedByPersistedHash(string $providedToken): bool
    {
        if (!is_file($this->authPath)) {
            return false;
        }

        $raw = file_get_contents($this->authPath);
        if ($raw === false || trim($raw) === '') {
            return false;
        }

        try {
            $payload = json_decode($raw, true, 32, JSON_THROW_ON_ERROR);
        } catch (JsonException) {
            return false;
        }

        $hash = is_array($payload) ? ($payload['token_hash'] ?? null) : null;
        return is_string($hash) && $hash !== '' && password_verify($providedToken, $hash);
    }
}
