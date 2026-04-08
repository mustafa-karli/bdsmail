package model

import "time"

// UserHistory tracks user account events for audit trail.
type UserHistory struct {
	UserEmail   string
	ActionTime  time.Time
	ActionType  string // LOGIN, FAILED_LOGIN, LOCK, UNLOCK, SUSPEND, ACTIVATE, PASSWORD_CHANGE, CREATED, ROLE_CHANGE
	PerformedBy string // admin email or "system"
	ClientIP    string
	Detail      string
}

// Action type constants
const (
	ActionLogin          = "LOGIN"
	ActionFailedLogin    = "FAILED_LOGIN"
	ActionLock           = "LOCK"
	ActionUnlock         = "UNLOCK"
	ActionSuspend        = "SUSPEND"
	ActionActivate       = "ACTIVATE"
	ActionPasswordChange = "PASSWORD_CHANGE"
	ActionCreated        = "CREATED"
	ActionRoleChange     = "ROLE_CHANGE"
)

// Status constants
const (
	StatusActive    = "A"
	StatusLocked    = "L"
	StatusSuspended = "S"
)
