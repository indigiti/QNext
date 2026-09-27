package access

import "errors"

var ErrSelfApproval = errors.New("applicant cannot approve own registration")

// EvaluateStatus derives the registration state from immutable approval events
// and the current broker/data account gate.
func EvaluateStatus(reg Registration, policy ApprovalPolicy, events []ApprovalEvent) RegistrationStatus {
	latest := latestDecisions(events)

	adminApproved := false
	userApprovals := 0
	adminRejected := false

	for _, event := range latest {
		if event.Decision == DecisionRejected && event.ApproverRole == RoleAdmin {
			adminRejected = true
		}
		if event.Decision != DecisionApproved {
			continue
		}
		if event.ApproverRole == RoleAdmin {
			adminApproved = true
			continue
		}
		if event.ApproverRole == RoleUser {
			userApprovals++
		}
	}

	if adminRejected {
		return StatusRejected
	}

	approved := (policy.AdminOverride && adminApproved) || userApprovals >= policy.UserQuorum
	if !approved {
		return StatusPendingApproval
	}

	if policy.RequireActiveAccount && !reg.HasActiveAccount {
		return StatusAccountRequired
	}

	return StatusActive
}

func ValidateApprovalEvent(reg Registration, event ApprovalEvent) error {
	if reg.ApplicantUserID != "" && reg.ApplicantUserID == event.ApproverUserID {
		return ErrSelfApproval
	}
	return nil
}

func latestDecisions(events []ApprovalEvent) map[string]ApprovalEvent {
	latest := make(map[string]ApprovalEvent)
	for _, event := range events {
		current, ok := latest[event.ApproverUserID]
		if !ok || event.CreatedAt.After(current.CreatedAt) {
			latest[event.ApproverUserID] = event
		}
	}
	return latest
}
