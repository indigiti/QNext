<?php

declare(strict_types=1);

use QNext\Ops\AtomicFile;
use QNext\Ops\Auth;
use QNext\Ops\OpsConfig;
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

expect((new Auth('token'))->authorized('token'), 'auth should accept matching token');
expect(!(new Auth('token'))->authorized('wrong'), 'auth should reject wrong token');

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

$control = new ServiceControl($config->helperPath);
try {
    $control->run('shell');
    expect(false, 'arbitrary helper actions must be rejected');
} catch (RuntimeException) {
}

echo "QNext Ops API smoke: PASS\n";
