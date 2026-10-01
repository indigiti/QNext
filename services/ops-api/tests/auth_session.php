<?php

declare(strict_types=1);

use QNext\Ops\Auth;

require_once dirname(__DIR__) . '/bootstrap.php';

function expect_session(bool $condition, string $message): void
{
    if (!$condition) {
        fwrite(STDERR, $message . PHP_EOL);
        exit(1);
    }
}

$root = sys_get_temp_dir() . '/qnext-auth-session-' . bin2hex(random_bytes(4));
mkdir($root, 0750, true);
$authPath = $root . '/ops-auth.json';
$token = '0123456789abcdef-session';
$auth = new Auth($authPath);
$auth->initialize($token);
expect_session($auth->authorized($token), 'persisted token should authorize');

$issuedAt = 1_800_000_000;
$session = $auth->issueSession($issuedAt);
expect_session($auth->authorizedSession($session, $issuedAt + 1), 'fresh session should authorize');
expect_session(
    !$auth->authorizedSession($session, $issuedAt + Auth::sessionTTLSeconds()),
    'expired session must be rejected',
);

$tampered = substr($session, 0, -1) . (str_ends_with($session, '0') ? '1' : '0');
expect_session(!$auth->authorizedSession($tampered, $issuedAt + 1), 'tampered session must be rejected');

$legacy = new Auth($root . '/missing.json', 'legacy-token-0123456789');
$legacySession = $legacy->issueSession($issuedAt);
expect_session(
    $legacy->authorizedSession($legacySession, $issuedAt + 60),
    'legacy environment credential should support migration sessions',
);

$other = new Auth($root . '/other-missing.json', 'different-legacy-token');
expect_session(
    !$other->authorizedSession($legacySession, $issuedAt + 60),
    'session must be bound to current private auth material',
);

echo "QNext Ops auth session: PASS\n";
