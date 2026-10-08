<?php

declare(strict_types=1);

header('Content-Type: application/json; charset=utf-8');
header('Cache-Control: no-store');
header('X-Content-Type-Options: nosniff');

function advisory_value(string $key): string
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

function advisory_private_root(): string
{
    $configured = rtrim(advisory_value('QNEXT_PRIVATE_ROOT'), '/');
    if ($configured !== '') {
        return $configured;
    }

    $publicQnextRoot = dirname(__DIR__, 3);
    $publicHtmlRoot = dirname($publicQnextRoot);
    $accountRoot = dirname($publicHtmlRoot);
    $inferred = $accountRoot . '/private_html/qnext';
    return is_dir($inferred) ? $inferred : '';
}

function advisory_read_json(string $path): ?array
{
    if (!is_file($path) || !is_readable($path)) {
        return null;
    }
    $raw = file_get_contents($path);
    if (!is_string($raw) || trim($raw) === '') {
        return null;
    }
    $decoded = json_decode($raw, true);
    return is_array($decoded) ? $decoded : null;
}

function advisory_string(array $value, string $key, int $max = 160): string
{
    $raw = $value[$key] ?? '';
    if (!is_string($raw)) {
        return '';
    }
    return substr(trim($raw), 0, $max);
}

function advisory_int(array $value, string $key): ?int
{
    $raw = $value[$key] ?? null;
    if (!is_int($raw) && !is_float($raw)) {
        return null;
    }
    $number = (int) $raw;
    return $number > 0 ? $number : null;
}

function advisory_float(array $value, string $key): ?float
{
    $raw = $value[$key] ?? null;
    if (!is_int($raw) && !is_float($raw)) {
        return null;
    }
    $number = (float) $raw;
    return is_finite($number) ? $number : null;
}

$privateRoot = advisory_private_root();
if ($privateRoot === '') {
    echo json_encode([
        'available' => false,
        'advisories' => [],
    ], JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
    exit;
}

$root = $privateRoot . '/storage/intelligence-lab/experiments';
if (!is_dir($root)) {
    echo json_encode([
        'available' => true,
        'advisories' => [],
    ], JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
    exit;
}

$advisories = [];

foreach (glob($root . '/*', GLOB_ONLYDIR) ?: [] as $dir) {
    $manifest = advisory_read_json($dir . '/manifest.json');
    if (!is_array($manifest) || ($manifest['lifecycle_state'] ?? '') !== 'CERTIFIED') {
        continue;
    }

    $latest = advisory_read_json($dir . '/advisory/latest.json');
    if (!is_array($latest) || ($latest['schema'] ?? '') !== 'QNEXT.INTELLIGENCE.CERTIFIED_ADVISORY/1') {
        continue;
    }

    $decision = strtoupper(advisory_string($latest, 'decision', 16));
    if (!in_array($decision, ['BUY', 'SELL', 'NO_TRADE'], true)) {
        continue;
    }

    $probabilitiesRaw = is_array($latest['probabilities'] ?? null)
        ? $latest['probabilities']
        : [];
    $buy = advisory_float($probabilitiesRaw, 'BUY');
    $sell = advisory_float($probabilitiesRaw, 'SELL');
    $noTrade = advisory_float($probabilitiesRaw, 'NO_TRADE');
    if (
        $buy === null
        || $sell === null
        || $noTrade === null
        || $buy < 0.0
        || $buy > 1.0
        || $sell < 0.0
        || $sell > 1.0
        || $noTrade < 0.0
        || $noTrade > 1.0
        || abs(($buy + $sell + $noTrade) - 1.0) > 0.01
    ) {
        continue;
    }

    $instrumentId = advisory_string($latest, 'instrument_id', 128);
    $timeframe = advisory_string($latest, 'timeframe', 32);
    $configurationHash = advisory_string($latest, 'indicator_configuration_hash', 64);
    $featureSchemaVersion = advisory_string($latest, 'feature_schema_version', 128);
    $modelHash = advisory_string($latest, 'model_hash', 64);
    $advisoryId = advisory_string($latest, 'advisory_id', 96);
    $experimentId = advisory_string($latest, 'experiment_id', 96);

    if (
        $instrumentId === ''
        || $timeframe === ''
        || preg_match('/^[a-f0-9]{64}$/', $configurationHash) !== 1
        || $featureSchemaVersion === ''
        || preg_match('/^[a-f0-9]{64}$/', $modelHash) !== 1
        || $advisoryId === ''
        || $experimentId === ''
    ) {
        continue;
    }

    $advisories[] = [
        'advisoryId' => $advisoryId,
        'experimentId' => $experimentId,
        'instrumentId' => $instrumentId,
        'timeframe' => $timeframe,
        'indicatorConfigurationHash' => $configurationHash,
        'featureSchemaVersion' => $featureSchemaVersion,
        'certifiedAtMs' => advisory_int($latest, 'certified_at_ms'),
        'barTimeMs' => advisory_int($latest, 'bar_time_ms'),
        'asOfTimeMs' => advisory_int($latest, 'as_of_time_ms'),
        'createdAtMs' => advisory_int($latest, 'created_at_ms'),
        'modelAlgorithm' => advisory_string($latest, 'model_algorithm', 96),
        'modelHash' => $modelHash,
        'decision' => $decision,
        'probabilities' => [
            'BUY' => $buy,
            'SELL' => $sell,
            'NO_TRADE' => $noTrade,
        ],
        'entryPrice' => advisory_float($latest, 'entry_price'),
        'target1Price' => advisory_float($latest, 'target1_price'),
        'target2Price' => advisory_float($latest, 'target2_price'),
        'invalidationPrice' => advisory_float($latest, 'invalidation_price'),
        'expectedReturnPct' => advisory_float($latest, 'expected_return_pct'),
        'expectedHorizonBars' => advisory_float($latest, 'expected_horizon_bars'),
        'recommendationPolicyId' => advisory_string($latest, 'recommendation_policy_id', 96),
    ];
}

usort($advisories, static function (array $left, array $right): int {
    $timeOrder = ((int) ($right['asOfTimeMs'] ?? 0)) <=> ((int) ($left['asOfTimeMs'] ?? 0));
    if ($timeOrder !== 0) {
        return $timeOrder;
    }
    return strcmp((string) ($left['advisoryId'] ?? ''), (string) ($right['advisoryId'] ?? ''));
});

echo json_encode([
    'available' => true,
    'advisories' => array_slice($advisories, 0, 64),
], JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
