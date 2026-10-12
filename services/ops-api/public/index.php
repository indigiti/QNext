<?php

declare(strict_types=1);

use QNext\Ops\Auth;
use QNext\Ops\IntelligenceControl;
use QNext\Ops\IntelligenceLabControl;
use QNext\Ops\OpsConfig;
use QNext\Ops\OpsController;
use QNext\Ops\PaperProxy;

require_once dirname(__DIR__) . '/bootstrap.php';
require_once dirname(__DIR__) . '/src/IntelligenceControl.php';
require_once dirname(__DIR__) . '/src/IntelligenceLabControl.php';

const QNEXT_OPS_SESSION_COOKIE = 'qnext_ops_session';

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

function request_is_https(): bool
{
    $https = strtolower(trim((string) ($_SERVER['HTTPS'] ?? '')));
    if ($https !== '' && $https !== 'off' && $https !== '0') {
        return true;
    }
    return strtolower(trim((string) ($_SERVER['HTTP_X_FORWARDED_PROTO'] ?? ''))) === 'https';
}

function set_admin_session_cookie(string $session, int $ttl): void
{
    setcookie(QNEXT_OPS_SESSION_COOKIE, $session, [
        'expires' => time() + $ttl,
        'path' => '/qnext/admin/',
        'secure' => request_is_https(),
        'httponly' => true,
        'samesite' => 'Strict',
    ]);
}

function clear_admin_session_cookie(): void
{
    setcookie(QNEXT_OPS_SESSION_COOKIE, '', [
        'expires' => 1,
        'path' => '/qnext/admin/',
        'secure' => request_is_https(),
        'httponly' => true,
        'samesite' => 'Strict',
    ]);
}

function provided_admin_token(): ?string
{
    $custom = $_SERVER['HTTP_X_QNEXT_OPS_TOKEN'] ?? null;
    if (is_string($custom) && trim($custom) !== '') {
        return trim($custom);
    }

    $authorization = $_SERVER['HTTP_AUTHORIZATION']
        ?? $_SERVER['REDIRECT_HTTP_AUTHORIZATION']
        ?? null;
    if (is_string($authorization)
        && preg_match('/^Bearer\s+(.+)$/i', trim($authorization), $matches) === 1) {
        return trim($matches[1]);
    }
    return null;
}

