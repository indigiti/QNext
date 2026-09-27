package access

import "time"

type RegistrationStatus string

const (
	StatusInvited          RegistrationStatus = "INVITED"
	StatusRegistered       RegistrationStatus = "REGISTERED"
	StatusPendingApproval  RegistrationStatus = "PENDING_APPROVAL"
	StatusApproved         RegistrationStatus = "APPROVED"
	StatusAccountRequired  RegistrationStatus = "ACCOUNT_REQUIRED"
	StatusActive           RegistrationStatus = "ACTIVE"
	StatusSuspended        RegistrationStatus = "SUSPENDED"
	StatusRejected         RegistrationStatus = "REJECTED"
	StatusInviteExpired    RegistrationStatus = "INVITE_EXPIRED"
)

type Decision string

const (
	DecisionApproved Decision = "APPROVED"
	DecisionRejected Decision = "REJECTED"
	DecisionRevoked  Decision = "REVOKED"
)

type ApproverRole string

const (
	RoleUser  ApproverRole = "USER"
	RoleAdmin ApproverRole = "ADMIN"
)

type ApprovalEvent struct {
	ID               string       `json:"id"`
	RegistrationID   string       `json:"registration_id"`
	ApplicantUserID  string       `json:"applicant_user_id"`
	ApproverUserID   string       `json:"approver_user_id"`
	ApproverRole     ApproverRole `json:"approver_role"`
	Decision         Decision     `json:"decision"`
	Comment          string       `json:"comment,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
}

type Registration struct {
	ID                string             `json:"id"`
	ApplicantUserID   string             `json:"applicant_user_id"`
	InvitedByUserID   string             `json:"invited_by_user_id"`
	Status            RegistrationStatus `json:"status"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
	HasActiveAccount  bool               `json:"has_active_account"`
}

type ApprovalPolicy struct {
	UserQuorum          int  `json:"user_quorum"`
	AdminOverride       bool `json:"admin_override"`
	RequireActiveAccount bool `json:"require_active_account"`
}

func DefaultApprovalPolicy() ApprovalPolicy {
	return ApprovalPolicy{UserQuorum: 3, AdminOverride: true, RequireActiveAccount: true}
}
