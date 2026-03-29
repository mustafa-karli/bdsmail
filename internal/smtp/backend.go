package smtp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/mail"
	"strings"

	gosmtp "github.com/emersion/go-smtp"
	"github.com/mustafakarli/bdsmail/config"
	"github.com/mustafakarli/bdsmail/internal/filter"
	"github.com/mustafakarli/bdsmail/internal/mimeutil"
	"github.com/mustafakarli/bdsmail/internal/model"
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

	if b.checker != nil && !b.checker.AllowConnection(remoteIP) {
		return nil, &gosmtp.SMTPError{
			Code:    421,
			Message: "Too many connections from your IP, try again later",
		}
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
	requireTLS   bool
}

// AuthPlain authenticates with full email (user@domain) as username.
func (s *Session) AuthPlain(username, password string) error {
	if s.backend.checker != nil && s.backend.checker.IsLockedOut(s.remoteIP) {
		return &gosmtp.SMTPError{
			Code:    421,
			Message: "Too many failed login attempts, try again later",
		}
	}

	user, err := s.backend.store.DB.GetUserByEmail(username)
	if err != nil {
		if s.backend.checker != nil {
			s.backend.checker.RecordAuthResult(s.remoteIP, false)
		}
		return &gosmtp.SMTPError{
			Code:    535,
			Message: "Authentication failed",
		}
	}
	if !user.CheckPassword(password) {
		if s.backend.checker != nil {
			s.backend.checker.RecordAuthResult(s.remoteIP, false)
		}
		return &gosmtp.SMTPError{
			Code:    535,
			Message: "Authentication failed",
		}
	}

	if s.backend.checker != nil {
		s.backend.checker.RecordAuthResult(s.remoteIP, true)
	}
	s.authed = true
	s.user = user.Email()
	return nil
}

func (s *Session) Mail(from string, opts *gosmtp.MailOptions) error {
	s.from = from
	if opts != nil {
		s.requireTLS = opts.RequireTLS
	}
	return nil
}

