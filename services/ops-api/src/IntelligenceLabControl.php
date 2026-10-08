<?php

declare(strict_types=1);

namespace QNext\Ops;

use RuntimeException;

final class IntelligenceLabControl
{
    public function __construct(private readonly OpsConfig $config)
    {
    }

    public function status(): array
    {
        $experiments = [];
        $root = $this->storageRoot() . '/experiments';
        if (is_dir($root)) {
            $dirs = glob($root . '/*', GLOB_ONLYDIR) ?: [];
            foreach ($dirs as $dir) {
                $manifest = $this->readJson($dir . '/manifest.json');
                if (!is_array($manifest)) {
                    continue;
                }
                $manifest['backtest'] = $this->readJson($dir . '/results/backtest.json');
                $manifest['shadow'] = $this->readJson($dir . '/results/shadow.json');
                $manifest['candidates'] = [];
                $models = glob($dir . '/models/*.json') ?: [];
                foreach ($models as $model) {
                    $candidate = $this->readJson($model);
                    if (is_array($candidate)) {
                        $manifest['candidates'][] = $candidate;
                    }
                }
                $experiments[] = $manifest;
            }
        }

        usort($experiments, static function (array $left, array $right): int {
            return ((int) ($right['created_at_ms'] ?? 0)) <=> ((int) ($left['created_at_ms'] ?? 0));
        });

        return [
            'storage_root' => $this->storageRoot(),
            'production_storage_root' => $this->productionStorageRoot(),
            'experiments' => $experiments,
            'operation' => $this->operationStatus(),
        ];
    }

    public function importSnapshot(array $payload): array
    {
        $snapshot = $payload['snapshot'] ?? null;
        if (!is_array($snapshot)) {
            throw new RuntimeException('snapshot is required');
        }
        if (($snapshot['schema'] ?? null) !== 'QNEXT.INTELLIGENCE.LAB.CHART_SNAPSHOT/1') {
            throw new RuntimeException('unsupported chart snapshot schema');
        }
        $indicators = $snapshot['indicators'] ?? null;
        $rows = $snapshot['feature_rows'] ?? null;
        if (!is_array($indicators) || count($indicators) < 1 || count($indicators) > 100) {
            throw new RuntimeException('snapshot indicator count is invalid');
        }
        if (!is_array($rows) || count($rows) < 1 || count($rows) > 10000) {
            throw new RuntimeException('snapshot feature row count is invalid');
        }
        $encoded = json_encode($snapshot, JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
        if (strlen($encoded) > 8 * 1024 * 1024) {
            throw new RuntimeException('snapshot exceeds the 8 MiB Lab import limit');
        }

        $name = $payload['name'] ?? '';
        if (!is_string($name) || strlen(trim($name)) > 160) {
            throw new RuntimeException('name is invalid');
        }

        return $this->queue('import', [
            'name' => trim($name),
            'snapshot' => $snapshot,
        ]);
    }

    public function backtest(array $payload): array
    {
        return $this->queue('backtest', [
            'experimentId' => $this->requiredToken($payload, 'experimentId', 96),
            'horizonBars' => $this->intValue($payload['horizonBars'] ?? 3, 'horizonBars', 1, 100),
            'minSamples' => $this->intValue($payload['minSamples'] ?? 60, 'minSamples', 30, 100000),
            'minTestSamples' => $this->intValue($payload['minTestSamples'] ?? 12, 'minTestSamples', 6, 50000),
            'minAverageReturnImprovement' => $this->floatValue($payload['minAverageReturnImprovement'] ?? 0.0, 'minAverageReturnImprovement', -1.0, 1.0),
            'maxAccuracyRegression' => $this->floatValue($payload['maxAccuracyRegression'] ?? 0.02, 'maxAccuracyRegression', 0.0, 1.0),
            'maxDrawdownSlack' => $this->floatValue($payload['maxDrawdownSlack'] ?? 0.01, 'maxDrawdownSlack', 0.0, 10.0),
        ]);
    }

    private function queue(string $action, array $payload): array
    {
        $requestPath = $this->requestPath();
        if (is_file($requestPath) && filesize($requestPath) > 0) {
            throw new RuntimeException('an Intelligence Lab action is already queued');
        }
        $request = [
            'schema' => 'QNEXT.INTELLIGENCE.LAB.REQUEST/1',
            'request_id' => bin2hex(random_bytes(12)),
            'action' => $action,
            'requested_at_ms' => (int) floor(microtime(true) * 1000),
            'payload' => $payload,
        ];
        AtomicFile::writeJson($requestPath, $request);
        return [
            'queued' => true,
            'requestId' => $request['request_id'],
            'action' => $action,
        ];
    }

    private function operationStatus(): array
    {
        $queued = $this->readJson($this->requestPath());
        if (is_array($queued)) {
            return [
                'state' => 'QUEUED',
                'request_id' => $queued['request_id'] ?? null,
                'action' => $queued['action'] ?? null,
                'requested_at_ms' => $queued['requested_at_ms'] ?? null,
            ];
        }
        $result = $this->readJson($this->resultPath());
        return is_array($result) ? $result : ['state' => 'IDLE'];
    }

    private function storageRoot(): string
    {
        return $this->config->privateRoot . '/storage/intelligence-lab';
    }

    private function productionStorageRoot(): string
    {
        return $this->config->privateRoot . '/storage/intelligence';
    }

    private function requestPath(): string
    {
        return $this->config->privateRoot . '/run/intelligence-lab-request.json';
    }

    private function resultPath(): string
    {
        return $this->config->privateRoot . '/run/intelligence-lab-result.json';
    }

    private function readJson(string $path): ?array
    {
        if (!is_file($path)) {
            return null;
        }
        $raw = file_get_contents($path);
        if ($raw === false || trim($raw) === '') {
            return null;
        }
        $decoded = json_decode($raw, true);
        return is_array($decoded) ? $decoded : null;
    }

    private function requiredToken(array $payload, string $key, int $maxLength): string
    {
        $value = $payload[$key] ?? null;
        if (!is_string($value) || !preg_match('/^[A-Za-z0-9._:-]{1,' . $maxLength . '}$/', $value)) {
            throw new RuntimeException($key . ' is invalid');
        }
        return $value;
    }

    private function intValue(mixed $value, string $key, int $min, int $max): int
    {
        if (!is_int($value) || $value < $min || $value > $max) {
            throw new RuntimeException($key . ' is out of range');
        }
        return $value;
    }

    private function floatValue(mixed $value, string $key, float $min, float $max): float
    {
        if (!is_int($value) && !is_float($value)) {
            throw new RuntimeException($key . ' must be numeric');
        }
        $number = (float) $value;
        if (!is_finite($number) || $number < $min || $number > $max) {
            throw new RuntimeException($key . ' is out of range');
        }
        return $number;
    }
}
