package model

// Alias represents an email alias that forwards to one or more target addresses.
type Alias struct {
	AliasEmail   string
	TargetEmails []string
	IsCatchAll   bool // true for catch-all aliases (@domain.com)
}
