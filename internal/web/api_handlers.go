package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mustafakarli/bdsmail/internal/mimeutil"
	"github.com/mustafakarli/bdsmail/internal/model"
	"github.com/mustafa-karli/basis/common"
	oauthpkg "github.com/mustafakarli/bdsmail/internal/oauth"
	"github.com/mustafakarli/bdsmail/internal/store"
)

// APIHandlers serves JSON responses for the Vue SPA.
type APIHandlers struct {
	handlers *Handlers
	admin    *AdminHandlers
	oauth    *oauthpkg.Handler
}

func NewAPIHandlers(h *Handlers, admin *AdminHandlers, o *oauthpkg.Handler) *APIHandlers {
	return &APIHandlers{handlers: h, admin: admin, oauth: o}
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	common.WriteError(w, code, http.StatusText(code), msg)
}

func (a *APIHandlers) requireAuth(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := getSessionUser(r, a.handlers.sessions)
	if email == "" {
		jsonError(w, 401, "Unauthorized")
		return "", false
	}
	return email, true
}

// --- Auth ---

func (a *APIHandlers) HandleAuthMe(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAuth(w, r)
	if !ok {
		return
	}
	username, domain := store.SplitEmail(email)
	displayName := ""
	if user, err := a.handlers.store.DB.GetUserByEmail(email); err == nil {
		displayName = user.DisplayName
	}
	jsonOK(w, map[string]string{
		"username": username, "displayName": displayName,
		"email": email, "domain": domain,
	})
}

func (a *APIHandlers) HandleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, 400, "Invalid request")
		return
	}

	clientIP := extractClientIP(r)
	if a.handlers.checker != nil && a.handlers.checker.IsLockedOut(clientIP) {
		jsonError(w, 429, "Too many failed login attempts")
		return
	}

	domain := a.handlers.getDomain(r)
	user, err := a.handlers.store.DB.GetUser(body.Username, domain)
	if err != nil || !user.CheckPassword(body.Password) {
		if a.handlers.checker != nil {
			a.handlers.checker.RecordAuthResult(clientIP, false)
		}
		jsonError(w, 401, "Invalid username or password")
		return
	}

	if a.handlers.checker != nil {
		a.handlers.checker.RecordAuthResult(clientIP, true)
	}

	email := user.Email()
	if err := createSession(w, a.handlers.sessions, email); err != nil {
		jsonError(w, 500, "Failed to create session")
		return
	}

	username, dom := store.SplitEmail(email)
	jsonOK(w, map[string]string{
		"username": username, "displayName": user.DisplayName,
		"email": email, "domain": dom,
	})
}

