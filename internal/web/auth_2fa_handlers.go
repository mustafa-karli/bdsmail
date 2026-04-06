package web

import (
	"encoding/json"
	"net/http"

	"github.com/mustafakarli/bdsmail/internal/auth"
	"github.com/mustafakarli/bdsmail/internal/model"
	"github.com/mustafakarli/bdsmail/internal/store"
)

// TwoFAHandlers manages 2FA setup, verification, and trusted devices.
type TwoFAHandlers struct {
	handlers *Handlers
	auth     *auth.Service
}

func NewTwoFAHandlers(h *Handlers, a *auth.Service) *TwoFAHandlers {
	return &TwoFAHandlers{handlers: h, auth: a}
}

// HandleVerify2FA renders the TOTP input page (GET) and verifies the code (POST).
func (t *TwoFAHandlers) HandleVerify2FA(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	if r.Method == http.MethodGet {
		tmpl.render(w, "layout", pageData{})
		return
	}

	loginToken := r.FormValue("login_token")
	code := r.FormValue("code")
	trustDevice := r.FormValue("trust_device") == "true"
	fingerprint := r.FormValue("device_fingerprint")
	deviceName := r.FormValue("device_name")

	email, err := t.auth.ConsumeLoginToken(loginToken)
	if err != nil {
		tmpl.render(w, "layout", pageData{Error: "Login session expired. Please log in again."})
		return
	}

	// Try TOTP first, then backup code
	valid, _ := t.auth.Verify2FA(email, code)
	if !valid {
		valid, _ = t.auth.VerifyBackupCode(email, code)
	}
	if !valid {
		// Reissue login token for retry
		newToken, _ := t.auth.CreateLoginToken(email)
		pd := pageData{Error: "Invalid code. Please try again.", OAuthState: newToken}
		tmpl.render(w, "layout", pd)
		return
	}

	// Trust device if requested
	if trustDevice && fingerprint != "" {
		t.auth.RegisterTrustedDevice(email, fingerprint, deviceName)
	}

	// Create session
	if err := createSession(w, t.handlers.sessions, email); err != nil {
		tmpl.render(w, "layout", pageData{Error: "Failed to create session"})
		return
	}
	http.Redirect(w, r, "/inbox", http.StatusSeeOther)
}

// HandleSetup2FA handles 2FA setup page.
func (t *TwoFAHandlers) HandleSetup2FA(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	email, ok := t.handlers.requireAuth(w, r)
	if !ok {
		return
	}
	pd := t.handlers.userPageData(email)

	if r.Method == http.MethodPost {
		action := r.FormValue("action")
		switch action {
		case "setup":
			secret, qrURI, backupCodes, err := t.auth.Setup2FA(email)
			if err != nil {
				pd.Error = "Failed to set up 2FA: " + err.Error()
			} else {
				pd.OAuthState = secret // reuse field for template
				pd.OAuthScope = qrURI
				pd.Success = "2FA enabled. Scan the QR code and save your backup codes."
				pd.AdminData = backupCodes
			}
		case "disable":
			code := r.FormValue("code")
			if err := t.auth.Disable2FA(email, code); err != nil {
				pd.Error = "Failed to disable 2FA: " + err.Error()
			} else {
				pd.Success = "2FA has been disabled"
			}
		}
	}

	enabled, _, _, _ := t.handlers.store.DB.Get2FAStatus(email)
	pd.OAuthNonce = "enabled"
	if !enabled {
		pd.OAuthNonce = ""
	}
	tmpl.render(w, "layout", pd)
}

// --- API Handlers ---

func (t *TwoFAHandlers) APIVerify2FA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LoginToken       string `json:"loginToken"`
		Code             string `json:"code"`
		TrustDevice      bool   `json:"trustDevice"`
		DeviceFingerprint string `json:"deviceFingerprint"`
		DeviceName       string `json:"deviceName"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	email, err := t.auth.ConsumeLoginToken(body.LoginToken)
	if err != nil {
		jsonError(w, 401, "Login session expired")
		return
	}

	valid, _ := t.auth.Verify2FA(email, body.Code)
	if !valid {
		valid, _ = t.auth.VerifyBackupCode(email, body.Code)
	}
	if !valid {
		newToken, _ := t.auth.CreateLoginToken(email)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"error": "Invalid code", "loginToken": newToken})
		return
	}

	if body.TrustDevice && body.DeviceFingerprint != "" {
		t.auth.RegisterTrustedDevice(email, body.DeviceFingerprint, body.DeviceName)
	}

	if err := createSession(w, t.handlers.sessions, email); err != nil {
		jsonError(w, 500, "Session creation failed")
		return
	}

	username, domain := store.SplitEmail(email)
	displayName := ""
	if user, err := t.handlers.store.DB.GetUserByEmail(email); err == nil {
		displayName = user.DisplayName
	}
	jsonOK(w, map[string]string{"username": username, "displayName": displayName, "email": email, "domain": domain})
}

func (t *TwoFAHandlers) APISetup2FA(w http.ResponseWriter, r *http.Request) {
	email, ok := requireAPIAuth(w, r, t.handlers)
	if !ok {
		return
	}
	secret, qrURI, backupCodes, err := t.auth.Setup2FA(email)
	if err != nil {
		jsonError(w, 500, "2FA setup failed: "+err.Error())
		return
	}
	jsonOK(w, map[string]any{"secret": secret, "qrUri": qrURI, "backupCodes": backupCodes})
}

func (t *TwoFAHandlers) APIDisable2FA(w http.ResponseWriter, r *http.Request) {
	email, ok := requireAPIAuth(w, r, t.handlers)
	if !ok {
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if err := t.auth.Disable2FA(email, body.Code); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (t *TwoFAHandlers) APIListTrustedDevices(w http.ResponseWriter, r *http.Request) {
	email, ok := requireAPIAuth(w, r, t.handlers)
	if !ok {
		return
	}
	devices, _ := t.auth.ListTrustedDevices(email)
	if devices == nil {
		devices = []*model.TrustedDevice{}
	}
	jsonOK(w, devices)
}

func (t *TwoFAHandlers) APIRevokeTrustedDevice(w http.ResponseWriter, r *http.Request) {
	_, ok := requireAPIAuth(w, r, t.handlers)
	if !ok {
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	t.auth.RevokeTrustedDevice(body.ID)
	jsonOK(w, map[string]string{"status": "ok"})
}

// helpers

func requireAPIAuth(w http.ResponseWriter, r *http.Request, h *Handlers) (string, bool) {
	email := getSessionUser(r, h.sessions)
	if email == "" {
		jsonError(w, 401, "Unauthorized")
		return "", false
	}
	return email, true
}
