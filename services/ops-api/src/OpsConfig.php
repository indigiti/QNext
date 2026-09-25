<?php

declare(strict_types=1);

namespace QNext\Ops;

use RuntimeException;

final class OpsConfig
{
    public function __construct(
        public readonly string $privateRoot,
        public readonly string $publicRoot,
        public readonly string $marketCoreUrl,
        public readonly string $adminToken,
        public readonly string $helperPath,
    ) {
    }

    public static function fromEnvironment(): self
    {
        return new self(
            self::required('QNEXT_PRIVATE_ROOT'),
            self::required('QNEXT_PUBLIC_ROOT'),
            rtrim(self::value('QNEXT_MARKET_CORE_URL', 'http://127.0.0.1:8080'), '/'),
            self::value('QNEXT_OPS_ADMIN_TOKEN'),
            self::value('QNEXT_OPS_HELPER', '/usr/local/bin/qnext-ops-web'),
        );
    }

    public function configPath(): string
    {
        return $this->privateRoot . '/config/q1-market.json';
    }

    public function secretsPath(): string
    {
        return $this->privateRoot . '/secrets/qnext.env';
    }

    public function authPath(): string
    {
        return $this->privateRoot . '/secrets/ops-auth.json';
    }

    public function releasesRoot(): string
    {
        return $this->privateRoot . '/releases';
    }

    public function currentLink(): string
    {
        return $this->privateRoot . '/current';
    }

    private static function value(string $key, string $default = ''): string
    {
        $environment = getenv($key);
        if (is_string($environment) && trim($environment) !== '') {
            return trim($environment);
        }

        if (isset($_SERVER[$key]) && trim((string) $_SERVER[$key]) !== '') {
            return trim((string) $_SERVER[$key]);
        }

        if (isset($_ENV[$key]) && trim((string) $_ENV[$key]) !== '') {
            return trim((string) $_ENV[$key]);
        }

        return $default;
    }

    private static function required(string $key): string
    {
        $value = self::value($key);
        if ($value === '') {
            throw new RuntimeException($key . ' is required');
        }
        return rtrim($value, '/');
    }
}