func (a *APIHandlers) HandleAuthLogout(w http.ResponseWriter, r *http.Request) {
	clearSession(w, r, a.handlers.sessions)
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- Messages ---

type apiMessages struct {
	Messages    []*model.Message `json:"messages"`
	Page        int              `json:"page"`
	TotalPages  int              `json:"totalPages"`
	UnreadCount int              `json:"unreadCount"`
}

func (a *APIHandlers) HandleMessages(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "INBOX"
	}

	messages, err := a.handlers.store.DB.ListMessages(email, folder)
	if err != nil {
		jsonError(w, 500, "Failed to list messages")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	paged, currentPage, totalPages := paginateMessages(messages, page)
	unread := a.handlers.store.DB.CountUnread(email, "INBOX")

	jsonOK(w, apiMessages{
		Messages: paged, Page: currentPage,
		TotalPages: totalPages, UnreadCount: unread,
	})
}

func (a *APIHandlers) HandleMessage(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/messages/")
	if strings.HasSuffix(id, "/delete") {
		a.handleDeleteMessage(w, r, email, strings.TrimSuffix(id, "/delete"))
		return
	}

	ctx := context.Background()
	msg, err := a.handlers.store.GetMessageWithBody(ctx, id)
	if err != nil {
		jsonError(w, 404, "Message not found")
		return
	}
	if msg.OwnerUser != email {
		jsonError(w, 403, "Forbidden")
		return
	}

	a.handlers.store.DB.MarkSeen(id)
	jsonOK(w, msg)
}

func (a *APIHandlers) handleDeleteMessage(w http.ResponseWriter, r *http.Request, email, id string) {
	msg, err := a.handlers.store.DB.GetMessage(id)
	if err != nil || msg.OwnerUser != email {
		jsonError(w, 404, "Message not found")
		return
	}
	ctx := context.Background()
	if err := a.handlers.store.DeleteMessageFull(ctx, id); err != nil {
		jsonError(w, 500, "Failed to delete")
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- Compose ---

func (a *APIHandlers) HandleCompose(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		r.ParseForm()
	}

	to := parseAddresses(r.FormValue("to"))
	cc := parseAddresses(r.FormValue("cc"))
	bcc := parseAddresses(r.FormValue("bcc"))
	subject := r.FormValue("subject")
	contentType := r.FormValue("content_type")
	body := r.FormValue("body")

	var attachments []mimeutil.ParsedAttachment
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		for _, fileHeaders := range r.MultipartForm.File {
			for _, fh := range fileHeaders {
				if a.handlers.cfg != nil && fh.Size > a.handlers.cfg.MaxAttachmentBytes {
					jsonError(w, 400, fmt.Sprintf("Attachment %q exceeds maximum size", fh.Filename))
					return
				}
				f, err := fh.Open()
				if err != nil {
					continue
				}
				data, err := io.ReadAll(f)
				f.Close()
				if err != nil {
					continue
				}
				attachments = append(attachments, mimeutil.ParsedAttachment{
					Filename: fh.Filename, ContentType: fh.Header.Get("Content-Type"), Data: data,
				})
			}
		}
	}

	if len(to) == 0 {
		jsonError(w, 400, "At least one recipient is required")
		return
	}

	fromAddr := email
	if user, err := a.handlers.store.DB.GetUserByEmail(email); err == nil && user.DisplayName != "" {
		fromAddr = user.DisplayName + " <" + email + ">"
	}

	ctx := context.Background()

	if a.handlers.checker != nil {
		var attData [][]byte
		for _, att := range attachments {
			attData = append(attData, att.Data)
		}
		result := a.handlers.checker.CheckOutbound(ctx, body, contentType, attData...)
		if result.Reject {
			jsonError(w, 400, "Message blocked: "+result.Reason)
			return
		}
	}

	messageID, err := a.handlers.store.SaveOutgoingMail(ctx, email, fromAddr, to, cc, bcc, subject, contentType, body, attachments)
	if err != nil {
		jsonError(w, 500, "Failed to send message")
		return
	}

	allExternal := collectExternalAddrs(a.handlers.cfg, to, cc, bcc)
	if len(allExternal) > 0 && a.handlers.relay != nil {
		go func() {
			if err := a.handlers.relay.Send(email, allExternal, subject, contentType, body, messageID); err != nil {
				log.Printf("relay error: %v", err)
			}
		}()
	}

	jsonOK(w, map[string]string{"status": "ok"})
}

// --- Search ---

func (a *APIHandlers) HandleSearch(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAuth(w, r)
	if !ok {
		return
	}
	query := r.URL.Query().Get("q")
	if query == "" {
		jsonOK(w, apiMessages{Messages: []*model.Message{}})
		return
	}
	messages, err := a.handlers.store.DB.SearchMessages(email, query)
	if err != nil {
		jsonError(w, 500, "Search failed")
		return
	}
	jsonOK(w, apiMessages{Messages: messages, Page: 1, TotalPages: 1})
}

// --- Folders ---

func (a *APIHandlers) HandleFolders(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAuth(w, r)
	if !ok {
		return
	}
	folders, _ := a.handlers.store.DB.ListUserFolders(email)
	if folders == nil {
		folders = []string{}
	}
	jsonOK(w, folders)
}

// --- Unread ---

func (a *APIHandlers) HandleUnread(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAuth(w, r)
	if !ok {
		return
	}
	count := a.handlers.store.DB.CountUnread(email, "INBOX")
	jsonOK(w, map[string]int{"count": count})
}

// --- Filters ---

func (a *APIHandlers) HandleFilters(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		filters, _ := a.handlers.store.DB.ListFilters(email)
		if filters == nil {
			filters = []*model.Filter{}
		}
		jsonOK(w, filters)

	case http.MethodPost:
		var body struct {
			Name        string `json:"name"`
			Field       string `json:"field"`
			Operator    string `json:"operator"`
			Value       string `json:"value"`
			ActionType  string `json:"actionType"`
			ActionValue string `json:"actionValue"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		f := &model.Filter{
			ID: uuid.New().String(), UserEmail: email, Name: body.Name, Priority: 5,
			Conditions: []model.FilterCondition{{Field: body.Field, Operator: body.Operator, Value: body.Value}},
			Actions:    []model.FilterAction{{Type: body.ActionType, Value: body.ActionValue}},
			Enabled:    true,
		}
		if err := a.handlers.store.DB.CreateFilter(f); err != nil {
			jsonError(w, 500, "Failed to create filter")
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})

	case http.MethodDelete:
		id := strings.TrimPrefix(r.URL.Path, "/api/filters/")
		if err := a.handlers.store.DB.DeleteFilter(id); err != nil {
			jsonError(w, 500, "Failed to delete filter")
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	}
}

// --- Auto-Reply ---

func (a *APIHandlers) HandleAutoReply(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		reply, err := a.handlers.store.DB.GetAutoReply(email)
		if err != nil {
			jsonOK(w, map[string]any{"enabled": false, "subject": "", "body": "", "startDate": "", "endDate": ""})
			return
		}
		sd, ed := "", ""
		if !reply.StartDate.IsZero() {
			sd = reply.StartDate.Format("2006-01-02")
		}
		if !reply.EndDate.IsZero() {
			ed = reply.EndDate.Format("2006-01-02")
		}
		jsonOK(w, map[string]any{
			"enabled": reply.Enabled, "subject": reply.Subject, "body": reply.Body,
			"startDate": sd, "endDate": ed,
		})

	case http.MethodPost:
		var body struct {
			Enabled   bool   `json:"enabled"`
			Subject   string `json:"subject"`
			Body      string `json:"body"`
			StartDate string `json:"startDate"`
			EndDate   string `json:"endDate"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		reply := &model.AutoReply{
			UserEmail: email, Enabled: body.Enabled,
			Subject: body.Subject, Body: body.Body,
		}
		if body.StartDate != "" {
			reply.StartDate, _ = time.Parse("2006-01-02", body.StartDate)
		}
		if body.EndDate != "" {
			reply.EndDate, _ = time.Parse("2006-01-02", body.EndDate)
		}
		if err := a.handlers.store.DB.SetAutoReply(reply); err != nil {
			jsonError(w, 500, "Failed to save")
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	}
}

// --- Contacts ---

func (a *APIHandlers) HandleContacts(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		contacts, _ := a.handlers.store.DB.ListContacts(email)
		var views []contactView
		for _, c := range contacts {
			name, cemail, phone := vcardToContact(c.VCardData)
			views = append(views, contactView{ID: c.ID, Name: name, Email: cemail, Phone: phone})
		}
		if views == nil {
			views = []contactView{}
		}
		jsonOK(w, views)

	case http.MethodPost:
		var body struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Phone string `json:"phone"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		vcard := contactToVCard(body.Name, body.Email, body.Phone)
		c := &model.Contact{
			ID: uuid.New().String(), OwnerEmail: email,
			VCardData: vcard, ETag: uuid.New().String()[:8],
		}
		if err := a.handlers.store.DB.CreateContact(c); err != nil {
			jsonError(w, 500, "Failed to add contact")
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})

	case http.MethodDelete:
		id := strings.TrimPrefix(r.URL.Path, "/api/contacts/")
		if err := a.handlers.store.DB.DeleteContact(id); err != nil {
			jsonError(w, 500, "Failed to delete contact")
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	}
}

// --- Admin ---

func (a *APIHandlers) HandleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Secret string `json:"secret"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if a.handlers.cfg == nil || body.Secret != a.handlers.cfg.AdminSecret {
		jsonError(w, 401, "Invalid admin secret")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "bdsmail_admin", Value: body.Secret,
		Path: "/", HttpOnly: true, Secure: true,
	})
	jsonOK(w, map[string]string{"status": "ok"})
}

func (a *APIHandlers) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie("bdsmail_admin")
	if err != nil || a.handlers.cfg == nil || cookie.Value != a.handlers.cfg.AdminSecret {
		jsonError(w, 401, "Admin auth required")
		return false
	}
	return true
}

