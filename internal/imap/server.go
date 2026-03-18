package imap

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mustafakarli/bdsmail/config"
	"github.com/mustafakarli/bdsmail/internal/mimeutil"
	"github.com/mustafakarli/bdsmail/internal/model"
	"github.com/mustafakarli/bdsmail/internal/store"
	"github.com/mustafakarli/bdsmail/internal/tlsutil"
)

type Server struct {
	cfg          *config.Config
	store        *store.Store
	certReloader *tlsutil.CertReloader
	listener     net.Listener
}

func NewServer(cfg *config.Config, s *store.Store, certReloader *tlsutil.CertReloader) *Server {
	return &Server{cfg: cfg, store: s, certReloader: certReloader}
}

func (s *Server) Start() error {
	addr := ":" + s.cfg.IMAPPort
	log.Printf("IMAP server starting on %s", addr)

	var err error
	if s.certReloader != nil {
		s.listener, err = tls.Listen("tcp", addr, s.certReloader.TLSConfig())
	} else {
		s.listener, err = net.Listen("tcp", addr)
	}

	if err != nil {
		return fmt.Errorf("IMAP listen error: %w", err)
	}

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Printf("IMAP accept error: %v", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

type imapSession struct {
	store           *store.Store
	conn            net.Conn
	reader          *lineReader
	username        string
	authed          bool
	selectedMailbox string
	messages        []*model.Message
	mu              sync.Mutex
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Minute))

	sess := &imapSession{
		store:  s.store,
		conn:   conn,
		reader: newLineReader(conn),
	}

	sess.send("* OK BDS Mail IMAP4rev1 server ready")

	for {
		line, err := sess.reader.readLine()
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// IMAP commands are: tag command [args]
		parts := strings.SplitN(line, " ", 3)
		if len(parts) < 2 {
			sess.send("* BAD invalid command")
			continue
		}

		tag := parts[0]
		cmd := strings.ToUpper(parts[1])
		args := ""
		if len(parts) > 2 {
			args = parts[2]
		}

		switch cmd {
		case "CAPABILITY":
			sess.send("* CAPABILITY IMAP4rev1 AUTH=PLAIN")
			sess.send(fmt.Sprintf("%s OK CAPABILITY completed", tag))
		case "LOGIN":
			sess.handleLogin(tag, args)
		case "LIST":
			sess.handleList(tag, args)
		case "SELECT":
			sess.handleSelect(tag, args)
		case "EXAMINE":
			sess.handleSelect(tag, args) // same as SELECT but read-only
		case "FETCH":
			sess.handleFetch(tag, args)
		case "STORE":
			sess.handleStore(tag, args)
		case "SEARCH":
			sess.handleSearch(tag, args)
		case "NOOP":
			sess.send(fmt.Sprintf("%s OK NOOP completed", tag))
		case "LOGOUT":
			sess.send("* BYE BDS Mail server logging out")
			sess.send(fmt.Sprintf("%s OK LOGOUT completed", tag))
			return
		case "CLOSE":
			sess.handleClose(tag)
		default:
			sess.send(fmt.Sprintf("%s BAD command not supported", tag))
		}
	}
}

func (sess *imapSession) send(msg string) {
	fmt.Fprintf(sess.conn, "%s\r\n", msg)
}

func (sess *imapSession) handleLogin(tag, args string) {
	parts := parseIMAPArgs(args)
	if len(parts) < 2 {
		sess.send(fmt.Sprintf("%s BAD invalid arguments", tag))
		return
	}

	email := unquote(parts[0]) // expects user@domain
	password := unquote(parts[1])

	user, err := sess.store.DB.GetUserByEmail(email)
	if err != nil || !user.CheckPassword(password) {
		sess.send(fmt.Sprintf("%s NO authentication failed", tag))
		return
	}

	sess.username = user.Email() // normalized full email
	sess.authed = true
	sess.send(fmt.Sprintf("%s OK LOGIN completed", tag))
}

