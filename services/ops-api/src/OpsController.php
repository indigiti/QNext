<?php

declare(strict_types=1);

namespace QNext\Ops;

use JsonException;
use RuntimeException;

final class OpsController
{
    private ReleaseCatalog $releases;
    private ServiceControl $service;

    public function __construct(private readonly OpsConfig $config)
    {
        $this->releases = new ReleaseCatalog(
            $config->releasesRoot(),
            $config->currentLink(),
            $config->publicManifestPath(),
        );
        $this->service = new ServiceControl(
            $config->helperPath,
            [
                'QNEXT_PRIVATE_ROOT' => $config->privateRoot,
                'QNEXT_PUBLIC_ROOT' => $config->publicRoot,
                'QNEXT_MARKET_CORE_URL' => $config->marketCoreUrl,
            ],
            $config->controlRequestPath(),
            $config->desiredStatePath(),
            $config->cronHeartbeatPath(),
        );
    }

    public function status(): array
    {
        $health = $this->probe('/health');
        $ready = $this->probe('/ready');
        $version = $this->probe('/version');
        $controlMode = $this->service->controlMode();

        $service = ['ok' => false, 'output' => ''];
        $state = 'unavailable';

        if ($health['ok']) {
            $service = ['ok' => true, 'output' => $controlMode];
            $state = 'active';
        } elseif ($controlMode === 'direct') {
            try {
                $service = $this->service->run('status');
                $state = $service['ok'] ? ($service['output'] ?: 'active') : 'unavailable';
            } catch (RuntimeException $error) {
                $service = ['ok' => false, 'output' => $error->getMessage()];
            }
        } elseif ($controlMode === 'cron') {
            $desired = $this->service->desiredState();
            $service = ['ok' => true, 'output' => 'Cloudways cron supervisor'];
            $state = $desired === 'running' ? 'starting' : 'stopped';
        } else {
            $service = [
                'ok' => false,
                'output' => 'Cloudways cron supervisor setup is required',
            ];
            $state = 'setup required';
        }

        return [
            'release' => $this->releases->snapshot(),
            'service' => [
                'ok' => (bool) ($service['ok'] ?? false),
                'state' => $state,
                'output' => $service['output'] ?? '',
            ],
            'marketCore' => [
                'health' => $health,
                'ready' => $ready,
                'version' => $version,
            ],
            'storageRoot' => $this->config->privateRoot . '/storage',
            'configPath' => $this->config->configPath(),
            'host' => [
                'processControl' => $this->service->processControlAvailable(),
                'cronControl' => $this->service->cronControlAvailable(),
                'controlMode' => $controlMode,
                'helperAvailable' => $this->service->helperAvailable(),
                'helperPath' => $this->config->helperPath,
                'cronCommand' => $this->service->cronCommand(),
            ],
        ];
    }

    public function feedStatus(): array
    {
        $probe = $this->probe('/api/v1/feed-status');
        if (!($probe['ok'] ?? false)) {
            return [
                'ok' => false,
                'status' => $probe['status'] ?? null,
                'error' => $probe['error'] ?? 'market feed status unavailable',
            ];
        }

        $body = $probe['body'] ?? null;
        return [
            'ok' => true,
            'status' => $probe['status'] ?? 200,
            'body' => is_array($body) ? $body : [],
        ];
    }


    public function historicalRepairStatus(): array
    {
        $probe = $this->probe('/api/v1/history-repair');
        if (!($probe['ok'] ?? false)) {
            return [
                'ok' => false,
                'status' => $probe['status'] ?? null,
                'error' => $probe['error'] ?? 'historical repair status unavailable',
            ];
        }

        return [
            'ok' => true,
            'status' => $probe['status'] ?? 200,
            'body' => is_array($probe['body'] ?? null) ? $probe['body'] : [],
        ];
    }

