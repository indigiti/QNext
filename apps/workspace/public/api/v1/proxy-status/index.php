<?php

declare(strict_types=1);

require dirname(__DIR__) . '/_market_core_proxy.php';

if (strtoupper($_SERVER['REQUEST_METHOD'] ?? 'GET') !== 'GET') {
    header('Allow: GET');
    qnext_proxy_fail(405, 'method not allowed');
}

echo json_encode(qnext_proxy_status_snapshot(), JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
