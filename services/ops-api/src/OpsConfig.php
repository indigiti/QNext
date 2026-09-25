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
        public readonly string $bootstrapToken = '',
    ) {
    }

    public static function fromEnvironment(): self
    {
        $privateRoot = self::required('QNEXT_PRIVATE_ROOT');
        $publicRoot = self::required('QNEXT_PUBLIC_ROOT');
        $helper = self::value('QNEXT_OPS_HELPER');

        if ($helper === '') {
            $candidates = [
                $privateRoot . '/deploy/qnext-ops-user',
                $privateRoot . '/private/deploy/qnext-ops-user',
                $privateRoot . '/current/private/deploy/qnext-ops-user',
                '/usr/local/bin/qnext-ops-web',
            ];
            foreach ($candidates as $candidate) {
                if (is_file($candidate)) {
                    $helper = $candidate;
                    break;
                }
            }
            if ($helper === '') {
                $helper = $candidates[0];
            }
        }

        if (is_file($helper) && !is_executable($helper)) {
            @chmod($helper, 0750);
        }

        $bootstrapToken = self::value('QNEXT_OPS_BOOTSTRAP_TOKEN');
        if ($bootstrapToken !== '' && strlen($bootstrapToken) < 16) {
            throw new RuntimeException('QNEXT_OPS_BOOTSTRAP_TOKEN must be at least 16 characters');
        }

        return new self(
            $privateRoot,
            $publicRoot,
            rtrim(self::value('QNEXT_MARKET_CORE_URL', 'http://127.0.0.1:18080'), '/'),
            self::value('QNEXT_OPS_ADMIN_TOKEN'),
            $helper,
            $bootstrapToken,
        );
    }

    public function configPath(): string
    {
        return $this->privateRoot . '/config/q1-market.json';
    }

    public function configCandidates(): array
    {
        return [
            $this->privateRoot . '/config/q1-market.example.json',
            $this->privateRoot . '/current/private/config/q1-market.example.json',
            $this->privateRoot . '/private/config/q1-market.example.json',
        ];
    }

    public function secretsPath(): string
    {
        return $this->privateRoot . '/secrets/qnext.env';
    }

    public function resilienceConfigPath(): string
    {
        return $this->privateRoot . '/config/q3-resilience.json';
    }

    public function indicatorCatalogPath(): string
    {
        return $this->privateRoot . '/config/qnext-indicators.json';
    }

    public function indicatorCatalogCandidates(): array
    {
        return [
            $this->privateRoot . '/config/qnext-indicators.example.json',
            $this->privateRoot . '/current/private/config/qnext-indicators.example.json',
            $this->privateRoot . '/private/config/qnext-indicators.example.json',
        ];
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

    public function publicManifestPath(): string
    {
        return $this->publicRoot . '/qnext-release.json';
    }

    public function controlRequestPath(): string
    {
        return $this->privateRoot . '/run/control-request';
    }

    public function desiredStatePath(): string
    {
        return $this->privateRoot . '/run/desired-state';
    }

    public function cronHeartbeatPath(): string
    {
        return $this->privateRoot . '/run/cron-heartbeat';
    }

    public function marketCorePidPath(): string
    {
        return $this->privateRoot . '/run/market-core.pid';
    }

    public function marketCoreLogPath(): string
    {
        return $this->privateRoot . '/logs/market-core.log';
    }

    public function marketCoreBinaryCandidates(): array
    {
        $candidates = [];

        foreach ([
            $this->privateRoot . '/bin',
            $this->privateRoot . '/private/bin',
            $this->privateRoot . '/current/private/bin',
        ] as $binRoot) {
            $pointer = $binRoot . '/qnext-market-core.current';
            if (is_file($pointer)) {
                $name = trim((string) file_get_contents($pointer));
                if (preg_match('/^qnext-market-core-[A-Za-z0-9._-]+$/', $name)) {
                    $candidates[] = $binRoot . '/' . $name;
                }
            }

            $candidates[] = $binRoot . '/qnext-market-core';
        }

        return array_values(array_unique($candidates));
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