    public function runHistoricalRepair(array $payload): array
    {
        $days = $payload['days'] ?? null;
        if (!is_int($days) || !in_array($days, [3, 7, 15, 30], true)) {
            throw new RuntimeException('historical repair days must be 3, 7, 15, or 30');
        }

        $markets = $payload['markets'] ?? [];
        if (!is_array($markets)) {
            throw new RuntimeException('historical repair markets must be an array');
        }
        foreach ($markets as $market) {
            if (!is_string($market) || trim($market) === '') {
                throw new RuntimeException('historical repair market names must be strings');
            }
        }

        $probe = $this->requestCore(
            '/api/v1/history-repair',
            'POST',
            [
                'days' => $days,
                'markets' => array_values($markets),
                'reason' => 'admin_manual',
            ],
            120.0,
        );
        if (!($probe['ok'] ?? false)) {
            $body = $probe['body'] ?? null;
            $message = is_array($body)
                ? (string) ($body['message'] ?? $body['error'] ?? 'historical repair failed')
                : (string) ($probe['error'] ?? 'historical repair failed');
            throw new RuntimeException($message);
        }

        return is_array($probe['body'] ?? null) ? $probe['body'] : [];
    }

    public function diagnostics(): array
    {
        $pid = null;
        $pidPath = $this->config->marketCorePidPath();
        if (is_file($pidPath)) {
            $rawPid = trim((string) file_get_contents($pidPath));
            if (preg_match('/^[1-9][0-9]*$/', $rawPid)) {
                $pid = (int) $rawPid;
            }
        }

        $heartbeatAt = null;
        $heartbeatAgeSeconds = null;
        $heartbeatPath = $this->config->cronHeartbeatPath();
        if (is_file($heartbeatPath)) {
            $modified = filemtime($heartbeatPath);
            if (is_int($modified)) {
                $heartbeatAt = gmdate(DATE_ATOM, $modified);
                $heartbeatAgeSeconds = max(0, time() - $modified);
            }
        }

        $binary = null;
        foreach ($this->config->marketCoreBinaryCandidates() as $candidate) {
            if (is_file($candidate)) {
                $binary = $candidate;
                break;
            }
        }

        return [
            'desiredState' => $this->service->desiredState(),
            'pid' => $pid,
            'pidAlive' => $pid !== null && is_dir('/proc/' . $pid),
            'pidPath' => $pidPath,
            'binaryPath' => $binary,
            'binaryFound' => $binary !== null,
            'logPath' => $this->config->marketCoreLogPath(),
            'logLines' => $this->tailLog($this->config->marketCoreLogPath(), 80, 65536),
            'cronHeartbeatAt' => $heartbeatAt,
            'cronHeartbeatAgeSeconds' => $heartbeatAgeSeconds,
            'controlMode' => $this->service->controlMode(),
            'helperPath' => $this->config->helperPath,
        ];
    }

    public function getConfig(): array
    {
        $path = $this->config->configPath();
        if (!is_file($path)) {
            $this->seedDefaultConfig($path);
        }
        if (!is_file($path)) {
            return [];
        }

        $contents = file_get_contents($path);
        if ($contents === false) {
            throw new RuntimeException('cannot read market configuration');
        }

        try {
            $decoded = json_decode($contents, true, 512, JSON_THROW_ON_ERROR);
        } catch (JsonException $error) {
            throw new RuntimeException('stored market configuration is invalid JSON', 0, $error);
        }

        if (!is_array($decoded)) {
            throw new RuntimeException('stored market configuration must be an object');
        }

        if ($decoded === []) {
            $this->seedDefaultConfig($path);
            $contents = file_get_contents($path);
            if ($contents === false) {
                throw new RuntimeException('cannot read seeded market configuration');
            }
            try {
                $decoded = json_decode($contents, true, 512, JSON_THROW_ON_ERROR);
            } catch (JsonException $error) {
                throw new RuntimeException('seeded market configuration is invalid JSON', 0, $error);
            }
            if (!is_array($decoded)) {
                throw new RuntimeException('seeded market configuration must be an object');
            }
        }

        return $decoded;
    }