func (a *APIHandlers) HandleAdminDomains(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		jsonOK(w, a.handlers.cfg.Domains)
	case http.MethodPost:
		a.admin.HandleAdminAPI(w, r)
	}
}

func (a *APIHandlers) HandleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		users, _ := a.handlers.store.DB.ListUsers()
		type uv struct {
			Email       string `json:"email"`
			DisplayName string `json:"displayName"`
			Domain      string `json:"domain"`
		}
		var result []uv
		for _, u := range users {
			result = append(result, uv{Email: u.Email(), DisplayName: u.DisplayName, Domain: u.Domain})
		}
		if result == nil {
			result = []uv{}
		}
		jsonOK(w, result)
	case http.MethodPost:
		var body struct {
			Email       string `json:"email"`
			DisplayName string `json:"displayName"`
			Password    string `json:"password"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		username, domain := store.SplitEmail(body.Email)
		hash, _ := model.HashPassword(body.Password)
		if err := a.handlers.store.DB.CreateUser(username, domain, body.DisplayName, hash); err != nil {
			jsonError(w, 500, "Failed to create user: "+err.Error())
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	case http.MethodDelete:
		var body struct {
			Email string `json:"email"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		a.handlers.store.DB.DeleteUserMessages(body.Email)
		if err := a.handlers.store.DB.DeleteUser(body.Email); err != nil {
			jsonError(w, 500, "Failed to delete user")
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	}
}

