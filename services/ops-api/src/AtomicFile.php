<?php

declare(strict_types=1);

namespace QNext\Ops;

use RuntimeException;

final class AtomicFile
{
    public static function writeJson(string $path, array $payload, int $mode = 0640): void
    {
        $json = json_encode($payload, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
        self::write($path, $json . PHP_EOL, $mode);
    }

    public static function writeSecrets(string $path, array $secrets): array
    {
        $allowed = [
            'UPSTOX_ACCESS_TOKEN',
            'DHAN_CLIENT_ID',
            'DHAN_ACCESS_TOKEN',
        ];

        $stored = [];
        $existing = self::readEnv($path);
        foreach ($secrets as $key => $value) {
            if (!in_array($key, $allowed, true)) {
                throw new RuntimeException('unsupported secret key: ' . $key);
            }
            if (!is_string($value) || trim($value) === '') {
                continue;
            }
            if (str_contains($value, "\n") || str_contains($value, "\r") || str_contains($value, "\0")) {
                throw new RuntimeException('secret values cannot contain control characters');
            }
            $existing[$key] = $value;
            $stored[] = $key;
        }

        if ($stored === []) {
            throw new RuntimeException('no secret values supplied');
        }

        ksort($existing);
        $lines = [];
        foreach ($existing as $key => $value) {
            $lines[] = $key . '=' . self::quoteEnv($value);
        }
        self::write($path, implode(PHP_EOL, $lines) . PHP_EOL, 0600);
        sort($stored);

        return $stored;
    }

    private static function write(string $path, string $contents, int $mode): void
    {
        $directory = dirname($path);
        if (!is_dir($directory) && !mkdir($directory, 0750, true) && !is_dir($directory)) {
            throw new RuntimeException('cannot create directory: ' . $directory);
        }

        $temporary = tempnam($directory, '.qnext-');
        if ($temporary === false) {
            throw new RuntimeException('cannot allocate temporary file');
        }

        try {
            if (file_put_contents($temporary, $contents, LOCK_EX) === false) {
                throw new RuntimeException('cannot write temporary file');
            }
            if (!chmod($temporary, $mode)) {
                throw new RuntimeException('cannot set file permissions');
            }
            if (!rename($temporary, $path)) {
                throw new RuntimeException('cannot atomically replace file');
            }
        } finally {
            if (is_file($temporary)) {
                @unlink($temporary);
            }
        }
    }

    private static function readEnv(string $path): array
    {
        if (!is_file($path)) {
            return [];
        }

        $result = [];
        $lines = file($path, FILE_IGNORE_NEW_LINES | FILE_SKIP_EMPTY_LINES);
        if ($lines === false) {
            throw new RuntimeException('cannot read existing secret file');
        }

        foreach ($lines as $line) {
            if (str_starts_with(trim($line), '#') || !str_contains($line, '=')) {
                continue;
            }
            [$key, $value] = explode('=', $line, 2);
            $key = trim($key);
            $value = trim($value);
            if ($key === '') {
                continue;
            }
            if (strlen($value) >= 2 && $value[0] === "'" && $value[strlen($value) - 1] === "'") {
                $value = substr($value, 1, -1);
                $value = str_replace("'\\''", "'", $value);
            }
            $result[$key] = $value;
        }

        return $result;
    }

    private static function quoteEnv(string $value): string
    {
        return "'" . str_replace("'", "'\\''", $value) . "'";
    }
}
