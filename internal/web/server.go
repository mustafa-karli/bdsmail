package web

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/mustafakarli/bdsmail/config"
	"github.com/mustafakarli/bdsmail/internal/carddav"
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
	certReloader *tlsutil.CertReloader
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

func NewServer(cfg *config.Config, s *store.Store, relay *smtp.Relay, checker *security.Checker, certReloader *tlsutil.CertReloader) (*Server, error) {
	tmpl, err := loadTemplates("web/templates")
	if err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	sessions := NewSessionStore()
	handlers := NewHandlers(s, relay, sessions, cfg, checker)
	adminHandlers := NewAdminHandlers(cfg, s, relay, certReloader)

	return &Server{
		cfg:          cfg,
		handlers:     handlers,
		admin:        adminHandlers,
		tmpl:         tmpl,
		certReloader: certReloader,
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

	// CardDAV
	carddavHandler := carddav.NewHandler(s.handlers.store)
	mux.Handle("/carddav/", carddavHandler)
	mux.HandleFunc("/.well-known/carddav", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/carddav/", http.StatusMovedPermanently)
	})

	addr := ":" + s.cfg.HTTPSPort
	log.Printf("Web server starting on %s", addr)

	if s.certReloader != nil {
		tlsCfg := s.certReloader.TLSConfig()
		server := &http.Server{
			Addr:      addr,
			Handler:   mux,
			TLSConfig: tlsCfg,
		}
		return server.ListenAndServeTLS("", "")
	}

	if s.cfg.TLSCert != "" && s.cfg.TLSKey != "" {
		return http.ListenAndServeTLS(addr, s.cfg.TLSCert, s.cfg.TLSKey, mux)
	}
	return http.ListenAndServe(addr, mux)
}
