// Package pop3 exposes every inbox as a POP3 mailbox (RFC 1939), authenticated
// with the inbox's own SMTP credentials. Messages are marked read on RETR and
// removed on QUIT when deleted with DELE.
package pop3

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/matiasrohleder/PostDream/internal/events"
	"github.com/matiasrohleder/PostDream/internal/metrics"
	"github.com/matiasrohleder/PostDream/internal/ratelimit"
	"github.com/matiasrohleder/PostDream/internal/store"
)

// MaxMessages is how many of the newest messages a session sees.
const MaxMessages = 1000

type Server struct {
	Store     *store.Store
	Hub       *events.Hub
	TLSConfig *tls.Config // enables STLS when set
	Hostname  string

	limiter *ratelimit.Limiter
	mu      sync.Mutex
	ln      net.Listener
	conns   map[net.Conn]struct{}
	closed  bool
}

func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	s.ln = ln
	s.conns = map[net.Conn]struct{}{}
	if s.limiter == nil {
		s.limiter = ratelimit.New(10, 10*time.Minute)
	}
	s.mu.Unlock()
	for {
		c, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			return err
		}
		s.mu.Lock()
		s.conns[c] = struct{}{}
		s.mu.Unlock()
		go func() {
			defer func() {
				s.mu.Lock()
				delete(s.conns, c)
				s.mu.Unlock()
				c.Close()
			}()
			s.handle(c)
		}()
	}
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for c := range s.conns {
		c.Close()
	}
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}

type session struct {
	s       *Server
	conn    net.Conn
	r       *bufio.Reader
	w       *bufio.Writer
	ip      string
	user    string
	inbox   *store.Inbox
	msgs    []store.MessageRef
	deleted map[int]bool
	tls     bool
}

func (se *session) reply(ok bool, format string, args ...any) {
	prefix := "+OK"
	if !ok {
		prefix = "-ERR"
	}
	msg := fmt.Sprintf(format, args...)
	if msg != "" {
		prefix += " " + msg
	}
	se.w.WriteString(prefix + "\r\n")
	se.w.Flush()
}

func (s *Server) handle(c net.Conn) {
	ip, _, _ := net.SplitHostPort(c.RemoteAddr().String())
	se := &session{s: s, conn: c, r: bufio.NewReader(c), w: bufio.NewWriter(c), ip: ip, deleted: map[int]bool{}}
	host := s.Hostname
	if host == "" {
		host = "postdream"
	}
	se.reply(true, "%s PostDream POP3 ready", host)
	for {
		c.SetDeadline(time.Now().Add(10 * time.Minute))
		line, err := se.r.ReadString('\n')
		if err != nil {
			return
		}
		if len(line) > 512 {
			se.reply(false, "line too long")
			return
		}
		cmd, arg, _ := strings.Cut(strings.TrimRight(line, "\r\n"), " ")
		if quit := se.dispatch(strings.ToUpper(cmd), strings.TrimSpace(arg)); quit {
			return
		}
	}
}

func (se *session) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// msgArg parses a 1-based message number in the current listing.
func (se *session) msgArg(arg string) (int, bool) {
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 || n > len(se.msgs) || se.deleted[n] {
		se.reply(false, "no such message")
		return 0, false
	}
	return n, true
}

func (se *session) dispatch(cmd, arg string) (quit bool) {
	authed := se.inbox != nil
	switch cmd {
	case "CAPA":
		se.w.WriteString("+OK Capability list follows\r\nUSER\r\nUIDL\r\nTOP\r\nRESP-CODES\r\n")
		if se.s.TLSConfig != nil && !se.tls && !authed {
			se.w.WriteString("STLS\r\n")
		}
		se.w.WriteString(".\r\n")
		se.w.Flush()
	case "STLS":
		if se.s.TLSConfig == nil || se.tls || authed {
			se.reply(false, "STLS not available")
			return false
		}
		se.reply(true, "Begin TLS negotiation")
		tc := tls.Server(se.conn, se.s.TLSConfig)
		if err := tc.Handshake(); err != nil {
			return true
		}
		se.conn, se.tls = tc, true
		se.r, se.w = bufio.NewReader(tc), bufio.NewWriter(tc)
	case "USER":
		if authed {
			se.reply(false, "already authenticated")
			return false
		}
		se.user = arg
		se.reply(true, "send PASS")
	case "PASS":
		if authed || se.user == "" {
			se.reply(false, "send USER first")
			return false
		}
		if se.s.limiter.Blocked(se.ip) {
			se.reply(false, "[AUTH] too many failed attempts, try later")
			return true
		}
		ctx, cancel := se.ctx()
		defer cancel()
		inbox, err := se.s.Store.InboxBySMTPCredentials(ctx, se.user, arg)
		if err != nil {
			se.s.limiter.Fail(se.ip)
			metrics.Inc(metrics.POP3Logins, "failure")
			slog.Warn("pop3 auth failed", "ip", se.ip, "user", se.user)
			time.Sleep(500 * time.Millisecond)
			se.reply(false, "[AUTH] invalid credentials")
			return false
		}
		se.s.limiter.Reset(se.ip)
		msgs, err := se.s.Store.ListMessageRefs(ctx, inbox.ID, MaxMessages)
		if err != nil {
			se.reply(false, "[SYS/TEMP] mailbox unavailable")
			return false
		}
		metrics.Inc(metrics.POP3Logins, "success")
		se.inbox, se.msgs = inbox, msgs
		se.reply(true, "%s has %d message(s)", inbox.Slug, len(msgs))
	case "QUIT":
		if authed {
			se.commit()
		}
		se.reply(true, "bye")
		return true
	case "NOOP":
		if !authed {
			se.reply(false, "not authenticated")
			return false
		}
		se.reply(true, "")
	default:
		if !authed {
			se.reply(false, "not authenticated")
			return false
		}
		se.transaction(cmd, arg)
	}
	return false
}

