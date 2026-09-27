package access

import (
	"testing"
	"time"
)

func TestEvaluateStatusRequiresThreeUserApprovals(t *testing.T) {
	reg := Registration{ID: "r1", ApplicantUserID: "u-new", HasActiveAccount: false}
	policy := DefaultApprovalPolicy()
	now := time.Now()
	events := []ApprovalEvent{
		{ApproverUserID: "u1", ApproverRole: RoleUser, Decision: DecisionApproved, CreatedAt: now},
		{ApproverUserID: "u2", ApproverRole: RoleUser, Decision: DecisionApproved, CreatedAt: now.Add(time.Second)},
	}
	if got := EvaluateStatus(reg, policy, events); got != StatusPendingApproval {
		t.Fatalf("got %s want %s", got, StatusPendingApproval)
	}
	events = append(events, ApprovalEvent{ApproverUserID: "u3", ApproverRole: RoleUser, Decision: DecisionApproved, CreatedAt: now.Add(2 * time.Second)})
	if got := EvaluateStatus(reg, policy, events); got != StatusAccountRequired {
		t.Fatalf("got %s want %s", got, StatusAccountRequired)
	}
}

func TestEvaluateStatusAdminOverride(t *testing.T) {
	reg := Registration{ID: "r1", ApplicantUserID: "u-new", HasActiveAccount: true}
	policy := DefaultApprovalPolicy()
	events := []ApprovalEvent{{ApproverUserID: "admin1", ApproverRole: RoleAdmin, Decision: DecisionApproved, CreatedAt: time.Now()}}
	if got := EvaluateStatus(reg, policy, events); got != StatusActive {
		t.Fatalf("got %s want %s", got, StatusActive)
	}
}

func TestRevocationRemovesEffectiveApproval(t *testing.T) {
	reg := Registration{ID: "r1", ApplicantUserID: "u-new", HasActiveAccount: false}
	policy := DefaultApprovalPolicy()
	now := time.Now()
	events := []ApprovalEvent{
		{ApproverUserID: "u1", ApproverRole: RoleUser, Decision: DecisionApproved, CreatedAt: now},
		{ApproverUserID: "u2", ApproverRole: RoleUser, Decision: DecisionApproved, CreatedAt: now},
		{ApproverUserID: "u3", ApproverRole: RoleUser, Decision: DecisionApproved, CreatedAt: now},
		{ApproverUserID: "u3", ApproverRole: RoleUser, Decision: DecisionRevoked, CreatedAt: now.Add(time.Second)},
	}
	if got := EvaluateStatus(reg, policy, events); got != StatusPendingApproval {
		t.Fatalf("got %s want %s", got, StatusPendingApproval)
	}
}

func TestValidateApprovalRejectsSelfApproval(t *testing.T) {
	reg := Registration{ApplicantUserID: "u-new"}
	event := ApprovalEvent{ApproverUserID: "u-new", Decision: DecisionApproved}
	if err := ValidateApprovalEvent(reg, event); err != ErrSelfApproval {
		t.Fatalf("got %v want %v", err, ErrSelfApproval)
	}
}
