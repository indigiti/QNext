<?php

declare(strict_types=1);

namespace QNext\Ops;

use FilesystemIterator;
use RuntimeException;
use Throwable;

/**
 * Canonical market-history day inventory and conservative retention controls.
 *
 * Only finalized candle JSONL files inside storage/market are managed here.
 * Raw captures, futures-volume-ledger, research, and strategy artifacts are
 * deliberately excluded. No arbitrary filesystem paths are accepted.
 */
final class StorageDays
{
    private const SCHEMA = 'QNEXT.STORAGE.KEEP/1';
    private const MIN_AGE_DAYS = 35;
    private const MAX_SCAN_ENTRIES = 6000;
    private const SCAN_BUDGET_SECONDS = 0.25;
    private const MAX_FILES_PER_ACTION = 64;

    public function __construct(private readonly string $privateRoot)
    {
    }

    public function inventory(?string $symbol = null): array
    {
        $market = $this->activeRoot();
        $symbols = $this->symbols();
        if ($symbol === null || $symbol === '') {
            $symbol = $symbols[0] ?? null;
        } elseif (!self::validSymbol($symbol)) {
            throw new RuntimeException('Invalid storage symbol key');
        }
        $pins = $this->readPins();
        $days = [];
        $complete = true;
        $entries = 0;
        $started = microtime(true);
        if ($symbol !== null) {
            foreach ([
                ['root' => $market, 'source' => 'active'],
                ['root' => $this->quarantineRoot(), 'source' => 'quarantined'],
            ] as $source) {
                $root = $source['root'] . '/' . $symbol;
                if (!is_dir($root) || is_link($root)) {
                    continue;
                }
                foreach ($this->children($root) as $tf) {
                    if (!$this->withinBudget($started, $entries)) { $complete = false; break; }
                    $entries++;
                    if (!self::validPart($tf) || !is_dir($root . '/' . $tf) || is_link($root . '/' . $tf)) continue;
                    foreach ($this->children($root . '/' . $tf) as $year) {
                        if (!$this->withinBudget($started, $entries)) { $complete = false; break 2; }
                        $entries++;
                        if (!preg_match('/^20[0-9]{2}$/', $year)) continue;
                        $yr = $root . '/' . $tf . '/' . $year;
                        if (!is_dir($yr) || is_link($yr)) continue;
                        foreach ($this->children($yr) as $month) {
                            if (!$this->withinBudget($started, $entries)) { $complete = false; break 3; }
                            $entries++;
                            if (!preg_match('/^(0[1-9]|1[0-2])$/', $month)) continue;
                            $dir = $yr . '/' . $month;
                            if (!is_dir($dir) || is_link($dir)) continue;
                            foreach ($this->children($dir) as $file) {
                                if (!$this->withinBudget($started, $entries)) { $complete = false; break 4; }
                                $entries++;
                                if (!preg_match('/^(\d{4}-\d{2}-\d{2})\.jsonl$/', $file, $matches)) continue;
                                $date = $matches[1];
                                if (!self::validDate($date) || substr($date, 0, 4) !== $year || substr($date, 5, 2) !== $month) continue;
                                $path = $dir . '/' . $file;
                                if (is_link($path) || !is_file($path)) continue;
                                $bytes = filesize($path);
                                $mtime = filemtime($path);
                                if ($bytes === false || $mtime === false) continue;
                                if (!isset($days[$date])) {
                                    $days[$date] = ['date' => $date, 'bytes' => 0, 'files' => 0, 'timeframes' => [], 'modified_at' => null, 'active_files' => 0, 'quarantined_files' => 0];
                                }
                                $days[$date]['bytes'] += max(0, $bytes);
                                $days[$date]['files']++;
                                $days[$date]['timeframes'][$tf] = true;
                                $days[$date][$source['source'] . '_files']++;
                                $previous = $days[$date]['modified_at'] === null ? 0 : strtotime($days[$date]['modified_at']);
                                if ($mtime > $previous) $days[$date]['modified_at'] = gmdate('c', $mtime);
                            }
                        }
                    }
                }
            }
        }
        krsort($days);
        $rows = [];
        foreach ($days as $date => $row) {
            $row['timeframes'] = array_keys($row['timeframes']);
            sort($row['timeframes']);
            $row['kept'] = isset($pins[$symbol . '|' . $date]);
            $row['protected'] = !$this->oldEnough($date);
            $row['state'] = $row['active_files'] > 0 && $row['quarantined_files'] > 0
                ? 'mixed' : ($row['active_files'] > 0 ? 'active' : 'quarantined');
            $row['removal_eligible'] = !$row['protected'] && !$row['kept'] && $row['state'] === 'active';
            $rows[] = $row;
        }
        if (count($rows) > 120) $complete = false;
        return [
            'symbol' => $symbol,
            'symbols' => $symbols,
            'days' => array_slice($rows, 0, 120),
            'complete' => $complete,
            'minimum_age_days' => self::MIN_AGE_DAYS,
            'notice' => 'Storage days use UTC date filenames. Only market candle JSONL files are managed. Quarantine does not reclaim disk space.',
        ];
    }

