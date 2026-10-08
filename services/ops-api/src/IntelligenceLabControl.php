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
                $manifest['selection'] = $this->readJson($dir . '/results/selection.json');
                $manifest['recommendation'] = $this->readJson($dir . '/results/recommendation.json');
                $manifest['shadow_config'] = $this->readJson($dir . '/shadow/config.json');
                $manifest['shadow_summary'] = $this->readJson($dir . '/shadow/summary.json');
                $manifest['shadow_latest'] = $this->readJson($dir . '/shadow/latest-observation.json');
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
            'ml_runtime' => $this->readJson($this->mlRuntimeStatusPath()) ?? [
                'schema' => 'QNEXT.INTELLIGENCE.LAB.PYTHON/1',
                'state' => 'UNKNOWN',
                'message' => 'isolated Lab ML runtime has not been checked yet',
            ],
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
        if (!is_array($rows) || count($rows) > 10000) {
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

    public function startShadow(array $payload): array
    {
        return $this->queue('start-shadow', [
            'experimentId' => $this->requiredToken($payload, 'experimentId', 96),
            'horizonBars' => $this->intValue($payload['horizonBars'] ?? 3, 'horizonBars', 1, 100),
            'minSamples' => $this->intValue($payload['minSamples'] ?? 30, 'minSamples', 12, 100000),
            'maxAccuracyRegression' => $this->floatValue($payload['maxAccuracyRegression'] ?? 0.05, 'maxAccuracyRegression', 0.0, 1.0),
            'maxAverageReturnRegression' => $this->floatValue($payload['maxAverageReturnRegression'] ?? 0.002, 'maxAverageReturnRegression', 0.0, 1.0),
            'maxDrawdownSlack' => $this->floatValue($payload['maxDrawdownSlack'] ?? 0.02, 'maxDrawdownSlack', 0.0, 10.0),
            'maxBrier' => $this->floatValue($payload['maxBrier'] ?? 0.35, 'maxBrier', 0.0, 1.0),
            'minCoverage' => $this->floatValue($payload['minCoverage'] ?? 0.10, 'minCoverage', 0.0, 1.0),
            'minTarget1BeforeInvalidation' => $this->floatValue($payload['minTarget1BeforeInvalidation'] ?? 0.30, 'minTarget1BeforeInvalidation', 0.0, 1.0),
        ]);
    }

    public function certifyShadow(array $payload): array
    {
        return $this->queue('certify-shadow', [
            'experimentId' => $this->requiredToken($payload, 'experimentId', 96),
        ]);
    }

    public function shadowTargets(): array
    {
        $targets = [];
        $root = $this->storageRoot() . '/experiments';
        if (is_dir($root)) {
            foreach (glob($root . '/*', GLOB_ONLYDIR) ?: [] as $dir) {
                $manifest = $this->readJson($dir . '/manifest.json');
                if (!is_array($manifest) || ($manifest['lifecycle_state'] ?? '') !== 'SHADOW') {
                    continue;
                }
                $config = $this->readJson($dir . '/shadow/config.json');
                if (!is_array($config)) {
                    continue;
                }
                $targets[] = [
                    'experiment_id' => $manifest['experiment_id'] ?? '',
                    'instrument_id' => $manifest['instrument_id'] ?? '',
                    'timeframe' => $manifest['timeframe'] ?? '',
                    'indicator_configuration_hash' => $manifest['indicator_configuration_hash'] ?? '',
                    'feature_schema_version' => $manifest['feature_schema_version'] ?? '',
                    'horizon_bars' => $config['horizon_bars'] ?? 0,
                    'started_at_ms' => $config['started_at_ms'] ?? 0,
                ];
            }
        }
        return ['targets' => $targets];
    }

    public function submitShadowObservation(array $payload): array
    {
        $experimentId = $this->requiredToken($payload, 'experimentId', 96);
        $configurationHash = $payload['indicatorConfigurationHash'] ?? null;
        if (!is_string($configurationHash) || preg_match('/^[a-f0-9]{64}$/', $configurationHash) !== 1) {
            throw new RuntimeException('indicatorConfigurationHash is invalid');
        }
        $featureSchemaVersion = $payload['featureSchemaVersion'] ?? null;
        if (!is_string($featureSchemaVersion) || trim($featureSchemaVersion) === '' || strlen($featureSchemaVersion) > 128) {
            throw new RuntimeException('featureSchemaVersion is invalid');
        }
        $rows = $payload['featureRows'] ?? null;
        if (!is_array($rows) || count($rows) < 1 || count($rows) > 256) {
            throw new RuntimeException('featureRows must contain between 1 and 256 rows');
        }
        $currentFeatures = $payload['currentFeatures'] ?? [];
        if (!is_array($currentFeatures)) {
            throw new RuntimeException('currentFeatures must be an object');
        }
        $barTimeMs = $payload['barTimeMs'] ?? null;
        if (!is_int($barTimeMs) || $barTimeMs <= 0) {
            throw new RuntimeException('barTimeMs is invalid');
        }

        $request = [
            'schema' => 'QNEXT.INTELLIGENCE.LAB.REQUEST/1',
            'request_id' => bin2hex(random_bytes(12)),
            'action' => 'shadow-observation',
            'requested_at_ms' => (int) floor(microtime(true) * 1000),
            'payload' => [
                'experimentId' => $experimentId,
                'indicatorConfigurationHash' => $configurationHash,
                'featureSchemaVersion' => trim($featureSchemaVersion),
                'barTimeMs' => $barTimeMs,
                'createdAtMs' => (int) ($payload['createdAtMs'] ?? floor(microtime(true) * 1000)),
                'featureRows' => $rows,
                'currentFeatures' => $currentFeatures,
            ],
        ];
        $encoded = json_encode($request, JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
        if (strlen($encoded) > 2 * 1024 * 1024) {
            throw new RuntimeException('shadow observation exceeds the 2 MiB limit');
        }

        $dir = $this->shadowInboxPath();
        if (!is_dir($dir) && !mkdir($dir, 0750, true) && !is_dir($dir)) {
            throw new RuntimeException('failed to create Shadow-Live inbox');
        }
        $path = $dir . '/' . $barTimeMs . '-' . $request['request_id'] . '.json';
        AtomicFile::writeJson($path, $request);
        return [
            'queued' => true,
            'requestId' => $request['request_id'],
            'action' => 'shadow-observation',
        ];
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

    private function shadowInboxPath(): string
    {
        return $this->config->privateRoot . '/run/intelligence-lab-shadow-inbox';
    }

    private function mlRuntimeStatusPath(): string
    {
        return $this->config->privateRoot . '/run/intelligence-lab-python.json';
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
