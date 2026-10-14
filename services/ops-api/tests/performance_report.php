<?php

declare(strict_types=1);

use QNext\Ops\PerformanceReport;

require_once dirname(__DIR__) . '/bootstrap.php';

function assertPerformance(bool $ok, string $message): void
{
    if (!$ok) {
        fwrite(STDERR, $message . PHP_EOL);
        exit(1);
    }
}

function syntheticFeed(int $observed, int $samples, int $errors = 0): array
{
    $latencies = [];
    foreach (['tick_handle_latency', 'publish_latency',
              'publish_to_ws_write_latency', 'ws_write_latency'] as $metric) {
        $latencies[$metric] = [
            'samples' => $samples,
            'p50_us' => 4,
            'p95_us' => 16,
            'p99_us' => 64,
        ];
    }
    return [
        'telemetry' => [
            'providers' => ['upstox-secret-account' => ['observed' => $observed]],
            'instruments' => ['QNEXT:SECRET' => ['price' => 1234]],
            'runtime' => [
                'broker' => [
                    ...$latencies,
                    'streams' => 29,
                    'subscribers' => 1,
                    'slow_disconnects' => 0,
                    'dropped_forming' => 0,
                    'coalesced_forming' => 12,
                    'details' => [['instrument_id' => 'PRIVATE:NIFTY-LEG']],
                ],
                'history' => ['persistence' => [
                    'errors' => $errors, 'pending' => 0, 'last_error' => 'UPSTOX_ACCESS_TOKEN=private',
                ]],
                'capture' => ['errors' => 0, 'dropped' => 0],
            ],
        ],
    ];
}

$root = sys_get_temp_dir() . '/qnext-performance-' . bin2hex(random_bytes(8));
$file = $root . '/run/performance-baseline.json';
$collector = new PerformanceReport($file);
$empty = $collector->latest();
assertPerformance(($empty['status'] ?? null) === 'warming', 'empty report should warm');

$release = ['commit' => 'd74b82f100f2abc', 'version' => '20261014', 'started_at' => '2026-10-14T04:00:00Z'];
$requests = 0;
$first = 1_800_000_000_000;
for ($i = 0; $i <= 6; $i++) {
    $report = $collector->capture(function () use ($release, $i, &$requests): array {
        $requests++;
        return [$release, syntheticFeed(100 + $i * 20, 200 + $i * 24)];
    }, $first + $i * 10_000);
    if ($i < 6) {
        assertPerformance($report['status'] === 'warming', 'report should warm for 60 seconds');
    }
}
assertPerformance($report['status'] === 'ready', 'report should be ready at 60s');
assertPerformance($report['window']['collected_samples'] === 7, 'expected 7 samples');
assertPerformance($report['provider_observations_delta'] === 120, 'incorrect provider delta');
assertPerformance($report['latencies']['publish_latency']['new_samples'] === 144, 'incorrect sample delta');
assertPerformance($report['latencies']['publish_latency']['latest_p99_us'] === 64, 'incorrect quantile');
assertPerformance($report['broker']['streams'] === 29, 'broker totals missing');
assertPerformance($report['release']['commit'] === $release['commit'], 'release mismatch');
assertPerformance(is_file($file), 'private baseline not written');
assertPerformance((fileperms($file) & 0777) === 0600, 'private report permissions are not 0600');
assertPerformance($collector->latest()['status'] === 'ready', 'persisted baseline unavailable');
$raw = json_encode($collector->latest(), JSON_THROW_ON_ERROR);
assertPerformance(!str_contains($raw, 'SECRET') && !str_contains($raw, 'TOKEN'), 'report leaked provider secrets');
assertPerformance(!str_contains((string) file_get_contents($file), 'PRIVATE:NIFTY-LEG'), 'stored private baseline leaked instruments');

$collector->capture(function () use (&$requests, $release): array {
    $requests++;
    return [$release, syntheticFeed(9999, 9999)];
}, $first + 62_000);
assertPerformance($requests === 7, '8s sampler throttle did not prevent backend request');

$nextRelease = $release;
$nextRelease['started_at'] = '2026-10-14T05:00:00Z';
$restarted = $collector->capture(static fn (): array => [
    $nextRelease, syntheticFeed(10, 5),
], $first + 75_000);
assertPerformance($restarted['status'] === 'warming' && $restarted['window']['collected_samples'] === 1,
    'process restart must clear prior measurements');

try {
    PerformanceReport::sanitize(['telemetry' => ['runtime' => ['broker' => []]]], $first);
    assertPerformance(false, 'missing Phase 4 metrics should reject');
} catch (RuntimeException $ignored) {
}

@unlink($file);
@unlink($file . '.lock');
@rmdir($root . '/run');
@rmdir($root);
echo "QNext backend performance report: PASS\n";
