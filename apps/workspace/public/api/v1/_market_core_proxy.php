<?php

declare(strict_types=1);

header('Content-Type: application/json; charset=utf-8');
header('Cache-Control: no-store');
header('X-Content-Type-Options: nosniff');
header('Referrer-Policy: no-referrer');
header("Content-Security-Policy: default-src 'none'; frame-ancestors 'none'");

function qnext_proxy_fail(int $status, string $message): never
{
    http_response_code($status);
    echo json_encode(['error' => $message], JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
    exit;
}

function qnext_proxy_value(string $key, string $default = ''): string
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

function qnext_proxy_market_core_get(string $path, array $allowedQueryKeys = []): never
{
    if (strtoupper($_SERVER['REQUEST_METHOD'] ?? 'GET') !== 'GET') {
        header('Allow: GET');
        qnext_proxy_fail(405, 'method not allowed');
    }

    $query = [];
    foreach ($allowedQueryKeys as $key) {
        if (isset($_GET[$key]) && is_scalar($_GET[$key])) {
            $query[$key] = (string) $_GET[$key];
        }
    }

    $base = rtrim(qnext_proxy_value('QNEXT_MARKET_CORE_URL', 'http://127.0.0.1:18080'), '/');
    $url = $base . $path;
    if ($query !== []) {
        $url .= '?' . http_build_query($query, '', '&', PHP_QUERY_RFC3986);
    }

    if (function_exists('curl_init')) {
        $handle = curl_init($url);
        if ($handle !== false) {
            curl_setopt_array($handle, [
                CURLOPT_RETURNTRANSFER => true,
                CURLOPT_CONNECTTIMEOUT_MS => 750,
                CURLOPT_TIMEOUT_MS => 3000,
                CURLOPT_HTTPHEADER => ['Accept: application/json'],
            ]);
            $body = curl_exec($handle);
            $status = (int) curl_getinfo($handle, CURLINFO_RESPONSE_CODE);
            curl_close($handle);

            if (is_string($body) && $body !== '') {
                http_response_code($status > 0 ? $status : 200);
                echo $body;
                exit;
            }
        }
    }

    $context = stream_context_create([
        'http' => [
            'method' => 'GET',
            'timeout' => 3.0,
            'ignore_errors' => true,
            'header' => "Accept: application/json\r\n",
        ],
    ]);
    $body = @file_get_contents($url, false, $context);
    $status = 0;
    foreach ($http_response_header ?? [] as $responseHeader) {
        if (preg_match('/^HTTP\/\S+\s+(\d{3})/', $responseHeader, $matches)) {
            $status = (int) $matches[1];
            break;
        }
    }

    if ($body === false) {
        qnext_proxy_fail(502, 'market data unavailable');
    }

    http_response_code($status > 0 ? $status : 200);
    echo $body;
    exit;
}
