<?php

declare(strict_types=1);

use QNext\Ops\Auth;

require_once dirname(__DIR__) . '/bootstrap.php';

function auth_expect(bool $condition, string $message): void
{
    if (!$condition) {
        fwrite(STDERR, $message . PHP_EOL);
        exit(1);
    }
}

$root = sys_get_temp_dir() . '/qnext-auth-migration-' . bin2hex(random_bytes(4));
$path = $root . '/secrets/ops-auth.json';

$persisted = new Auth($path);
$persisted->initialize('persisted-token-0123456789');

$migrating = new Auth($path, 'legacy-token-0123456789');
auth_expect(
    $migrating->authorized('persisted-token-0123456789'),
    'persisted token must remain valid when a legacy environment token is still configured',
);
auth_expect(
    $migrating->authorized('legacy-token-0123456789'),
    'legacy token should remain a migration fallback while configured',
);
auth_expect(
    !$migrating->authorized('wrong-token'),
    'unknown token must be rejected',
);

echo "QNext Ops auth migration: PASS\n";
