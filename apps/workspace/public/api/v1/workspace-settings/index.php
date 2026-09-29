<?php

declare(strict_types=1);

header('Content-Type: application/json; charset=utf-8');
header('Cache-Control: no-store');
header('X-Content-Type-Options: nosniff');

function workspace_value(string $key): string
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
    return '';
}

function workspace_private_root(): string
{
    $configured = rtrim(workspace_value('QNEXT_PRIVATE_ROOT'), '/');
    if ($configured !== '') {
        return $configured;
    }

    $publicQnextRoot = dirname(__DIR__, 3);
    $publicHtmlRoot = dirname($publicQnextRoot);
    $accountRoot = dirname($publicHtmlRoot);
    $inferred = $accountRoot . '/private_html/qnext';
    return is_dir($inferred) ? $inferred : '';
}

function workspace_market_config(string $privateRoot): array
{
    $candidates = [
        $privateRoot . '/config/q1-market.json',
        $privateRoot . '/config/q1-market.example.json',
        $privateRoot . '/current/private/config/q1-market.example.json',
        $privateRoot . '/private/config/q1-market.example.json',
    ];

    foreach ($candidates as $candidate) {
        if (!is_file($candidate) || !is_readable($candidate)) {
            continue;
        }
        $raw = file_get_contents($candidate);
        if (!is_string($raw) || trim($raw) === '') {
            continue;
        }
        $decoded = json_decode($raw, true);
        if (is_array($decoded)) {
            return $decoded;
        }
    }

    return [];
}

$privateRoot = workspace_private_root();
if ($privateRoot === '') {
    http_response_code(503);
    echo json_encode(['error' => 'QNext private runtime root is not configured'], JSON_UNESCAPED_SLASHES);
    exit;
}

$config = workspace_market_config($privateRoot);
$workspace = is_array($config['workspace'] ?? null) ? $config['workspace'] : [];

$engine = $workspace['chart_engine'] ?? 'vela';
if (!is_string($engine) || !in_array($engine, ['auto', 'vela', 'lightweight'], true)) {
    $engine = 'vela';
}

$layout = $workspace['lightweight_layout'] ?? 1;
if (!is_int($layout) || !in_array($layout, [1, 2, 4], true)) {
    $layout = 1;
}

$sync = $workspace['lightweight_sync'] ?? true;
if (!is_bool($sync)) {
    $sync = true;
}

echo json_encode([
    'chartEngine' => $engine,
    'lightweightLayout' => $layout,
    'lightweightSync' => $sync,
], JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
