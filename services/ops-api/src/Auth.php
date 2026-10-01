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
        return $this->persistedHash() !== ''
            || $this->recoveryConfigured()
            || ($this->legacyExpectedToken !== '' && !$this->legacyDisabled());
    }

    public function initialize(string $token): void
    {
        if ($this->initialized()) {
            throw new RuntimeException('admin token is already initialized');
        }
        $this->writeToken($token, false, 'initial_setup');
    }

    public function replaceToken(string $token, string $reason = 'token_rotation'): void
    {
        $this->writeToken($token, true, $reason);
    }

    public function authorized(?string $providedToken): bool
    {
        if ($providedToken === null || $providedToken === '') {
            return false;
        }

        if ($this->authorizedByPersistedHash($providedToken)) {
            return true;
        }

        if ($this->legacyDisabled()) {
            return false;
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

    public function generateRecoveryCode(): array
    {
        if (!$this->initialized()) {
            throw new RuntimeException('admin authentication is not initialized');
        }

        $code = rtrim(strtr(base64_encode(random_bytes(32)), '+/', '-_'), '=');
        $generatedAt = gmdate(DATE_ATOM);
        $payload = $this->persistedPayload() ?? [];
        $payload['schema'] = 'QNEXT.AUTH/2';
        $payload['initialized'] = true;
        $payload['recovery_sha256'] = hash('sha256', $code);
        $payload['recovery_generated_at'] = $generatedAt;
        $payload['updated_at'] = $generatedAt;
        if (!array_key_exists('legacy_disabled', $payload)) {
            $payload['legacy_disabled'] = false;
        }
        AtomicFile::writeJson($this->authPath, $payload, 0600);

        return [
            'code' => $code,
            'generated_at' => $generatedAt,
        ];
    }

    public function recoveryConfigured(): bool
    {
        $payload = $this->persistedPayload();
        $hash = is_array($payload) ? ($payload['recovery_sha256'] ?? null) : null;
        return is_string($hash) && preg_match('/^[a-f0-9]{64}$/', $hash) === 1;
    }

    public function recoverWithCode(string $recoveryCode, string $newToken): bool
    {
        $recoveryCode = trim($recoveryCode);
        if ($recoveryCode === '' || strlen($recoveryCode) > 256) {
            return false;
        }

        $payload = $this->persistedPayload();
        $expected = is_array($payload) ? ($payload['recovery_sha256'] ?? null) : null;
        if (!is_string($expected) || preg_match('/^[a-f0-9]{64}$/', $expected) !== 1) {
            return false;
        }
        if (!hash_equals($expected, hash('sha256', $recoveryCode))) {
            return false;
        }

        $this->writeToken($newToken, true, 'web_recovery');
        return true;
    }

    public function revokeRecoveryCode(): void
    {
        $payload = $this->persistedPayload();
        if (!is_array($payload)) {
            return;
        }
        unset($payload['recovery_sha256'], $payload['recovery_generated_at']);
        $payload['updated_at'] = gmdate(DATE_ATOM);
        AtomicFile::writeJson($this->authPath, $payload, 0600);
    }

    public static function sessionTTLSeconds(): int
    {
        return self::SESSION_TTL_SECONDS;
    }

    private function writeToken(string $token, bool $disableLegacy, string $reason): void
    {
        $token = trim($token);
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

        $existing = $this->persistedPayload();
        $createdAt = is_array($existing) && is_string($existing['created_at'] ?? null)
            ? $existing['created_at']
            : gmdate(DATE_ATOM);

        AtomicFile::writeJson($this->authPath, [
            'schema' => 'QNEXT.AUTH/2',
            'initialized' => true,
            'token_hash' => $hash,
            'legacy_disabled' => $disableLegacy,
            'reason' => $reason,
            'created_at' => $createdAt,
            'updated_at' => gmdate(DATE_ATOM),
        ], 0600);
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
        if ($this->legacyExpectedToken !== '' && !$this->legacyDisabled()) {
            return 'legacy:' . hash('sha256', $this->legacyExpectedToken);
        }
        return '';
    }

    private function legacyDisabled(): bool
    {
        $payload = $this->persistedPayload();
        return is_array($payload) && ($payload['legacy_disabled'] ?? false) === true;
    }

    private function persistedHash(): string
    {
        $payload = $this->persistedPayload();
        $hash = is_array($payload) ? ($payload['token_hash'] ?? null) : null;
        return is_string($hash) ? $hash : '';
    }

    private function persistedPayload(): ?array
    {
        if (!is_file($this->authPath)) {
            return null;
        }

        $raw = file_get_contents($this->authPath);
        if ($raw === false || trim($raw) === '') {
            return null;
        }

        try {
            $payload = json_decode($raw, true, 32, JSON_THROW_ON_ERROR);
        } catch (JsonException) {
            return null;
        }

        return is_array($payload) ? $payload : null;
    }
}