func (sess *imapSession) handleList(tag, args string) {
	if !sess.authed {
		sess.send(fmt.Sprintf("%s NO not authenticated", tag))
		return
	}

	sess.send(`* LIST (\HasNoChildren) "/" "INBOX"`)
	sess.send(`* LIST (\HasNoChildren) "/" "Sent"`)
	sess.send(fmt.Sprintf("%s OK LIST completed", tag))
}

func (sess *imapSession) handleSelect(tag, args string) {
	if !sess.authed {
		sess.send(fmt.Sprintf("%s NO not authenticated", tag))
		return
	}

	mailbox := unquote(strings.TrimSpace(args))
	if mailbox == "" {
		mailbox = "INBOX"
	}

	msgs, err := sess.store.DB.ListMessages(sess.username, mailbox)
	if err != nil {
		sess.send(fmt.Sprintf("%s NO failed to select mailbox", tag))
		return
	}

	sess.mu.Lock()
	sess.selectedMailbox = mailbox
	sess.messages = msgs
	sess.mu.Unlock()

	// Sort by date for sequence numbering
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].ReceivedAt.Before(msgs[j].ReceivedAt)
	})

	unseen := 0
	for _, m := range msgs {
		if !m.Seen {
			unseen++
		}
	}

	sess.send(fmt.Sprintf("* %d EXISTS", len(msgs)))
	sess.send("* 0 RECENT")
	if unseen > 0 {
		sess.send(fmt.Sprintf("* OK [UNSEEN %d]", findFirstUnseen(msgs)))
	}
	sess.send("* FLAGS (\\Seen \\Deleted)")
	sess.send("* OK [PERMANENTFLAGS (\\Seen \\Deleted)]")
	sess.send(fmt.Sprintf("%s OK [READ-WRITE] SELECT completed", tag))
}

func (sess *imapSession) handleFetch(tag, args string) {
	if !sess.authed || sess.selectedMailbox == "" {
		sess.send(fmt.Sprintf("%s NO no mailbox selected", tag))
		return
	}

	sess.mu.Lock()
	msgs := sess.messages
	sess.mu.Unlock()

	// Parse sequence set and data items
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		sess.send(fmt.Sprintf("%s BAD invalid arguments", tag))
		return
	}

	seqSet := parts[0]
	dataItems := strings.ToUpper(parts[1])

	indices := parseSequenceSet(seqSet, len(msgs))

	for _, idx := range indices {
		if idx < 0 || idx >= len(msgs) {
			continue
		}
		msg := msgs[idx]
		seqNum := idx + 1

		var response strings.Builder
		response.WriteString(fmt.Sprintf("* %d FETCH (", seqNum))

		items := parseDataItems(dataItems)
		first := true

		for _, item := range items {
			if !first {
				response.WriteString(" ")
			}
			first = false

			switch item {
			case "FLAGS":
				flags := ""
				if msg.Seen {
					flags = `\Seen`
				}
				response.WriteString(fmt.Sprintf("FLAGS (%s)", flags))
			case "UID":
				response.WriteString(fmt.Sprintf("UID %d", idx+1))
			case "ENVELOPE":
				response.WriteString(fmt.Sprintf(`ENVELOPE ("%s" "%s" (("%s" NIL "" "")) NIL NIL (("%s" NIL "" "")) NIL NIL NIL "%s")`,
					msg.ReceivedAt.Format("Mon, 02 Jan 2006 15:04:05 -0700"),
					escapeIMAPString(msg.Subject),
					escapeIMAPString(msg.From),
					escapeIMAPString(strings.Join(msg.To, ", ")),
					escapeIMAPString(msg.MessageID),
				))
			case "BODY[HEADER]", "BODY.PEEK[HEADER]", "RFC822.HEADER":
				header := buildHeader(msg)
				response.WriteString(fmt.Sprintf("BODY[HEADER] {%d}\r\n%s", len(header), header))
			case "BODY[]", "BODY.PEEK[]", "RFC822":
				ctx := context.Background()
				attData, _ := sess.store.LoadAttachments(ctx, msg)
				fullMsg := mimeutil.BuildFullMessage(msg, attData)
				response.WriteString(fmt.Sprintf("BODY[] {%d}\r\n%s", len(fullMsg), fullMsg))
				sess.store.DB.MarkSeen(msg.ID)
			case "BODY[TEXT]", "BODY.PEEK[TEXT]", "RFC822.TEXT":
				response.WriteString(fmt.Sprintf("BODY[TEXT] {%d}\r\n%s", len(msg.Body), msg.Body))
			case "INTERNALDATE":
				response.WriteString(fmt.Sprintf(`INTERNALDATE "%s"`, msg.ReceivedAt.Format("02-Jan-2006 15:04:05 -0700")))
			case "RFC822.SIZE":
				response.WriteString(fmt.Sprintf("RFC822.SIZE %d", estimateRFC822Size(msg)))
			default:
				response.WriteString(fmt.Sprintf("%s NIL", item))
			}
		}

		response.WriteString(")")
		sess.send(response.String())
	}

	sess.send(fmt.Sprintf("%s OK FETCH completed", tag))
}

