package filter

import (
	"strconv"
	"strings"

	"github.com/mustafakarli/bdsmail/internal/model"
)

// FilterResult contains the actions to take on a message after filter evaluation.
type FilterResult struct {
	Folder   string   // override destination folder (empty = no change)
	MarkRead bool
	Delete   bool
	Forward  []string // email addresses to forward to
	Flagged  bool
}

// Apply evaluates all enabled filters for a user against a message.
// rawHeaders is a map of lowercase header names to values.
func Apply(filters []*model.Filter, msg *model.Message, rawHeaders map[string]string) *FilterResult {
	result := &FilterResult{}
	for _, f := range filters {
		if !f.Enabled {
			continue
		}
		if matchesAllConditions(f.Conditions, msg, rawHeaders) {
			applyActions(f.Actions, result)
		}
	}
	return result
}

func matchesAllConditions(conditions []model.FilterCondition, msg *model.Message, headers map[string]string) bool {
	if len(conditions) == 0 {
		return false
	}
	for _, c := range conditions {
		if !matchCondition(c, msg, headers) {
			return false
		}
	}
	return true
}

func matchCondition(c model.FilterCondition, msg *model.Message, headers map[string]string) bool {
	var fieldValue string

	switch c.Field {
	case "from":
		fieldValue = strings.ToLower(msg.From)
	case "to":
		fieldValue = strings.ToLower(strings.Join(msg.To, ", "))
	case "subject":
		fieldValue = strings.ToLower(msg.Subject)
	case "header":
		fieldValue = strings.ToLower(headers[strings.ToLower(c.Header)])
	case "attachment_size_gt":
		threshold, _ := strconv.ParseInt(c.Value, 10, 64)
		return msg.TotalAttachmentSize() > threshold
	default:
		return false
	}

	target := strings.ToLower(c.Value)

	switch c.Operator {
	case "contains":
		// Support OR with | separator
		for _, part := range strings.Split(target, "|") {
			if strings.Contains(fieldValue, strings.TrimSpace(part)) {
				return true
			}
		}
		return false
	case "equals":
		return fieldValue == target
	case "exists":
		return fieldValue != ""
	case "not_contains":
		return !strings.Contains(fieldValue, target)
	default:
		return false
	}
}

func applyActions(actions []model.FilterAction, result *FilterResult) {
	for _, a := range actions {
		switch a.Type {
		case "move":
			result.Folder = a.Value
		case "mark_read":
			result.MarkRead = true
		case "delete":
			result.Delete = true
		case "forward":
			result.Forward = append(result.Forward, a.Value)
		case "flag":
			result.Flagged = true
		}
	}
}

// DefaultFilters returns the preset filters created for new users.
func DefaultFilters(userEmail string) []*model.Filter {
	return []*model.Filter{
		{
			UserEmail: userEmail,
			Name:      "Newsletters",
			Priority:  10,
			Conditions: []model.FilterCondition{
				{Field: "header", Operator: "exists", Header: "List-Unsubscribe"},
			},
			Actions: []model.FilterAction{
				{Type: "move", Value: "Newsletters"},
			},
			Enabled: true,
		},
		{
			UserEmail: userEmail,
			Name:      "Social",
			Priority:  9,
			Conditions: []model.FilterCondition{
				{Field: "from", Operator: "contains", Value: "facebook.com|twitter.com|linkedin.com|instagram.com|facebookmail.com|x.com"},
			},
			Actions: []model.FilterAction{
				{Type: "move", Value: "Social"},
			},
			Enabled: true,
		},
		{
			UserEmail: userEmail,
			Name:      "Auto-read noreply",
			Priority:  5,
			Conditions: []model.FilterCondition{
				{Field: "from", Operator: "contains", Value: "noreply|no-reply|donotreply|do-not-reply"},
			},
			Actions: []model.FilterAction{
				{Type: "mark_read"},
			},
			Enabled: true,
		},
		{
			UserEmail: userEmail,
			Name:      "Large attachments",
			Priority:  1,
			Conditions: []model.FilterCondition{
				{Field: "attachment_size_gt", Value: "5242880"},
			},
			Actions: []model.FilterAction{
				{Type: "flag"},
			},
			Enabled: true,
		},
	}
}
