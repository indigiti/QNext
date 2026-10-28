<?php

declare(strict_types=1);

namespace QNext\Ops;

use FilesystemIterator;
use RuntimeException;
use UnexpectedValueException;

/**
 * Read-only, bounded filesystem metadata snapshot for the authenticated Admin UI.
 * Never opens market data files or follows symlinks. This is NOT a data-integrity audit.
 */
final class StorageHealth
{
    private const MAX_ENTRIES = 3000;
    private const MAX_SCAN_SECONDS = 0.20;

    public static function snapshot(string $root): array
    {
        $base = realpath($root);
        $baseResponse = [
            'checked_at' => gmdate('c'),
            'storage_root' => $root,
            'root_exists' => $base !== false && is_dir($base),
            'php_can_read' => $base !== false && is_readable($base),
            'php_can_write' => $base !== false && is_writable($base),
            'disk_total_bytes' => null,
            'disk_free_bytes' => null,
            'disk_used_percent' => null,
            'scanned_bytes' => 0,
            'scanned_files' => 0,
            'scan_complete' => false,
            'last_modified_at' => null,
            'categories' => [],
            'warnings' => [],
            'status' => 'unavailable',
        ];

        if ($base === false || !is_dir($base) || !is_readable($base)) {
            $baseResponse['warnings'][] = 'Storage directory is missing or unreadable by the Admin PHP process.';
            return $baseResponse;
        }

        $total = @disk_total_space($base);
        $free = @disk_free_space($base);
        if (is_numeric($total) && is_numeric($free) && $total > 0) {
            $baseResponse['disk_total_bytes'] = (int) $total;
            $baseResponse['disk_free_bytes'] = (int) $free;
            $baseResponse['disk_used_percent'] = round(100 * (1 - (float) $free / (float) $total), 1);
            if ($free / $total < 0.05) {
                $baseResponse['warnings'][] = 'Filesystem has less than 5% free space.';
            } elseif ($free / $total < 0.15) {
                $baseResponse['warnings'][] = 'Filesystem has less than 15% free space.';
            }
        } else {
            $baseResponse['warnings'][] = 'Filesystem capacity information is unavailable.';
        }

        $started = microtime(true);
        $queue = [[$base, 'Other', 0]];
        $categories = [];
        $files = 0;
        $entries = 0;
        $bytes = 0;
        $latest = 0;
        $complete = true;
        $unreadable = false;

        while ($queue !== []) {
            if ($entries >= self::MAX_ENTRIES || microtime(true) - $started >= self::MAX_SCAN_SECONDS) {
                $complete = false;
                break;
            }
            [$directory, $category, $depth] = array_pop($queue);
            try {
                foreach (new FilesystemIterator($directory, FilesystemIterator::SKIP_DOTS) as $entry) {
                    if ($entries >= self::MAX_ENTRIES || microtime(true) - $started >= self::MAX_SCAN_SECONDS) {
                        $complete = false;
                        break;
                    }
                    $entries++;
                    // Never traverse a symlink, even if it targets a directory under storage.
                    if ($entry->isLink()) {
                        continue;
                    }
                    if ($entry->isDir()) {
                        $childCategory = $depth === 0 ? $entry->getFilename() : $category;
                        if (!isset($categories[$childCategory])) {
                            $categories[$childCategory] = ['name' => $childCategory, 'scanned_bytes' => 0, 'scanned_files' => 0, 'last_modified_at' => null];
                        }
                        $queue[] = [$entry->getPathname(), $childCategory, $depth + 1];
                        continue;
                    }
                    if (!$entry->isFile()) {
                        continue;
                    }
                    if (!isset($categories[$category])) {
                        $categories[$category] = ['name' => $category, 'scanned_bytes' => 0, 'scanned_files' => 0, 'last_modified_at' => null];
                    }
                    $fileBytes = max(0, $entry->getSize());
                    $modified = $entry->getMTime();
                    $bytes += $fileBytes;
                    $files++;
                    $latest = max($latest, $modified);
                    $categories[$category]['scanned_bytes'] += $fileBytes;
                    $categories[$category]['scanned_files']++;
                    $previous = $categories[$category]['last_modified_at'];
                    if ($previous === null || $modified > strtotime($previous)) {
                        $categories[$category]['last_modified_at'] = gmdate('c', $modified);
                    }
                }
            } catch (UnexpectedValueException | RuntimeException $error) {
                $unreadable = true;
                $complete = false;
            }
            if (!$complete && !$unreadable) {
                break;
            }
        }

        ksort($categories);
        $baseResponse['categories'] = array_values($categories);
        $baseResponse['scanned_bytes'] = $bytes;
        $baseResponse['scanned_files'] = $files;
        $baseResponse['scan_complete'] = $complete;
        $baseResponse['last_modified_at'] = $latest ? gmdate('c', $latest) : null;

        if (!$complete) {
            $baseResponse['warnings'][] = 'The metadata scan was incomplete; reported category sizes and counts are partial.';
        }
        if ($unreadable) {
            $baseResponse['warnings'][] = 'One or more storage directories were inaccessible to Admin PHP.';
        }
        // The PHP user and Market Core service user may have different permissions.
        // Do not treat PHP write permission as a Market Core write-health signal.
        $baseResponse['status'] = $baseResponse['warnings'] === [] ? 'healthy' : 'warning';
        return $baseResponse;
    }
}
