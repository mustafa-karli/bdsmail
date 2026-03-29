package model

// FilterCondition defines a single condition for matching emails.
type FilterCondition struct {
	Field    string `json:"field"`              // "from", "to", "subject", "header", "attachment_size_gt"
	Operator string `json:"operator"`           // "contains", "equals", "matches", "exists"
	Value    string `json:"value"`
	Header   string `json:"header,omitempty"`   // header name for "header" field type
}

// FilterAction defines what to do when a filter matches.
type FilterAction struct {
	Type  string `json:"type"`  // "move", "mark_read", "delete", "forward", "flag"
	Value string `json:"value"` // folder name, email address, flag name
}

// Filter represents a server-side email filtering rule.
type Filter struct {
	ID         string
	UserEmail  string
	Name       string
	Priority   int
	Conditions []FilterCondition
	Actions    []FilterAction
	Enabled    bool
}
