package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mustafakarli/bdsmail/internal/admin"
	"github.com/mustafakarli/bdsmail/internal/cryptoutil"
	dnsverify "github.com/mustafakarli/bdsmail/internal/dns"
	"github.com/mustafakarli/bdsmail/internal/model"
	"github.com/mustafakarli/bdsmail/internal/store"
)

// SignupHandlers manages public domain signup flow.
type SignupHandlers struct {
	handlers *Handlers
}

func NewSignupHandlers(h *Handlers) *SignupHandlers {
	return &SignupHandlers{handlers: h}
}

// --- Go Template Handlers ---

// HandleSignup renders the signup form (GET) or creates a pending signup (POST).
func (s *SignupHandlers) HandleSignup(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	if r.Method == http.MethodGet {
		tmpl.render(w, "layout", pageData{Domain: s.handlers.cfg.MailHostname})
		return
	}

	domain := strings.TrimSpace(strings.ToLower(r.FormValue("domain")))
	username := strings.TrimSpace(r.FormValue("username"))
	displayName := strings.TrimSpace(r.FormValue("display_name"))
	password := r.FormValue("password")

	signupID, err := s.createSignup(domain, username, displayName, password)
	if err != nil {
		tmpl.render(w, "layout", pageData{Domain: s.handlers.cfg.MailHostname, Error: err.Error()})
		return
	}

	http.Redirect(w, r, "/signup/verify?id="+signupID, http.StatusSeeOther)
}

// HandleSignupVerify shows DNS records and verifies them.
func (s *SignupHandlers) HandleSignupVerify(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	signupID := r.URL.Query().Get("id")
	if signupID == "" {
		http.Redirect(w, r, "/signup", http.StatusSeeOther)
		return
	}

	signup, err := s.handlers.store.DB.GetSignup(signupID)
	if err != nil || signup.IsExpired() {
		tmpl.render(w, "layout", pageData{Error: "Signup expired or not found. Please start again."})
		return
	}

	pd := pageData{
		Domain:    signup.Domain,
		OAuthState: signupID, // reuse field for signup ID
	}

	// Build DNS records to show the user
	mailHostname := s.handlers.cfg.MailHostname
	pd.AdminData = s.buildSignupDNSRecords(signup.Domain, mailHostname)

	if r.Method == http.MethodPost {
		// Verify DNS
		err := dnsverify.VerifyDomainOwnership(signup.Domain, mailHostname)
		if err != nil {
			pd.Error = fmt.Sprintf("DNS verification failed: %s. Please add the MX record and wait for propagation.", err)
			tmpl.render(w, "layout", pd)
			return
		}

		// DNS verified — provision everything
		result, err := s.provisionDomain(signup)
		if err != nil {
			pd.Error = "Provisioning failed: " + err.Error()
			tmpl.render(w, "layout", pd)
			return
		}

		// Auto-login
		email := signup.Username + "@" + signup.Domain
		createSession(w, s.handlers.sessions, email)

		// Show completion page with remaining DNS records (DKIM, SES)
		pd.Success = fmt.Sprintf("Domain %s is ready! You are logged in as %s.", signup.Domain, email)
		pd.AdminData = result.DNSRecords
		pd.Error = ""
		tmpl.render(w, "layout", pd)
		return
	}

	tmpl.render(w, "layout", pd)
}

// --- JSON API Handlers ---

func (s *SignupHandlers) APICreateSignup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Domain      string `json:"domain"`
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, 400, "Invalid request")
		return
	}

	signupID, err := s.createSignup(body.Domain, body.Username, body.DisplayName, body.Password)
	if err != nil {
		jsonError(w, 400, err.Error())
		return
	}

	mailHostname := s.handlers.cfg.MailHostname
	records := s.buildSignupDNSRecords(body.Domain, mailHostname)

	jsonOK(w, map[string]any{
		"signupId":   signupID,
		"domain":     body.Domain,
		"dnsRecords": records,
	})
}

