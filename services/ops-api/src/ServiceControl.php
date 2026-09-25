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
    ) {
    }

    public function available(): bool
    {
        return function_exists('proc_open')
            && function_exists('proc_close')
            && is_file($this->helperPath)
            && is_executable($this->helperPath);
    }

    public function run(string $action, ?string $argument = null): array
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

        if (!function_exists('proc_open') || !function_exists('proc_close')) {
            throw new RuntimeException(
                'Cloudways PHP process control is disabled; enable proc_open and proc_close for this application'
            );
        }
        if (!is_file($this->helperPath) || !is_executable($this->helperPath)) {
            throw new RuntimeException('QNext service helper is unavailable: ' . $this->helperPath);
        }

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
}
