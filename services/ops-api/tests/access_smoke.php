<?php

declare(strict_types=1);

require_once dirname(__DIR__) . '/bootstrap.php';

use QNext\Ops\AccessPolicy;
use QNext\Ops\InviteApprovalStore;

function assertSameValue(mixed $expected, mixed $actual, string $label): void
{
    if ($expected !== $actual) {
        fwrite(STDERR, sprintf("FAIL %s: expected %s got %s\n", $label, var_export($expected, true), var_export($actual, true)));
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

$adminEvents = [['approver_user_id' => 'a1', 'approver_role' => 'ADMIN', 'decision' => 'APPROVED', 'created_at' => '2026-09-27T01:03:00Z']];
assertSameValue(AccessPolicy::STATUS_ACTIVE, $policy->evaluate($registration, $adminEvents), 'admin override activates');

$events[] = ['approver_user_id' => 'u3', 'approver_role' => 'USER', 'decision' => 'REVOKED', 'created_at' => '2026-09-27T01:04:00Z'];
assertSameValue(AccessPolicy::STATUS_PENDING_APPROVAL, $policy->evaluate($registration, $events), 'revocation removes quorum');

assertThrows(static function () use ($policy, $registration): void {
    $policy->validateEvent($registration, ['approver_user_id' => 'u-new', 'approver_role' => 'USER', 'decision' => 'APPROVED']);
}, 'self approval');

$tmp = sys_get_temp_dir() . '/qnext-access-' . bin2hex(random_bytes(6)) . '.json';
$store = new InviteApprovalStore($tmp);
$store->putRegistration(['id' => 'r-store', 'applicant_user_id' => 'u-store', 'has_active_account' => false]);
$store->appendApprovalEvent([
    'id' => 'e1',
    'registration_id' => 'r-store',
    'applicant_user_id' => 'u-store',
    'approver_user_id' => 'u1',
    'approver_role' => 'USER',
    'decision' => 'APPROVED',
    'created_at' => '2026-09-27T01:00:00Z',
]);
assertSameValue(1, count($store->approvalEventsFor('r-store')), 'event persisted');
@unlink($tmp);

fwrite(STDOUT, "PASS access governance smoke\n");
