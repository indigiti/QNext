<?php
declare(strict_types=1);

namespace QNext\Ops;

use RuntimeException;

/**
 * File-backed admin entry-point throttle. REMOTE_ADDR is used directly;
 * client-controlled X-Forwarded-For headers are deliberately ignored.
 */
final class LoginThrottle
{
    private const WINDOW_SECONDS = 900;
    private const MAX_ATTEMPTS = ['session' => 12, 'recover' => 6];

    public function __construct(private readonly string $directory)
    {
    }

    /** Seconds until retry is allowed; zero means a slot was consumed. */
    public function consume(string $action, string $peer, ?int $now = null): int
    {
        return $this->change($action, $peer, true, $now);
    }

    public function reset(string $action, string $peer): void
    {
        $this->change($action, $peer, false, null);
    }

    private function change(string $action, string $peer, bool $consume, ?int $now): int
    {
        if (!isset(self::MAX_ATTEMPTS[$action])) {
            throw new RuntimeException('invalid auth limit action');
        }
        if (!is_dir($this->directory)
            && !mkdir($this->directory, 0750, true) && !is_dir($this->directory)) {
            throw new RuntimeException('auth rate-limit directory unavailable');
        }
        $path = $this->directory . '/' . hash('sha256', $action . ':' . $peer) . '.json';
        $handle = @fopen($path, 'c+');
        if ($handle === false) {
            throw new RuntimeException('auth rate-limit storage unavailable');
        }
        try {
            @chmod($path, 0600);
            if (!flock($handle, LOCK_EX)) {
                throw new RuntimeException('auth rate-limit lock unavailable');
            }
            try {
                $current = $now ?? time();
                rewind($handle);
                $raw = stream_get_contents($handle, 2048);
                $state = is_string($raw) ? json_decode($raw, true) : null;
                $start = is_array($state) && is_int($state['start'] ?? null) ? $state['start'] : $current;
                $count = is_array($state) && is_int($state['count'] ?? null) ? $state['count'] : 0;
                if ($start > $current || $start + self::WINDOW_SECONDS <= $current) {
                    $start = $current;
                    $count = 0;
                }
                if (!$consume) {
                    $start = $current;
                    $count = 0;
                } elseif ($count >= self::MAX_ATTEMPTS[$action]) {
                    return max(1, $start + self::WINDOW_SECONDS - $current);
                } else {
                    $count++;
                }
                $encoded = json_encode(['start' => $start, 'count' => $count], JSON_THROW_ON_ERROR);
                rewind($handle);
                if (!ftruncate($handle, 0) || fwrite($handle, $encoded) !== strlen($encoded)
                    || !fflush($handle)) {
                    throw new RuntimeException('unable to update auth rate-limit state');
                }
                return 0;
            } finally {
                flock($handle, LOCK_UN);
            }
        } finally {
            fclose($handle);
        }
    }
}
