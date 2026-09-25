<?php

declare(strict_types=1);

header('Content-Type: application/json; charset=utf-8');
header('Cache-Control: no-store');
header('X-Content-Type-Options: nosniff');

function bridge_fail(int $status, string $message): never
{
    http_response_code($status);
    echo json_encode(['error' => $message], JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
    exit;
}

function bridge_value(string $key): string
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

$publicQnextRoot = dirname(__DIR__, 2);
$privateRoot = rtrim(bridge_value('QNEXT_PRIVATE_ROOT'), '/');

if ($privateRoot === '') {
    $publicHtmlRoot = dirname($publicQnextRoot);
    $accountRoot = dirname($publicHtmlRoot);
    $inferred = $accountRoot . '/private_html/qnext';
    if (is_dir($inferred)) {
        $privateRoot = $inferred;
    }
}

if ($privateRoot === '') {
    bridge_fail(503, 'QNext private runtime root is not configured');
}

/*
 * Cloudways may disable putenv(). Pass inferred deployment values to the
 * included private API through the request server array instead.
 */
$_SERVER['QNEXT_PRIVATE_ROOT'] = $privateRoot;
if (bridge_value('QNEXT_PUBLIC_ROOT') === '') {
    $_SERVER['QNEXT_PUBLIC_ROOT'] = $publicQnextRoot;
}

$candidates = [
    $privateRoot . '/current/private/ops-api/public/index.php',
    $privateRoot . '/ops-api/public/index.php',
    $privateRoot . '/private/ops-api/public/index.php',
];

foreach ($candidates as $candidate) {
    if (is_file($candidate)) {
        require $candidate;
        exit;
    }
}

bridge_fail(503, 'QNext private Ops API payload is not installed');
