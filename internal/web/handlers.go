package web

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/mustafakarli/bdsmail/config"
	"github.com/mustafakarli/bdsmail/internal/mimeutil"
	"github.com/mustafakarli/bdsmail/internal/model"
	"github.com/mustafakarli/bdsmail/internal/security"
	"github.com/mustafakarli/bdsmail/internal/smtp"
	"github.com/mustafakarli/bdsmail/internal/store"
)

type Handlers struct {
	store    *store.Store
	relay    *smtp.Relay
	sessions *SessionStore
	cfg      *config.Config
	checker  *security.Checker
}

func NewHandlers(s *store.Store, relay *smtp.Relay, sessions *SessionStore, cfg *config.Config, checker *security.Checker) *Handlers {
	return &Handlers{store: s, relay: relay, sessions: sessions, cfg: cfg, checker: checker}
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

	// Parse multipart form (32MB max in-memory)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		r.ParseForm() // fallback to regular form
	}

	to := parseAddresses(r.FormValue("to"))
	cc := parseAddresses(r.FormValue("cc"))
	bcc := parseAddresses(r.FormValue("bcc"))
	subject := r.FormValue("subject")
	contentType := r.FormValue("content_type")
	body := r.FormValue("body")

	// Parse uploaded attachments
	var attachments []mimeutil.ParsedAttachment
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		for _, fileHeaders := range r.MultipartForm.File {
			for _, fh := range fileHeaders {
				if h.cfg != nil && fh.Size > h.cfg.MaxAttachmentBytes {
					pd.Error = fmt.Sprintf("Attachment %q exceeds maximum size of %d MB", fh.Filename, h.cfg.MaxAttachmentBytes/(1024*1024))
					pd.FormTo = r.FormValue("to")
					pd.FormCC = r.FormValue("cc")
					pd.FormBCC = r.FormValue("bcc")
					pd.FormSubject = subject
					pd.FormBody = body
					pd.FormContentType = contentType
					tmpl.render(w, "layout", pd)
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
					Filename:    fh.Filename,
					ContentType: fh.Header.Get("Content-Type"),
					Data:        data,
				})
			}
		}
	}

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

	// Run security checks on outbound mail
	if h.checker != nil {
		var attData [][]byte
		for _, att := range attachments {
			attData = append(attData, att.Data)
		}
		result := h.checker.CheckOutbound(ctx, body, contentType, attData...)
		if result.Reject {
			log.Printf("blocked outbound mail from %s: %s", email, result.Reason)
			pd.Error = "Message blocked: " + result.Reason
			pd.FormTo = r.FormValue("to")
			pd.FormCC = r.FormValue("cc")
			pd.FormBCC = r.FormValue("bcc")
			pd.FormSubject = subject
			pd.FormBody = body
			pd.FormContentType = contentType
			tmpl.render(w, "layout", pd)
			return
		}
	}

	messageID, err := h.store.SaveOutgoingMail(ctx, email, fromAddr, to, cc, bcc, subject, contentType, body, attachments)
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

func (h *Handlers) HandleAttachment(w http.ResponseWriter, r *http.Request) {
	email, ok := h.requireAuth(w, r)
	if !ok {
		return
	}

	// URL: /attachment/{messageID}/{attachmentID}
	path := strings.TrimPrefix(r.URL.Path, "/attachment/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	msgID, attID := parts[0], parts[1]

	msg, err := h.store.DB.GetMessage(msgID)
	if err != nil || msg.OwnerUser != email {
		http.NotFound(w, r)
		return
	}

	// Find the attachment
	var found *model.Attachment
	for i := range msg.Attachments {
		if msg.Attachments[i].ID == attID {
			found = &msg.Attachments[i]
			break
		}
	}
	if found == nil {
		http.NotFound(w, r)
		return
	}

	ctx := context.Background()
	data, err := h.store.GetAttachmentData(ctx, found.BucketKey)
	if err != nil {
		log.Printf("error reading attachment: %v", err)
		http.Error(w, "Failed to load attachment", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", found.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", found.Filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Write(data)
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
