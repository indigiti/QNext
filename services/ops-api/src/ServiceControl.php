<?php

declare(strict_types=1);

namespace QNext\Ops;

use RuntimeException;

final class ServiceControl
{
    private const ACTIONS = ['status', 'start', 'stop', 'restart', 'smoke', 'rollback', 'activate'];

    public function __construct(
        private readonly string $helperPath,
        private readonly array $environment = [],
        private readonly string $controlRequestPath = '',
        private readonly string $desiredStatePath = '',
        private readonly string $cronHeartbeatPath = '',
    ) {
    }

    public function helperAvailable(): bool
    {
        return is_file($this->helperPath) && is_executable($this->helperPath);
    }

    public function processControlAvailable(): bool
    {
        return function_exists('proc_open')
            && function_exists('proc_close')
            && $this->helperAvailable();
    }

    public function cronControlAvailable(): bool
    {
        if (!$this->helperAvailable() || $this->cronHeartbeatPath === '' || !is_file($this->cronHeartbeatPath)) {
            return false;
        }

        $modified = filemtime($this->cronHeartbeatPath);
        return is_int($modified) && (time() - $modified) <= 150;
    }

    public function controlMode(): string
    {
        if ($this->processControlAvailable()) {
            return 'direct';
        }
        if ($this->cronControlAvailable()) {
            return 'cron';
        }
        return 'setup';
    }

    public function cronCommand(): string
    {
        if (!$this->helperAvailable()) {
            return '';
        }

        return '* * * * * ' . $this->helperPath . ' reconcile >/dev/null 2>&1';
    }

    public function desiredState(): string
    {
        if ($this->desiredStatePath === '' || !is_file($this->desiredStatePath)) {
            return 'stopped';
        }

        $value = trim((string) file_get_contents($this->desiredStatePath));
        return $value === 'running' ? 'running' : 'stopped';
    }

    public function run(string $action, ?string $argument = null): array
    {
        $this->validateAction($action, $argument);

        if ($this->processControlAvailable()) {
            return $this->runDirect($action, $argument);
        }

        if (in_array($action, ['start', 'stop', 'restart'], true)) {
            if (!$this->cronControlAvailable()) {
                throw new RuntimeException(
                    'Cloudways cron supervisor is not active; add the cron entry shown in QNext Operations'
                );
            }

            $desired = $action === 'stop' ? 'stopped' : 'running';
            $this->atomicWrite($this->desiredStatePath, $desired . PHP_EOL, 0640);
            $this->atomicWrite($this->controlRequestPath, $action . PHP_EOL, 0640);

            return [
                'ok' => true,
                'action' => $action,
                'exitCode' => 0,
                'output' => 'queued for Cloudways cron supervisor',
                'error' => '',
            ];
        }

        if ($action === 'status') {
            return [
                'ok' => true,
                'action' => 'status',
                'exitCode' => 0,
                'output' => $this->desiredState(),
                'error' => '',
            ];
        }

        throw new RuntimeException('QNext direct process control is unavailable for this action');
    }

    private function validateAction(string $action, ?string $argument): void
    {
        if (!in_array($action, self::ACTIONS, true)) {
            throw new RuntimeException('unsupported service action');
        }
        if ($action === 'activate') {
            if ($argument === null || !preg_match('/^[A-Za-z0-9._-]{1,80}$/', $argument)) {
                throw new RuntimeException('invalid release version');
            }
        } elseif ($argument !== null) {
            throw new RuntimeException('unexpected service action argument');
        }
    }

    private function runDirect(string $action, ?string $argument): array
    {
        $command = [$this->helperPath, $action];
        if ($argument !== null) {
            $command[] = $argument;
        }

        $pipes = [];
        $environment = null;
        if ($this->environment !== []) {
            $environment = array_merge([
                'PATH' => getenv('PATH') ?: '/usr/local/bin:/usr/bin:/bin',
                'HOME' => getenv('HOME') ?: '/',
            ], $this->environment);
        }

        $process = proc_open(
            $command,
            [
                0 => ['file', '/dev/null', 'r'],
                1 => ['pipe', 'w'],
                2 => ['pipe', 'w'],
            ],
            $pipes,
            null,
            $environment,
            ['bypass_shell' => true],
        );

        if (!is_resource($process)) {
            throw new RuntimeException('cannot start QNext service helper');
        }

        $stdout = stream_get_contents($pipes[1]) ?: '';
        $stderr = stream_get_contents($pipes[2]) ?: '';
        fclose($pipes[1]);
        fclose($pipes[2]);
        $exitCode = proc_close($process);

        return [
            'ok' => $exitCode === 0,
            'action' => $action,
            'exitCode' => $exitCode,
            'output' => trim($stdout),
            'error' => trim($stderr),
        ];
    }

    private function atomicWrite(string $path, string $contents, int $mode): void
    {
        if ($path === '') {
            throw new RuntimeException('QNext control state path is not configured');
        }

        $directory = dirname($path);
        if (!is_dir($directory) && !mkdir($directory, 0750, true) && !is_dir($directory)) {
            throw new RuntimeException('cannot create QNext runtime state directory');
        }

        $temporary = tempnam($directory, '.qnext-control-');
        if ($temporary === false) {
            throw new RuntimeException('cannot allocate QNext control state');
        }

        try {
            if (file_put_contents($temporary, $contents, LOCK_EX) === false) {
                throw new RuntimeException('cannot write QNext control state');
            }
            @chmod($temporary, $mode);
            if (!rename($temporary, $path)) {
                throw new RuntimeException('cannot publish QNext control state');
            }
        } finally {
            if (is_file($temporary)) {
                @unlink($temporary);
            }
        }
    }
}
