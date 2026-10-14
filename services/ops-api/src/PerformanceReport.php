<?php

declare(strict_types=1);

namespace QNext\Ops;

use RuntimeException;

// Backend-owned, bounded read-only snapshot history for authenticated Ops.
// No subscription, profiling listener, shell command or public market route.
final class PerformanceReport
{
    private const SAMPLE_INTERVAL_MS = 8_000;
    private const WINDOW_MS = 120_000;
    private const READY_MS = 60_000;
    private const MAX_SAMPLES = 24;
    private const HISTOGRAMS = [
        'tick_handle_latency',
        'publish_latency',
        'publish_to_ws_write_latency',
        'ws_write_latency',
    ];
    private const BROKER_FIELDS = [
        'streams', 'active_streams', 'idle_streams', 'subscribers',
        'websocket_connections', 'replay_slots', 'replay_bytes_estimated',
        'slow_disconnects', 'coalesced_forming', 'dropped_forming',
        'compacted_streams', 'reclaimed_replay_slots',
    ];

    public function __construct(private readonly string $filePath)
    {
    }

    public function latest(): array
    {
        return $this->makeReport($this->readState());
    }

    // Snapshot reader is invoked only after the throttle and lock checks.
    // It must return [Market Core /version response, /feed-status response].
    public function capture(callable $readSnapshot, ?int $nowMS = null): array
    {
        $nowMS ??= (int) floor(microtime(true) * 1_000);
        $directory = dirname($this->filePath);
        if (!is_dir($directory) && !mkdir($directory, 0750, true) && !is_dir($directory)) {
            throw new RuntimeException('performance report private directory unavailable');
        }
        $lock = fopen($this->filePath . '.lock', 'c');
        if ($lock === false) {
            throw new RuntimeException('performance report lock unavailable');
        }
        try {
            @chmod($this->filePath . '.lock', 0600);
            if (!flock($lock, LOCK_EX)) {
                throw new RuntimeException('cannot lock performance report');
            }
            $state = $this->readState();
            $samples = $state['samples'] ?? [];
            $last = $samples !== [] ? end($samples) : null;
            if (is_array($last) && $nowMS - (int) ($last['at_ms'] ?? 0) < self::SAMPLE_INTERVAL_MS
                && $nowMS >= (int) ($last['at_ms'] ?? 0)) {
                return $this->makeReport($state);
            }

            [$version, $feed] = $readSnapshot();
            if (!is_array($version) || !is_array($feed)) {
                throw new RuntimeException('market core performance telemetry unavailable');
            }
            $commit = $version['commit'] ?? null;
            $started = $version['started_at'] ?? null;
            if (!is_string($commit) || trim($commit) === ''
                || !is_string($started) || trim($started) === '') {
                throw new RuntimeException('market core version information unavailable');
            }
            $newSample = self::sanitize($feed, $nowMS);
            $identity = hash('sha256', $commit . "\n" . $started);
            if (($state['identity'] ?? null) !== $identity) {
                $samples = [];
            }
            $samples[] = $newSample;
            $samples = array_values(array_filter(
                $samples,
                static fn (mixed $sample): bool => is_array($sample)
                    && isset($sample['at_ms'])
                    && $sample['at_ms'] >= $nowMS - self::WINDOW_MS
                    && $sample['at_ms'] <= $nowMS,
            ));
            $samples = array_slice($samples, -self::MAX_SAMPLES);
            $state = [
                'schema' => 'QNEXT.PERF.SAMPLES/1',
                'identity' => $identity,
                'release' => ['commit' => $commit, 'version' => $version['version'] ?? '',
                              'started_at' => $started],
                'samples' => $samples,
            ];
            AtomicFile::writeJson($this->filePath, $state, 0600);
            return $this->makeReport($state);
        } finally {
            flock($lock, LOCK_UN);
            fclose($lock);
        }
    }

    public static function sanitize(array $feed, int $nowMS): array
    {
        $telemetry = $feed['telemetry'] ?? null;
        $runtime = is_array($telemetry) ? ($telemetry['runtime'] ?? null) : null;
        $broker = is_array($runtime) ? ($runtime['broker'] ?? null) : null;
        if (!is_array($broker)) {
            throw new RuntimeException('missing Market Core broker metrics');
        }

        $latencies = [];
        foreach (self::HISTOGRAMS as $metric) {
            if (!is_array($broker[$metric] ?? null)) {
                throw new RuntimeException('Phase 4 latency metric missing: ' . $metric);
            }
            $latencies[$metric] = [];
            foreach (['samples', 'p50_us', 'p95_us', 'p99_us'] as $field) {
                $latencies[$metric][$field] = self::number($broker[$metric][$field] ?? null);
            }
        }
        $counts = [];
        foreach (self::BROKER_FIELDS as $field) {
            $counts[$field] = self::number($broker[$field] ?? null);
        }
        $providers = is_array($telemetry['providers'] ?? null) ? $telemetry['providers'] : [];
        $observed = 0;
        foreach ($providers as $provider) {
            if (is_array($provider)) {
                $observed += (int) (self::number($provider['observed'] ?? null) ?? 0);
            }
        }
        $history = is_array($runtime['history'] ?? null) ? $runtime['history'] : [];
        $persistence = is_array($history['persistence'] ?? null) ? $history['persistence'] : [];
        $capture = is_array($runtime['capture'] ?? null) ? $runtime['capture'] : [];

        return [
            'at_ms' => $nowMS,
            'broker' => $counts,
            'latencies' => $latencies,
            'provider_observations' => $observed,
            'persistence' => self::selectNumbers($persistence, ['queued', 'pending', 'errors', 'flush_lag_ms']),
            'capture' => self::selectNumbers($capture, ['dropped', 'errors', 'queued']),
        ];
    }

