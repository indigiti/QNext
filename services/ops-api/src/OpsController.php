<?php

declare(strict_types=1);

namespace QNext\Ops;

use JsonException;
use RuntimeException;

final class OpsController
{
    private ReleaseCatalog $releases;
    private ServiceControl $service;

    public function __construct(private readonly OpsConfig $config)
    {
        $this->releases = new ReleaseCatalog(
            $config->releasesRoot(),
            $config->currentLink(),
            $config->publicManifestPath(),
        );
        $this->service = new ServiceControl(
            $config->helperPath,
            [
                'QNEXT_PRIVATE_ROOT' => $config->privateRoot,
                'QNEXT_PUBLIC_ROOT' => $config->publicRoot,
                'QNEXT_MARKET_CORE_URL' => $config->marketCoreUrl,
            ],
            $config->controlRequestPath(),
            $config->desiredStatePath(),
            $config->cronHeartbeatPath(),
        );
    }

    public function status(): array
    {
        $health = $this->probe('/health');
        $ready = $this->probe('/ready');
        $version = $this->probe('/version');
        $controlMode = $this->service->controlMode();

        $service = ['ok' => false, 'output' => ''];
        $state = 'unavailable';

        if ($health['ok']) {
            $service = ['ok' => true, 'output' => $controlMode];
            $state = 'active';
        } elseif ($controlMode === 'direct') {
            try {
                $service = $this->service->run('status');
                $state = $service['ok'] ? ($service['output'] ?: 'active') : 'unavailable';
            } catch (RuntimeException $error) {
                $service = ['ok' => false, 'output' => $error->getMessage()];
            }
        } elseif ($controlMode === 'cron') {
            $desired = $this->service->desiredState();
            $service = ['ok' => true, 'output' => 'Cloudways cron supervisor'];
            $state = $desired === 'running' ? 'starting' : 'stopped';
        } else {
            $service = [
                'ok' => false,
                'output' => 'Cloudways cron supervisor setup is required',
            ];
            $state = 'setup required';
        }

        return [
            'release' => $this->releases->snapshot(),
            'service' => [
                'ok' => (bool) ($service['ok'] ?? false),
                'state' => $state,
                'output' => $service['output'] ?? '',
            ],
            'marketCore' => [
                'health' => $health,
                'ready' => $ready,
                'version' => $version,
            ],
            'storageRoot' => $this->config->privateRoot . '/storage',
            'configPath' => $this->config->configPath(),
            'host' => [
                'processControl' => $this->service->processControlAvailable(),
                'cronControl' => $this->service->cronControlAvailable(),
                'controlMode' => $controlMode,
                'helperAvailable' => $this->service->helperAvailable(),
                'helperPath' => $this->config->helperPath,
                'cronCommand' => $this->service->cronCommand(),
            ],
        ];
    }

    public function diagnostics(): array
    {
        $pid = null;
        $pidPath = $this->config->marketCorePidPath();
        if (is_file($pidPath)) {
            $rawPid = trim((string) file_get_contents($pidPath));
            if (preg_match('/^[1-9][0-9]*$/', $rawPid)) {
                $pid = (int) $rawPid;
            }
        }

        $heartbeatAt = null;
        $heartbeatAgeSeconds = null;
        $heartbeatPath = $this->config->cronHeartbeatPath();
        if (is_file($heartbeatPath)) {
            $modified = filemtime($heartbeatPath);
            if (is_int($modified)) {
                $heartbeatAt = gmdate(DATE_ATOM, $modified);
                $heartbeatAgeSeconds = max(0, time() - $modified);
            }
        }

        $binary = null;
        foreach ($this->config->marketCoreBinaryCandidates() as $candidate) {
            if (is_file($candidate)) {
                $binary = $candidate;
                break;
            }
        }

        return [
            'desiredState' => $this->service->desiredState(),
            'pid' => $pid,
            'pidAlive' => $pid !== null && is_dir('/proc/' . $pid),
            'pidPath' => $pidPath,
            'binaryPath' => $binary,
            'binaryFound' => $binary !== null,
            'logPath' => $this->config->marketCoreLogPath(),
            'logLines' => $this->tailLog($this->config->marketCoreLogPath(), 80, 65536),
            'cronHeartbeatAt' => $heartbeatAt,
            'cronHeartbeatAgeSeconds' => $heartbeatAgeSeconds,
            'controlMode' => $this->service->controlMode(),
            'helperPath' => $this->config->helperPath,
        ];
    }

    public function getConfig(): array
    {
        $path = $this->config->configPath();
        if (!is_file($path)) {
            $this->seedDefaultConfig($path);
        }
        if (!is_file($path)) {
            return [];
        }

        $contents = file_get_contents($path);
        if ($contents === false) {
            throw new RuntimeException('cannot read market configuration');
        }

        try {
            $decoded = json_decode($contents, true, 512, JSON_THROW_ON_ERROR);
        } catch (JsonException $error) {
            throw new RuntimeException('stored market configuration is invalid JSON', 0, $error);
        }

        if (!is_array($decoded)) {
            throw new RuntimeException('stored market configuration must be an object');
        }

        if ($decoded === []) {
            $this->seedDefaultConfig($path);
            $contents = file_get_contents($path);
            if ($contents === false) {
                throw new RuntimeException('cannot read seeded market configuration');
            }
            try {
                $decoded = json_decode($contents, true, 512, JSON_THROW_ON_ERROR);
            } catch (JsonException $error) {
                throw new RuntimeException('seeded market configuration is invalid JSON', 0, $error);
            }
            if (!is_array($decoded)) {
                throw new RuntimeException('seeded market configuration must be an object');
            }
        }

        return $decoded;
    }

