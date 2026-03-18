package smtp

import (
	"log"
	"time"

	gosmtp "github.com/emersion/go-smtp"
	"github.com/mustafakarli/bdsmail/config"
	"github.com/mustafakarli/bdsmail/internal/security"
	"github.com/mustafakarli/bdsmail/internal/store"
	"github.com/mustafakarli/bdsmail/internal/tlsutil"
)

type Server struct {
	srv *gosmtp.Server
}

func NewServer(cfg *config.Config, s *store.Store, checker *security.Checker, relay *Relay, certReloader *tlsutil.CertReloader) *Server {
	backend := NewBackend(s, checker, relay, cfg)

	srv := gosmtp.NewServer(backend)
	srv.Addr = ":" + cfg.SMTPPort
	if len(cfg.Domains) > 0 {
		srv.Domain = cfg.Domains[0]
	}
	srv.ReadTimeout = 30 * time.Second
	srv.WriteTimeout = 30 * time.Second
	srv.MaxMessageBytes = 10 * 1024 * 1024
	srv.MaxRecipients = 50
	srv.AllowInsecureAuth = true

	if certReloader != nil {
		srv.TLSConfig = certReloader.TLSConfig()
	}

	return &Server{srv: srv}
}

func (s *Server) Start() error {
	log.Printf("SMTP server starting on %s", s.srv.Addr)
	return s.srv.ListenAndServe()
}

func (s *Server) Close() error {
	return s.srv.Close()
}