    public function marketActivation(): array
    {
        $available = ['NIFTY', 'BANKNIFTY', 'MIDCPNIFTY', 'FINNIFTY', 'SENSEX', 'BANKEX'];
        $config = $this->getConfig();
        $configured = $config['active_markets'] ?? null;

        $active = $available;
        if (is_array($configured) && $configured !== []) {
            $active = [];
            foreach ($configured as $symbol) {
                if (!is_string($symbol)) {
                    continue;
                }
                $normalized = strtoupper(trim($symbol));
                if (in_array($normalized, $available, true) && !in_array($normalized, $active, true)) {
                    $active[] = $normalized;
                }
            }
            if ($active === []) {
                $active = $available;
            }
        }

        return [
            'available' => $available,
            'active' => $active,
        ];
    }

    public function candleTimeframes(): array
    {
        $available = $this->availableTimeframes();
        $defaults = $this->defaultTimeframes();
        $config = $this->getConfig();
        $configured = $config['timeframes'] ?? null;

        $enabled = $defaults;
        if (is_array($configured) && $configured !== []) {
            $enabled = $this->orderedSubset($available, $configured);
        }
        if (!in_array('1m', $enabled, true)) {
            $enabled[] = '1m';
        }

        return [
            'available' => $available,
            'enabled' => $enabled,
            'protected' => ['1m'],
            'defaults' => $defaults,
        ];
    }

    public function saveCandleTimeframes(array $payload): array
    {
        $available = $this->availableTimeframes();
        $requested = $payload['enabled'] ?? null;
        if (!is_array($requested)) {
            throw new RuntimeException('enabled candle timeframes must be an array');
        }

        foreach ($requested as $timeframe) {
            if (!is_string($timeframe) || !in_array($timeframe, $available, true)) {
                throw new RuntimeException('unknown candle timeframe');
            }
        }
        if (!in_array('1m', $requested, true)) {
            throw new RuntimeException('1m is protected and must remain enabled');
        }

        $enabled = $this->orderedSubset($available, $requested);
        $config = $this->getConfig();
        $config['timeframes'] = $enabled;

        $chartConfigured = $config['chart_timeframes'] ?? $this->defaultTimeframes();
        $chartRequested = is_array($chartConfigured) ? $chartConfigured : $this->defaultTimeframes();
        $chartEnabled = $this->orderedSubset($enabled, $chartRequested);
        if ($chartEnabled === []) {
            $chartEnabled = ['1m'];
        }
        $config['chart_timeframes'] = $chartEnabled;

        AtomicFile::writeJson($this->config->configPath(), $config);

        return [
            'saved' => true,
            'enabled' => $enabled,
            'chartEnabled' => $chartEnabled,
        ];
    }

    public function chartTimeframes(): array
    {
        $available = $this->availableTimeframes();
        $defaults = $this->defaultTimeframes();
        $config = $this->getConfig();

        $candleConfigured = $config['timeframes'] ?? $defaults;
        $candleEnabled = is_array($candleConfigured) && $candleConfigured !== []
            ? $this->orderedSubset($available, $candleConfigured)
            : $defaults;

        $chartConfigured = $config['chart_timeframes'] ?? $defaults;
        $chartEnabled = is_array($chartConfigured) && $chartConfigured !== []
            ? $this->orderedSubset($candleEnabled, $chartConfigured)
            : $this->orderedSubset($candleEnabled, $defaults);
        if ($chartEnabled === [] && in_array('1m', $candleEnabled, true)) {
            $chartEnabled = ['1m'];
        }

        return [
            'available' => $available,
            'enabled' => $chartEnabled,
            'candleEnabled' => $candleEnabled,
            'defaults' => $defaults,
        ];
    }

    public function saveChartTimeframes(array $payload): array
    {
        $available = $this->availableTimeframes();
        $requested = $payload['enabled'] ?? null;
        if (!is_array($requested) || $requested === []) {
            throw new RuntimeException('at least one chart display timeframe must remain enabled');
        }

        foreach ($requested as $timeframe) {
            if (!is_string($timeframe) || !in_array($timeframe, $available, true)) {
                throw new RuntimeException('unknown chart display timeframe');
            }
        }

        $config = $this->getConfig();
        $candleConfigured = $config['timeframes'] ?? $this->defaultTimeframes();
        $candleEnabled = is_array($candleConfigured)
            ? $this->orderedSubset($available, $candleConfigured)
            : $this->defaultTimeframes();

        foreach ($requested as $timeframe) {
            if (!in_array($timeframe, $candleEnabled, true)) {
                throw new RuntimeException(
                    'chart timeframe ' . $timeframe . ' requires Candle Formation to be enabled first'
                );
            }
        }

        $enabled = $this->orderedSubset($candleEnabled, $requested);
        if ($enabled === []) {
            throw new RuntimeException('at least one chart display timeframe must remain enabled');
        }

        $config['chart_timeframes'] = $enabled;
        AtomicFile::writeJson($this->config->configPath(), $config);

        return [
            'saved' => true,
            'enabled' => $enabled,
        ];
    }

