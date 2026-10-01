<?php

declare(strict_types=1);

namespace QNext\Ops;

use JsonException;
use RuntimeException;

final class Auth
{
    private const SESSION_VERSION = 'v1';
    private const SESSION_TTL_SECONDS = 28800;
    private const MAX_SESSION_TTL_SECONDS = 43200;

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

        if ($this->authorizedByPersistedHash($providedToken)) {
            return true;
        }

        return $this->legacyExpectedToken !== ''
            && hash_equals($this->legacyExpectedToken, $providedToken);
    }

    public function issueSession(?int $now = null): string
    {
        $material = $this->sessionMaterial();
        if ($material === '') {
            throw new RuntimeException('admin authentication is not initialized');
        }

        $issuedAt = $now ?? time();
        $expiresAt = $issuedAt + self::SESSION_TTL_SECONDS;
        $nonce = bin2hex(random_bytes(16));
        $payload = self::SESSION_VERSION . '.' . $expiresAt . '.' . $nonce;
        $signature = hash_hmac('sha256', $payload, $material);
        return $payload . '.' . $signature;
    }

    public function authorizedSession(?string $session, ?int $now = null): bool
    {
        if ($session === null || $session === '') {
            return false;
        }

        $parts = explode('.', $session);
        if (count($parts) !== 4) {
            return false;
        }
        [$version, $expiresRaw, $nonce, $signature] = $parts;
        if ($version !== self::SESSION_VERSION
            || !ctype_digit($expiresRaw)
            || !preg_match('/^[a-f0-9]{32}$/', $nonce)
            || !preg_match('/^[a-f0-9]{64}$/', $signature)) {
            return false;
        }

        $current = $now ?? time();
        $expiresAt = (int) $expiresRaw;
        if ($expiresAt <= $current || $expiresAt > $current + self::MAX_SESSION_TTL_SECONDS) {
            return false;
        }

        $material = $this->sessionMaterial();
        if ($material === '') {
            return false;
        }
        $payload = $version . '.' . $expiresRaw . '.' . $nonce;
        $expected = hash_hmac('sha256', $payload, $material);
        return hash_equals($expected, $signature);
    }

    public static function sessionTTLSeconds(): int
    {
        return self::SESSION_TTL_SECONDS;
    }

    private function authorizedByPersistedHash(string $providedToken): bool
    {
        $hash = $this->persistedHash();
        return $hash !== '' && password_verify($providedToken, $hash);
    }

    private function sessionMaterial(): string
    {
        $hash = $this->persistedHash();
        if ($hash !== '') {
            return 'persisted:' . $hash;
        }
        if ($this->legacyExpectedToken !== '') {
            return 'legacy:' . hash('sha256', $this->legacyExpectedToken);
        }
        return '';
    }

    private function persistedHash(): string
    {
        if (!is_file($this->authPath)) {
            return '';
        }

        $raw = file_get_contents($this->authPath);
        if ($raw === false || trim($raw) === '') {
            return '';
        }

        try {
            $payload = json_decode($raw, true, 32, JSON_THROW_ON_ERROR);
        } catch (JsonException) {
            return '';
        }

        $hash = is_array($payload) ? ($payload['token_hash'] ?? null) : null;
        return is_string($hash) ? $hash : '';
    }
}