    public function saveConfig(array $payload): array
    {
        foreach (['timeframes', 'nifty', 'synthetic'] as $key) {
            if (!array_key_exists($key, $payload)) {
                throw new RuntimeException('market configuration is missing ' . $key);
            }
        }
        if (!is_array($payload['timeframes']) || $payload['timeframes'] === []) {
            throw new RuntimeException('market configuration requires timeframes');
        }
        if (!is_array($payload['nifty']) || !is_array($payload['synthetic'])) {
            throw new RuntimeException('nifty and synthetic configuration must be objects');
        }

        AtomicFile::writeJson($this->config->configPath(), $payload);

        return ['saved' => true];
    }

    public function saveSecrets(array $payload): array
    {
        return [
            'stored' => AtomicFile::writeSecrets($this->config->secretsPath(), $payload),
        ];
    }

    public function serviceAction(string $action): array
    {
        if (!in_array($action, ['start', 'stop', 'restart'], true)) {
            throw new RuntimeException('unsupported service action');
        }
        $result = $this->service->run($action);
        if (!$result['ok']) {
            throw new RuntimeException($result['error'] ?: 'service action failed');
        }

        return $result;
    }

    public function smoke(): array
    {
        $probes = [
            'health' => $this->probe('/health'),
            'ready' => $this->probe('/ready'),
            'version' => $this->probe('/version'),
        ];

        $ok = true;
        foreach ($probes as $probe) {
            $ok = $ok && $probe['ok'];
        }

        return ['ok' => $ok, 'probes' => $probes];
    }

    public function activate(string $version): array
    {
        $result = $this->service->run('activate', $version);
        if (!$result['ok']) {
            throw new RuntimeException($result['error'] ?: 'release activation failed');
        }

        return [
            'ok' => true,
            'version' => $version,
            'output' => $result['output'],
        ];
    }

    public function rollback(): array
    {
        $result = $this->service->run('rollback');
        if (!$result['ok']) {
            throw new RuntimeException($result['error'] ?: 'rollback failed');
        }

        return ['ok' => true, 'output' => $result['output']];
    }

    private function seedDefaultConfig(string $path): void
    {
        foreach ($this->config->configCandidates() as $candidate) {
            if (!is_file($candidate)) {
                continue;
            }

            $contents = file_get_contents($candidate);
            if ($contents === false || trim($contents) === '') {
                continue;
            }

            try {
                $decoded = json_decode($contents, true, 512, JSON_THROW_ON_ERROR);
            } catch (JsonException) {
                continue;
            }

            if (is_array($decoded)) {
                AtomicFile::writeJson($path, $decoded);
                return;
            }
        }
    }

    private function tailLog(string $path, int $maxLines, int $maxBytes): array
    {
        if (!is_file($path) || !is_readable($path)) {
            return [];
        }

        $handle = fopen($path, 'rb');
        if ($handle === false) {
            return [];
        }

        try {
            $size = filesize($path);
            if (is_int($size) && $size > $maxBytes) {
                fseek($handle, -$maxBytes, SEEK_END);
                fgets($handle);
            }

            $contents = stream_get_contents($handle);
            if (!is_string($contents) || $contents === '') {
                return [];
            }

            $lines = preg_split('/\R/', trim($contents)) ?: [];
            $lines = array_slice($lines, -$maxLines);

            return array_values(array_map(
                static function (string $line): string {
                    $line = preg_replace(
                        '/((?:access[_ -]?token|authorization|bearer|api[_ -]?key|client[_ -]?secret)\s*[:=]\s*)\S+/i',
                        '$1[REDACTED]',
                        $line,
                    ) ?? $line;
                    return substr($line, 0, 2000);
                },
                $lines,
            ));
        } finally {
            fclose($handle);
        }
    }

    private function probe(string $path): array
    {
        $url = $this->config->marketCoreUrl . $path;

        if (function_exists('curl_init')) {
            $handle = curl_init($url);
            if ($handle !== false) {
                curl_setopt_array($handle, [
                    CURLOPT_RETURNTRANSFER => true,
                    CURLOPT_CONNECTTIMEOUT_MS => 750,
                    CURLOPT_TIMEOUT_MS => 2000,
                    CURLOPT_HTTPHEADER => ['Accept: application/json'],
                ]);
                $body = curl_exec($handle);
                $status = (int) curl_getinfo($handle, CURLINFO_RESPONSE_CODE);
                $error = curl_error($handle);
                curl_close($handle);

                if (is_string($body)) {
                    $decoded = json_decode($body, true);
                    return [
                        'ok' => $status >= 200 && $status < 300,
                        'status' => $status,
                        'body' => is_array($decoded) ? $decoded : $body,
                    ];
                }

                if ($error !== '') {
                    return [
                        'ok' => false,
                        'status' => $status ?: null,
                        'error' => 'connection failed: ' . $error,
                    ];
                }
            }
        }

        $context = stream_context_create([
            'http' => [
                'method' => 'GET',
                'timeout' => 2.0,
                'ignore_errors' => true,
                'header' => "Accept: application/json\r\n",
            ],
        ]);
        $body = @file_get_contents($url, false, $context);
        $status = $this->responseStatus($http_response_header ?? []);
        if ($body === false) {
            return ['ok' => false, 'status' => $status ?: null, 'error' => 'connection failed'];
        }

        $decoded = json_decode($body, true);
        return [
            'ok' => $status >= 200 && $status < 300,
            'status' => $status,
            'body' => is_array($decoded) ? $decoded : $body,
        ];
    }

    private function responseStatus(array $headers): int
    {
        foreach ($headers as $header) {
            if (preg_match('/^HTTP\/\S+\s+(\d{3})/', $header, $matches)) {
                return (int) $matches[1];
            }
        }

        return 0;
    }
}