func (sess *imapSession) handleStore(tag, args string) {
	if !sess.authed || sess.selectedMailbox == "" {
		sess.send(fmt.Sprintf("%s NO no mailbox selected", tag))
		return
	}

	sess.mu.Lock()
	msgs := sess.messages
	sess.mu.Unlock()

	parts := strings.SplitN(args, " ", 3)
	if len(parts) < 3 {
		sess.send(fmt.Sprintf("%s BAD invalid arguments", tag))
		return
	}

	seqSet := parts[0]
	// action := parts[1] // +FLAGS, -FLAGS, FLAGS
	flagsStr := parts[2]

	indices := parseSequenceSet(seqSet, len(msgs))

	for _, idx := range indices {
		if idx < 0 || idx >= len(msgs) {
			continue
		}
		msg := msgs[idx]

		if strings.Contains(strings.ToUpper(flagsStr), `\SEEN`) {
			sess.store.DB.MarkSeen(msg.ID)
			msg.Seen = true
		}
		if strings.Contains(strings.ToUpper(flagsStr), `\DELETED`) {
			sess.store.DB.MarkDeleted(msg.ID)
			msg.Deleted = true
		}

		flags := ""
		if msg.Seen {
			flags += `\Seen`
		}
		if msg.Deleted {
			if flags != "" {
				flags += " "
			}
			flags += `\Deleted`
		}
		sess.send(fmt.Sprintf("* %d FETCH (FLAGS (%s))", idx+1, flags))
	}

	sess.send(fmt.Sprintf("%s OK STORE completed", tag))
}

func (sess *imapSession) handleSearch(tag, args string) {
	if !sess.authed || sess.selectedMailbox == "" {
		sess.send(fmt.Sprintf("%s NO no mailbox selected", tag))
		return
	}

	sess.mu.Lock()
	msgs := sess.messages
	sess.mu.Unlock()

	upperArgs := strings.ToUpper(args)
	var result []string

	for i, msg := range msgs {
		match := true
		if strings.Contains(upperArgs, "UNSEEN") && msg.Seen {
			match = false
		}
		if strings.Contains(upperArgs, "SEEN") && !strings.Contains(upperArgs, "UNSEEN") && !msg.Seen {
			match = false
		}
		if match {
			result = append(result, fmt.Sprintf("%d", i+1))
		}
	}

	sess.send(fmt.Sprintf("* SEARCH %s", strings.Join(result, " ")))
	sess.send(fmt.Sprintf("%s OK SEARCH completed", tag))
}

