<?php

declare(strict_types=1);

use QNext\Ops\Auth;
use QNext\Ops\OpsConfig;
use QNext\Ops\OpsController;

require_once dirname(__DIR__) . '/bootstrap.php';

header('Content-Type: application/json; charset=utf-8');
header('Cache-Control: no-store');
header('X-Content-Type-Options: nosniff');
header('Referrer-Policy: no-referrer');
header("Content-Security-Policy: default-src 'none'; frame-ancestors 'none'");

function respond(int $status, array $payload): never
{
    http_response_code($status);
    echo json_encode($payload, JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
    exit;
}

function request_body(): array
{
    $raw = file_get_contents('php://input');
    if ($raw === false || trim($raw) === '') {
        return [];
    }
    $decoded = json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
    if (!is_array($decoded)) {
        throw new RuntimeException('request body must be a JSON object');
    }
    return $decoded;
}

function request_path(): string
{
    $route = $_GET['route'] ?? null;
    if (is_string($route) && str_starts_with($route, '/')) {
        return $route;
    }

    $path = parse_url($_SERVER['REQUEST_URI'] ?? '/', PHP_URL_PATH) ?: '/';
    $prefix = '/qnext/admin/api';
    if (str_starts_with($path, $prefix)) {
        $path = substr($path, strlen($prefix)) ?: '/';
    }
    if (str_starts_with($path, '/index.php')) {
        $path = substr($path, strlen('/index.php')) ?: '/';
    }
    return $path;
}

try {
    $config = OpsConfig::fromEnvironment();
    $auth = new Auth($config->authPath(), $config->adminToken);
    $method = strtoupper($_SERVER['REQUEST_METHOD'] ?? 'GET');
    $path = request_path();

    if ($method === 'GET' && $path === '/setup-status') {
        respond(200, ['initialized' => $auth->initialized()]);
    }

    if ($method === 'POST' && $path === '/setup') {
        if ($auth->initialized()) {
            respond(409, ['error' => 'admin token is already initialized']);
        }
        $body = request_body();
        $token = $body['token'] ?? null;
        if (!is_string($token)) {
            respond(400, ['error' => 'token is required']);
        }
        $auth->initialize($token);
        respond(201, ['initialized' => true]);
    }

    $provided = $_SERVER['HTTP_X_QNEXT_OPS_TOKEN'] ?? null;
    if (!$auth->authorized(is_string($provided) ? $provided : null)) {
        respond(403, ['error' => 'forbidden']);
    }

    $controller = new OpsController($config);

    if ($method === 'GET' && $path === '/status') {
        respond(200, $controller->status());
    }
    if ($method === 'GET' && $path === '/config') {
        respond(200, $controller->getConfig());
    }
    if ($method === 'GET' && $path === '/diagnostics') {
        respond(200, $controller->diagnostics());
    }
    if ($method === 'GET' && $path === '/feed-status') {
        respond(200, $controller->feedStatus());
    }
    if ($method === 'GET' && $path === '/active-markets') {
        respond(200, $controller->marketActivation());
    }
    if ($method === 'GET' && $path === '/history-repair') {
        respond(200, $controller->historicalRepairStatus());
    }
    if ($method === 'POST' && $path === '/history-repair') {
        respond(200, $controller->runHistoricalRepair(request_body()));
    }
    if ($method === 'POST' && $path === '/dhan-standby-check') {
        respond(200, $controller->verifyDhanStandby());
    }
    if ($method === 'PUT' && $path === '/config') {
        respond(200, $controller->saveConfig(request_body()));
    }
    if ($method === 'PUT' && $path === '/active-markets') {
        respond(200, $controller->saveActiveMarkets(request_body()));
    }
    if ($method === 'POST' && $path === '/secrets') {
        respond(200, $controller->saveSecrets(request_body()));
    }
    if ($method === 'POST' && preg_match('#^/service/(start|stop|restart)$#', $path, $matches)) {
        respond(200, $controller->serviceAction($matches[1]));
    }
    if ($method === 'POST' && $path === '/smoke') {
        respond(200, $controller->smoke());
    }
    if ($method === 'POST' && preg_match('#^/releases/([A-Za-z0-9._-]{1,80})/activate$#', $path, $matches)) {
        respond(200, $controller->activate($matches[1]));
    }
    if ($method === 'POST' && $path === '/rollback') {
        respond(200, $controller->rollback());
    }

    respond(404, ['error' => 'not found']);
} catch (JsonException $error) {
    respond(400, ['error' => 'invalid JSON']);
} catch (RuntimeException $error) {
    respond(400, ['error' => $error->getMessage()]);
} catch (Throwable $error) {
    error_log('QNext Ops API error: ' . $error->getMessage());
    respond(500, ['error' => 'internal error']);
}
