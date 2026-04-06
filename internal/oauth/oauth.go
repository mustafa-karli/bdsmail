package oauth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/mustafakarli/bdsmail/internal/cryptoutil"
	"github.com/mustafakarli/bdsmail/internal/model"
	"github.com/mustafakarli/bdsmail/internal/store"
)

// Handler implements OAuth 2.0 + OpenID Connect provider endpoints.
type Handler struct {
	db         store.Database
	signingKey *rsa.PrivateKey
	keyID      string
	issuer     string
}

// NewHandler creates a new OAuth/OIDC handler. Returns error instead of fatal.
func NewHandler(db store.Database, issuer string) (*Handler, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("RSA key generation failed: %w", err)
	}
	pubBytes := key.PublicKey.N.Bytes()
	hash := sha256.Sum256(pubBytes)
	kid := hex.EncodeToString(hash[:8])
	return &Handler{db: db, signingKey: key, keyID: kid, issuer: issuer}, nil
}

// --- Client Registration (for developer portal) ---

type ClientInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ClientID    string `json:"clientId"`
	RedirectURI string `json:"redirectUri"`
	CreatedAt   string `json:"createdAt"`
}

type NewClientResponse struct {
	ClientInfo
	ClientSecret string `json:"clientSecret"` // Only shown once at creation
}

func (h *Handler) RegisterClient(name, redirectURI, domain, createdBy string) (*NewClientResponse, error) {
	clientID, err := cryptoutil.RandomHex(16)
	if err != nil {
		return nil, fmt.Errorf("generate client_id: %w", err)
	}
	clientSecret, err := cryptoutil.RandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generate client_secret: %w", err)
	}
	id, err := cryptoutil.RandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generate id: %w", err)
	}
	secretHash, err := cryptoutil.HashSecret(clientSecret)
	if err != nil {
		return nil, fmt.Errorf("hash secret: %w", err)
	}

	client := &model.OAuthClient{
		ID:          id,
		Name:        name,
		ClientID:    clientID,
		SecretHash:  secretHash,
		RedirectURI: redirectURI,
		Domain:      domain,
		CreatedBy:   createdBy,
	}
	if err := h.db.CreateOAuthClient(client); err != nil {
		return nil, err
	}
	return &NewClientResponse{
		ClientInfo: ClientInfo{
			ID: client.ID, Name: name, ClientID: clientID,
			RedirectURI: redirectURI, CreatedAt: time.Now().Format(time.RFC3339),
		},
		ClientSecret: clientSecret,
	}, nil
}

func (h *Handler) ListClients(domain string) ([]ClientInfo, error) {
	clients, err := h.db.ListOAuthClients(domain)
	if err != nil {
		return nil, err
	}
	var result []ClientInfo
	for _, c := range clients {
		result = append(result, ClientInfo{
			ID: c.ID, Name: c.Name, ClientID: c.ClientID,
			RedirectURI: c.RedirectURI, CreatedAt: c.CreatedAt.Format(time.RFC3339),
		})
	}
	if result == nil {
		result = []ClientInfo{}
	}
	return result, nil
}

func (h *Handler) DeleteClient(id string) error {
	return h.db.DeleteOAuthClient(id)
}

// --- Authorization Endpoint ---

// AuthorizeParams are extracted from the /oauth/authorize request.
type AuthorizeParams struct {
	ClientID     string
	RedirectURI  string
	ResponseType string
	Scope        string
	State        string
	Nonce        string
	ClientName   string // looked up from DB
}

// ValidateAuthorize validates the authorization request and returns params for the consent screen.
func (h *Handler) ValidateAuthorize(r *http.Request) (*AuthorizeParams, error) {
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	responseType := r.URL.Query().Get("response_type")
	scope := r.URL.Query().Get("scope")
	state := r.URL.Query().Get("state")
	nonce := r.URL.Query().Get("nonce")

	if responseType != "code" {
		return nil, fmt.Errorf("unsupported response_type: must be 'code'")
	}
	if clientID == "" {
		return nil, fmt.Errorf("client_id is required")
	}

	client, err := h.db.GetOAuthClient(clientID)
	if err != nil {
		return nil, fmt.Errorf("unknown client_id")
	}
	if redirectURI == "" {
		redirectURI = client.RedirectURI
	}
	if redirectURI != client.RedirectURI {
		return nil, fmt.Errorf("redirect_uri mismatch")
	}

	return &AuthorizeParams{
		ClientID: clientID, RedirectURI: redirectURI,
		ResponseType: responseType, Scope: scope,
		State: state, Nonce: nonce, ClientName: client.Name,
	}, nil
}

// IssueCode creates an authorization code after user consent.
func (h *Handler) IssueCode(clientID, userEmail, redirectURI, scope, nonce string) (string, error) {
	code, err := cryptoutil.RandomHex(32)
	if err != nil {
		return "", fmt.Errorf("generate auth code: %w", err)
	}
	oauthCode := &model.OAuthCode{
		Code: code, ClientID: clientID, UserEmail: userEmail,
		RedirectURI: redirectURI, Scope: scope, Nonce: nonce,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	if err := h.db.CreateOAuthCode(oauthCode); err != nil {
		return "", err
	}
	return code, nil
}

// --- Token Endpoint ---

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	IDToken     string `json:"id_token,omitempty"`
	Scope       string `json:"scope,omitempty"`
}

