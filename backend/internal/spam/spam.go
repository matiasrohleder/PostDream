// Package spam scores messages with an external engine: SpamAssassin (spamd
// protocol) or Rspamd (HTTP /checkv2). The engine address comes from the
// operator's configuration, so it is trusted and may be on a private network.
package spam

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/matiasrohleder/PostDream/internal/config"
)

// JobKind is the background job that analyses a freshly received message.
const JobKind = "spam.check"

type Rule struct {
	Name        string  `json:"name"`
	Score       float64 `json:"score"`
	Description string  `json:"description"`
}

type Report struct {
	Engine    string    `json:"engine"`
	Score     float64   `json:"score"`
	Threshold float64   `json:"threshold"`
	IsSpam    bool      `json:"is_spam"`
	Action    string    `json:"action,omitempty"`
	Rules     []Rule    `json:"rules"`
	CheckedAt time.Time `json:"checked_at"`
}

type Checker interface {
	Engine() string
	Check(ctx context.Context, raw []byte) (*Report, error)
}

// New returns the configured checker, or nil when no engine is configured.
func New(cfg *config.Config) Checker {
	switch cfg.SpamEngine() {
	case "rspamd":
		return &Rspamd{URL: cfg.RspamdURL, Password: cfg.RspamdPassword, Client: &http.Client{Timeout: 30 * time.Second}}
	case "spamassassin":
		return &SpamAssassin{Addr: cfg.SpamAssassinAddr, Timeout: 30 * time.Second}
	}
	return nil
}

// ---------- SpamAssassin ----------

type SpamAssassin struct {
	Addr    string
	Timeout time.Duration
}

func (s *SpamAssassin) Engine() string { return "spamassassin" }

func (s *SpamAssassin) Check(ctx context.Context, raw []byte) (*Report, error) {
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return nil, fmt.Errorf("connect spamd: %w", err)
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	} else {
		conn.SetDeadline(time.Now().Add(s.Timeout))
	}
	// spamd expects CRLF line endings in the message body it scores.
	body := bytes.ReplaceAll(bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n")), []byte("\n"), []byte("\r\n"))
	fmt.Fprintf(conn, "REPORT SPAMC/1.5\r\nContent-length: %d\r\nUser: postdream\r\n\r\n", len(body))
	if _, err := conn.Write(body); err != nil {
		return nil, err
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.CloseWrite()
	}
	return parseSpamd(conn)
}

var (
	spamHeaderRe = regexp.MustCompile(`(?i)^Spam:\s*(\w+)\s*;\s*(-?[\d.]+)\s*/\s*(-?[\d.]+)`)
	ruleLineRe   = regexp.MustCompile(`^\s*(-?\d+(?:\.\d+)?)\s+([A-Z0-9_]+)\s+(.*)$`)
)

func parseSpamd(r io.Reader) (*Report, error) {
	br := bufio.NewReader(r)
	status, err := br.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read spamd status: %w", err)
	}
	if !strings.Contains(status, "EX_OK") {
		return nil, fmt.Errorf("spamd error: %s", strings.TrimSpace(status))
	}
	rep := &Report{Engine: "spamassassin", Rules: []Rule{}, CheckedAt: time.Now().UTC()}
	for {
		line, err := br.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if m := spamHeaderRe.FindStringSubmatch(line); m != nil {
			rep.IsSpam = strings.EqualFold(m[1], "true") || strings.EqualFold(m[1], "yes")
			rep.Score, _ = strconv.ParseFloat(m[2], 64)
			rep.Threshold, _ = strconv.ParseFloat(m[3], 64)
		}
		if line == "" || err != nil {
			break
		}
	}
	inTable := false
	sc := bufio.NewScanner(br)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.HasPrefix(strings.TrimSpace(line), "----") {
			inTable = true
			continue
		}
		if !inTable || strings.TrimSpace(line) == "" {
			continue
		}
		if m := ruleLineRe.FindStringSubmatch(line); m != nil {
			score, _ := strconv.ParseFloat(m[1], 64)
			rep.Rules = append(rep.Rules, Rule{Name: m[2], Score: score, Description: strings.TrimSpace(m[3])})
		} else if n := len(rep.Rules); n > 0 {
			rep.Rules[n-1].Description += " " + strings.TrimSpace(line)
		}
	}
	sortRules(rep.Rules)
	return rep, nil
}

// ---------- Rspamd ----------

type Rspamd struct {
	URL      string
	Password string
	Client   *http.Client
}

func (r *Rspamd) Engine() string { return "rspamd" }

func (r *Rspamd) Check(ctx context.Context, raw []byte) (*Report, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL+"/checkv2", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if r.Password != "" {
		req.Header.Set("Password", r.Password)
	}
	res, err := r.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rspamd: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("rspamd returned HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return parseRspamd(body)
}

func parseRspamd(body []byte) (*Report, error) {
	var out struct {
		Score         float64 `json:"score"`
		RequiredScore float64 `json:"required_score"`
		Action        string  `json:"action"`
		Symbols       map[string]struct {
			Name        string   `json:"name"`
			Score       float64  `json:"score"`
			Description string   `json:"description"`
			Options     []string `json:"options"`
		} `json:"symbols"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("rspamd: invalid response: %w", err)
	}
	if out.Action == "" && out.Symbols == nil {
		return nil, errors.New("rspamd: empty response")
	}
	rep := &Report{Engine: "rspamd", Score: out.Score, Threshold: out.RequiredScore, Action: out.Action,
		Rules: []Rule{}, CheckedAt: time.Now().UTC()}
	switch out.Action {
	case "reject", "add header", "rewrite subject":
		rep.IsSpam = true
	}
	if out.RequiredScore > 0 && out.Score >= out.RequiredScore {
		rep.IsSpam = true
	}
	for name, s := range out.Symbols {
		desc := s.Description
		if len(s.Options) > 0 {
			desc = strings.TrimSpace(desc + " [" + strings.Join(s.Options, ", ") + "]")
		}
		rep.Rules = append(rep.Rules, Rule{Name: name, Score: s.Score, Description: desc})
	}
	sortRules(rep.Rules)
	return rep, nil
}

// sortRules puts the most impactful rules first.
func sortRules(rules []Rule) {
	sort.SliceStable(rules, func(i, j int) bool {
		ai, aj := rules[i].Score, rules[j].Score
		if ai < 0 {
			ai = -ai
		}
		if aj < 0 {
			aj = -aj
		}
		if ai != aj {
			return ai > aj
		}
		return rules[i].Name < rules[j].Name
	})
}
