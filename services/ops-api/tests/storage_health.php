<?php

declare(strict_types=1);

use QNext\Ops\StorageHealth;

require_once dirname(__DIR__) . '/src/StorageHealth.php';

function storageExpect(bool $condition, string $message): void
{
    if (!$condition) {
        fwrite(STDERR, $message . PHP_EOL);
        exit(1);
    }
}

$missing = StorageHealth::snapshot(sys_get_temp_dir() . '/qnext-storage-does-not-exist-' . bin2hex(random_bytes(5)));
storageExpect($missing['status'] === 'unavailable', 'Missing storage must not report healthy.');
storageExpect($missing['scan_complete'] === false, 'Missing storage scan cannot be complete.');

$root = sys_get_temp_dir() . '/qnext-storage-test-' . bin2hex(random_bytes(5));
mkdir($root . '/market/NIFTY/1m', 0750, true);
mkdir($root . '/futures-volume-ledger', 0750, true);
file_put_contents($root . '/market/NIFTY/1m/today.jsonl', "one\ntwo\n");
file_put_contents($root . '/futures-volume-ledger/deltas.jsonl', "three\n");
$external = tempnam(sys_get_temp_dir(), 'qnext-outside-');
file_put_contents($external, str_repeat('x', 1024));
symlink($external, $root . '/market/outsider');
$snapshot = StorageHealth::snapshot($root);
storageExpect($snapshot['status'] === 'healthy', 'Small readable storage must be healthy.');
storageExpect($snapshot['scan_complete'] === true, 'Small storage scan must finish.');
storageExpect($snapshot['scanned_files'] === 2, 'Symlinks must not be counted or opened.');
storageExpect($snapshot['scanned_bytes'] === strlen("one\ntwo\n") + strlen("three\n"), 'Scanned bytes must cover only local files.');
storageExpect(count($snapshot['categories']) === 2, 'Directories must remain distinguishable.');
storageExpect($snapshot['disk_total_bytes'] !== null, 'Disk capacity must be available.');
unlink($root . '/market/outsider');
unlink($root . '/market/NIFTY/1m/today.jsonl');
unlink($root . '/futures-volume-ledger/deltas.jsonl');
rmdir($root . '/market/NIFTY/1m');
rmdir($root . '/market/NIFTY');
rmdir($root . '/market');
rmdir($root . '/futures-volume-ledger');
rmdir($root);
unlink($external);
echo "storage health OK\n";