    private function availableTimeframes(): array
    {
        return [
            '15s', '30s',
            '1m', '2m', '3m', '5m', '10m', '15m', '30m', '45m',
            '1h', '2h', '3h', '4h',
            '1D', '1W', '1M',
        ];
    }

    private function defaultTimeframes(): array
    {
        return ['15s', '30s', '1m', '2m', '3m', '5m', '15m', '30m', '1h', '1D'];
    }

    private function orderedSubset(array $order, array $requested): array
    {
        $enabled = [];
        foreach ($order as $timeframe) {
            if (in_array($timeframe, $requested, true)) {
                $enabled[] = $timeframe;
            }
        }
        return $enabled;
    }

    public function saveActiveMarkets(array $payload): array
    {
        $available = ['NIFTY', 'BANKNIFTY', 'MIDCPNIFTY', 'FINNIFTY', 'SENSEX', 'BANKEX'];
        $requested = $payload['active'] ?? null;
        if (!is_array($requested)) {
            throw new RuntimeException('active markets must be an array');
        }

        $active = [];
        foreach ($requested as $symbol) {
            if (!is_string($symbol)) {
                throw new RuntimeException('active market names must be strings');
            }
            $normalized = strtoupper(trim($symbol));
            if (!in_array($normalized, $available, true)) {
                throw new RuntimeException('unknown active market: ' . $normalized);
            }
            if (!in_array($normalized, $active, true)) {
                $active[] = $normalized;
            }
        }
        if ($active === []) {
            throw new RuntimeException('at least one market must remain active');
        }

        $config = $this->getConfig();
        $config['active_markets'] = $active;
        AtomicFile::writeJson($this->config->configPath(), $config);

        return [
            'saved' => true,
            'active' => $active,
        ];
    }


    public function customIndicators(): array
    {
        $catalog = $this->loadIndicatorCatalog();
        $items = is_array($catalog['indicators'] ?? null) ? $catalog['indicators'] : [];

        return [
            'schema' => 'QNEXT.INDICATORS/1',
            'revision' => (int) ($catalog['revision'] ?? 0),
            'kinds' => [[
                'id' => 'adaptive-ema-qalg',
                'label' => 'Adaptive EMA [QALG]',
            ]],
            'indicators' => array_values($items),
        ];
    }

    public function createCustomIndicator(array $payload): array
    {
        $catalog = $this->loadIndicatorCatalog();
        $indicator = $this->validateIndicator($payload);
        $items = is_array($catalog['indicators'] ?? null) ? $catalog['indicators'] : [];

        foreach ($items as $existing) {
            if (is_array($existing) && ($existing['id'] ?? null) === $indicator['id']) {
                throw new RuntimeException('custom indicator id already exists');
            }
        }

        $items[] = $indicator;
        $catalog['indicators'] = array_values($items);
        $catalog['revision'] = ((int) ($catalog['revision'] ?? 0)) + 1;
        $this->writeIndicatorCatalog($catalog);

        return [
            'saved' => true,
            'revision' => $catalog['revision'],
            'indicator' => $indicator,
        ];
    }

