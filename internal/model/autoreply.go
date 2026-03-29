package model

import "time"

// AutoReply represents a user's vacation/auto-reply settings.
type AutoReply struct {
	UserEmail string
	Enabled   bool
	Subject   string
	Body      string
	StartDate time.Time
	EndDate   time.Time
}