func (s *SignupHandlers) APIVerifySignup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SignupID string `json:"signupId"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	signup, err := s.handlers.store.DB.GetSignup(body.SignupID)
	if err != nil || signup.IsExpired() {
		jsonError(w, 400, "Signup expired or not found")
		return
	}

	mailHostname := s.handlers.cfg.MailHostname
	if err := dnsverify.VerifyDomainOwnership(signup.Domain, mailHostname); err != nil {
		jsonError(w, 400, fmt.Sprintf("DNS verification failed: %s", err))
		return
	}

	result, err := s.provisionDomain(signup)
	if err != nil {
		jsonError(w, 500, "Provisioning failed: "+err.Error())
		return
	}

	// Auto-login
	email := signup.Username + "@" + signup.Domain
	createSession(w, s.handlers.sessions, email)

	username, domain := store.SplitEmail(email)
	jsonOK(w, map[string]any{
		"status":     "verified",
		"email":      email,
		"username":   username,
		"domain":     domain,
		"dnsRecords": result.DNSRecords,
	})
}

// --- Internal helpers ---

func (s *SignupHandlers) createSignup(domain, username, displayName, password string) (string, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	username = strings.TrimSpace(strings.ToLower(username))

	if domain == "" || !strings.Contains(domain, ".") {
		return "", fmt.Errorf("invalid domain")
	}
	if username == "" {
		return "", fmt.Errorf("username is required")
	}
	if len(password) < 8 {
		return "", fmt.Errorf("password must be at least 8 characters")
	}
	if s.handlers.cfg.IsDomainServed(domain) {
		return "", fmt.Errorf("domain %s is already registered", domain)
	}
	email := username + "@" + domain
	if s.handlers.store.DB.UserExistsByEmail(email) {
		return "", fmt.Errorf("user %s already exists", email)
	}

	hash, err := model.HashPassword(password)
	if err != nil {
		return "", fmt.Errorf("password hashing failed")
	}

	id, err := cryptoutil.RandomHex(16)
	if err != nil {
		return "", fmt.Errorf("failed to generate signup ID")
	}

	signup := &model.DomainSignup{
		ID:           id,
		Domain:       domain,
		Username:     username,
		DisplayName:  displayName,
		PasswordHash: hash,
		Status:       "pending",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	if err := s.handlers.store.DB.CreateSignup(signup); err != nil {
		return "", fmt.Errorf("failed to create signup: %w", err)
	}
	return id, nil
}

func (s *SignupHandlers) provisionDomain(signup *model.DomainSignup) (*admin.DomainResult, error) {
	cfg := s.handlers.cfg
	relay := s.handlers.relay

	// Get certStore from the server (it's on the Handlers' parent)
	// For now, pass nil — cert issuance handled separately
	result, err := admin.RegisterDomain(cfg, relay, nil, signup.Domain)
	if err != nil {
		return nil, err
	}

	// Create domain in DB
	apiKeyHash, _ := cryptoutil.HashSecret(result.DomainAPIKey)
	if err := s.handlers.store.DB.CreateDomain(&model.Domain{
		Name:       signup.Domain,
		APIKeyHash: apiKeyHash,
		SESStatus:  result.SESStatus,
		Status:     "active",
		CreatedBy:  signup.Username + "@" + signup.Domain,
	}); err != nil {
		return nil, fmt.Errorf("create domain: %w", err)
	}

	// Create user account
	if err := s.handlers.store.DB.CreateUser(
		signup.Username, signup.Domain, signup.DisplayName, signup.PasswordHash,
	); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	// Add domain to running config
	cfg.AddDomain(signup.Domain)

	// Clean up signup record
	s.handlers.store.DB.DeleteSignup(signup.ID)

	log.Printf("Domain %s provisioned via self-service signup by %s@%s", signup.Domain, signup.Username, signup.Domain)
	return result, nil
}

func (s *SignupHandlers) buildSignupDNSRecords(domain, mailHostname string) []admin.DNSRecord {
	return []admin.DNSRecord{
		{Type: "CNAME", Name: "mail", Value: mailHostname},
		{Type: "MX", Name: "@", Value: mailHostname, Priority: "10"},
		{Type: "TXT", Name: "@", Value: fmt.Sprintf("v=spf1 include:amazonses.com ~all")},
		{Type: "TXT", Name: "_dmarc", Value: fmt.Sprintf("v=DMARC1; p=none; rua=mailto:postmaster@%s", domain)},
		{Type: "TXT", Name: "bounce", Value: "v=spf1 include:amazonses.com ~all"},
	}
}