    public function updateCustomIndicator(string $id, array $payload): array
    {
        $catalog = $this->loadIndicatorCatalog();
        $items = is_array($catalog['indicators'] ?? null) ? $catalog['indicators'] : [];
        $found = false;
        $updated = null;

        foreach ($items as $index => $existing) {
            if (!is_array($existing) || ($existing['id'] ?? null) !== $id) {
                continue;
            }

            $candidate = array_replace_recursive($existing, $payload);
            $candidate['id'] = $id;
            $updated = $this->validateIndicator($candidate);
            $items[$index] = $updated;
            $found = true;
            break;
        }

        if (!$found || $updated === null) {
            throw new RuntimeException('custom indicator not found');
        }

        $catalog['indicators'] = array_values($items);
        $catalog['revision'] = ((int) ($catalog['revision'] ?? 0)) + 1;
        $this->writeIndicatorCatalog($catalog);

        return [
            'saved' => true,
            'revision' => $catalog['revision'],
            'indicator' => $updated,
        ];
    }

    public function deleteCustomIndicator(string $id): array
    {
        $catalog = $this->loadIndicatorCatalog();
        $items = is_array($catalog['indicators'] ?? null) ? $catalog['indicators'] : [];
        $remaining = [];
        $deleted = false;

        foreach ($items as $existing) {
            if (is_array($existing) && ($existing['id'] ?? null) === $id) {
                $deleted = true;
                continue;
            }
            $remaining[] = $existing;
        }

        if (!$deleted) {
            throw new RuntimeException('custom indicator not found');
        }

        $catalog['indicators'] = array_values($remaining);
        $catalog['revision'] = ((int) ($catalog['revision'] ?? 0)) + 1;
        $this->writeIndicatorCatalog($catalog);

        return [
            'deleted' => true,
            'revision' => $catalog['revision'],
            'id' => $id,
        ];
    }

    public function saveConfig(array $payload): array
    {
        foreach (['timeframes', 'nifty', 'synthetic'] as $key) {
            if (!array_key_exists($key, $payload)) {
                throw new RuntimeException('market configuration is missing ' . $key);
            }
        }
        if (!is_array($payload['timeframes']) || $payload['timeframes'] === []) {
            throw new RuntimeException('market configuration requires timeframes');
        }
        if (!is_array($payload['nifty']) || !is_array($payload['synthetic'])) {
            throw new RuntimeException('nifty and synthetic configuration must be objects');
        }

        AtomicFile::writeJson($this->config->configPath(), $payload);

        return ['saved' => true];
    }

    public function saveSecrets(array $payload): array
    {
        $stored = AtomicFile::writeSecrets($this->config->secretsPath(), $payload);
        $resilienceConfigured = false;

        if (in_array('DHAN_CLIENT_ID', $stored, true) || in_array('DHAN_ACCESS_TOKEN', $stored, true)) {
            $resilienceConfigured = $this->ensureAutoResilienceConfig();
        }

        return [
            'stored' => $stored,
            'resilienceConfigured' => $resilienceConfigured,
        ];
    }

    public function verifyDhanStandby(): array
    {
        $feed = $this->feedStatus();
        if (!($feed['ok'] ?? false) || !isset($feed['body']) || !is_array($feed['body'])) {
            return [
                'ok' => false,
                'reason' => $feed['error'] ?? 'market feed status unavailable',
            ];
        }

        $body = $feed['body'];
        if (!(bool) ($body['resilience_configured'] ?? false)) {
            return [
                'ok' => false,
                'reason' => 'Dhan resilience is not configured in the running Market Core',
            ];
        }

        $niftyID = is_string($body['nifty_instrument_id'] ?? null)
            ? $body['nifty_instrument_id']
            : 'NSE:NIFTY50';
        $resilience = is_array($body['resilience'] ?? null) ? $body['resilience'] : [];
        $providers = is_array($resilience['providers'] ?? null) ? $resilience['providers'] : [];
        $dhan = is_array($providers['dhan'] ?? null) ? $providers['dhan'] : [];
        $authorities = is_array($resilience['active_authorities'] ?? null)
            ? $resilience['active_authorities']
            : [];

        $lastEventMS = (int) ($dhan['last_event_time_ms'] ?? 0);
        $ageMS = $lastEventMS > 0 ? max(0, (int) floor(microtime(true) * 1000) - $lastEventMS) : null;
        $received = (int) ($dhan['received'] ?? 0);
        $errors = (int) ($dhan['errors'] ?? 0);
        $authority = is_string($authorities[$niftyID] ?? null) ? $authorities[$niftyID] : null;
        $fresh = $ageMS !== null && $ageMS <= 5000 && $received > 0;

        return [
            'ok' => $fresh,
            'fresh' => $fresh,
            'ageMs' => $ageMS,
            'received' => $received,
            'errors' => $errors,
            'authority' => $authority,
            'niftyInstrumentId' => $niftyID,
            'safeForFailoverDrill' => $fresh && $authority === 'upstox',
            'reason' => $fresh
                ? 'Dhan standby is receiving fresh NIFTY quotes'
                : 'Dhan standby has not produced a fresh NIFTY quote yet',
        ];
    }