func (a *APIHandlers) HandleAdminAliases(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		aliases, _ := a.handlers.store.DB.ListAliases()
		if aliases == nil {
			aliases = []*model.Alias{}
		}
		jsonOK(w, aliases)
	case http.MethodPost:
		var body struct {
			AliasEmail   string `json:"aliasEmail"`
			TargetEmails string `json:"targetEmails"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		targets := parseAddresses(body.TargetEmails)
		if err := a.handlers.store.DB.CreateAlias(body.AliasEmail, targets); err != nil {
			jsonError(w, 500, "Failed to create alias: "+err.Error())
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	case http.MethodDelete:
		var body struct {
			AliasEmail string `json:"aliasEmail"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if err := a.handlers.store.DB.DeleteAlias(body.AliasEmail); err != nil {
			jsonError(w, 500, "Failed to delete alias")
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	}
}

func (a *APIHandlers) HandleAdminLists(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		lists, _ := a.handlers.store.DB.ListMailingLists()
		if lists == nil {
			lists = []*model.MailingList{}
		}
		jsonOK(w, lists)
	case http.MethodPost:
		var body struct {
			ListAddress string `json:"listAddress"`
			Name        string `json:"name"`
			OwnerEmail  string `json:"ownerEmail"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if err := a.handlers.store.DB.CreateMailingList(body.ListAddress, body.Name, "", body.OwnerEmail); err != nil {
			jsonError(w, 500, "Failed to create list: "+err.Error())
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	case http.MethodDelete:
		var body struct {
			ListAddress string `json:"listAddress"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if err := a.handlers.store.DB.DeleteMailingList(body.ListAddress); err != nil {
			jsonError(w, 500, "Failed to delete list")
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	}
}

func (a *APIHandlers) HandleAdminListMembers(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var body struct {
		ListAddress string `json:"listAddress"`
		MemberEmail string `json:"memberEmail"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if err := a.handlers.store.DB.AddListMember(body.ListAddress, body.MemberEmail); err != nil {
		jsonError(w, 500, "Failed to add member: "+err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- OAuth Client API ---

func (a *APIHandlers) HandleOAuthClients(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		clients, err := a.oauth.ListClients(email)
		if err != nil {
			jsonError(w, 500, "Failed to list clients")
			return
		}
		jsonOK(w, clients)

	case http.MethodPost:
		var body struct {
			Name        string `json:"name"`
			RedirectUri string `json:"redirectUri"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		_, domain := store.SplitEmail(email)
		client, err := a.oauth.RegisterClient(body.Name, body.RedirectUri, domain, email)
		if err != nil {
			jsonError(w, 500, "Failed to register: "+err.Error())
			return
		}
		jsonOK(w, client)

	case http.MethodDelete:
		id := strings.TrimPrefix(r.URL.Path, "/api/oauth/clients/")
		if err := a.oauth.DeleteClient(id); err != nil {
			jsonError(w, 500, "Failed to delete")
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	}
}

