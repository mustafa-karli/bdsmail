package pop3

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"strings"
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
	certStore *tlsutil.CertStore
	listener     net.Listener
}

func NewServer(cfg *config.Config, s *store.Store, certStore *tlsutil.CertStore) *Server {
	return &Server{cfg: cfg, store: s, certStore: certStore}
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.cfg.POP3Port)
	log.Printf("POP3 server starting on %s", addr)

	var err error
	if s.certStore != nil {
		s.listener, err = tls.Listen("tcp", addr, s.certStore.TLSConfig())
	} else {
		s.listener, err = net.Listen("tcp", addr)
	}

	if err != nil {
		return fmt.Errorf("POP3 listen error: %w", err)
	}

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Printf("POP3 accept error: %v", err)
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

type pop3Session struct {
	store    *store.Store
	conn     net.Conn
	reader   *bufio.Reader
	username string
	authed   bool
	messages []*model.Message
	deleted  map[int]bool
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Minute))

	sess := &pop3Session{
		store:   s.store,
		conn:    conn,
		reader:  bufio.NewReader(conn),
		deleted: make(map[int]bool),
	}

	sess.send("+OK BDS Mail POP3 server ready")

	for {
		line, err := sess.reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		cmd := strings.ToUpper(parts[0])
		arg := ""
		if len(parts) > 1 {
			arg = parts[1]
		}

		switch cmd {
		case "USER":
			sess.handleUser(arg)
		case "PASS":
			sess.handlePass(arg)
		case "STAT":
			sess.handleStat()
		case "LIST":
			sess.handleList(arg)
		case "UIDL":
			sess.handleUIDL(arg)
		case "RETR":
			sess.handleRetr(arg)
		case "DELE":
			sess.handleDele(arg)
		case "NOOP":
			sess.send("+OK")
		case "RSET":
			sess.handleRset()
		case "QUIT":
			sess.handleQuit()
			return
		default:
			sess.send("-ERR unknown command")
		}
	}
}

func (sess *pop3Session) send(msg string) {
	fmt.Fprintf(sess.conn, "%s\r\n", msg)
}

func (sess *pop3Session) handleUser(username string) {
	sess.username = strings.TrimSpace(username)
	sess.send("+OK")
}

func (sess *pop3Session) handlePass(password string) {
	if sess.username == "" {
		sess.send("-ERR send USER first")
		return
	}

	user, err := sess.store.DB.GetUserByEmail(sess.username)
	if err != nil || !user.CheckPassword(strings.TrimSpace(password)) {
		sess.send("-ERR authentication failed")
		return
	}

	sess.username = user.Email()

	msgs, err := sess.store.DB.ListMessages(sess.username, "INBOX")
	if err != nil {
		sess.send("-ERR internal error")
		return
	}

	sess.messages = msgs
	sess.authed = true
	sess.send(fmt.Sprintf("+OK %d messages", len(msgs)))
}

func (sess *pop3Session) handleStat() {
	if !sess.authed {
		sess.send("-ERR not authenticated")
		return
	}
	count := 0
	size := 0
	for i, msg := range sess.messages {
		if !sess.deleted[i] {
			count++
			size += len(msg.GCSKey)
		}
	}
	sess.send(fmt.Sprintf("+OK %d %d", count, size))
}

func (sess *pop3Session) handleList(arg string) {
	if !sess.authed {
		sess.send("-ERR not authenticated")
		return
	}

	if arg != "" {
		idx := parseIndex(arg)
		if idx < 0 || idx >= len(sess.messages) || sess.deleted[idx] {
			sess.send("-ERR no such message")
			return
		}
		sess.send(fmt.Sprintf("+OK %d %d", idx+1, estimateSize(sess.messages[idx])))
		return
	}

	sess.send(fmt.Sprintf("+OK %d messages", len(sess.messages)))
	for i, msg := range sess.messages {
		if !sess.deleted[i] {
			sess.send(fmt.Sprintf("%d %d", i+1, estimateSize(msg)))
		}
	}
	sess.send(".")
}

func (sess *pop3Session) handleUIDL(arg string) {
	if !sess.authed {
		sess.send("-ERR not authenticated")
		return
	}

	if arg != "" {
		idx := parseIndex(arg)
		if idx < 0 || idx >= len(sess.messages) || sess.deleted[idx] {
			sess.send("-ERR no such message")
			return
		}
		sess.send(fmt.Sprintf("+OK %d %s", idx+1, sess.messages[idx].ID))
		return
	}

	sess.send("+OK")
	for i, msg := range sess.messages {
		if !sess.deleted[i] {
			sess.send(fmt.Sprintf("%d %s", i+1, msg.ID))
		}
	}
	sess.send(".")
}

func (sess *pop3Session) handleRetr(arg string) {
	if !sess.authed {
		sess.send("-ERR not authenticated")
		return
	}

	idx := parseIndex(arg)
	if idx < 0 || idx >= len(sess.messages) || sess.deleted[idx] {
		sess.send("-ERR no such message")
		return
	}

	msg := sess.messages[idx]
	ctx := context.Background()
	attData, _ := sess.store.LoadAttachments(ctx, msg)
	content := mimeutil.BuildFullMessage(msg, attData)

	sess.send(fmt.Sprintf("+OK %d octets", len(content)))
	sess.send(content)
	sess.send(".")
}

func (sess *pop3Session) handleDele(arg string) {
	if !sess.authed {
		sess.send("-ERR not authenticated")
		return
	}

	idx := parseIndex(arg)
	if idx < 0 || idx >= len(sess.messages) || sess.deleted[idx] {
		sess.send("-ERR no such message")
		return
	}

	sess.deleted[idx] = true
	sess.send("+OK deleted")
}

func (sess *pop3Session) handleRset() {
	sess.deleted = make(map[int]bool)
	sess.send("+OK")
}

func (sess *pop3Session) handleQuit() {
	if sess.authed {
		ctx := context.Background()
		for idx := range sess.deleted {
			if idx < len(sess.messages) {
				sess.store.DeleteMessageFull(ctx, sess.messages[idx].ID)
			}
		}
	}
	sess.send("+OK bye")
}

func parseIndex(s string) int {
	s = strings.TrimSpace(s)
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n - 1
}

func estimateSize(msg *model.Message) int {
	return 500
}