    public function serviceAction(string $action): array
    {
        if (!in_array($action, ['start', 'stop', 'restart'], true)) {
            throw new RuntimeException('unsupported service action');
        }
        $result = $this->service->run($action);
        if (!$result['ok']) {
            throw new RuntimeException($result['error'] ?: 'service action failed');
        }

        return $result;
    }

    public function smoke(): array
    {
        $probes = [
            'health' => $this->probe('/health'),
            'ready' => $this->probe('/ready'),
            'version' => $this->probe('/version'),
        ];

        $ok = true;
        foreach ($probes as $probe) {
            $ok = $ok && $probe['ok'];
        }

        return ['ok' => $ok, 'probes' => $probes];
    }

    public function activate(string $version): array
    {
        $result = $this->service->run('activate', $version);
        if (!$result['ok']) {
            throw new RuntimeException($result['error'] ?: 'release activation failed');
        }

        return [
            'ok' => true,
            'version' => $version,
            'output' => $result['output'],
        ];
    }

    public function rollback(): array
    {
        $result = $this->service->run('rollback');
        if (!$result['ok']) {
            throw new RuntimeException($result['error'] ?: 'rollback failed');
        }

        return ['ok' => true, 'output' => $result['output']];
    }


    private function loadIndicatorCatalog(): array
    {
        $path = $this->config->indicatorCatalogPath();
        if (!is_file($path)) {
            foreach ($this->config->indicatorCatalogCandidates() as $candidate) {
                if (!is_file($candidate)) {
                    continue;
                }
                $raw = file_get_contents($candidate);
                if (!is_string($raw) || trim($raw) === '') {
                    continue;
                }
                try {
                    $decoded = json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
                } catch (JsonException) {
                    continue;
                }
                if (is_array($decoded)) {
                    AtomicFile::writeJson($path, $decoded);
                    break;
                }
            }
        }

        if (!is_file($path)) {
            return [
                'schema' => 'QNEXT.INDICATORS/1',
                'revision' => 0,
                'indicators' => [],
            ];
        }

        $raw = file_get_contents($path);
        if (!is_string($raw)) {
            throw new RuntimeException('cannot read custom indicator catalog');
        }
        try {
            $decoded = json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
        } catch (JsonException $error) {
            throw new RuntimeException('stored custom indicator catalog is invalid JSON', 0, $error);
        }
        if (!is_array($decoded)) {
            throw new RuntimeException('stored custom indicator catalog must be an object');
        }

        return $decoded;
    }

    private function writeIndicatorCatalog(array $catalog): void
    {
        $catalog['schema'] = 'QNEXT.INDICATORS/1';
        $catalog['revision'] = max(0, (int) ($catalog['revision'] ?? 0));
        $catalog['indicators'] = array_values(
            is_array($catalog['indicators'] ?? null) ? $catalog['indicators'] : []
        );
        AtomicFile::writeJson($this->config->indicatorCatalogPath(), $catalog);
    }

