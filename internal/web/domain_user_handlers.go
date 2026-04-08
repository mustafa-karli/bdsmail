package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mustafakarli/bdsmail/internal/cryptoutil"
	"github.com/mustafakarli/bdsmail/internal/model"
	"github.com/mustafakarli/bdsmail/internal/store"
)

// logHistory records a user event for audit trail.
func (h *Handlers) logHistory(userEmail, actionType, performedBy, clientIP, detail string) {
	h.store.DB.AddHistory(&model.UserHistory{
		UserEmail:   userEmail,
		ActionType:  actionType,
		PerformedBy: performedBy,
		ClientIP:    clientIP,
		Detail:      detail,
	})
}

// isOwner checks if user has active "owner" permission.
func (h *Handlers) isOwner(email string) bool {
	ok, _ := h.store.DB.HasPermission(email, "owner")
	return ok
}

// isAdmin checks if user has active "owner" or "admin" permission.
func (h *Handlers) isAdmin(email string) bool {
	if h.isOwner(email) {
		return true
	}
	ok, _ := h.store.DB.HasPermission(email, "admin")
	return ok
}

// grantPermission creates a permission record with default end date.
func (h *Handlers) grantPermission(userEmail, role, domain, createdBy string, endDate time.Time) error {
	id, _ := cryptoutil.RandomHex(16)
	if endDate.IsZero() {
		endDate = model.DefaultEndDate
	}
	return h.store.DB.GrantPermission(&model.UserPermission{
		ID:        id,
		UserEmail: userEmail,
		Role:      role,
		Domain:    domain,
		StartDate: time.Now(),
		EndDate:   endDate,
		CreatedBy: createdBy,
	})
}

// HandleDomainUsers manages users within the logged-in user's domain.
func (h *Handlers) HandleDomainUsers(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	email, ok := h.requireAuth(w, r)
	if !ok {
		return
	}
	if !h.isAdmin(email) {
		http.Error(w, "Forbidden — owner or admin role required", http.StatusForbidden)
		return
	}

	pd := h.userPageData(email)
	_, domain := store.SplitEmail(email)

	if h.isOwner(email) {
		pd.OAuthNonce = "owner"
	}

	if r.Method == http.MethodPost {
		action := r.FormValue("action")
		switch action {
		case "create":
			username := strings.TrimSpace(strings.ToLower(r.FormValue("username")))
			displayName := strings.TrimSpace(r.FormValue("display_name"))
			password := r.FormValue("password")
			role := r.FormValue("role")

			if username == "" || len(password) < 8 {
				pd.Error = "Username required, password must be 8+ characters"
			} else if role == "owner" && !h.isOwner(email) {
				pd.Error = "Only owners can assign owner role"
			} else {
				hash, _ := model.HashPassword(password)
				newEmail := username + "@" + domain
				if h.store.DB.UserExistsByEmail(newEmail) {
					pd.Error = "User " + newEmail + " already exists"
				} else if err := h.store.DB.CreateUser(username, domain, displayName, hash); err != nil {
					pd.Error = "Failed to create user: " + err.Error()
				} else {
					if role == "owner" || role == "admin" {
						h.grantPermission(newEmail, role, domain, email, time.Time{})
						h.logHistory(newEmail, model.ActionRoleChange, email, "", "role="+role)
					}
					h.logHistory(newEmail, model.ActionCreated, email, "", "")
					pd.Success = "User " + newEmail + " created"
				}
			}

		case "delete":
			targetEmail := r.FormValue("email")
			if h.isOwner(targetEmail) && !h.isOwner(email) {
				pd.Error = "Only owners can delete other owners"
			} else if targetEmail == email {
				pd.Error = "Cannot delete yourself"
			} else {
				h.store.DB.DeleteUserMessages(targetEmail)
				h.store.DB.DeleteUser(targetEmail)
				pd.Success = "User " + targetEmail + " deleted"
			}

		case "change_role":
			targetEmail := r.FormValue("email")
			newRole := r.FormValue("role")
			if newRole == "owner" && !h.isOwner(email) {
				pd.Error = "Only owners can assign owner role"
			} else if newRole == "user" {
				perms, _ := h.store.DB.GetUserPermissions(targetEmail)
				for _, p := range perms {
					h.store.DB.RevokePermission(p.ID)
				}
				h.logHistory(targetEmail, model.ActionRoleChange, email, "", "role=user")
				pd.Success = "Role changed to user"
			} else {
				h.grantPermission(targetEmail, newRole, domain, email, time.Time{})
				h.logHistory(targetEmail, model.ActionRoleChange, email, "", "role="+newRole)
				pd.Success = "Role updated"
			}

		case "suspend":
			targetEmail := r.FormValue("email")
			if targetEmail == email {
				pd.Error = "Cannot suspend yourself"
			} else {
				h.store.DB.UpdateUserStatus(targetEmail, model.StatusSuspended)
				h.logHistory(targetEmail, model.ActionSuspend, email, "", "")
				pd.Success = "User " + targetEmail + " suspended"
			}

		case "activate":
			targetEmail := r.FormValue("email")
			h.store.DB.UpdateUserStatus(targetEmail, model.StatusActive)
			h.logHistory(targetEmail, model.ActionActivate, email, "", "")
			pd.Success = "User " + targetEmail + " activated"

		case "edit":
			targetEmail := r.FormValue("email")
			targetUser, err := h.store.DB.GetUserByEmail(targetEmail)
			if err == nil {
				pd.OAuthClientID = targetEmail
				pd.OAuthClientName = targetUser.DisplayName
			}

		case "save_edit":
			targetEmail := r.FormValue("email")
			displayName := strings.TrimSpace(r.FormValue("display_name"))
			password := r.FormValue("password")
			if password != "" && len(password) < 8 {
				pd.Error = "Password must be at least 8 characters"
			} else {
				passHash := ""
				if password != "" {
					passHash, _ = model.HashPassword(password)
				}
				if displayName != "" || passHash != "" {
					h.store.DB.UpdateUser(targetEmail, displayName, passHash)
					if passHash != "" {
						h.logHistory(targetEmail, model.ActionPasswordChange, email, "", "changed by admin")
					}
					pd.Success = "User " + targetEmail + " updated"
				}
			}
		}
	}

	users, _ := h.store.DB.ListUsersByDomain(domain)
	type userView struct {
		Email       string
		DisplayName string
		Role        string
		Status      string
	}
	var views []userView
	for _, u := range users {
		role := "user"
		if h.isOwner(u.Email()) {
			role = "owner"
		} else if h.isAdmin(u.Email()) {
			role = "admin"
		}
		views = append(views, userView{
			Email:       u.Email(),
			DisplayName: u.DisplayName,
			Role:        role,
			Status:      u.Status,
		})
	}
	pd.AdminData = views
	pd.DNSRecords, _ = h.store.DB.ListDNSRecords(domain)
	tmpl.render(w, "layout", pd)
}