func (se *session) transaction(cmd, arg string) {
	switch cmd {
	case "STAT":
		n, size := 0, 0
		for i, m := range se.msgs {
			if !se.deleted[i+1] {
				n++
				size += m.Size
			}
		}
		se.reply(true, "%d %d", n, size)
	case "LIST", "UIDL":
		value := func(i int) string {
			if cmd == "LIST" {
				return strconv.Itoa(se.msgs[i-1].Size)
			}
			return se.msgs[i-1].ID
		}
		if arg != "" {
			if n, ok := se.msgArg(arg); ok {
				se.reply(true, "%d %s", n, value(n))
			}
			return
		}
		se.w.WriteString("+OK\r\n")
		for i := range se.msgs {
			if !se.deleted[i+1] {
				fmt.Fprintf(se.w, "%d %s\r\n", i+1, value(i+1))
			}
		}
		se.w.WriteString(".\r\n")
		se.w.Flush()
	case "RETR", "TOP":
		numArg, linesArg, _ := strings.Cut(arg, " ")
		n, ok := se.msgArg(numArg)
		if !ok {
			return
		}
		maxLines := -1
		if cmd == "TOP" {
			l, err := strconv.Atoi(strings.TrimSpace(linesArg))
			if err != nil || l < 0 {
				se.reply(false, "usage: TOP msg lines")
				return
			}
			maxLines = l
		}
		ctx, cancel := se.ctx()
		defer cancel()
		id := se.msgs[n-1].ID
		raw, _, err := se.s.Store.GetMessageRaw(ctx, id)
		if err != nil {
			se.reply(false, "message no longer available")
			return
		}
		se.w.WriteString("+OK message follows\r\n")
		writeDotStuffed(se.w, raw, maxLines)
		se.w.WriteString(".\r\n")
		se.w.Flush()
		if cmd == "RETR" {
			if n, _ := se.s.Store.SetRead(ctx, []string{id}, true); n > 0 && se.s.Hub != nil {
				se.s.Hub.Publish(events.Event{Type: "messages.changed", InboxID: se.inbox.ID})
			}
		}
	case "DELE":
		if n, ok := se.msgArg(arg); ok {
			se.deleted[n] = true
			se.reply(true, "message %d deleted", n)
		}
	case "RSET":
		se.deleted = map[int]bool{}
		se.reply(true, "")
	default:
		se.reply(false, "unknown command")
	}
}

// commit applies DELE marks (UPDATE state, RFC 1939 section 6).
func (se *session) commit() {
	var ids []string
	for n := range se.deleted {
		ids = append(ids, se.msgs[n-1].ID)
	}
	if len(ids) == 0 {
		return
	}
	ctx, cancel := se.ctx()
	defer cancel()
	if _, err := se.s.Store.DeleteMessages(ctx, ids); err != nil {
		slog.Error("pop3 delete failed", "err", err)
		return
	}
	if se.s.Hub != nil {
		se.s.Hub.Publish(events.Event{Type: "messages.changed", InboxID: se.inbox.ID})
	}
}

// writeDotStuffed writes raw with CRLF line endings and leading dots doubled.
// maxLines >= 0 limits the body to that many lines (TOP).
func writeDotStuffed(w *bufio.Writer, raw []byte, maxLines int) {
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	inBody, bodyLines := false, 0
	for _, l := range lines {
		if inBody && maxLines >= 0 {
			if bodyLines >= maxLines {
				break
			}
			bodyLines++
		}
		if strings.HasPrefix(l, ".") {
			l = "." + l
		}
		w.WriteString(l + "\r\n")
		if !inBody && l == "" {
			inBody = true
		}
	}
}