func (s *Session) Rcpt(to string, opts *gosmtp.RcptOptions) error {
	s.to = append(s.to, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	rawEmail, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	msg, err := mail.ReadMessage(bytes.NewReader(rawEmail))
	if err != nil {
		return s.storeRaw(string(rawEmail))
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

	// Parse MIME to extract body and attachments
	parsed, err := mimeutil.Parse(msg)
	if err != nil {
		return s.storeRaw(string(rawEmail))
	}

	// Enforce max attachment size
	if s.backend.cfg != nil {
		maxSize := s.backend.cfg.MaxAttachmentBytes
		for _, att := range parsed.Attachments {
			if int64(len(att.Data)) > maxSize {
				return &gosmtp.SMTPError{
					Code:    552,
					Message: fmt.Sprintf("Attachment %q exceeds maximum size of %d bytes", att.Filename, maxSize),
				}
			}
		}
	}

	ctx := context.Background()

	if s.authed {
		return s.handleOutbound(ctx, rawEmail, from, to, cc, bcc, subject, parsed)
	}
	return s.handleInbound(ctx, rawEmail, from, to, cc, bcc, subject, parsed)
}

func (s *Session) handleOutbound(ctx context.Context, rawEmail []byte, from string, to, cc, bcc []string, subject string, parsed *mimeutil.ParsedEmail) error {
	// Run outbound security checks (scan body + attachments)
	if s.backend.checker != nil {
		var attData [][]byte
		for _, att := range parsed.Attachments {
			attData = append(attData, att.Data)
		}
		result := s.backend.checker.CheckOutbound(ctx, parsed.TextBody, parsed.ContentType, attData...)
		if result.Reject {
			log.Printf("blocked outbound mail from %s: %s", s.user, result.Reason)
			return &gosmtp.SMTPError{
				Code:    550,
				Message: "Message rejected: " + result.Reason,
			}
		}
	}

	// Save sender's copy in Sent folder + deliver to local recipients
	messageID, err := s.backend.store.SaveOutgoingMail(ctx, s.user, from, to, cc, bcc, subject, parsed.ContentType, parsed.TextBody, parsed.Attachments)
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
		var opts []SendOption
		if s.requireTLS {
			opts = append(opts, WithRequireTLS())
		}
		go func() {
			err := s.backend.relay.Send(s.user, external, subject, parsed.ContentType, parsed.TextBody, messageID, opts...)
			if err != nil {
				log.Printf("SMTP relay error: %v", err)
			}
		}()
	}

	return nil
}

func (s *Session) handleInbound(ctx context.Context, rawEmail []byte, from string, to, cc, bcc []string, subject string, parsed *mimeutil.ParsedEmail) error {
	folder := "INBOX"

	// Run inbound security checks (scan body + attachments)
	if s.backend.checker != nil {
		result := s.backend.checker.CheckInbound(ctx, rawEmail, parsed.TextBody, parsed.ContentType, s.remoteIP, from, s.ehloHostname)
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

	// Check mailing lists — distribute to all members
	allRecipients := append(append(to, cc...), bcc...)
	for _, addr := range allRecipients {
		if s.backend.store.DB.IsMailingList(addr) {
			if err := s.backend.store.DistributeToList(ctx, addr, from, to, cc, bcc, subject, parsed.ContentType, parsed.TextBody, parsed.Attachments); err != nil {
				log.Printf("mailing list distribution error for %s: %v", addr, err)
			}
			// Remove list address from recipients so it's not delivered as normal
			to = removeAddr(to, addr)
			cc = removeAddr(cc, addr)
			bcc = removeAddr(bcc, addr)
		}
	}

	// Apply filters per recipient and save
	if len(to) > 0 || len(cc) > 0 || len(bcc) > 0 {
		// Parse raw headers for filter matching
		rawHeaders := parseRawHeaders(rawEmail)

		// Build a temporary message for filter evaluation
		tmpMsg := &model.Message{
			From:        from,
			To:          to,
			CC:          cc,
			Subject:     subject,
			ContentType: parsed.ContentType,
			Body:        parsed.TextBody,
		}
		for _, att := range parsed.Attachments {
			tmpMsg.Attachments = append(tmpMsg.Attachments, model.Attachment{
				Filename: att.Filename,
				Size:     int64(len(att.Data)),
			})
		}

		// Resolve recipients (aliases) and deliver with per-user filters
		allAddrs := append(append(to, cc...), bcc...)
		delivered := make(map[string]bool)
		for _, addr := range allAddrs {
			resolved := s.backend.store.ResolveRecipient(addr, 0)
			for _, target := range resolved {
				if delivered[target] {
					continue
				}
				delivered[target] = true

				userFolder := folder
				userSeen := false

				// Apply user filters
				filters, err := s.backend.store.DB.ListFilters(target)
				if err == nil && len(filters) > 0 {
					result := filter.Apply(filters, tmpMsg, rawHeaders)
					if result.Delete {
						continue // skip delivery
					}
					if result.Folder != "" {
						userFolder = result.Folder
					}
					if result.MarkRead {
						userSeen = true
					}
				}

				if err := s.backend.store.SaveIncomingMailForUser(ctx, target, from, to, cc, subject, parsed.ContentType, parsed.TextBody, userFolder, userSeen, parsed.Attachments); err != nil {
					log.Printf("failed to save mail for %s: %v", target, err)
				}

				// Auto-reply in background
				go s.backend.store.ProcessAutoReply(context.Background(), target, from, s.backend.relay)
			}
		}
	}

	return nil
}

func removeAddr(addrs []string, target string) []string {
	var result []string
	for _, a := range addrs {
		if a != target {
			result = append(result, a)
		}
	}
	return result
}

func parseRawHeaders(rawEmail []byte) map[string]string {
	headers := make(map[string]string)
	lines := strings.SplitN(string(rawEmail), "\r\n\r\n", 2)
	if len(lines) == 0 {
		return headers
	}
	for _, line := range strings.Split(lines[0], "\r\n") {
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.ToLower(strings.TrimSpace(line[:idx]))
			value := strings.TrimSpace(line[idx+1:])
			headers[key] = value
		}
	}
	return headers
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
	return s.backend.store.SaveIncomingMail(ctx, s.from, s.to, nil, nil, "(no subject)", "text/plain", body, "INBOX", nil)
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
