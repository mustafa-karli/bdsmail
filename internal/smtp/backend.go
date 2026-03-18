package smtp

import (
	"bytes"
	"context"
	"io"
	"log"
	"mime"
	"net"
	"net/mail"
	"strings"

	gosmtp "github.com/emersion/go-smtp"
	"github.com/mustafakarli/bdsmail/config"
	"github.com/mustafakarli/bdsmail/internal/security"
	"github.com/mustafakarli/bdsmail/internal/store"
)

type Backend struct {
	store   *store.Store
	checker *security.Checker
	relay   *Relay
	cfg     *config.Config
}

func NewBackend(s *store.Store, checker *security.Checker, relay *Relay, cfg *config.Config) *Backend {
	return &Backend{store: s, checker: checker, relay: relay, cfg: cfg}
}

func (b *Backend) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	var remoteIP net.IP
	if addr, ok := c.Conn().RemoteAddr().(*net.TCPAddr); ok {
		remoteIP = addr.IP
	}
	return &Session{
		backend:      b,
		remoteIP:     remoteIP,
		ehloHostname: c.Hostname(),
	}, nil
}

type Session struct {
	backend      *Backend
	from         string
	to           []string
	authed       bool
	user         string
	remoteIP     net.IP
	ehloHostname string
}

// AuthPlain authenticates with full email (user@domain) as username.
func (s *Session) AuthPlain(username, password string) error {
	user, err := s.backend.store.DB.GetUserByEmail(username)
	if err != nil {
		return &gosmtp.SMTPError{
			Code:    535,
			Message: "Authentication failed",
		}
	}
	if !user.CheckPassword(password) {
		return &gosmtp.SMTPError{
			Code:    535,
			Message: "Authentication failed",
		}
	}
	s.authed = true
	s.user = user.Email()
	return nil
}

func (s *Session) Mail(from string, opts *gosmtp.MailOptions) error {
	s.from = from
	return nil
}

func (s *Session) Rcpt(to string, opts *gosmtp.RcptOptions) error {
	s.to = append(s.to, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	msg, err := mail.ReadMessage(bytes.NewReader(body))
	if err != nil {
		return s.storeRaw(string(body))
	}

	subject := decodeHeader(msg.Header.Get("Subject"))
	from := s.from
	if from == "" {
		from = msg.Header.Get("From")
	}

	to := parseHeaderAddrs(msg.Header.Get("To"))
	if len(to) == 0 {
		to = s.to
	}
	cc := parseHeaderAddrs(msg.Header.Get("Cc"))
	bcc := s.collectBCC(to, cc)

	contentType := "text/plain"
	ct := msg.Header.Get("Content-Type")
	if strings.Contains(ct, "text/html") {
		contentType = "text/html"
	}

	bodyBytes, err := io.ReadAll(msg.Body)
	if err != nil {
		return err
	}

	ctx := context.Background()
	bodyText := string(bodyBytes)

	if s.authed {
		// Authenticated user: treat as outbound mail
		return s.handleOutbound(ctx, body, from, to, cc, bcc, subject, contentType, bodyText)
	}

	// Unauthenticated: treat as inbound mail
	return s.handleInbound(ctx, body, from, to, cc, bcc, subject, contentType, bodyText)
}

func (s *Session) handleOutbound(ctx context.Context, rawEmail []byte, from string, to, cc, bcc []string, subject, contentType, bodyText string) error {
	// Run outbound security checks
	if s.backend.checker != nil {
		result := s.backend.checker.CheckOutbound(ctx, bodyText, contentType)
		if result.Reject {
			log.Printf("blocked outbound mail from %s: %s", s.user, result.Reason)
			return &gosmtp.SMTPError{
				Code:    550,
				Message: "Message rejected: " + result.Reason,
			}
		}
	}

	// Save sender's copy in Sent folder + deliver to local recipients
	messageID, err := s.backend.store.SaveOutgoingMail(ctx, s.user, from, to, cc, bcc, subject, contentType, bodyText)
	if err != nil {
		log.Printf("failed to save outgoing mail: %v", err)
		return &gosmtp.SMTPError{
			Code:    451,
			Message: "Internal server error",
		}
	}

	// Relay to external recipients in background
	allAddrs := make([]string, 0, len(to)+len(cc)+len(bcc))
	allAddrs = append(allAddrs, to...)
	allAddrs = append(allAddrs, cc...)
	allAddrs = append(allAddrs, bcc...)

	var external []string
	for _, addr := range allAddrs {
		_, domain := store.SplitEmail(addr)
		if !s.backend.cfg.IsDomainServed(domain) {
			external = append(external, addr)
		}
	}

	if len(external) > 0 && s.backend.relay != nil {
		go func() {
			err := s.backend.relay.Send(s.user, external, subject, contentType, bodyText, messageID)
			if err != nil {
				log.Printf("SMTP relay error: %v", err)
			}
		}()
	}

	return nil
}

func (s *Session) handleInbound(ctx context.Context, rawEmail []byte, from string, to, cc, bcc []string, subject, contentType, bodyText string) error {
	folder := "INBOX"

	// Run inbound security checks
	if s.backend.checker != nil {
		result := s.backend.checker.CheckInbound(ctx, rawEmail, bodyText, contentType, s.remoteIP, from, s.ehloHostname)
		if result.Reject {
			log.Printf("rejected mail from %s: %s", from, result.Reason)
			return &gosmtp.SMTPError{
				Code:    550,
				Message: "Message rejected: " + result.Reason,
			}
		}
		if result.SubjectPrefix != "" {
			subject = result.SubjectPrefix + " " + subject
		}
		if result.Folder != "" {
			folder = result.Folder
		}
	}

	err := s.backend.store.SaveIncomingMail(ctx, from, to, cc, bcc, subject, contentType, bodyText, folder)
	if err != nil {
		log.Printf("failed to save incoming mail: %v", err)
		return &gosmtp.SMTPError{
			Code:    451,
			Message: "Internal server error",
		}
	}

	return nil
}

func (s *Session) Reset() {
	s.from = ""
	s.to = nil
}

func (s *Session) Logout() error {
	return nil
}

func (s *Session) storeRaw(body string) error {
	ctx := context.Background()
	return s.backend.store.SaveIncomingMail(ctx, s.from, s.to, nil, nil, "(no subject)", "text/plain", body, "INBOX")
}

func (s *Session) collectBCC(to, cc []string) []string {
	headerAddrs := make(map[string]bool)
	for _, a := range to {
		headerAddrs[a] = true
	}
	for _, a := range cc {
		headerAddrs[a] = true
	}
	var bcc []string
	for _, a := range s.to {
		if !headerAddrs[a] {
			bcc = append(bcc, a)
		}
	}
	return bcc
}

func parseHeaderAddrs(header string) []string {
	if header == "" {
		return nil
	}
	addrs, err := mail.ParseAddressList(header)
	if err != nil {
		parts := strings.Split(header, ",")
		var result []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		return result
	}
	var result []string
	for _, a := range addrs {
		result = append(result, a.Address)
	}
	return result
}

func decodeHeader(s string) string {
	dec := new(mime.WordDecoder)
	decoded, err := dec.DecodeHeader(s)
	if err != nil {
		return s
	}
	return decoded
}
