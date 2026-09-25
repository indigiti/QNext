<?php
declare(strict_types=1);
require dirname(__DIR__) . '/_market_core_proxy.php';
qnext_proxy_market_core_get('/api/v1/calendar', ['instrument_id', 'from_ms', 'to_ms', 'session']);
