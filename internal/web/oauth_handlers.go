package web

import (
	"net/http"
	"net/url"

	"github.com/mustafakarli/bdsmail/internal/oauth"
)

// OAuthWebHandlers handles the web UI portions of OAuth (consent screen, developer portal).
type OAuthWebHandlers struct {
	handlers *Handlers
	oauth    *oauth.Handler
}

func NewOAuthWebHandlers(h *Handlers, o *oauth.Handler) *OAuthWebHandlers {
	return &OAuthWebHandlers{handlers: h, oauth: o}
}

// HandleDeveloper renders the self-service developer portal.
func (o *OAuthWebHandlers) HandleDeveloper(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	email, ok := o.handlers.requireAuth(w, r)
	if !ok {
		return
	}
	pd := o.handlers.userPageData(email)

	if r.Method == http.MethodPost {
		action := r.FormValue("action")
		switch action {
		case "create":
			name := r.FormValue("name")
			redirectURI := r.FormValue("redirect_uri")
			client, err := o.oauth.RegisterClient(name, redirectURI, email)
			if err != nil {
				pd.Error = "Failed to register app: " + err.Error()
			} else {
				pd.Success = "Application registered"
				pd.NewClient = client
			}
		case "delete":
			if err := o.oauth.DeleteClient(r.FormValue("client_db_id")); err != nil {
				pd.Error = "Failed to delete app"
			} else {
				pd.Success = "Application deleted"
			}
		}
	}

	clients, _ := o.oauth.ListClients(email)
	pd.OAuthClients = clients
	tmpl.render(w, "layout", pd)
}

// HandleAuthorize handles GET (consent screen) and POST (user approves/denies).
func (o *OAuthWebHandlers) HandleAuthorize(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	if r.Method == http.MethodGet {
		o.showConsent(w, r, tmpl)
		return
	}
	o.processConsent(w, r)
}

func (o *OAuthWebHandlers) showConsent(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	email, ok := o.handlers.requireAuth(w, r)
	if !ok {
		return
	}

	params, err := o.oauth.ValidateAuthorize(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pd := o.handlers.userPageData(email)
	pd.OAuthClientID = params.ClientID
	pd.OAuthClientName = params.ClientName
	pd.OAuthRedirectURI = params.RedirectURI
	pd.OAuthScope = params.Scope
	pd.OAuthState = params.State
	pd.OAuthNonce = params.Nonce
	tmpl.render(w, "layout", pd)
}

func (o *OAuthWebHandlers) processConsent(w http.ResponseWriter, r *http.Request) {
	email, ok := o.handlers.requireAuth(w, r)
	if !ok {
		return
	}

	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	scope := r.FormValue("scope")
	state := r.FormValue("state")
	nonce := r.FormValue("nonce")
	action := r.FormValue("action")

	if action != "approve" {
		// User denied — redirect with error
		u, _ := url.Parse(redirectURI)
		q := u.Query()
		q.Set("error", "access_denied")
		if state != "" {
			q.Set("state", state)
		}
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
		return
	}

	code, err := o.oauth.IssueCode(clientID, email, redirectURI, scope, nonce)
	if err != nil {
		http.Error(w, "Failed to issue authorization code", http.StatusInternalServerError)
		return
	}

	u, _ := url.Parse(redirectURI)
	q := u.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}
