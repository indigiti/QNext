<?php

declare(strict_types=1);

require_once dirname(__DIR__) . '/bootstrap.php';

use QNext\Ops\AccessGovernance;
use QNext\Ops\AccessPolicy;
use QNext\Ops\InviteApprovalStore;

function assertSameValue(mixed $expected, mixed $actual, string $label): void
{
    if ($expected !== $actual) {
        fwrite(STDERR, sprintf("FAIL %s: expected %s got %s\n", $label, var_export($expected, true), var_export($actual, true)));
        exit(1);
    }
}

function assertTrue(bool $condition, string $label): void
{
    if (!$condition) {
        fwrite(STDERR, "FAIL {$label}\n");
        exit(1);
    }
}

function assertThrows(callable $fn, string $label): void
{
    try {
        $fn();
    } catch (Throwable) {
        return;
    }
    fwrite(STDERR, "FAIL {$label}: expected exception\n");
    exit(1);
}

// Domain policy behavior.
$policy = new AccessPolicy();
$registration = ['id' => 'r1', 'applicant_user_id' => 'u-new', 'has_active_account' => false];
$events = [
    ['approver_user_id' => 'u1', 'approver_role' => 'USER', 'decision' => 'APPROVED', 'created_at' => '2026-09-27T01:00:00Z'],
    ['approver_user_id' => 'u2', 'approver_role' => 'USER', 'decision' => 'APPROVED', 'created_at' => '2026-09-27T01:01:00Z'],
];
assertSameValue(AccessPolicy::STATUS_PENDING_APPROVAL, $policy->evaluate($registration, $events), 'two approvals remain pending');
$events[] = ['approver_user_id' => 'u3', 'approver_role' => 'USER', 'decision' => 'APPROVED', 'created_at' => '2026-09-27T01:02:00Z'];
assertSameValue(AccessPolicy::STATUS_ACCOUNT_REQUIRED, $policy->evaluate($registration, $events), 'three approvals require account');
$registration['has_active_account'] = true;
assertSameValue(AccessPolicy::STATUS_ACTIVE, $policy->evaluate($registration, $events), 'three approvals plus account activates');
$events[] = ['approver_user_id' => 'u3', 'approver_role' => 'USER', 'decision' => 'REVOKED', 'created_at' => '2026-09-27T01:04:00Z'];
assertSameValue(AccessPolicy::STATUS_PENDING_APPROVAL, $policy->evaluate($registration, $events), 'revocation removes quorum');
assertThrows(static function () use ($policy, $registration): void {
    $policy->validateEvent($registration, ['approver_user_id' => 'u-new', 'approver_role' => 'USER', 'decision' => 'APPROVED']);
}, 'self approval');

// File-backed end-to-end governance behavior.
$tmp = sys_get_temp_dir() . '/qnext-access-' . bin2hex(random_bytes(6)) . '.json';
$store = new InviteApprovalStore($tmp);
$governance = new AccessGovernance($store);

$updatedPolicy = $governance->updatePolicy([
    'user_quorum' => 3,
    'admin_override' => true,
    'require_active_account' => true,
    'invite_ttl_hours' => 168,
]);
assertSameValue(3, $updatedPolicy['user_quorum'], 'policy quorum persisted');
assertThrows(static fn () => $governance->updatePolicy(['user_quorum' => 0]), 'invalid quorum rejected');

$invite = $governance->issueInvite(['email' => 'new.user@example.com'], 'admin-1');
assertTrue(isset($invite['invite_token']) && strlen((string) $invite['invite_token']) >= 32, 'invite token returned once');
assertSameValue(true, $governance->inspectInvite((string) $invite['invite_token'])['valid'], 'fresh invite valid');

$rawState = file_get_contents($tmp) ?: '';
assertTrue(!str_contains($rawState, (string) $invite['invite_token']), 'raw invite token never persisted');
assertTrue(str_contains($rawState, 'token_hash'), 'invite token hash persisted');

$snapshot = $governance->redeemInvite([
    'invite_token' => $invite['invite_token'],
    'applicant_user_id' => 'user-new',
]);
$registrationId = (string) $snapshot['registration']['id'];
assertSameValue(AccessPolicy::STATUS_PENDING_APPROVAL, $snapshot['registration']['status'], 'redeemed invite awaits approvals');
assertSameValue(false, $governance->inspectInvite((string) $invite['invite_token'])['valid'], 'used invite cannot be reused');
assertThrows(static fn () => $governance->redeemInvite([
    'invite_token' => $invite['invite_token'],
    'applicant_user_id' => 'another-user',
]), 'used invite redemption rejected');

foreach (['u1', 'u2', 'u3'] as $approver) {
    $governance->setApprover($approver, true, 'USER', 'admin-1');
}
$governance->setApprover('disabled-user', false, 'USER', 'admin-1');
assertThrows(static fn () => $governance->recordDecision($registrationId, 'disabled-user', 'USER', 'APPROVED'), 'disabled approver blocked');
assertThrows(static fn () => $governance->recordDecision($registrationId, 'user-new', 'USER', 'APPROVED'), 'applicant cannot approve own registration');

$snapshot = $governance->recordDecision($registrationId, 'u1', 'USER', 'APPROVED');
assertSameValue(1, $snapshot['approval_summary']['user_approvals'], 'first user approval counted');
$governance->recordDecision($registrationId, 'u2', 'USER', 'APPROVED');
$snapshot = $governance->recordDecision($registrationId, 'u3', 'USER', 'APPROVED');
assertSameValue(AccessPolicy::STATUS_ACCOUNT_REQUIRED, $snapshot['registration']['status'], 'quorum reached but account required');

$snapshot = $governance->setAccountGate($registrationId, true, 'admin-1');
assertSameValue(AccessPolicy::STATUS_ACTIVE, $snapshot['registration']['status'], 'active account plus quorum activates user');

$snapshot = $governance->recordDecision($registrationId, 'u3', 'USER', 'REVOKED', 'approval withdrawn');
assertSameValue(AccessPolicy::STATUS_PENDING_APPROVAL, $snapshot['registration']['status'], 'revocation deactivates registration when quorum is lost');

$snapshot = $governance->recordDecision($registrationId, 'admin-1', 'ADMIN', 'APPROVED', 'admin override');
assertSameValue(AccessPolicy::STATUS_ACTIVE, $snapshot['registration']['status'], 'admin override restores active state');

$snapshot = $governance->recordDecision($registrationId, 'admin-1', 'ADMIN', 'REJECTED', 'admin rejection');
assertSameValue(AccessPolicy::STATUS_REJECTED, $snapshot['registration']['status'], 'admin rejection overrides approvals');

$admin = $governance->adminSnapshot();
assertSameValue(1, count($admin['invites']), 'admin snapshot includes invite');
assertSameValue(1, count($admin['registrations']), 'admin snapshot includes registration');
assertSameValue(4, count($admin['approvers']), 'admin snapshot includes approver registry');
assertTrue(!array_key_exists('token_hash', $admin['invites'][0]), 'admin snapshot does not expose token hash');

@unlink($tmp);
fwrite(STDOUT, "PASS access governance smoke\n");
