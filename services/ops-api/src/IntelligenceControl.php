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
        $status = $this->runCli(['status']);
        $status['history'] = $this->readJsonl($this->storageRoot() . '/models/promotion_events.jsonl');
        return $status;
    }

    public function train(array $payload): array
    {
        $arguments = ['train'];
        $this->appendInt($arguments, '--min-samples', $payload['minSamples'] ?? 60, 30, 100000);
        $this->appendInt($arguments, '--min-test-samples', $payload['minTestSamples'] ?? 12, 6, 50000);
        $this->appendFloat($arguments, '--min-average-return-improvement', $payload['minAverageReturnImprovement'] ?? 0.0, -1.0, 1.0);
        $this->appendFloat($arguments, '--max-accuracy-regression', $payload['maxAccuracyRegression'] ?? 0.02, 0.0, 1.0);
        $this->appendFloat($arguments, '--max-drawdown-slack', $payload['maxDrawdownSlack'] ?? 0.01, 0.0, 10.0);
        return $this->runCli($arguments, 180.0);
    }

    public function promote(array $payload): array
    {
        return $this->runCli([
            'promote',
            '--candidate-id', $this->requiredToken($payload, 'candidateId', 96),
            '--expected-model-hash', $this->requiredHash($payload, 'expectedModelHash'),
            '--approved-by', $this->requiredText($payload, 'approvedBy', 120),
            '--note', $this->optionalText($payload['note'] ?? '', 500),
        ]);
    }

    public function rollback(array $payload): array
    {
        $arguments = [
            'rollback',
            '--approved-by', $this->requiredText($payload, 'approvedBy', 120),
            '--note', $this->optionalText($payload['note'] ?? '', 500),
        ];
        if (isset($payload['candidateId']) && $payload['candidateId'] !== null && $payload['candidateId'] !== '') {
            $arguments[] = '--candidate-id';
            $arguments[] = $this->requiredToken($payload, 'candidateId', 96);
        }
        return $this->runCli($arguments);
    }

    private function runCli(array $arguments, float $timeoutSeconds = 30.0): array
    {
        $packageRoot = $this->packageRoot();
        $storageRoot = $this->storageRoot();
        if (!is_dir($packageRoot . '/qnext_intelligence')) {
            throw new RuntimeException('Quant Intelligence runtime is not deployed');
        }
        if (!is_dir($storageRoot) && !@mkdir($storageRoot, 0750, true) && !is_dir($storageRoot)) {
            throw new RuntimeException('cannot create Quant Intelligence storage root');
        }

        $command = ['python3', '-m', 'qnext_intelligence.learning_cli', ...$arguments, '--storage-root', $storageRoot];
        $environment = [];
        foreach (array_merge($_SERVER, $_ENV) as $key => $value) {
            if (is_string($key) && is_string($value)) {
                $environment[$key] = $value;
            }
        }
        $environment['PYTHONPATH'] = $packageRoot;

        $descriptorSpec = [0 => ['pipe', 'r'], 1 => ['pipe', 'w'], 2 => ['pipe', 'w']];
        $process = proc_open($command, $descriptorSpec, $pipes, null, $environment, ['bypass_shell' => true]);
        if (!is_resource($process)) {
            throw new RuntimeException('cannot start Quant Intelligence runtime');
        }
        fclose($pipes[0]);
        stream_set_blocking($pipes[1], false);
        stream_set_blocking($pipes[2], false);
        $stdout = '';
        $stderr = '';
        $deadline = microtime(true) + $timeoutSeconds;
        $exitCode = null;
        while (true) {
            $stdout .= stream_get_contents($pipes[1]) ?: '';
            $stderr .= stream_get_contents($pipes[2]) ?: '';
            $status = proc_get_status($process);
            if (!($status['running'] ?? false)) {
                $exitCode = (int) ($status['exitcode'] ?? 1);
                break;
            }
            if (microtime(true) >= $deadline) {
                proc_terminate($process, 15);
                usleep(200000);
                $status = proc_get_status($process);
                if ($status['running'] ?? false) {
                    proc_terminate($process, 9);
                }
                fclose($pipes[1]);
                fclose($pipes[2]);
                proc_close($process);
                throw new RuntimeException('Quant Intelligence action timed out');
            }
            usleep(20000);
        }
        $stdout .= stream_get_contents($pipes[1]) ?: '';
        $stderr .= stream_get_contents($pipes[2]) ?: '';
        fclose($pipes[1]);
        fclose($pipes[2]);
        proc_close($process);

        if ($exitCode !== 0) {
            $message = trim($stderr) ?: trim($stdout) ?: 'Quant Intelligence action failed';
            throw new RuntimeException(substr($message, 0, 1000));
        }
        $decoded = json_decode(trim($stdout), true);
        if (!is_array($decoded)) {
            throw new RuntimeException('Quant Intelligence returned invalid JSON');
        }
        return $decoded;
    }

    private function packageRoot(): string
    {
        $candidates = [
            $this->config->privateRoot . '/intelligence',
            $this->config->privateRoot . '/private/intelligence',
            $this->config->privateRoot . '/current/private/intelligence',
        ];
        foreach ($candidates as $candidate) {
            if (is_dir($candidate . '/qnext_intelligence')) {
                return $candidate;
            }
        }
        return $candidates[0];
    }

    private function storageRoot(): string
    {
        return $this->config->privateRoot . '/storage/intelligence';
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
        return array_slice($records, -50);
    }

    private function appendInt(array &$arguments, string $flag, mixed $value, int $min, int $max): void
    {
        if (!is_int($value) || $value < $min || $value > $max) {
            throw new RuntimeException($flag . ' is out of range');
        }
        $arguments[] = $flag;
        $arguments[] = (string) $value;
    }

    private function appendFloat(array &$arguments, string $flag, mixed $value, float $min, float $max): void
    {
        if (!is_int($value) && !is_float($value)) {
            throw new RuntimeException($flag . ' must be numeric');
        }
        $number = (float) $value;
        if (!is_finite($number) || $number < $min || $number > $max) {
            throw new RuntimeException($flag . ' is out of range');
        }
        $arguments[] = $flag;
        $arguments[] = (string) $number;
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