// --- API ---

func (h *Handlers) APIDomainUsers(w http.ResponseWriter, r *http.Request) {
	email := getSessionUser(r, h.sessions)
	if email == "" {
		jsonError(w, 401, "Unauthorized")
		return
	}
	if !h.isAdmin(email) {
		jsonError(w, 403, "Owner or admin role required")
		return
	}

	_, domain := store.SplitEmail(email)

	switch r.Method {
	case http.MethodGet:
		users, _ := h.store.DB.ListUsersByDomain(domain)
		type uv struct {
			Email       string `json:"email"`
			DisplayName string `json:"displayName"`
			Role        string `json:"role"`
			Status      string `json:"status"`
		}
		var result []uv
		for _, u := range users {
			role := "user"
			if h.isOwner(u.Email()) {
				role = "owner"
			} else if h.isAdmin(u.Email()) {
				role = "admin"
			}
			result = append(result, uv{u.Email(), u.DisplayName, role, u.Status})
		}
		if result == nil {
			result = []uv{}
		}
		jsonOK(w, result)

	case http.MethodPost:
		var body struct {
			Username    string `json:"username"`
			DisplayName string `json:"displayName"`
			Password    string `json:"password"`
			Role        string `json:"role"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		if body.Role == "owner" && !h.isOwner(email) {
			jsonError(w, 403, "Only owners can assign owner role")
			return
		}

		hash, _ := model.HashPassword(body.Password)
		newEmail := body.Username + "@" + domain
		if err := h.store.DB.CreateUser(body.Username, domain, body.DisplayName, hash); err != nil {
			jsonError(w, 400, "Failed to create user: "+err.Error())
			return
		}
		if body.Role == "owner" || body.Role == "admin" {
			h.grantPermission(newEmail, body.Role, domain, email, time.Time{})
		}
		jsonOK(w, map[string]string{"status": "ok", "email": newEmail})

	case http.MethodDelete:
		var body struct {
			Email string `json:"email"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		if h.isOwner(body.Email) && !h.isOwner(email) {
			jsonError(w, 403, "Only owners can delete owners")
			return
		}
		h.store.DB.DeleteUserMessages(body.Email)
		h.store.DB.DeleteUser(body.Email)
		jsonOK(w, map[string]string{"status": "ok"})
	}
}
