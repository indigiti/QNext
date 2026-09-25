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
            rtrim(getenv('QNEXT_MARKET_CORE_URL') ?: 'http://127.0.0.1:8080', '/'),
            self::required('QNEXT_OPS_ADMIN_TOKEN'),
            getenv('QNEXT_OPS_HELPER') ?: '/usr/local/bin/qnext-ops-web',
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

    public function releasesRoot(): string
    {
        return $this->privateRoot . '/releases';
    }

    public function currentLink(): string
    {
        return $this->privateRoot . '/current';
    }

    private static function required(string $key): string
    {
        $value = trim((string) getenv($key));
        if ($value === '') {
            throw new RuntimeException($key . ' is required');
        }
        return rtrim($value, '/');
    }
}
