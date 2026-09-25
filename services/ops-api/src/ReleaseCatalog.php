<?php

declare(strict_types=1);

namespace QNext\Ops;

final class ReleaseCatalog
{
    public function __construct(
        private readonly string $releasesRoot,
        private readonly string $currentLink,
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
        if (is_link($this->currentLink)) {
            $target = readlink($this->currentLink);
            if (is_string($target) && $target !== '') {
                $current = basename($target);
            }
        }

        return [
            'current' => $current,
            'available' => $available,
        ];
    }
}
