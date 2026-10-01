<?php

declare(strict_types=1);

namespace QNext\Ops;

use RuntimeException;

final class IntelligenceControl
{
    public function __construct(private readonly OpsConfig $config)
    {
    }

    public function status(): array
    {
        $root = $this->storageRoot();
        $models = $root . '/models';
        return [
            'production' => $this->readJson($models . '/production.json'),
            'candidates' => $this->readJsonl($models . '/candidates.jsonl'),
            'history' => array_slice($this->readJsonl($models . '/promotion_events.jsonl'), -50),
            'data' => [
                'features' => $this->countJsonl($root . '/features.jsonl'),
                'predictions' => $this->countJsonl($root . '/predictions.jsonl'),
                'outcomes' => $this->countJsonl($root . '/outcomes.jsonl'),
            ],
            'operation' => $this->operationStatus(),
        ];
    }

    public function train(array $payload): array
    {
        return $this->queue('train', [
            'minSamples' => $this->intValue($payload['minSamples'] ?? 60, 'minSamples', 30, 100000),
            'minTestSamples' => $this->intValue($payload['minTestSamples'] ?? 12, 'minTestSamples', 6, 50000),
            'minAverageReturnImprovement' => $this->floatValue($payload['minAverageReturnImprovement'] ?? 0.0, 'minAverageReturnImprovement', -1.0, 1.0),
            'maxAccuracyRegression' => $this->floatValue($payload['maxAccuracyRegression'] ?? 0.02, 'maxAccuracyRegression', 0.0, 1.0),
            'maxDrawdownSlack' => $this->floatValue($payload['maxDrawdownSlack'] ?? 0.01, 'maxDrawdownSlack', 0.0, 10.0),
        ]);
    }

    public function promote(array $payload): array
    {
        return $this->queue('promote', [
            'candidateId' => $this->requiredToken($payload, 'candidateId', 96),
            'expectedModelHash' => $this->requiredHash($payload, 'expectedModelHash'),
            'approvedBy' => $this->requiredText($payload, 'approvedBy', 120),
            'note' => $this->optionalText($payload['note'] ?? '', 500),
        ]);
    }

    public function rollback(array $payload): array
    {
        $candidateId = null;
        if (isset($payload['candidateId']) && $payload['candidateId'] !== null && $payload['candidateId'] !== '') {
            $candidateId = $this->requiredToken($payload, 'candidateId', 96);
        }
        return $this->queue('rollback', [
            'candidateId' => $candidateId,
            'approvedBy' => $this->requiredText($payload, 'approvedBy', 120),
            'note' => $this->optionalText($payload['note'] ?? '', 500),
        ]);
    }

    private function queue(string $action, array $payload): array
    {
        $requestPath = $this->requestPath();
        if (is_file($requestPath) && filesize($requestPath) > 0) {
            throw new RuntimeException('a Quant Intelligence action is already queued');
        }
        $request = [
            'schema' => 'QNEXT.INTELLIGENCE.REQUEST/1',
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
        if (is_array($result)) {
            return $result;
        }
        return ['state' => 'IDLE'];
    }

    private function storageRoot(): string
    {
        return $this->config->privateRoot . '/storage/intelligence';
    }

    private function requestPath(): string
    {
        return $this->config->privateRoot . '/run/intelligence-request.json';
    }

    private function resultPath(): string
    {
        return $this->config->privateRoot . '/run/intelligence-result.json';
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

    private function readJsonl(string $path): array
    {
        if (!is_file($path)) {
            return [];
        }
        $records = [];
        $handle = fopen($path, 'rb');
        if ($handle === false) {
            return [];
        }
        while (($line = fgets($handle)) !== false) {
            $line = trim($line);
            if ($line === '') {
                continue;
            }
            $decoded = json_decode($line, true);
            if (is_array($decoded)) {
                $records[] = $decoded;
            }
        }
        fclose($handle);
        return $records;
    }

    private function countJsonl(string $path): int
    {
        if (!is_file($path)) {
            return 0;
        }
        $count = 0;
        $handle = fopen($path, 'rb');
        if ($handle === false) {
            return 0;
        }
        while (($line = fgets($handle)) !== false) {
            if (trim($line) !== '') {
                $count++;
            }
        }
        fclose($handle);
        return $count;
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

    private function requiredToken(array $payload, string $key, int $maxLength): string
    {
        $value = $payload[$key] ?? null;
        if (!is_string($value) || !preg_match('/^[A-Za-z0-9._:-]{1,' . $maxLength . '}$/', $value)) {
            throw new RuntimeException($key . ' is invalid');
        }
        return $value;
    }

    private function requiredHash(array $payload, string $key): string
    {
        $value = $payload[$key] ?? null;
        if (!is_string($value) || !preg_match('/^[a-f0-9]{64}$/', $value)) {
            throw new RuntimeException($key . ' must be a SHA-256 hash');
        }
        return $value;
    }

    private function requiredText(array $payload, string $key, int $maxLength): string
    {
        $value = $payload[$key] ?? null;
        if (!is_string($value)) {
            throw new RuntimeException($key . ' is required');
        }
        return $this->validateText($value, $key, $maxLength, true);
    }

    private function optionalText(mixed $value, int $maxLength): string
    {
        if (!is_string($value)) {
            throw new RuntimeException('note must be a string');
        }
        return $this->validateText($value, 'note', $maxLength, false);
    }

    private function validateText(string $value, string $key, int $maxLength, bool $required): string
    {
        $value = trim($value);
        if (($required && $value === '') || strlen($value) > $maxLength || preg_match('/[\x00-\x1F\x7F]/', $value)) {
            throw new RuntimeException($key . ' is invalid');
        }
        return $value;
    }
}
