<?php

declare(strict_types=1);

use QNext\Ops\AtomicFile;
use QNext\Ops\Auth;
use QNext\Ops\OpsConfig;
use QNext\Ops\OpsController;
use QNext\Ops\ReleaseCatalog;
use QNext\Ops\ServiceControl;

require_once dirname(__DIR__) . '/bootstrap.php';

function expect(bool $condition, string $message): void
{
    if (!$condition) {
        fwrite(STDERR, $message . PHP_EOL);
        exit(1);
    }
}

$root = sys_get_temp_dir() . '/qnext-ops-' . bin2hex(random_bytes(4));
mkdir($root . '/releases/r1', 0750, true);
mkdir($root . '/releases/r2', 0750, true);
symlink($root . '/releases/r2', $root . '/current');

$config = new OpsConfig(
    $root,
    $root . '/public',
    'http://127.0.0.1:65535',
    'token',
    $root . '/missing-helper',
);

$authPath = $root . '/secrets/ops-auth.json';
$bootstrapToken = 'bootstrap-0123456789abcdef';
$auth = new Auth($authPath, '', $bootstrapToken);
expect(!$auth->initialized(), 'auth should start uninitialized');
expect($auth->bootstrapConfigured(), 'first-time auth should require a configured bootstrap token');
expect($auth->bootstrapAuthorized($bootstrapToken), 'matching bootstrap token should authorize first-time setup');
expect(!$auth->bootstrapAuthorized('wrong-bootstrap'), 'wrong bootstrap token must be rejected');
$auth->initialize('0123456789abcdef');
expect($auth->initialized(), 'auth should initialize once');
expect(!$auth->bootstrapAuthorized($bootstrapToken), 'bootstrap authorization must disable after initialization');
expect($auth->authorized('0123456789abcdef'), 'auth should accept matching initialized token');
expect(!$auth->authorized('wrong'), 'auth should reject wrong token');
expect(is_file($authPath), 'auth hash should be persisted');
$authPayload = json_decode(file_get_contents($authPath) ?: '{}', true);
expect(
    is_array($authPayload)
    && isset($authPayload['token_hash'])
    && !str_contains((string) $authPayload['token_hash'], '0123456789abcdef'),
    'plaintext admin token must not be persisted'
);

AtomicFile::writeJson($config->configPath(), ['timeframes' => ['1m'], 'nifty' => [], 'synthetic' => []]);
expect(is_file($config->configPath()), 'config should be atomically written');

$stored = AtomicFile::writeSecrets($config->secretsPath(), [
    'UPSTOX_ACCESS_TOKEN' => 'abc123',
]);
expect($stored === ['UPSTOX_ACCESS_TOKEN'], 'secret write should report key only');
$secretFile = file_get_contents($config->secretsPath()) ?: '';
expect(str_contains($secretFile, 'abc123'), 'secret should be persisted for systemd EnvironmentFile use');

$release = (new ReleaseCatalog($config->releasesRoot(), $config->currentLink()))->snapshot();
expect($release['current'] === 'r2', 'current release should resolve from symlink');
expect($release['available'] === ['r2', 'r1'], 'release list should be sorted newest-first');
expect($release['mode'] === 'staged', 'symlink release should report staged mode');

$directRoot = $root . '/direct';
mkdir($directRoot . '/public', 0750, true);
AtomicFile::writeJson($directRoot . '/public/qnext-release.json', [
    'version' => 'abc123def456',
]);
$direct = (new ReleaseCatalog(
    $directRoot . '/releases',
    $directRoot . '/current',
    $directRoot . '/public/qnext-release.json',
))->snapshot();
expect($direct['current'] === 'abc123def456', 'direct deployment should read public manifest version');
expect($direct['available'] === [], 'direct deployment should not invent staged releases');
expect($direct['mode'] === 'direct', 'direct deployment should report direct mode');

$control = new ServiceControl($config->helperPath);
try {
    $control->run('shell');
    expect(false, 'arbitrary helper actions must be rejected');
} catch (RuntimeException) {
}

$flatRoot = $root . '/flat';
mkdir($flatRoot . '/deploy', 0750, true);
mkdir($flatRoot . '/config', 0750, true);
file_put_contents($flatRoot . '/deploy/qnext-ops-user', "#!/bin/bash\n");
chmod($flatRoot . '/deploy/qnext-ops-user', 0640);
AtomicFile::writeJson($flatRoot . '/config/q1-market.example.json', [
    'timeframes' => ['1m'],
    'nifty' => ['instrument_id' => 'NSE:NIFTY50', 'provider_key' => 'NSE_INDEX|Nifty 50'],
    'synthetic' => [
        'instrument_id' => 'QNEXT:NIFTY-SYN',
        'version' => 'nifty-syn-v2',
        'minimum_valid_candidates' => 3,
        'max_leg_age_ms' => 2000,
        'max_leg_time_skew_ms' => 1000,
        'auto' => [
            'strike_interval' => 50,
            'active_strikes' => 5,
            'warm_strikes' => 7,
            'atm_hysteresis_points' => 5,
            'atm_confirmation_ms' => 750,
        ],
    ],
]);
AtomicFile::writeJson($flatRoot . '/config/q1-market.json', []);