    private function validateIndicator(array $payload): array
    {
        $id = $payload['id'] ?? null;
        $name = $payload['name'] ?? null;
        $kind = $payload['kind'] ?? null;

        if (!is_string($id) || !preg_match('/^[a-z0-9][a-z0-9-]{0,63}$/', $id)) {
            throw new RuntimeException('custom indicator id must use lowercase letters, numbers and hyphens');
        }
        if (!is_string($name) || trim($name) === '' || strlen(trim($name)) > 120) {
            throw new RuntimeException('custom indicator name is required and must be 120 characters or fewer');
        }
        if ($kind !== 'adaptive-ema-qalg') {
            throw new RuntimeException('unsupported custom indicator kind');
        }

        $defaults = is_array($payload['defaults'] ?? null) ? $payload['defaults'] : [];
        $source = $defaults['priceSource'] ?? 'close';
        if (!is_string($source) || !in_array($source, ['close', 'open', 'high', 'low', 'hl2', 'hlc3', 'ohlc4'], true)) {
            throw new RuntimeException('invalid price source');
        }

        $intValue = static function (array $values, string $key, int $fallback, int $min, int $max): int {
            $value = $values[$key] ?? $fallback;
            if (!is_int($value) && !is_float($value)) {
                throw new RuntimeException($key . ' must be numeric');
            }
            $value = (int) round((float) $value);
            if ($value < $min || $value > $max) {
                throw new RuntimeException($key . ' is outside the allowed range');
            }
            return $value;
        };
        $floatValue = static function (array $values, string $key, float $fallback, float $min, float $max): float {
            $value = $values[$key] ?? $fallback;
            if (!is_int($value) && !is_float($value)) {
                throw new RuntimeException($key . ' must be numeric');
            }
            $value = (float) $value;
            if (!is_finite($value) || $value < $min || $value > $max) {
                throw new RuntimeException($key . ' is outside the allowed range');
            }
            return $value;
        };
        $colorValue = static function (array $values, string $key, string $fallback): string {
            $value = $values[$key] ?? $fallback;
            if (!is_string($value) || !preg_match('/^#[0-9A-Fa-f]{6}$/', $value)) {
                throw new RuntimeException($key . ' must be a six-digit hex color');
            }
            return strtolower($value);
        };

        return [
            'id' => $id,
            'name' => trim($name),
            'category' => 'QNext',
            'kind' => 'adaptive-ema-qalg',
            'enabled' => isset($payload['enabled']) ? (bool) $payload['enabled'] : true,
            'description' => is_string($payload['description'] ?? null)
                ? substr(trim((string) $payload['description']), 0, 500)
                : '',
            'defaults' => [
                'priceSource' => $source,
                'emaLength' => $intValue($defaults, 'emaLength', 20, 1, 1000),
                'lookbackPeriod' => $intValue($defaults, 'lookbackPeriod', 30, 2, 1000),
                'stddevMultiplier' => $floatValue($defaults, 'stddevMultiplier', 2.0, 0.1, 20.0),
                'atrLength' => $intValue($defaults, 'atrLength', 14, 1, 1000),
                'atrMultiplier' => $floatValue($defaults, 'atrMultiplier', 1.5, 0.1, 20.0),
                'upColor' => $colorValue($defaults, 'upColor', '#00ffaa'),
                'downColor' => $colorValue($defaults, 'downColor', '#ff0000'),
                'colorBars' => isset($defaults['colorBars']) ? (bool) $defaults['colorBars'] : true,
            ],
            'attribution' => is_string($payload['attribution'] ?? null)
                ? substr(trim((string) $payload['attribution']), 0, 120)
                : '',
            'license' => is_string($payload['license'] ?? null)
                ? substr(trim((string) $payload['license']), 0, 80)
                : '',
            'licenseUrl' => is_string($payload['licenseUrl'] ?? null)
                ? substr(trim((string) $payload['licenseUrl']), 0, 300)
                : '',
        ];
    }

