package model

import "time"

// UserPermission represents a time-bounded role assignment for a user.
type UserPermission struct {
	ID        string
	UserEmail string
	Role      string // "owner", "admin"
	Domain    string
	StartDate time.Time
	EndDate   time.Time
	CreatedBy string
	CreatedAt time.Time
}

// DefaultEndDate is used when no end date is provided (effectively permanent).
var DefaultEndDate = time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)

func (p *UserPermission) IsActive() bool {
	now := time.Now()
	return now.After(p.StartDate) && now.Before(p.EndDate)
}