    public function keep(string $symbol, string $date, bool $keep): array
    {
        $this->validate($symbol, $date);
        return $this->locked(function () use ($symbol, $date, $keep): array {
            if (!$this->dayFiles($this->activeRoot(), $symbol, $date) &&
                !$this->dayFiles($this->quarantineRoot(), $symbol, $date)) {
                throw new RuntimeException('No matching market history day exists');
            }
            $pins = $this->readPins();
            $key = $symbol . '|' . $date;
            if ($keep) $pins[$key] = true;
            else unset($pins[$key]);
            $kept = [];
            foreach (array_keys($pins) as $entry) {
                $pieces = explode('|', $entry, 2);
                if (count($pieces) === 2 && self::validSymbol($pieces[0]) && self::validDate($pieces[1])) {
                    $kept[] = ['symbol' => $pieces[0], 'date' => $pieces[1]];
                }
            }
            AtomicFile::writeJson($this->pinsPath(), ['schema' => self::SCHEMA, 'kept' => $kept], 0640);
            return ['ok' => true, 'symbol' => $symbol, 'date' => $date, 'kept' => $keep];
        });
    }

    public function quarantine(string $symbol, string $date, bool $coreStopped): array
    {
        $this->validate($symbol, $date);
        if (!$coreStopped) throw new RuntimeException('Market Core must be confirmed stopped before historical data removal.');
        if (!$this->oldEnough($date)) throw new RuntimeException('Recent history is protected for at least 35 days.');
        return $this->locked(function () use ($symbol, $date): array {
            $pins = $this->readPins();
            if (isset($pins[$symbol . '|' . $date])) {
                throw new RuntimeException('This symbol/day is marked Keep. Unpin it first.');
            }
            $files = $this->dayFiles($this->activeRoot(), $symbol, $date);
            if ($files === [] || count($files) > self::MAX_FILES_PER_ACTION) {
                throw new RuntimeException('No eligible day files, or too many timeframes for a safe action.');
            }
            if ($this->dayFiles($this->quarantineRoot(), $symbol, $date) !== []) {
                throw new RuntimeException('Quarantine already has this symbol/day; refusing to overwrite.');
            }
            foreach ($files as $path) {
                if (time() - (int) filemtime($path) < 48 * 3600) {
                    throw new RuntimeException('Recently modified history cannot be removed. Wait at least 48 hours after its last write.');
                }
            }
            $moved = [];
            try {
                foreach ($files as $path) {
                    $relative = substr($path, strlen($this->activeRoot()) + 1);
                    $destination = $this->quarantineRoot() . '/' . $relative;
                    $dir = dirname($destination);
                    if (!$this->safeParent($this->quarantineRoot(), $dir)) {
                        throw new RuntimeException('Quarantine destination is unsafe or unwritable.');
                    }
                    if (file_exists($destination) || is_link($destination) || !@rename($path, $destination)) {
                        throw new RuntimeException('Could not quarantine a historical file.');
                    }
                    $moved[] = [$path, $destination];
                }
                $this->audit('quarantine', $symbol, $date, count($moved));
            } catch (Throwable $error) {
                foreach (array_reverse($moved) as [$original, $destination]) {
                    @rename($destination, $original);
                }
                throw new RuntimeException('Quarantine failed; moved files were rolled back where possible.', 0, $error);
            }
            return ['ok' => true, 'state' => 'quarantined', 'files' => count($moved)];
        });
    }