    private function ensureAutoResilienceConfig(): bool
    {
        $market = $this->getConfig();
        $nifty = is_array($market['nifty'] ?? null) ? $market['nifty'] : [];
        $synthetic = is_array($market['synthetic'] ?? null) ? $market['synthetic'] : [];

        $markets = is_array($market['markets'] ?? null) ? $market['markets'] : [];
        foreach ($markets as $entry) {
            if (!is_array($entry) || strcasecmp((string) ($entry['symbol'] ?? ''), 'NIFTY') !== 0) {
                continue;
            }
            $nifty = is_array($entry['underlying'] ?? null) ? $entry['underlying'] : [];
            $synthetic = is_array($entry['synthetic'] ?? null) ? $entry['synthetic'] : [];
            break;
        }

        $auto = $synthetic['auto'] ?? null;

        if (!is_array($auto)) {
            return false;
        }

        $instrumentID = $nifty['instrument_id'] ?? null;
        if (!is_string($instrumentID) || trim($instrumentID) === '') {
            throw new RuntimeException('cannot configure Dhan resilience without NIFTY instrument_id');
        }

        AtomicFile::writeJson($this->config->resilienceConfigPath(), [
            'version' => 'q3.resilience.v1',
            'max_staleness_ms' => 2000,
            'gap_recovery_ms' => 2000,
            'dhan_poll_interval_ms' => 1000,
            'instruments' => [[
                'instrument_id' => $instrumentID,
                'dhan_provider_key' => 'IDX_I|13|INDEX',
                'recoverable' => true,
            ]],
            'failover_pending_ms' => 1000,
            'failback_pending_ms' => 5000,
            'authority_cooldown_ms' => 10000,
            'authority_policy_version' => 'q3-authority-v1',
        ]);

        return true;
    }

    private function seedDefaultConfig(string $path): void
    {
        foreach ($this->config->configCandidates() as $candidate) {
            if (!is_file($candidate)) {
                continue;
            }

            $contents = file_get_contents($candidate);
            if ($contents === false || trim($contents) === '') {
                continue;
            }

            try {
                $decoded = json_decode($contents, true, 512, JSON_THROW_ON_ERROR);
            } catch (JsonException) {
                continue;
            }

            if (is_array($decoded)) {
                AtomicFile::writeJson($path, $decoded);
                return;
            }
        }
    }

    private function tailLog(string $path, int $maxLines, int $maxBytes): array
    {
        if (!is_file($path) || !is_readable($path)) {
            return [];
        }

        $handle = fopen($path, 'rb');
        if ($handle === false) {
            return [];
        }

        try {
            $size = filesize($path);
            if (is_int($size) && $size > $maxBytes) {
                fseek($handle, -$maxBytes, SEEK_END);
                fgets($handle);
            }

            $contents = stream_get_contents($handle);
            if (!is_string($contents) || $contents === '') {
                return [];
            }

            $lines = preg_split('/\R/', trim($contents)) ?: [];
            $lines = array_slice($lines, -$maxLines);

            return array_values(array_map(
                static function (string $line): string {
                    $line = preg_replace(
                        '~\bBearer\s+\S+~i',
                        'Bearer [REDACTED]',
                        $line,
                    ) ?? $line;
                    $line = preg_replace(
                        '/((?:access[_ -]?token|authorization|api[_ -]?key|client[_ -]?secret)\s*[:=]\s*)\S+/i',
                        '$1[REDACTED]',
                        $line,
                    ) ?? $line;
                    return substr($line, 0, 2000);
                },
                $lines,
            ));
        } finally {
            fclose($handle);
        }
    }

    private function probe(string $path): array
    {
        return $this->requestCore($path, 'GET', null, 2.0);
    }

    private function requestCore(
        string $path,
        string $method,
        ?array $payload,
        float $timeout,
    ): array {
        $url = $this->config->marketCoreUrl . $path;
        $headers = "Accept: application/json\r\n";
        $content = null;
        if ($payload !== null) {
            $content = json_encode($payload, JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
            $headers .= "Content-Type: application/json\r\n";
        }

        $http = [
            'method' => $method,
            'timeout' => $timeout,
            'ignore_errors' => true,
            'header' => $headers,
        ];
        if ($content !== null) {
            $http['content'] = $content;
        }

        $context = stream_context_create(['http' => $http]);
        $body = @file_get_contents($url, false, $context);
        $status = $this->responseStatus($http_response_header ?? []);
        if ($body === false) {
            return ['ok' => false, 'status' => $status ?: null, 'error' => 'connection failed'];
        }

        $decoded = json_decode($body, true);
        return [
            'ok' => $status >= 200 && $status < 300,
            'status' => $status,
            'body' => is_array($decoded) ? $decoded : $body,
        ];
    }

    private function responseStatus(array $headers): int
    {
        foreach ($headers as $header) {
            if (preg_match('/^HTTP\/\S+\s+(\d{3})/', $header, $matches)) {
                return (int) $matches[1];
            }
        }

        return 0;
    }
}
