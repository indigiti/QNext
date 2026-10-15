<?php
declare(strict_types=1);

use QNext\Ops\LoginThrottle;

require_once dirname(__DIR__) . '/src/LoginThrottle.php';

function expect_throttle(bool $condition, string $message): void
{
    if (!$condition) {
        fwrite(STDERR, $message . PHP_EOL);
        exit(1);
    }
}

$root = sys_get_temp_dir() . '/qnext-throttle-' . bin2hex(random_bytes(6));
$limit = new LoginThrottle($root);
for ($i = 0; $i < 12; $i++) {
    expect_throttle($limit->consume('session', '198.51.100.10', 2000000000) === 0, 'accepted login slot');
}
expect_throttle($limit->consume('session', '198.51.100.10', 2000000000) === 900, 'login must be throttled');
expect_throttle($limit->consume('session', '198.51.100.10', 2000000001) === 899, 'countdown');
expect_throttle($limit->consume('session', '198.51.100.11', 2000000001) === 0, 'peer isolation');
for ($i = 0; $i < 6; $i++) {
    expect_throttle($limit->consume('recover', '198.51.100.10', 2000000001) === 0, 'accepted recovery slot');
}
expect_throttle($limit->consume('recover', '198.51.100.10', 2000000002) === 899, 'recovery must be throttled');
expect_throttle($limit->consume('session', '198.51.100.10', 2000000900) === 0, 'window should reopen');
$limit->reset('session', '198.51.100.10');
expect_throttle($limit->consume('session', '198.51.100.10') === 0, 'reset should clear limit');
foreach (glob($root . '/*.json') ?: [] as $path) unlink($path);
rmdir($root);
echo "QNext admin rate limit: PASS\n";