$_SERVER['QNEXT_PRIVATE_ROOT'] = $flatRoot;
$_SERVER['QNEXT_PUBLIC_ROOT'] = $flatRoot . '/public';
$flatConfig = OpsConfig::fromEnvironment();
expect(
    $flatConfig->helperPath === $flatRoot . '/deploy/qnext-ops-user',
    'flattened private layout helper should be detected'
);
$flatControl = new ServiceControl(
    $flatConfig->helperPath,
    [],
    $flatRoot . '/run/control-request',
    $flatRoot . '/run/desired-state',
    $flatRoot . '/run/cron-heartbeat',
);
expect(
    $flatControl->helperAvailable(),
    'readable helper should be available even without execute bit'
);
mkdir($flatRoot . '/run', 0750, true);
file_put_contents($flatRoot . '/run/cron-heartbeat', (string) time());
expect(
    $flatControl->controlMode() === 'cron',
    'active cron heartbeat should take precedence over direct process control'
);
$flatController = new OpsController($flatConfig);
$seeded = $flatController->getConfig();
expect(
    isset($seeded['timeframes']) && $seeded['timeframes'] === ['1m'],
    'empty market config should be replaced with packaged default'
);

$candleState = $flatController->candleTimeframes();
expect(
    ($candleState['enabled'] ?? []) === ['1m'],
    'candle formation should reflect configured timeframes'
);
$chartState = $flatController->chartTimeframes();
expect(
    ($chartState['enabled'] ?? []) === ['1m']
    && ($chartState['candleEnabled'] ?? []) === ['1m'],
    'chart display should default to the candle-enabled subset'
);
try {
    $flatController->saveChartTimeframes(['enabled' => ['15s']]);
    expect(false, 'chart display must reject a timeframe whose candle formation is disabled');
} catch (RuntimeException) {
}
$savedCandles = $flatController->saveCandleTimeframes(['enabled' => ['15s', '1m']]);
expect(
    ($savedCandles['enabled'] ?? []) === ['15s', '1m'],
    'candle formation selection should save independently'
);
$savedChart = $flatController->saveChartTimeframes(['enabled' => ['15s']]);
expect(
    ($savedChart['enabled'] ?? []) === ['15s'],
    'chart display selection should save independently'
);
$timeframeConfig = json_decode(file_get_contents($flatConfig->configPath()) ?: '{}', true);
expect(
    ($timeframeConfig['timeframes'] ?? []) === ['15s', '1m']
    && ($timeframeConfig['chart_timeframes'] ?? []) === ['15s'],
    'formation and chart display selections should persist separately'
);

$activation = $flatController->marketActivation();
expect(
    ($activation['active'] ?? []) === ['NIFTY', 'BANKNIFTY', 'MIDCPNIFTY', 'FINNIFTY', 'SENSEX', 'BANKEX'],
    'all markets should default active when active_markets is absent'
);
$savedActivation = $flatController->saveActiveMarkets(['active' => ['NIFTY', 'SENSEX']]);
expect(
    ($savedActivation['active'] ?? []) === ['NIFTY', 'SENSEX'],
    'active market selection should be saved'
);
$activeConfig = json_decode(file_get_contents($flatConfig->configPath()) ?: '{}', true);
expect(
    ($activeConfig['active_markets'] ?? []) === ['NIFTY', 'SENSEX'],
    'active market selection should persist in q1-market.json'
);


AtomicFile::writeJson($flatRoot . '/config/qnext-indicators.example.json', [
    'schema' => 'QNEXT.INDICATORS/1',
    'revision' => 1,
    'indicators' => [[
        'id' => 'adaptive-ema-qalg',
        'name' => 'Adaptive EMA [QALG]',
        'category' => 'QNext',
        'kind' => 'adaptive-ema-qalg',
        'enabled' => true,
        'defaults' => [
            'priceSource' => 'close',
            'emaLength' => 20,
            'lookbackPeriod' => 30,
            'stddevMultiplier' => 2.0,
            'atrLength' => 14,
            'atrMultiplier' => 1.5,
            'upColor' => '#00ffaa',
            'downColor' => '#ff0000',
            'colorBars' => true,
        ],
    ]],
]);

