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

$publicQnextRoot = dirname(__DIR__, 2);
$privateRoot = rtrim((string) getenv('QNEXT_PRIVATE_ROOT'), '/');

if ($privateRoot === '') {
    $publicHtmlRoot = dirname($publicQnextRoot);
    $accountRoot = dirname($publicHtmlRoot);
    $inferred = $accountRoot . '/private_html/qnext';
    if (is_dir($inferred)) {
        $privateRoot = $inferred;
        putenv('QNEXT_PRIVATE_ROOT=' . $privateRoot);
    }
}

if (getenv('QNEXT_PUBLIC_ROOT') === false || trim((string) getenv('QNEXT_PUBLIC_ROOT')) === '') {
    putenv('QNEXT_PUBLIC_ROOT=' . $publicQnextRoot);
}

if ($privateRoot === '') {
    bridge_fail(503, 'QNext private runtime root is not configured');
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
