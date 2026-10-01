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

$firstRecovery = $auth->generateRecoveryCode();
expect_session($auth->recoveryConfigured(), 'recovery code should be configured after generation');
expect_session(strlen($firstRecovery['code']) >= 40, 'recovery code must have high entropy');
expect_session(
    !$auth->recoverWithCode('wrong-recovery-code', 'new-admin-token-0123456789'),
    'wrong recovery code must be rejected',
);
expect_session($auth->authorized($token), 'wrong recovery attempt must not change the current token');

$secondRecovery = $auth->generateRecoveryCode();
expect_session(
    !$auth->recoverWithCode($firstRecovery['code'], 'new-admin-token-0123456789'),
    'generating a new recovery code must invalidate the previous one',
);

$newToken = 'new-admin-token-0123456789';
expect_session(
    $auth->recoverWithCode($secondRecovery['code'], $newToken),
    'current recovery code should replace the admin token',
);
expect_session(!$auth->authorized($token), 'old persisted token must be rejected after recovery');
expect_session($auth->authorized($newToken), 'new token must authorize after recovery');
expect_session(!$auth->recoveryConfigured(), 'recovery code must be consumed after use');
expect_session(
    !$auth->recoverWithCode($secondRecovery['code'], 'another-admin-token-0123456789'),
    'consumed recovery code must not be reusable',
);
expect_session(
    !$auth->authorizedSession($session, $issuedAt + 1),
    'token recovery must invalidate sessions signed with the previous token hash',
);

$thirdRecovery = $auth->generateRecoveryCode();
expect_session($auth->recoveryConfigured(), 'new admin should be able to provision another recovery code');
$auth->revokeRecoveryCode();
expect_session(!$auth->recoveryConfigured(), 'authenticated revocation must remove the recovery code');
expect_session(
    !$auth->recoverWithCode($thirdRecovery['code'], 'revoked-admin-token-0123456789'),
    'revoked recovery code must be rejected',
);

$legacyPath = $root . '/legacy-recovery.json';
$legacy = new Auth($legacyPath, 'legacy-token-0123456789');
$legacySession = $legacy->issueSession($issuedAt);
expect_session(
    $legacy->authorizedSession($legacySession, $issuedAt + 60),
    'legacy environment credential should support migration sessions',
);
$legacyRecovery = $legacy->generateRecoveryCode();
$recoveredLegacyToken = 'persisted-after-legacy-recovery';
expect_session(
    $legacy->recoverWithCode($legacyRecovery['code'], $recoveredLegacyToken),
    'recovery should migrate a legacy-only admin to a persisted token',
);
expect_session(
    !$legacy->authorized('legacy-token-0123456789'),
    'legacy token must be disabled after recovery',
);
expect_session($legacy->authorized($recoveredLegacyToken), 'recovered persisted token should authorize');

$other = new Auth($root . '/other-missing.json', 'different-legacy-token');
expect_session(
    !$other->authorizedSession($legacySession, $issuedAt + 60),
    'session must be bound to current private auth material',
);

echo "QNext Ops auth session + recovery: PASS\n";
