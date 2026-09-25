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
        private readonly string $bootstrapExpectedToken = '',
    ) {
    }

    public function initialized(): bool
    {
        return $this->legacyExpectedToken !== '' || is_file($this->authPath);
    }

    public function bootstrapConfigured(): bool
    {
        return $this->bootstrapExpectedToken !== '';
    }

    public function bootstrapAuthorized(?string $providedToken): bool
    {
        if ($this->initialized() || !$this->bootstrapConfigured()) {
            return false;
        }
        if ($providedToken === null || $providedToken === '') {
            return false;
        }

        return hash_equals($this->bootstrapExpectedToken, $providedToken);
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

        if ($this->legacyExpectedToken !== '') {
            return hash_equals($this->legacyExpectedToken, $providedToken);
        }

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
