package web

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/mustafakarli/bdsmail/config"
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
	err := t.tmpl.ExecuteTemplate(w, name, data)
	if err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func NewServer(cfg *config.Config, s *store.Store, relay *smtp.Relay, certReloader *tlsutil.CertReloader) (*Server, error) {
	tmpl, err := loadTemplates("web/templates")
	if err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	sessions := NewSessionStore()
	handlers := NewHandlers(s, relay, sessions, cfg)
	adminHandlers := NewAdminHandlers(cfg, relay, certReloader)

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

	mux.HandleFunc("/admin/logout", func(w http.ResponseWriter, r *http.Request) {
		s.admin.HandleAdminLogout(w, r)
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