    public function restore(string $symbol, string $date, bool $coreStopped): array
    {
        $this->validate($symbol, $date);
        if (!$coreStopped) throw new RuntimeException('Market Core must be confirmed stopped before restore.');
        return $this->locked(function () use ($symbol, $date): array {
            $files = $this->dayFiles($this->quarantineRoot(), $symbol, $date);
            if ($files === [] || count($files) > self::MAX_FILES_PER_ACTION) {
                throw new RuntimeException('No quarantined files found or too many timeframes.');
            }
            if ($this->dayFiles($this->activeRoot(), $symbol, $date) !== []) {
                throw new RuntimeException('Active history already contains files for this day; no overwrite permitted.');
            }
            $moved = [];
            try {
                foreach ($files as $path) {
                    $relative = substr($path, strlen($this->quarantineRoot()) + 1);
                    $destination = $this->activeRoot() . '/' . $relative;
                    if (!$this->safeParent($this->activeRoot(), dirname($destination))) {
                        throw new RuntimeException('Active storage destination is unsafe or unwritable.');
                    }
                    if (file_exists($destination) || is_link($destination) || !@rename($path, $destination)) {
                        throw new RuntimeException('Could not restore historical file.');
                    }
                    $moved[] = [$path, $destination];
                }
                $this->audit('restore', $symbol, $date, count($moved));
            } catch (Throwable $error) {
                foreach (array_reverse($moved) as [$original, $destination]) {
                    @rename($destination, $original);
                }
                throw new RuntimeException('Restore failed; moved files were rolled back where possible.', 0, $error);
            }
            return ['ok' => true, 'state' => 'active', 'files' => count($moved)];
        });
    }

    private function dayFiles(string $root, string $symbol, string $date): array
    {
        $base = $root . '/' . $symbol;
        if (is_link($root) || is_link(dirname($root)) || !is_dir($base) || is_link($base)) return [];
        $files = [];
        foreach ($this->children($base) as $tf) {
            if (!self::validPart($tf) || is_link($base . '/' . $tf) || !is_dir($base . '/' . $tf)) continue;
            $year = substr($date, 0, 4);
            $month = substr($date, 5, 2);
            $dir = $base . '/' . $tf . '/' . $year . '/' . $month;
            if (is_link($base . '/' . $tf . '/' . $year) || is_link($dir) || !is_dir($dir)) continue;
            $path = $dir . '/' . $date . '.jsonl';
            if (is_file($path) && !is_link($path)) $files[] = $path;
            if (count($files) > self::MAX_FILES_PER_ACTION) break;
        }
        return $files;
    }

    private function symbols(): array
    {
        $seen = [];
        foreach ([$this->activeRoot(), $this->quarantineRoot()] as $root) {
            if (!is_dir($root) || is_link($root)) continue;
            foreach ($this->children($root) as $entry) {
                if (self::validSymbol($entry) && !is_link($root . '/' . $entry) && is_dir($root . '/' . $entry)) {
                    $seen[$entry] = true;
                }
            }
        }
        $all = array_keys($seen);
        sort($all);
        return array_slice($all, 0, 250);
    }

    private function children(string $directory): array
    {
        if (!is_dir($directory) || is_link($directory) || !is_readable($directory)) return [];
        $items = [];
        try {
            foreach (new FilesystemIterator($directory, FilesystemIterator::SKIP_DOTS) as $entry) {
                if (count($items) >= self::MAX_SCAN_ENTRIES) break;
                $items[] = $entry->getFilename();
            }
        } catch (Throwable) {
            return [];
        }
        return $items;
    }

