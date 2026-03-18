package web

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/mustafakarli/bdsmail/config"
	"github.com/mustafakarli/bdsmail/internal/smtp"
	"github.com/mustafakarli/bdsmail/internal/store"
)

type Handlers struct {
	store    *store.Store
	relay    *smtp.Relay
	sessions *SessionStore
	cfg      *config.Config
}

func NewHandlers(s *store.Store, relay *smtp.Relay, sessions *SessionStore, cfg *config.Config) *Handlers {
	return &Handlers{store: s, relay: relay, sessions: sessions, cfg: cfg}
}

type pageData struct {
	Username        string      // local part (e.g. "alice")
	DisplayName     string      // display name (e.g. "Alice Smith")
	Email           string      // full email (e.g. "alice@domain1.com")
	Domain          string      // current domain
	Error           string
	Success         string
	Folder          string
	Messages        interface{}
	Message         interface{}
	FormTo          string
	FormCC          string
	FormBCC         string
	FormSubject     string
	FormBody        string
	FormContentType string
}

func (h *Handlers) getDomain(r *http.Request) string {
	return h.cfg.HostToDomain(r.Host)
}

func (h *Handlers) requireAuth(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := getSessionUser(r, h.sessions)
	if email == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return "", false
	}
	return email, true
}

// userPageData loads the user from DB and returns a pre-filled pageData.
func (h *Handlers) userPageData(email string) pageData {
	username, domain := store.SplitEmail(email)
	displayName := ""
	if user, err := h.store.DB.GetUserByEmail(email); err == nil {
		displayName = user.DisplayName
	}
	return pageData{
		Username:    username,
		DisplayName: displayName,
		Email:       email,
		Domain:      domain,
	}
}

