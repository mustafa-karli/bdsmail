package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/mustafakarli/bdsmail/internal/cryptoutil"
	"github.com/mustafakarli/bdsmail/internal/model"
	"github.com/mustafakarli/bdsmail/internal/store"
)

// --- /api/send endpoint ---

// HandleAPISend accepts a bearer token and sends an email.
func (h *Handlers) HandleAPISend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "POST required")
		return
	}

	// Extract bearer token
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		jsonError(w, 401, "Missing Bearer token")
		return
	}
	bearerToken := auth[7:]

	// Find matching app token by checking hash against all tokens
	appToken, err := h.validateAppToken(bearerToken)
	if err != nil {
		jsonError(w, 401, "Invalid API key")
		return
	}

	// Parse request
	var body struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		CC      []string `json:"cc"`
		Subject string   `json:"subject"`
		Body    string   `json:"body"`
		HTML    bool     `json:"html"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, 400, "Invalid request body")
		return
	}

	if len(body.To) == 0 {
		jsonError(w, 400, "At least one recipient required")
		return
	}

	// Verify sender matches token's allowed sender
	if body.From == "" {
		body.From = appToken.SenderEmail
	}
	if body.From != appToken.SenderEmail {
		jsonError(w, 403, "Token not authorized to send from "+body.From)
		return
	}

	contentType := "text/plain"
	if body.HTML {
		contentType = "text/html"
	}

	// Build from address with display name if available
	fromAddr := body.From
	if user, err := h.store.DB.GetUserByEmail(body.From); err == nil && user.DisplayName != "" {
		fromAddr = user.DisplayName + " <" + body.From + ">"
	}

	ctx := context.Background()

	// Save and relay the message
	messageID, err := h.store.SaveOutgoingMail(ctx, body.From, fromAddr, body.To, body.CC, nil, body.Subject, contentType, body.Body, nil)
	if err != nil {
		log.Printf("API send failed: %v", err)
		jsonError(w, 500, "Failed to send message")
		return
	}

	// Relay to external recipients
	allExternal := collectExternalAddrs(h.cfg, body.To, body.CC, nil)
	if len(allExternal) > 0 && h.relay != nil {
		go func() {
			if err := h.relay.Send(body.From, allExternal, body.Subject, contentType, body.Body, messageID); err != nil {
				log.Printf("API send relay error: %v", err)
			}
		}()
	}

	// Update last used timestamp
	h.store.DB.UpdateTokenLastUsed(appToken.ID)

	jsonOK(w, map[string]string{"status": "ok", "messageId": messageID})
}

// validateAppToken finds an app token by checking the bearer against all token hashes.
func (h *Handlers) validateAppToken(bearer string) (*model.AppToken, error) {
	// Strip bds_ak_ prefix if present
	raw := bearer
	if strings.HasPrefix(raw, "bds_ak_") {
		raw = raw[7:]
	}

	// SHA-256 hash → single indexed DB lookup
	hash := cryptoutil.SHA256Hex(raw)
	token, err := h.store.DB.GetAppTokenByHash(hash)
	if err != nil {
		return nil, fmt.Errorf("token not found")
	}
	return token, nil
}

// --- API Keys management page ---

func (h *Handlers) HandleAPIKeys(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	email, ok := h.requireAuth(w, r)
	if !ok {
		return
	}
	if !h.isAdmin(email) {
		http.Error(w, "Owner or admin role required", http.StatusForbidden)
		return
	}

	pd := h.userPageData(email)
	_, domain := store.SplitEmail(email)

	if r.Method == http.MethodPost {
		action := r.FormValue("action")
		switch action {
		case "create":
			name := strings.TrimSpace(r.FormValue("name"))
			senderEmail := strings.TrimSpace(r.FormValue("sender_email"))
			if name == "" || senderEmail == "" {
				pd.Error = "Name and sender email are required"
			} else {
				// Verify sender exists in this domain
				if !h.store.DB.UserExistsByEmail(senderEmail) {
					pd.Error = "Sender " + senderEmail + " does not exist"
				} else {
					_, senderDomain := store.SplitEmail(senderEmail)
					if senderDomain != domain {
						pd.Error = "Sender must be in your domain"
					} else {
						token, _ := cryptoutil.RandomHex(32)
						tokenHash := cryptoutil.SHA256Hex(token)
						id, _ := cryptoutil.RandomHex(16)

						if err := h.store.DB.CreateAppToken(&model.AppToken{
							ID:          id,
							Name:        name,
							TokenHash:   tokenHash,
							Domain:      domain,
							SenderEmail: senderEmail,
							CreatedBy:   email,
						}); err != nil {
							pd.Error = "Failed to create token: " + err.Error()
						} else {
							pd.Success = "API key created"
							// Show the token once — reuse OAuthState field
							pd.OAuthState = "bds_ak_" + token
						}
					}
				}
			}

		case "delete":
			tokenID := r.FormValue("token_id")
			h.store.DB.DeleteAppToken(tokenID)
			pd.Success = "API key deleted"
		}
	}

	tokens, _ := h.store.DB.ListAppTokens(domain)
	if tokens == nil {
		tokens = []*model.AppToken{}
	}
	pd.AdminData = tokens
	tmpl.render(w, "layout", pd)
}

// --- API endpoint for managing tokens ---

func (h *Handlers) HandleAPIKeysAPI(w http.ResponseWriter, r *http.Request) {
	email := getSessionUser(r, h.sessions)
	if email == "" || !h.isAdmin(email) {
		jsonError(w, 403, "Owner or admin role required")
		return
	}
	_, domain := store.SplitEmail(email)

	switch r.Method {
	case http.MethodGet:
		tokens, _ := h.store.DB.ListAppTokens(domain)
		type tv struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			SenderEmail string `json:"senderEmail"`
			CreatedBy   string `json:"createdBy"`
			CreatedAt   string `json:"createdAt"`
			LastUsedAt  string `json:"lastUsedAt"`
		}
		var result []tv
		for _, t := range tokens {
			lastUsed := ""
			if !t.LastUsedAt.IsZero() {
				lastUsed = t.LastUsedAt.Format("2006-01-02 15:04")
			}
			result = append(result, tv{t.ID, t.Name, t.SenderEmail, t.CreatedBy, t.CreatedAt.Format("2006-01-02 15:04"), lastUsed})
		}
		if result == nil {
			result = []tv{}
		}
		jsonOK(w, result)

	case http.MethodPost:
		var body struct {
			Name        string `json:"name"`
			SenderEmail string `json:"senderEmail"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		token, _ := cryptoutil.RandomHex(32)
		tokenHash := cryptoutil.SHA256Hex(token)
		id, _ := cryptoutil.RandomHex(16)

		if err := h.store.DB.CreateAppToken(&model.AppToken{
			ID: id, Name: body.Name, TokenHash: tokenHash,
			Domain: domain, SenderEmail: body.SenderEmail, CreatedBy: email,
		}); err != nil {
			jsonError(w, 500, "Failed to create token")
			return
		}
		jsonOK(w, map[string]string{"token": "bds_ak_" + token, "id": id})

	case http.MethodDelete:
		var body struct {
			ID string `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		h.store.DB.DeleteAppToken(body.ID)
		jsonOK(w, map[string]string{"status": "ok"})
	}
}
