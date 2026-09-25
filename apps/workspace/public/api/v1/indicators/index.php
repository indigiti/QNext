<?php

declare(strict_types=1);

header('Content-Type: application/json; charset=utf-8');
header('Cache-Control: no-store');
header('X-Content-Type-Options: nosniff');

function indicator_value(string $key): string
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

function indicator_private_root(): string
{
    $configured = rtrim(indicator_value('QNEXT_PRIVATE_ROOT'), '/');
    if ($configured !== '') {
        return $configured;
    }

    $publicQnextRoot = dirname(__DIR__, 3);
    $publicHtmlRoot = dirname($publicQnextRoot);
    $accountRoot = dirname($publicHtmlRoot);
    $inferred = $accountRoot . '/private_html/qnext';
    return is_dir($inferred) ? $inferred : '';
}

function indicator_catalog(string $privateRoot): array
{
    $candidates = [
        $privateRoot . '/config/qnext-indicators.json',
        $privateRoot . '/config/qnext-indicators.example.json',
        $privateRoot . '/current/private/config/qnext-indicators.example.json',
        $privateRoot . '/private/config/qnext-indicators.example.json',
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

    return ['schema' => 'QNEXT.INDICATORS/1', 'revision' => 0, 'indicators' => []];
}

$privateRoot = indicator_private_root();
if ($privateRoot === '') {
    http_response_code(503);
    echo json_encode(['error' => 'QNext private runtime root is not configured'], JSON_UNESCAPED_SLASHES);
    exit;
}

$catalog = indicator_catalog($privateRoot);
$entries = [];

foreach (($catalog['indicators'] ?? []) as $indicator) {
    if (!is_array($indicator) || !($indicator['enabled'] ?? false)) {
        continue;
    }
    if (($indicator['kind'] ?? '') !== 'adaptive-ema-qalg') {
        continue;
    }
    $id = $indicator['id'] ?? null;
    $name = $indicator['name'] ?? null;
    if (!is_string($id) || !preg_match('/^[a-z0-9][a-z0-9-]{0,63}$/', $id)) {
        continue;
    }
    if (!is_string($name) || trim($name) === '') {
        continue;
    }

    $definition = [
        'schema' => 'QNEXT.INDICATOR/1',
        'id' => $id,
        'name' => $name,
        'kind' => 'adaptive-ema-qalg',
        'defaults' => is_array($indicator['defaults'] ?? null) ? $indicator['defaults'] : [],
    ];

    $entries[] = [
        'name' => $name,
        'script' => json_encode($definition, JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR),
        'language' => 'qnext',
        'enabled' => false,
        'category' => 'QNext',
    ];
}

echo json_encode([
    'indicators' => $entries,
    'revision' => (int) ($catalog['revision'] ?? 0),
], JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