    private static function selectNumbers(array $source, array $fields): array
    {
        $result = [];
        foreach ($fields as $field) {
            $result[$field] = self::number($source[$field] ?? null);
        }
        return $result;
    }

    private static function number(mixed $value): int|float|null
    {
        return (is_int($value) || is_float($value)) && is_finite((float) $value) ? $value : null;
    }

    private function readState(): array
    {
        if (!is_file($this->filePath)) {
            return [];
        }
        $raw = @file_get_contents($this->filePath);
        if (!is_string($raw) || strlen($raw) > 128 * 1024) {
            return [];
        }
        $state = json_decode($raw, true);
        return is_array($state) && ($state['schema'] ?? null) === 'QNEXT.PERF.SAMPLES/1'
            && is_array($state['samples'] ?? null) ? $state : [];
    }

    private function makeReport(array $state): array
    {
        $samples = $state['samples'] ?? [];
        if (!is_array($samples) || $samples === []) {
            return [
                'schema' => 'QNEXT.PERFORMANCE/1',
                'status' => 'warming',
                'message' => 'No backend samples yet; open Admin Live Broker to collect.',
                'collected_samples' => 0,
            ];
        }

        $first = $samples[0];
        $last = $samples[count($samples) - 1];
        $span = max(0, (int) $last['at_ms'] - (int) $first['at_ms']);
        $ageMS = (int) floor(microtime(true) * 1_000) - (int) $last['at_ms'];
        $stale = $ageMS > self::WINDOW_MS;
        $ready = $span >= self::READY_MS && count($samples) >= 2 && !$stale;
        $warnings = [];
        if ($stale) {
            $warnings[] = 'Backend report is stale; open Admin and collect a fresh baseline';
        } elseif (!$ready) {
            $warnings[] = '60 seconds of backend observations required before the baseline is ready';
        }
        $latencies = [];
        foreach (self::HISTOGRAMS as $metric) {
            $begin = $first['latencies'][$metric];
            $end = $last['latencies'][$metric];
            $change = self::delta($begin['samples'], $end['samples']);
            $latencies[$metric] = [
                'new_samples' => $change,
                'total_samples_since_restart' => $end['samples'],
                'latest_p50_us' => $end['p50_us'],
                'latest_p95_us' => $end['p95_us'],
                'latest_p99_us' => $end['p99_us'],
            ];
            if ($ready && ($change === null || $change <= 0)) {
                $warnings[] = $metric . ': no new samples';
            } elseif ($ready && $change < 100) {
                $warnings[] = $metric . ': fewer than 100 new samples; p99 is provisional';
            }
        }
        $counters = [];
        foreach (['slow_disconnects', 'dropped_forming', 'coalesced_forming'] as $field) {
            $counters[$field . '_delta'] = self::delta($first['broker'][$field], $last['broker'][$field]);
        }
        $providerDelta = self::delta($first['provider_observations'], $last['provider_observations']);
        if ($ready && ($providerDelta === null || $providerDelta <= 0)) {
            $warnings[] = 'No new provider ticks observed in the measurement window';
        }
        foreach (['persistence' => ['errors'], 'capture' => ['errors', 'dropped']] as $area => $fields) {
            foreach ($fields as $field) {
                $difference = self::delta($first[$area][$field], $last[$area][$field]);
                if ($difference !== null && $difference > 0) {
                    $warnings[] = $area . '.' . $field . ' increased during collection';
                }
            }
        }
        foreach ($counters as $name => $difference) {
            if ($difference !== null && $difference < 0) {
                $warnings[] = $name . ' reset (possible process restart)';
            }
        }
        return [
            'schema' => 'QNEXT.PERFORMANCE/1',
            'status' => $stale ? 'stale' : ($ready ? 'ready' : 'warming'),
            'release' => $state['release'] ?? [],
            'window' => [
                'start_ms' => $first['at_ms'],
                'end_ms' => $last['at_ms'],
                'duration_ms' => $span,
                'collected_samples' => count($samples),
            ],
            'broker' => $last['broker'],
            'latencies' => $latencies,
            'provider_observations_delta' => $providerDelta,
            'persistence' => $last['persistence'],
            'capture' => $last['capture'],
            'counter_deltas' => $counters,
            'warnings' => $warnings,
            'notes' => [
                'Percentiles are cumulative log2 histogram upper bounds, not window percentiles',
                'No exchange-to-browser latency, browser paint, or CPU profiler measured',
                'No WebSocket subscriptions created and no candle or persistence modifications',
            ],
        ];
    }

    private static function delta(mixed $before, mixed $after): int|float|null
    {
        return is_numeric($before) && is_numeric($after) ? $after - $before : null;
    }
}