func (h *Handler) HandleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, 405, "method_not_allowed", "POST required")
		return
	}
	r.ParseForm()
	grantType := r.FormValue("grant_type")
	if grantType != "authorization_code" {
		jsonErr(w, 400, "unsupported_grant_type", "only authorization_code is supported")
		return
	}

	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")

	// Verify client
	client, err := h.db.GetOAuthClient(clientID)
	if err != nil || !client.CheckSecret(clientSecret) {
		jsonErr(w, 401, "invalid_client", "invalid client credentials")
		return
	}

	// Verify code
	oauthCode, err := h.db.GetOAuthCode(code)
	if err != nil {
		jsonErr(w, 400, "invalid_grant", "invalid authorization code")
		return
	}
	if oauthCode.Used || oauthCode.IsExpired() {
		jsonErr(w, 400, "invalid_grant", "authorization code expired or already used")
		return
	}
	if oauthCode.ClientID != clientID {
		jsonErr(w, 400, "invalid_grant", "code was not issued to this client")
		return
	}
	if redirectURI != "" && oauthCode.RedirectURI != redirectURI {
		jsonErr(w, 400, "invalid_grant", "redirect_uri mismatch")
		return
	}

	// Mark code as used
	h.db.MarkOAuthCodeUsed(code)

	// Issue access token
	accessToken, err := cryptoutil.RandomHex(32)
	if err != nil {
		jsonErr(w, 500, "server_error", "token generation failed")
		return
	}
	expiresIn := 3600 // 1 hour
	h.db.CreateOAuthToken(&model.OAuthToken{
		Token: accessToken, ClientID: clientID, UserEmail: oauthCode.UserEmail,
		Scope: oauthCode.Scope, ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second),
	})

	resp := TokenResponse{
		AccessToken: accessToken, TokenType: "Bearer",
		ExpiresIn: expiresIn, Scope: oauthCode.Scope,
	}

	// Issue ID token if openid scope was requested
	if strings.Contains(oauthCode.Scope, "openid") {
		idToken, err := h.createIDToken(oauthCode.UserEmail, clientID, oauthCode.Nonce)
		if err == nil {
			resp.IDToken = idToken
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(resp)
}

// --- UserInfo Endpoint ---

func (h *Handler) HandleUserInfo(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		jsonErr(w, 401, "invalid_token", "missing bearer token")
		return
	}

	oauthToken, err := h.db.GetOAuthToken(token)
	if err != nil || oauthToken.IsExpired() {
		jsonErr(w, 401, "invalid_token", "token expired or invalid")
		return
	}

	username, domain := store.SplitEmail(oauthToken.UserEmail)
	resp := map[string]string{
		"sub":    oauthToken.UserEmail,
		"email":  oauthToken.UserEmail,
		"name":   username,
		"domain": domain,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// --- OIDC Discovery ---

func (h *Handler) HandleDiscovery(w http.ResponseWriter, r *http.Request) {
	doc := map[string]any{
		"issuer":                 h.issuer,
		"authorization_endpoint": h.issuer + "/oauth/authorize",
		"token_endpoint":         h.issuer + "/oauth/token",
		"userinfo_endpoint":      h.issuer + "/oauth/userinfo",
		"jwks_uri":               h.issuer + "/oauth/jwks",
		"response_types_supported": []string{"code"},
		"grant_types_supported":    []string{"authorization_code"},
		"subject_types_supported":  []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported": []string{"openid", "email", "profile"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post"},
		"claims_supported": []string{"sub", "email", "name", "domain", "iss", "aud", "exp", "iat"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

// --- JWKS Endpoint ---

func (h *Handler) HandleJWKS(w http.ResponseWriter, r *http.Request) {
	pub := &h.signingKey.PublicKey
	jwks := map[string]any{
		"keys": []map[string]string{
			{
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"kid": h.keyID,
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jwks)
}

// --- JWT (ID Token) ---

func (h *Handler) createIDToken(email, audience, nonce string) (string, error) {
	username, domain := store.SplitEmail(email)
	now := time.Now()

	header := map[string]string{"alg": "RS256", "typ": "JWT", "kid": h.keyID}
	claims := map[string]any{
		"iss":    h.issuer,
		"sub":    email,
		"aud":    audience,
		"exp":    now.Add(1 * time.Hour).Unix(),
		"iat":    now.Unix(),
		"email":  email,
		"name":   username,
		"domain": domain,
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}

	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := headerB64 + "." + claimsB64
	hashed := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, h.signingKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", err
	}
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	return signingInput + "." + sigB64, nil
}

// --- Helpers ---

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return auth[7:]
	}
	return ""
}

func jsonErr(w http.ResponseWriter, code int, errType, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": errType, "error_description": desc})
}
