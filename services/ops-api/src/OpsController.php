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
        );
        $this->service = new ServiceControl($config->helperPath);
    }

    public function status(): array
    {
        try {
            $service = $this->service->run('status');
            $state = $service['ok'] ? ($service['output'] ?: 'active') : 'unavailable';
        } catch (RuntimeException $error) {
            $service = ['ok' => false, 'output' => $error->getMessage()];
            $state = 'unavailable';
        }

        return [
            'release' => $this->releases->snapshot(),
            'service' => [
                'ok' => (bool) ($service['ok'] ?? false),
                'state' => $state,
                'output' => $service['output'] ?? '',
            ],
            'marketCore' => [
                'health' => $this->probe('/health'),
                'ready' => $this->probe('/ready'),
                'version' => $this->probe('/version'),
            ],
            'storageRoot' => $this->config->privateRoot . '/storage',
            'configPath' => $this->config->configPath(),
        ];
    }

    public function getConfig(): array
    {
        $path = $this->config->configPath();
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

    private function probe(string $path): array
    {
        $context = stream_context_create([
            'http' => [
                'method' => 'GET',
                'timeout' => 2.0,
                'ignore_errors' => true,
                'header' => "Accept: application/json\r\n",
            ],
        ]);
        $body = @file_get_contents($this->config->marketCoreUrl . $path, false, $context);
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