func (h *Handlers) HandleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	email := getSessionUser(r, h.sessions)
	if email != "" {
		http.Redirect(w, r, "/inbox", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func (h *Handlers) HandleLogin(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	domain := h.getDomain(r)

	if r.Method == http.MethodGet {
		tmpl.render(w, "layout", pageData{Domain: domain})
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	user, err := h.store.DB.GetUser(username, domain)
	if err != nil || !user.CheckPassword(password) {
		tmpl.render(w, "layout", pageData{
			Domain: domain,
			Error:  "Invalid username or password",
		})
		return
	}

	email := user.Email()
	if err := createSession(w, h.sessions, email); err != nil {
		tmpl.render(w, "layout", pageData{
			Domain: domain,
			Error:  "Failed to create session",
		})
		return
	}

	http.Redirect(w, r, "/inbox", http.StatusSeeOther)
}

func (h *Handlers) HandleLogout(w http.ResponseWriter, r *http.Request) {
	clearSession(w, r, h.sessions)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handlers) HandleInbox(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	email, ok := h.requireAuth(w, r)
	if !ok {
		return
	}

	pd := h.userPageData(email)

	messages, err := h.store.DB.ListMessages(email, "INBOX")
	if err != nil {
		log.Printf("error listing messages: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	pd.Folder = "INBOX"
	pd.Messages = messages
	tmpl.render(w, "layout", pd)
}

func (h *Handlers) HandleSent(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	email, ok := h.requireAuth(w, r)
	if !ok {
		return
	}

	pd := h.userPageData(email)

	messages, err := h.store.DB.ListMessages(email, "Sent")
	if err != nil {
		log.Printf("error listing sent messages: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	pd.Folder = "Sent"
	pd.Messages = messages
	tmpl.render(w, "layout", pd)
}

func (h *Handlers) HandleCompose(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	email, ok := h.requireAuth(w, r)
	if !ok {
		return
	}

	pd := h.userPageData(email)

	if r.Method == http.MethodGet {
		pd.FormContentType = "text/plain"
		tmpl.render(w, "layout", pd)
		return
	}

	to := parseAddresses(r.FormValue("to"))
	cc := parseAddresses(r.FormValue("cc"))
	bcc := parseAddresses(r.FormValue("bcc"))
	subject := r.FormValue("subject")
	contentType := r.FormValue("content_type")
	body := r.FormValue("body")

	if len(to) == 0 {
		pd.Error = "At least one recipient is required"
		pd.FormTo = r.FormValue("to")
		pd.FormCC = r.FormValue("cc")
		pd.FormBCC = r.FormValue("bcc")
		pd.FormSubject = subject
		pd.FormBody = body
		pd.FormContentType = contentType
		tmpl.render(w, "layout", pd)
		return
	}

	// Use display name in From header if available
	fromAddr := email
	if pd.DisplayName != "" {
		fromAddr = pd.DisplayName + " <" + email + ">"
	}

	ctx := context.Background()

	messageID, err := h.store.SaveOutgoingMail(ctx, email, fromAddr, to, cc, bcc, subject, contentType, body)
	if err != nil {
		log.Printf("error saving outgoing mail: %v", err)
		pd.Error = "Failed to send message"
		pd.FormTo = r.FormValue("to")
		pd.FormCC = r.FormValue("cc")
		pd.FormBCC = r.FormValue("bcc")
		pd.FormSubject = subject
		pd.FormBody = body
		pd.FormContentType = contentType
		tmpl.render(w, "layout", pd)
		return
	}

	// Relay to external recipients in background
	allExternal := collectExternalAddrs(h.cfg, to, cc, bcc)
	if len(allExternal) > 0 && h.relay != nil {
		go func() {
			err := h.relay.Send(email, allExternal, subject, contentType, body, messageID)
			if err != nil {
				log.Printf("relay error: %v", err)
			}
		}()
	}

	pd2 := h.userPageData(email)
	pd2.Success = "Message sent successfully"
	pd2.FormContentType = "text/plain"
	tmpl.render(w, "layout", pd2)
}

func (h *Handlers) HandleReadMessage(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	email, ok := h.requireAuth(w, r)
	if !ok {
		return
	}

	pd := h.userPageData(email)

	id := extractMessageID(r.URL.Path, "/message/")
	if id == "" {
		http.NotFound(w, r)
		return
	}

	ctx := context.Background()
	msg, err := h.store.GetMessageWithBody(ctx, id)
	if err != nil {
		log.Printf("error reading message: %v", err)
		http.NotFound(w, r)
		return
	}

	if msg.OwnerUser != email {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	h.store.DB.MarkSeen(id)

	pd.Message = msg
	tmpl.render(w, "layout", pd)
}

func (h *Handlers) HandleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	email, ok := h.requireAuth(w, r)
	if !ok {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/message/")
	path = strings.TrimSuffix(path, "/delete")
	id := path

	if id == "" {
		http.NotFound(w, r)
		return
	}

	msg, err := h.store.DB.GetMessage(id)
	if err != nil || msg.OwnerUser != email {
		http.NotFound(w, r)
		return
	}

	ctx := context.Background()
	if err := h.store.DeleteMessageFull(ctx, id); err != nil {
		log.Printf("error deleting message: %v", err)
	}

	http.Redirect(w, r, "/inbox", http.StatusSeeOther)
}

// helpers

func parseAddresses(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func extractMessageID(path, prefix string) string {
	s := strings.TrimPrefix(path, prefix)
	if idx := strings.Index(s, "/"); idx != -1 {
		s = s[:idx]
	}
	return s
}

func collectExternalAddrs(cfg *config.Config, to, cc, bcc []string) []string {
	var result []string
	all := make([]string, 0, len(to)+len(cc)+len(bcc))
	all = append(all, to...)
	all = append(all, cc...)
	all = append(all, bcc...)
	for _, addr := range all {
		_, domain := store.SplitEmail(addr)
		if !cfg.IsDomainServed(domain) {
			result = append(result, addr)
		}
	}
	return result
}

type templateRenderer interface {
	render(w http.ResponseWriter, name string, data pageData)
}
