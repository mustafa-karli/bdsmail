package web

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/mustafakarli/bdsmail/config"
	authpkg "github.com/mustafakarli/bdsmail/internal/auth"
	"github.com/mustafakarli/bdsmail/internal/carddav"
	"github.com/mustafakarli/bdsmail/internal/oauth"
	"github.com/mustafakarli/bdsmail/internal/security"
	"github.com/mustafakarli/bdsmail/internal/smtp"
	"github.com/mustafakarli/bdsmail/internal/store"
	"github.com/mustafakarli/bdsmail/internal/tlsutil"
)

type Server struct {
	cfg          *config.Config
	handlers     *Handlers
	admin        *AdminHandlers
	tmpl         *template.Template
	certStore *tlsutil.CertStore
}

type tmplRenderer struct {
	tmpl     *template.Template
	pageName string
}

func (t *tmplRenderer) render(w http.ResponseWriter, name string, data pageData) {
	// Clone the template set and add page-specific aliases
	// so layout.html's {{template "page-title" .}} and {{template "page-content" .}}
	// resolve to the correct page-specific blocks (e.g. "title-login", "content-login").
	clone, err := t.tmpl.Clone()
	if err != nil {
		log.Printf("template clone error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	aliasTemplate := `{{define "page-title"}}{{template "title-` + t.pageName + `" .}}{{end}}` +
		`{{define "page-content"}}{{template "content-` + t.pageName + `" .}}{{end}}`
	if _, err := clone.Parse(aliasTemplate); err != nil {
		log.Printf("template alias error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := clone.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func NewServer(cfg *config.Config, s *store.Store, relay *smtp.Relay, checker *security.Checker, certStore *tlsutil.CertStore) (*Server, error) {
	tmpl, err := loadTemplates("web/templates")
	if err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	sessions := NewSessionStore()
	issuer := "bdsmail"
	if len(cfg.Domains) > 0 {
		issuer = "mail." + cfg.Domains[0]
	}
	authService := authpkg.NewService(s.DB, issuer)
	handlers := NewHandlers(s, relay, sessions, cfg, checker, authService)
	adminHandlers := NewAdminHandlers(cfg, s, relay, certStore)

	return &Server{
		cfg:          cfg,
		handlers:     handlers,
		admin:        adminHandlers,
		tmpl:         tmpl,
		certStore: certStore,
	}, nil
}

func (s *Server) renderer(pageName string) *tmplRenderer {
	return &tmplRenderer{tmpl: s.tmpl, pageName: pageName}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Static files
	fs := http.FileServer(http.Dir("web/static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// ACME challenge handler (also served on HTTPS in case certbot uses it)
	if s.cfg.AcmeWebroot != "" {
		acmeFS := http.FileServer(http.Dir(s.cfg.AcmeWebroot))
		mux.Handle("/.well-known/", acmeFS)
	}

	// Mail routes
	mux.HandleFunc("/", s.handlers.HandleIndex)

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		s.handlers.HandleLogin(w, r, s.renderer("login"))
	})

	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		s.handlers.HandleLogout(w, r)
	})

	mux.HandleFunc("/inbox", func(w http.ResponseWriter, r *http.Request) {
		s.handlers.HandleInbox(w, r, s.renderer("inbox"))
	})

	mux.HandleFunc("/sent", func(w http.ResponseWriter, r *http.Request) {
		s.handlers.HandleSent(w, r, s.renderer("inbox"))
	})

	mux.HandleFunc("/compose", func(w http.ResponseWriter, r *http.Request) {
		s.handlers.HandleCompose(w, r, s.renderer("compose"))
	})

	mux.HandleFunc("/attachment/", s.handlers.HandleAttachment)

	mux.HandleFunc("/message/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/delete") && r.Method == http.MethodPost {
			s.handlers.HandleDeleteMessage(w, r)
			return
		}
		s.handlers.HandleReadMessage(w, r, s.renderer("read"))
	})

	// Admin routes
	mux.HandleFunc("/admin/domains", func(w http.ResponseWriter, r *http.Request) {
		s.admin.HandleAdminDomains(w, r, s.renderer("admin"))
	})

	mux.HandleFunc("/admin/api/domains", s.admin.HandleAdminAPI)

	mux.HandleFunc("/admin/users", func(w http.ResponseWriter, r *http.Request) {
		s.admin.HandleAdminUsers(w, r, s.renderer("admin_users"))
	})

	mux.HandleFunc("/admin/aliases", func(w http.ResponseWriter, r *http.Request) {
		s.admin.HandleAdminAliases(w, r, s.renderer("admin_aliases"))
	})

	mux.HandleFunc("/admin/lists", func(w http.ResponseWriter, r *http.Request) {
		s.admin.HandleAdminLists(w, r, s.renderer("admin_lists"))
	})

	mux.HandleFunc("/admin/logout", func(w http.ResponseWriter, r *http.Request) {
		s.admin.HandleAdminLogout(w, r)
	})

	// User settings routes
	mux.HandleFunc("/filters", func(w http.ResponseWriter, r *http.Request) {
		s.handlers.HandleFilters(w, r, s.renderer("filters"))
	})

	mux.HandleFunc("/settings/autoreply", func(w http.ResponseWriter, r *http.Request) {
		s.handlers.HandleAutoReply(w, r, s.renderer("autoreply"))
	})

	mux.HandleFunc("/contacts", func(w http.ResponseWriter, r *http.Request) {
		s.handlers.HandleContacts(w, r, s.renderer("contacts"))
	})

	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		s.handlers.HandleSearch(w, r, s.renderer("inbox"))
	})

	mux.HandleFunc("/folder/", func(w http.ResponseWriter, r *http.Request) {
		s.handlers.HandleFolder(w, r, s.renderer("inbox"))
	})

	// 2FA
	twoFA := NewTwoFAHandlers(s.handlers, s.handlers.authService)
	mux.HandleFunc("/verify-2fa", func(w http.ResponseWriter, r *http.Request) {
		twoFA.HandleVerify2FA(w, r, s.renderer("verify_2fa"))
	})
	mux.HandleFunc("/settings/2fa", func(w http.ResponseWriter, r *http.Request) {
		twoFA.HandleSetup2FA(w, r, s.renderer("setup_2fa"))
	})
	mux.HandleFunc("/api/auth/verify-2fa", twoFA.APIVerify2FA)
	mux.HandleFunc("/api/auth/2fa/setup", twoFA.APISetup2FA)
	mux.HandleFunc("/api/auth/2fa/disable", twoFA.APIDisable2FA)
	mux.HandleFunc("/api/auth/trusted-devices", twoFA.APIListTrustedDevices)
	mux.HandleFunc("/api/auth/trusted-devices/revoke", twoFA.APIRevokeTrustedDevice)

	// OAuth / OIDC
	oauthIssuer := "https://mail." + s.cfg.Domains[0]
	oauthHandler, err := oauth.NewHandler(s.handlers.store.DB, oauthIssuer)
	if err != nil {
		return fmt.Errorf("oauth handler init failed: %w", err)
	}
	oauthWeb := NewOAuthWebHandlers(s.handlers, oauthHandler)

	mux.HandleFunc("/developer", func(w http.ResponseWriter, r *http.Request) {
		oauthWeb.HandleDeveloper(w, r, s.renderer("developer"))
	})
	mux.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		oauthWeb.HandleAuthorize(w, r, s.renderer("consent"))
	})
	mux.HandleFunc("/oauth/token", oauthHandler.HandleToken)
	mux.HandleFunc("/oauth/userinfo", oauthHandler.HandleUserInfo)
	mux.HandleFunc("/oauth/jwks", oauthHandler.HandleJWKS)
	mux.HandleFunc("/.well-known/openid-configuration", oauthHandler.HandleDiscovery)

	// CardDAV
	carddavHandler := carddav.NewHandler(s.handlers.store)
	mux.Handle("/carddav/", carddavHandler)
	mux.HandleFunc("/.well-known/carddav", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/carddav/", http.StatusMovedPermanently)
	})

	// JSON API routes (for Vue SPA)
	api := NewAPIHandlers(s.handlers, s.admin, oauthHandler)
	mux.HandleFunc("/api/auth/me", api.HandleAuthMe)
	mux.HandleFunc("/api/auth/login", api.HandleAuthLogin)
	mux.HandleFunc("/api/auth/logout", api.HandleAuthLogout)
	mux.HandleFunc("/api/messages", api.HandleMessages)
	mux.HandleFunc("/api/messages/", api.HandleMessage)
	mux.HandleFunc("/api/compose", api.HandleCompose)
	mux.HandleFunc("/api/search", api.HandleSearch)
	mux.HandleFunc("/api/folders", api.HandleFolders)
	mux.HandleFunc("/api/unread", api.HandleUnread)
	mux.HandleFunc("/api/filters", api.HandleFilters)
	mux.HandleFunc("/api/filters/", api.HandleFilters)
	mux.HandleFunc("/api/autoreply", api.HandleAutoReply)
	mux.HandleFunc("/api/contacts", api.HandleContacts)
	mux.HandleFunc("/api/contacts/", api.HandleContacts)
	mux.HandleFunc("/api/admin/login", api.HandleAdminLogin)
	mux.HandleFunc("/api/admin/domains", api.HandleAdminDomains)
	mux.HandleFunc("/api/admin/users", api.HandleAdminUsers)
	mux.HandleFunc("/api/admin/aliases", api.HandleAdminAliases)
	mux.HandleFunc("/api/admin/lists", api.HandleAdminLists)
	mux.HandleFunc("/api/admin/lists/members", api.HandleAdminListMembers)
	mux.HandleFunc("/api/oauth/clients", api.HandleOAuthClients)
	mux.HandleFunc("/api/oauth/clients/", api.HandleOAuthClients)
	mux.HandleFunc("/api/admin/logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "bdsmail_admin", Value: "", Path: "/", MaxAge: -1})
		jsonOK(w, map[string]string{"status": "ok"})
	})

	// Serve Vue SPA from web/vue/dist/ if it exists
	vueDist := "web/vue/dist"
	if info, err := os.Stat(vueDist); err == nil && info.IsDir() {
		vueFS := http.FileServer(http.Dir(vueDist))
		mux.HandleFunc("/app/", func(w http.ResponseWriter, r *http.Request) {
			// SPA fallback: serve index.html for non-file paths
			path := strings.TrimPrefix(r.URL.Path, "/app")
			if path == "" || path == "/" {
				http.ServeFile(w, r, vueDist+"/index.html")
				return
			}
			// Try serving the file; if not found, serve index.html
			if _, err := os.Stat(vueDist + path); os.IsNotExist(err) {
				http.ServeFile(w, r, vueDist+"/index.html")
				return
			}
			http.StripPrefix("/app", vueFS).ServeHTTP(w, r)
		})
		log.Printf("Vue SPA available at /app/")
	}

	// Wrap with CORS if Amplify frontend is configured
	var handler http.Handler = mux
	if s.cfg.AmplifyURL != "" {
		handler = corsMiddleware(mux, s.cfg)
	}

	addr := fmt.Sprintf(":%d", s.cfg.HTTPSPort)
	log.Printf("Web server starting on %s", addr)

	if s.certStore != nil && s.certStore.HasCerts() {
		server := &http.Server{
			Addr:      addr,
			Handler:   handler,
			TLSConfig: s.certStore.TLSConfig(),
		}
		return server.ListenAndServeTLS("", "")
	}

	log.Printf("No TLS certificates loaded, running HTTP only")
	return http.ListenAndServe(addr, handler)
}