try {
    $config = OpsConfig::fromEnvironment();
    $auth = new Auth($config->authPath(), $config->adminToken);
    $method = strtoupper($_SERVER['REQUEST_METHOD'] ?? 'GET');
    $path = request_path();

    if ($method === 'GET' && $path === '/setup-status') {
        respond(200, [
            'initialized' => $auth->initialized(),
            'recovery_configured' => $auth->recoveryConfigured(),
        ]);
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
        set_admin_session_cookie($auth->issueSession(), Auth::sessionTTLSeconds());
        respond(201, ['initialized' => true, 'authenticated' => true]);
    }

    if ($method === 'GET' && $path === '/session') {
        $session = $_COOKIE[QNEXT_OPS_SESSION_COOKIE] ?? null;
        $authenticated = $auth->authorizedSession(is_string($session) ? $session : null);
        respond(200, [
            'authenticated' => $authenticated,
            'expires_in_seconds' => $authenticated ? Auth::sessionTTLSeconds() : 0,
        ]);
    }

    if ($method === 'POST' && $path === '/session') {
        $body = request_body();
        $token = $body['token'] ?? null;
        if (!is_string($token) || !$auth->authorized(trim($token))) {
            clear_admin_session_cookie();
            respond(403, ['error' => 'forbidden']);
        }
        set_admin_session_cookie($auth->issueSession(), Auth::sessionTTLSeconds());
        respond(200, [
            'authenticated' => true,
            'expires_in_seconds' => Auth::sessionTTLSeconds(),
        ]);
    }

    if ($method === 'POST' && $path === '/auth/recover') {
        $body = request_body();
        $recoveryCode = $body['recovery_code'] ?? null;
        $newToken = $body['new_token'] ?? null;
        if (!is_string($recoveryCode) || !is_string($newToken)) {
            respond(400, ['error' => 'recovery_code and new_token are required']);
        }
        if (!$auth->recoverWithCode($recoveryCode, $newToken)) {
            usleep(350000);
            clear_admin_session_cookie();
            respond(403, ['error' => 'forbidden']);
        }
        set_admin_session_cookie($auth->issueSession(), Auth::sessionTTLSeconds());
        respond(200, [
            'authenticated' => true,
            'recovery_consumed' => true,
            'expires_in_seconds' => Auth::sessionTTLSeconds(),
        ]);
    }

    if ($method === 'DELETE' && $path === '/session') {
        clear_admin_session_cookie();
        respond(200, ['authenticated' => false]);
    }

    $provided = provided_admin_token();
    $session = $_COOKIE[QNEXT_OPS_SESSION_COOKIE] ?? null;
    $headerAuthorized = $auth->authorized($provided);
    $sessionAuthorized = $auth->authorizedSession(is_string($session) ? $session : null);
    if (!$headerAuthorized && !$sessionAuthorized) {
        respond(403, ['error' => 'forbidden']);
    }

    if ($method === 'POST' && $path === '/auth/recovery-code') {
        $recovery = $auth->generateRecoveryCode();
        respond(201, [
            'recovery_code' => $recovery['code'],
            'generated_at' => $recovery['generated_at'],
        ]);
    }

    if ($method === 'DELETE' && $path === '/auth/recovery-code') {
        $auth->revokeRecoveryCode();
        respond(200, ['recovery_configured' => false]);
    }

    $controller = new OpsController($config);
    $paper = new PaperProxy($config->paperRuntimeUrl);
    $intelligence = new IntelligenceControl($config);
    $intelligenceLab = new IntelligenceLabControl($config);

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
    if ($method === 'GET' && $path === '/intelligence') {
        respond(200, $intelligence->status());
    }
    if ($method === 'POST' && $path === '/intelligence/train') {
        respond(200, $intelligence->train(request_body()));
    }
    if ($method === 'POST' && $path === '/intelligence/promote') {
        respond(200, $intelligence->promote(request_body()));
    }
    if ($method === 'POST' && $path === '/intelligence/rollback') {
        respond(200, $intelligence->rollback(request_body()));
    }
    if ($method === 'GET' && $path === '/intelligence-lab') {
        respond(200, $intelligenceLab->status());
    }
    if ($method === 'POST' && $path === '/intelligence-lab/import') {
        respond(200, $intelligenceLab->importSnapshot(request_body()));
    }
    if ($method === 'POST' && $path === '/intelligence-lab/backtest') {
        respond(200, $intelligenceLab->backtest(request_body()));
    }
    if ($method === 'POST' && $path === '/intelligence-lab/shadow/start') {
        respond(200, $intelligenceLab->startShadow(request_body()));
    }
    if ($method === 'POST' && $path === '/intelligence-lab/shadow/certify') {
        respond(200, $intelligenceLab->certifyShadow(request_body()));
    }
    if ($method === 'GET' && $path === '/intelligence-lab/shadow-targets') {
        respond(200, $intelligenceLab->shadowTargets());
    }
    if ($method === 'POST' && $path === '/intelligence-lab/shadow-observation') {
        respond(200, $intelligenceLab->submitShadowObservation(request_body()));
    }
    if ($method === 'POST' && $path === '/intelligence-lab/advisory-observation') {
        respond(200, $intelligenceLab->submitAdvisoryObservation(request_body()));
    }
    if ($method === 'GET' && $path === '/syn-plus-paper') {
        respond(200, $paper->status());
    }
    if ($method === 'POST' && $path === '/syn-plus-paper') {
        respond(200, $paper->control(request_body()));
    }
    if ($method === 'GET' && $path === '/active-markets') {
        respond(200, $controller->marketActivation());
    }
    if ($method === 'GET' && $path === '/candle-timeframes') {
        respond(200, $controller->candleTimeframes());
    }
    if ($method === 'GET' && $path === '/chart-timeframes') {
        respond(200, $controller->chartTimeframes());
    }
    if ($method === 'GET' && $path === '/history-repair') {
        respond(200, $controller->historicalRepairStatus());
    }
    if ($method === 'GET' && $path === '/custom-indicators') {
        respond(200, $controller->customIndicators());
    }
    if ($method === 'POST' && $path === '/custom-indicators') {
        respond(201, $controller->createCustomIndicator(request_body()));
    }
    if ($method === 'PUT' && preg_match('#^/custom-indicators/([a-z0-9][a-z0-9-]{0,63})$#', $path, $matches)) {
        respond(200, $controller->updateCustomIndicator($matches[1], request_body()));
    }
    if ($method === 'DELETE' && preg_match('#^/custom-indicators/([a-z0-9][a-z0-9-]{0,63})$#', $path, $matches)) {
        respond(200, $controller->deleteCustomIndicator($matches[1]));
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
    if ($method === 'PUT' && $path === '/candle-timeframes') {
        respond(200, $controller->saveCandleTimeframes(request_body()));
    }
    if ($method === 'PUT' && $path === '/chart-timeframes') {
        respond(200, $controller->saveChartTimeframes(request_body()));
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
