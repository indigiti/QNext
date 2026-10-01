<?php

declare(strict_types=1);

namespace QNext\Ops;

final class BreakGlassRecovery
{
    /**
     * One-time recovery code digest for the October 2026 stage lockout.
     * The plaintext code is never committed to the repository or release.
     */
    private const CODE_SHA256 = '49980f844f8d0b17b0867c10c5e3a4520646f8e8a887d9c7d13ed83fb05fd7e9';
    private const EXPIRES_AT = 1791158399; // 2026-10-04T23:59:59Z

    public function __construct(private readonly string $markerPath)
    {
    }

    public function available(?int $now = null): bool
    {
        $current = $now ?? time();
        return $current <= self::EXPIRES_AT && !is_file($this->markerPath);
    }

    public function verify(string $code, ?int $now = null): bool
    {
        if (!$this->available($now)) {
            return false;
        }

        $code = trim($code);
        if ($code === '' || strlen($code) > 256) {
            return false;
        }

        return hash_equals(self::CODE_SHA256, hash('sha256', $code));
    }

    public function markUsed(array $metadata = []): void
    {
        AtomicFile::writeJson($this->markerPath, [
            'schema' => 'QNEXT.AUTH_RECOVERY/1',
            'used' => true,
            'used_at' => gmdate(DATE_ATOM),
            'metadata' => $metadata,
        ], 0600);
    }

    public static function expiresAtISO8601(): string
    {
        return gmdate(DATE_ATOM, self::EXPIRES_AT);
    }
}