$indicatorCatalog = $flatController->customIndicators();
expect(
    count($indicatorCatalog['indicators'] ?? []) === 1
    && ($indicatorCatalog['indicators'][0]['name'] ?? '') === 'Adaptive EMA [QALG]',
    'custom indicator catalog should seed from packaged defaults'
);
$createdIndicator = $flatController->createCustomIndicator([
    'id' => 'adaptive-ema-qalg-2',
    'name' => 'Adaptive EMA Test',
    'kind' => 'adaptive-ema-qalg',
    'enabled' => true,
    'defaults' => [
        'priceSource' => 'close',
        'emaLength' => 10,
        'lookbackPeriod' => 20,
        'stddevMultiplier' => 2.0,
        'atrLength' => 14,
        'atrMultiplier' => 1.5,
        'upColor' => '#00ffaa',
        'downColor' => '#ff0000',
        'colorBars' => true,
    ],
]);
expect(
    ($createdIndicator['saved'] ?? false) === true,
    'custom indicator create should persist'
);
$updatedIndicator = $flatController->updateCustomIndicator('adaptive-ema-qalg-2', ['enabled' => false]);
expect(
    ($updatedIndicator['indicator']['enabled'] ?? true) === false,
    'custom indicator update should persist'
);
$deletedIndicator = $flatController->deleteCustomIndicator('adaptive-ema-qalg-2');
expect(
    ($deletedIndicator['deleted'] ?? false) === true,
    'custom indicator delete should persist'
);

$pineIndicator = $flatController->createCustomIndicator([
    'id' => 'pine-ema-cross',
    'name' => 'Pine EMA Cross',
    'kind' => 'pine-v6',
    'enabled' => true,
    'script' => "//@version=6\nindicator(\"Pine EMA Cross\", overlay=true)\nplot(ta.ema(close, 9))\n",
    'description' => 'Pine v6 smoke test',
]);
expect(
    ($pineIndicator['indicator']['language'] ?? '') === 'pine'
    && str_contains((string) ($pineIndicator['indicator']['script'] ?? ''), '//@version=6'),
    'Pine v6 custom indicator source should persist'
);
try {
    $flatController->createCustomIndicator([
        'id' => 'pine-v5-rejected',
        'name' => 'Old Pine',
        'kind' => 'pine-v6',
        'enabled' => true,
        'script' => "//@version=5\nindicator(\"Old Pine\")\n",
    ]);
    expect(false, 'Pine v5 source must be rejected by the Pine v6 catalog');
} catch (RuntimeException) {
}

mkdir($flatRoot . '/logs', 0750, true);
mkdir($flatRoot . '/bin', 0750, true);
file_put_contents($flatRoot . '/bin/qnext-market-core', 'legacy-binary');
file_put_contents($flatRoot . '/bin/qnext-market-core-testv2', 'versioned-binary');
file_put_contents($flatRoot . '/bin/qnext-market-core.current', "qnext-market-core-testv2\n");
expect(
    $flatConfig->marketCoreBinaryCandidates()[0] === $flatRoot . '/bin/qnext-market-core-testv2',
    'diagnostics should prefer the versioned binary selected by qnext-market-core.current'
);
file_put_contents(
    $flatRoot . '/logs/market-core.log',
    "market-core starting\nAuthorization: Bearer secret-token\naccess_token=secret-value\nmarket-core stopped\n"
);
$secretResult = $flatController->saveSecrets([
    'DHAN_CLIENT_ID' => 'test-client',
    'DHAN_ACCESS_TOKEN' => 'test-token',
]);
expect(
    ($secretResult['resilienceConfigured'] ?? false) === true,
    'saving Dhan secrets should create auto-leg-compatible resilience config'
);
$resilienceConfig = json_decode(
    file_get_contents($flatConfig->resilienceConfigPath()) ?: '{}',
    true,
);
expect(
    is_array($resilienceConfig)
    && count($resilienceConfig['instruments'] ?? []) === 1
    && ($resilienceConfig['instruments'][0]['instrument_id'] ?? '') === 'NSE:NIFTY50'
    && ($resilienceConfig['instruments'][0]['dhan_provider_key'] ?? '') === 'IDX_I|13|INDEX',
    'web-managed Dhan resilience config should contain only the canonical NIFTY mapping in auto-leg mode'
);

$diagnostics = $flatController->diagnostics();
expect($diagnostics['binaryFound'] === true, 'diagnostics should find flattened Market Core binary');
expect(count($diagnostics['logLines']) === 4, 'diagnostics should return bounded log lines');
$diagnosticText = implode("\n", $diagnostics['logLines']);
expect(!str_contains($diagnosticText, 'secret-token'), 'diagnostics should redact bearer tokens');
expect(!str_contains($diagnosticText, 'secret-value'), 'diagnostics should redact access tokens');

echo "QNext Ops API smoke: PASS\n";