func (sess *imapSession) handleClose(tag string) {
	// Expunge deleted messages
	sess.mu.Lock()
	for _, msg := range sess.messages {
		if msg.Deleted {
			ctx := context.Background()
			sess.store.DeleteMessageFull(ctx, msg.ID)
		}
	}
	sess.selectedMailbox = ""
	sess.messages = nil
	sess.mu.Unlock()

	sess.send(fmt.Sprintf("%s OK CLOSE completed", tag))
}

// helpers

func findFirstUnseen(msgs []*model.Message) int {
	for i, m := range msgs {
		if !m.Seen {
			return i + 1
		}
	}
	return 1
}

func buildHeader(msg *model.Message) string {
	return mimeutil.BuildHeaders(msg)
}

func estimateRFC822Size(msg *model.Message) int {
	return len(buildHeader(msg)) + 500
}

func escapeIMAPString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func parseIMAPArgs(s string) []string {
	var result []string
	var current strings.Builder
	inQuote := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inQuote = !inQuote
			current.WriteByte(c)
		} else if c == ' ' && !inQuote {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}

func parseSequenceSet(s string, total int) []int {
	var result []int
	parts := strings.Split(s, ",")
	for _, part := range parts {
		if strings.Contains(part, ":") {
			rangeParts := strings.SplitN(part, ":", 2)
			start := parseSeqNum(rangeParts[0], total)
			end := parseSeqNum(rangeParts[1], total)
			if start > end {
				start, end = end, start
			}
			for i := start; i <= end; i++ {
				result = append(result, i-1) // 0-indexed
			}
		} else {
			n := parseSeqNum(part, total)
			result = append(result, n-1)
		}
	}
	return result
}

func parseSeqNum(s string, total int) int {
	s = strings.TrimSpace(s)
	if s == "*" {
		return total
	}
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	if n < 1 {
		return 1
	}
	if n > total {
		return total
	}
	return n
}

func parseDataItems(s string) []string {
	s = strings.TrimSpace(s)
	// Handle parenthesized lists
	s = strings.TrimPrefix(s, "(")
	s = strings.TrimSuffix(s, ")")

	// Handle BODY[xxx] items
	var items []string
	i := 0
	for i < len(s) {
		if s[i] == ' ' {
			i++
			continue
		}
		start := i
		if strings.HasPrefix(strings.ToUpper(s[i:]), "BODY") || strings.HasPrefix(strings.ToUpper(s[i:]), "RFC822") {
			// Find the end including any brackets
			for i < len(s) && s[i] != ' ' {
				if s[i] == '[' {
					for i < len(s) && s[i] != ']' {
						i++
					}
					if i < len(s) {
						i++ // skip ]
					}
				} else {
					i++
				}
			}
		} else {
			for i < len(s) && s[i] != ' ' {
				i++
			}
		}
		items = append(items, strings.ToUpper(s[start:i]))
	}
	return items
}

// lineReader wraps a net.Conn for reading lines
type lineReader struct {
	conn net.Conn
	buf  []byte
	pos  int
	end  int
}

func newLineReader(conn net.Conn) *lineReader {
	return &lineReader{
		conn: conn,
		buf:  make([]byte, 4096),
	}
}

func (lr *lineReader) readLine() (string, error) {
	var line strings.Builder
	for {
		// Check buffer for newline
		for i := lr.pos; i < lr.end; i++ {
			if lr.buf[i] == '\n' {
				line.Write(lr.buf[lr.pos:i])
				lr.pos = i + 1
				return strings.TrimRight(line.String(), "\r"), nil
			}
		}
		// Consume remaining buffer
		if lr.pos < lr.end {
			line.Write(lr.buf[lr.pos:lr.end])
		}
		// Read more
		n, err := lr.conn.Read(lr.buf)
		if err != nil {
			return line.String(), err
		}
		lr.pos = 0
		lr.end = n
	}
}
