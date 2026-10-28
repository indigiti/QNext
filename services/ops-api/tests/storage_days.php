<?php
declare(strict_types=1);

use QNext\Ops\AtomicFile;
use QNext\Ops\StorageDays;

require_once dirname(__DIR__) . '/src/AtomicFile.php';
require_once dirname(__DIR__) . '/src/StorageDays.php';

function checkStorage(bool $ok, string $message): void {
    if (!$ok) { fwrite(STDERR, $message . PHP_EOL); exit(1); }
}
$root = sys_get_temp_dir() . '/qnext-days-' . bin2hex(random_bytes(4));
$symbol = 'QNEXT_NIFTY-SYN+';
$date = '2025-01-10';
$dayRoot = $root . '/storage/market/' . $symbol;
mkdir($dayRoot . '/1m/2025/01', 0750, true);
mkdir($dayRoot . '/15s/2025/01', 0750, true);
file_put_contents($dayRoot . '/1m/2025/01/' . $date . '.jsonl', "abc\n");
file_put_contents($dayRoot . '/15s/2025/01/' . $date . '.jsonl', "abcd\n");
touch($dayRoot . '/1m/2025/01/' . $date . '.jsonl', time() - 3600 * 72);
touch($dayRoot . '/15s/2025/01/' . $date . '.jsonl', time() - 3600 * 72);

$days = new StorageDays($root);
$listing = $days->inventory($symbol);
checkStorage($listing['complete'] === true && count($listing['days']) === 1, 'Expected single day.');
checkStorage($listing['days'][0]['files'] === 2 && $listing['days'][0]['removal_eligible'], 'Expected eligible market files.');
$days->keep($symbol, $date, true);
checkStorage($days->inventory($symbol)['days'][0]['kept'], 'Keep must be persistent.');
try { $days->quarantine($symbol, $date, true); checkStorage(false, 'Kept day must be protected'); } catch (RuntimeException $e) { checkStorage(str_contains($e->getMessage(), 'Keep'), 'Expected keep guard'); }
$days->keep($symbol, $date, false);
try { $days->quarantine($symbol, $date, false); checkStorage(false, 'Market Core must be stopped'); } catch (RuntimeException $e) { checkStorage(str_contains($e->getMessage(), 'stopped'), 'Expected running-core guard'); }
$days->quarantine($symbol, $date, true);
checkStorage($days->inventory($symbol)['days'][0]['state'] === 'quarantined', 'Quarantine must be visible.');
checkStorage(!file_exists($dayRoot . '/1m/2025/01/' . $date . '.jsonl'), 'Active day should be absent.');
$days->restore($symbol, $date, true);
checkStorage($days->inventory($symbol)['days'][0]['state'] === 'active', 'Restore must be visible.');
try { $days->keep('../escape', $date, true); checkStorage(false, 'Traversal should fail'); } catch (RuntimeException) {}
try { $days->quarantine($symbol, date('Y-m-d'), true); checkStorage(false, 'Recent day must be protected'); } catch (RuntimeException) {}
echo "storage-days OK\n";
