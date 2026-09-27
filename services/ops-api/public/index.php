<?php

declare(strict_types=1);

use QNext\Ops\AccessGovernance;
use QNext\Ops\Auth;
use QNext\Ops\FeatureEntitlements;
use QNext\Ops\InviteApprovalStore;
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

function admin_actor(): string
{
    $actor = trim((string) ($_SERVER['HTTP_X_QNEXT_ACTOR_ID'] ?? ''));
    if ($actor === '' || strlen($actor) > 128 || preg_match('/^[A-Za-z0-9._:@+-]+$/', $actor) !== 1) {
        throw new RuntimeException('X-QNEXT-ACTOR-ID is required for access-governance changes');
    }
    return $actor;
}

try {
    $config = OpsConfig::fromEnvironment();
    $auth = new Auth($config->authPath(), $config->adminToken);
    $accessStore = new InviteApprovalStore($config->accessStorePath());
    $access = new AccessGovernance($accessStore);
    $entitlements = new FeatureEntitlements($accessStore);
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

    // Invite-token endpoints intentionally sit outside ops-admin auth. They disclose only
    // token-scoped registration state and never expose the stored token hash.
    if ($method === 'POST' && $path === '/registration/invite-status') {
        $body = request_body();
        respond(200, $access->inspectInvite((string) ($body['invite_token'] ?? '')));
    }
    if ($method === 'POST' && $path === '/registration/redeem') {
        respond(201, $access->redeemInvite(request_body()));
    }

    $provided = $_SERVER['HTTP_X_QNEXT_OPS_TOKEN'] ?? null;
    if (!$auth->authorized(is_string($provided) ? $provided : null)) {
        respond(403, ['error' => 'forbidden']);
    }

    if ($method === 'GET' && $path === '/access') {
        respond(200, $access->adminSnapshot());
    }
    if ($method === 'GET' && $path === '/access/policy') {
        respond(200, $access->policy());
    }
    if ($method === 'PUT' && $path === '/access/policy') {
        admin_actor();
        respond(200, $access->updatePolicy(request_body()));
    }
    if ($method === 'POST' && $path === '/access/invites') {
        respond(201, $access->issueInvite(request_body(), admin_actor()));
    }
    if ($method === 'PUT' && preg_match('#^/access/approvers/([A-Za-z0-9._:@+-]{1,128})$#', $path, $matches)) {
        $body = request_body();
        if (!array_key_exists('eligible', $body) || !is_bool($body['eligible'])) {
            throw new RuntimeException('eligible boolean is required');
        }
        respond(200, $access->setApprover(
            $matches[1],
            $body['eligible'],
            (string) ($body['role'] ?? 'USER'),
            admin_actor(),
        ));
    }
    if ($method === 'GET' && preg_match('#^/access/users/([A-Za-z0-9._:@+-]{1,128})/entitlements$#', $path, $matches)) {
        respond(200, [
            'configured' => $entitlements->forUser($matches[1]),
            'effective' => $entitlements->effectiveForUser($matches[1]),
        ]);
    }
    if ($method === 'PUT' && preg_match('#^/access/users/([A-Za-z0-9._:@+-]{1,128})/entitlements$#', $path, $matches)) {
        $configured = $entitlements->setForUser($matches[1], request_body(), admin_actor());
        respond(200, [
            'configured' => $configured,
            'effective' => $entitlements->effectiveForUser($matches[1]),
        ]);
    }
    if ($method === 'GET' && preg_match('#^/access/registrations/([A-Za-z0-9._-]{1,128})$#', $path, $matches)) {
        respond(200, $access->registrationSnapshot($matches[1]));
    }
    if ($method === 'POST' && preg_match('#^/access/registrations/([A-Za-z0-9._-]{1,128})/admin-decision$#', $path, $matches)) {
        $body = request_body();
        respond(200, $access->recordDecision(
            $matches[1],
            admin_actor(),
            'ADMIN',
            (string) ($body['decision'] ?? ''),
            (string) ($body['comment'] ?? ''),
        ));
    }
    if ($method === 'PUT' && preg_match('#^/access/registrations/([A-Za-z0-9._-]{1,128})/account-gate$#', $path, $matches)) {
        $body = request_body();
        if (!array_key_exists('active', $body) || !is_bool($body['active'])) {
            throw new RuntimeException('active boolean is required');
        }
        respond(200, $access->setAccountGate($matches[1], $body['active'], admin_actor()));
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
