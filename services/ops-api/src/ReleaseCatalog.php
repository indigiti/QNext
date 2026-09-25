<?php

declare(strict_types=1);

namespace QNext\Ops;

final class ReleaseCatalog
{
    public function __construct(
        private readonly string $releasesRoot,
        private readonly string $currentLink,
        private readonly ?string $publicManifestPath = null,
    ) {
    }

    public function snapshot(): array
    {
        $available = [];
        if (is_dir($this->releasesRoot)) {
            $entries = scandir($this->releasesRoot) ?: [];
            foreach ($entries as $entry) {
                if ($entry === '.' || $entry === '..') {
                    continue;
                }
                if (!preg_match('/^[A-Za-z0-9._-]{1,80}$/', $entry)) {
                    continue;
                }
                if (is_dir($this->releasesRoot . '/' . $entry)) {
                    $available[] = $entry;
                }
            }
        }
        rsort($available, SORT_NATURAL);

        $current = null;
        $mode = 'staged';
        if (is_link($this->currentLink)) {
            $target = readlink($this->currentLink);
            if (is_string($target) && $target !== '') {
                $current = basename($target);
            }
        }

        if ($current === null) {
            $directVersion = $this->directVersion();
            if ($directVersion !== null) {
                $current = $directVersion;
                $mode = 'direct';
            } elseif ($available === []) {
                $mode = 'direct';
            }
        }

        return [
            'current' => $current,
            'available' => $available,
            'mode' => $mode,
        ];
    }

    private function directVersion(): ?string
    {
        if ($this->publicManifestPath === null || !is_file($this->publicManifestPath)) {
            return null;
        }

        $raw = file_get_contents($this->publicManifestPath);
        if ($raw === false || trim($raw) === '') {
            return null;
        }

        $payload = json_decode($raw, true);
        $version = is_array($payload) ? ($payload['version'] ?? null) : null;
        if (!is_string($version) || !preg_match('/^[A-Za-z0-9._-]{1,80}$/', $version)) {
            return null;
        }

        return $version;
    }
}
