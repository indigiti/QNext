<?php

declare(strict_types=1);

header('Content-Type: application/json; charset=utf-8');
header('Cache-Control: no-store');
header('X-Content-Type-Options: nosniff');

function paper_marker_value(string $key, string $default = ''): string
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
    return $default;
}

function paper_marker_status(string $baseUrl): array
{
    $context = stream_context_create([
        'http' => [
            'method' => 'GET',
            'timeout' => 0.75,
            'ignore_errors' => true,
            'header' => "Accept: application/json\r\n",
        ],
    ]);

    $body = @file_get_contents(rtrim($baseUrl, '/') . '/status', false, $context);
    if (!is_string($body) || trim($body) === '') {
        return [];
    }
    $decoded = json_decode($body, true);
    return is_array($decoded) ? $decoded : [];
}

function paper_marker_string(array $value, string $key): string
{
    $raw = $value[$key] ?? '';
    return is_string($raw) ? trim($raw) : '';
}

$runtimeUrl = paper_marker_value('QNEXT_SYN_PLUS_PAPER_URL', 'http://127.0.0.1:18082');
$status = paper_marker_status($runtimeUrl);
if ($status === []) {
    echo json_encode([
        'available' => false,
        'enabled' => false,
        'instrumentId' => 'QNEXT:NIFTY-SYN+',
        'timeframe' => '1m',
        'sessionId' => '',
        'markers' => [],
    ], JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
    exit;
}

$instrumentId = paper_marker_string($status, 'instrument_id');
$timeframe = paper_marker_string($status, 'timeframe');
$sessionId = paper_marker_string($status, 'paper_session_id');
$rawMarkers = is_array($status['markers'] ?? null) ? $status['markers'] : [];
$markers = [];

foreach ($rawMarkers as $raw) {
    if (!is_array($raw)) {
        continue;
    }

    $id = paper_marker_string($raw, 'marker_id');
    $kind = strtoupper(paper_marker_string($raw, 'kind'));
    $side = strtoupper(paper_marker_string($raw, 'side'));
    $label = paper_marker_string($raw, 'label');
    $eventTimeMs = filter_var($raw['time_ms'] ?? null, FILTER_VALIDATE_INT);
    $anchorTimeMs = filter_var($raw['anchor_time_ms'] ?? null, FILTER_VALIDATE_INT);
    $price = filter_var($raw['price'] ?? null, FILTER_VALIDATE_FLOAT);

    if (
        $id === ''
        || !in_array($kind, ['SIGNAL', 'FILL'], true)
        || !in_array($side, ['BUY', 'SELL'], true)
        || $eventTimeMs === false
        || $eventTimeMs <= 0
        || $anchorTimeMs === false
        || $anchorTimeMs <= 0
        || $price === false
        || !is_finite((float) $price)
    ) {
        continue;
    }

    $markers[] = [
        'id' => $id,
        'eventTimeMs' => (int) $eventTimeMs,
        'anchorTimeMs' => (int) $anchorTimeMs,
        'kind' => $kind,
        'side' => $side,
        'price' => (float) $price,
        'label' => mb_substr($label !== '' ? $label : ($side . ' ' . strtolower($kind)), 0, 120),
    ];
}

usort($markers, static function (array $a, array $b): int {
    $timeOrder = $a['anchorTimeMs'] <=> $b['anchorTimeMs'];
    if ($timeOrder !== 0) {
        return $timeOrder;
    }
    return strcmp($a['id'], $b['id']);
});

echo json_encode([
    'available' => true,
    'enabled' => (bool) ($status['enabled'] ?? false),
    'instrumentId' => $instrumentId,
    'timeframe' => $timeframe,
    'sessionId' => $sessionId,
    'markers' => array_slice($markers, -100),
], JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