    private function safeParent(string $root, string $dir): bool
    {
        if (is_link($root) || is_link(dirname($root))) return false;
        if (!is_dir($root) && !@mkdir($root, 0750, true) && !is_dir($root)) return false;
        $relative = ltrim(substr($dir, strlen($root)), '/');
        $current = $root;
        foreach (explode('/', $relative) as $part) {
            if (!self::validPart($part)) return false;
            $current .= '/' . $part;
            if (is_link($current)) return false;
            if (!is_dir($current) && !@mkdir($current, 0750) && !is_dir($current)) return false;
        }
        return true;
    }

    private function locked(callable $fn): array
    {
        $dir = $this->privateRoot . '/run';
        if (!is_dir($dir) && !@mkdir($dir, 0750, true) && !is_dir($dir)) {
            throw new RuntimeException('Cannot create storage operation lock directory');
        }
        $file = fopen($dir . '/storage-days.lock', 'c');
        if ($file === false) throw new RuntimeException('Cannot open storage operation lock');
        try {
            if (!flock($file, LOCK_EX)) throw new RuntimeException('Cannot acquire storage operation lock');
            return $fn();
        } finally {
            flock($file, LOCK_UN);
            fclose($file);
        }
    }

    private function readPins(): array
    {
        $path = $this->pinsPath();
        if (!is_file($path)) return [];
        $payload = json_decode((string) file_get_contents($path), true);
        if (!is_array($payload) || ($payload['schema'] ?? null) !== self::SCHEMA || !is_array($payload['kept'] ?? null)) {
            throw new RuntimeException('Invalid storage kept-days manifest');
        }
        $pins = [];
        foreach ($payload['kept'] as $entry) {
            if (is_array($entry) && self::validSymbol((string) ($entry['symbol'] ?? '')) &&
                self::validDate((string) ($entry['date'] ?? ''))) {
                $pins[$entry['symbol'] . '|' . $entry['date']] = true;
            }
        }
        return $pins;
    }

    private function validate(string $symbol, string $date): void
    {
        if (!self::validSymbol($symbol) || !self::validDate($date)) {
            throw new RuntimeException('Invalid storage symbol or date');
        }
    }

    private function oldEnough(string $date): bool
    {
        $cutoff = (new \DateTimeImmutable('today', new \DateTimeZone('Asia/Kolkata')))
            ->modify('-' . self::MIN_AGE_DAYS . ' days')->format('Y-m-d');
        return $date < $cutoff;
    }

    private function withinBudget(float $started, int $entries): bool
    {
        return $entries < self::MAX_SCAN_ENTRIES && microtime(true) - $started < self::SCAN_BUDGET_SECONDS;
    }

    private static function validPart(string $part): bool
    {
        return $part !== '.' && $part !== '..' && preg_match('/^[A-Za-z0-9_.+-]{1,80}$/D', $part) === 1;
    }

    private static function validSymbol(string $symbol): bool
    {
        return self::validPart($symbol);
    }

    private static function validDate(string $date): bool
    {
        return preg_match('/^(\d{4})-(\d{2})-(\d{2})$/D', $date, $match) === 1 &&
            checkdate((int) $match[2], (int) $match[3], (int) $match[1]);
    }

    private function activeRoot(): string { return $this->privateRoot . '/storage/market'; }
    private function quarantineRoot(): string { return $this->privateRoot . '/storage-quarantine/market'; }
    private function pinsPath(): string { return $this->privateRoot . '/config/storage-kept-days.json'; }

    private function audit(string $action, string $symbol, string $date, int $files): void
    {
        $path = $this->privateRoot . '/run/storage-days-actions.jsonl';
        $record = json_encode(['at' => gmdate('c'), 'action' => $action, 'symbol' => $symbol, 'date' => $date, 'files' => $files], JSON_THROW_ON_ERROR);
        if (@file_put_contents($path, $record . "\n", FILE_APPEND | LOCK_EX) === false) {
            throw new RuntimeException('Could not write storage action audit');
        }
    }
}
