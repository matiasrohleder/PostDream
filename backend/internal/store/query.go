package store

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Term is one element of a search query: a plain word/phrase (Field == "") or a
// field filter such as from:alice, is:unread, larger:2M. Neg negates it.
type Term struct {
	Field string
	Value string
	Neg   bool
}

// SearchFields lists the supported field prefixes (also used by the UI help).
var SearchFields = []string{
	"from", "to", "cc", "bcc", "reply-to", "addressed", "subject", "message-id", "tag", "is", "has",
	"larger", "smaller", "before", "after", "inbox", "smtp-user", "ip", "header", "spam",
}

func knownField(f string) bool {
	for _, k := range SearchFields {
		if k == f {
			return true
		}
	}
	return false
}

// ParseSearch splits a query like `from:alice "password reset" -is:read larger:1M`
// into terms. Unknown prefixes (e.g. "https://x") are kept as plain words.
// Values are validated so that SQL generation cannot fail later.
func ParseSearch(q string) ([]Term, error) {
	var terms []Term
	for _, tok := range tokenize(q) {
		if !tok.quotedTail && (tok.text == "-" || tok.text == "!") {
			continue // incomplete negation while typing
		}
		if i := strings.IndexByte(tok.text, ':'); !tok.quotedTail && i > 0 && i == len(tok.text)-1 &&
			knownField(strings.ToLower(strings.TrimLeft(tok.text[:i], "-!"))) {
			continue // field without a value yet ("from:")
		}
		t := Term{}
		if strings.HasPrefix(tok.text, "-") || strings.HasPrefix(tok.text, "!") {
			if len(tok.text) > 1 || tok.quotedTail {
				t.Neg = true
				tok.text = tok.text[1:]
			}
		}
		if i := strings.IndexByte(tok.text, ':'); i > 0 && (i < len(tok.text)-1 || tok.quotedTail) && knownField(strings.ToLower(tok.text[:i])) {
			t.Field = strings.ToLower(tok.text[:i])
			t.Value = tok.text[i+1:]
		} else {
			t.Value = tok.text
		}
		if t.Value == "" {
			continue
		}
		if err := validateTerm(&t); err != nil {
			return nil, err
		}
		terms = append(terms, t)
	}
	return terms, nil
}

type token struct {
	text       string
	quotedTail bool // the token ended with a quoted part (value may contain spaces)
}

// tokenize splits on whitespace, honouring double quotes anywhere in a token:
// `subject:"hello world"` -> `subject:hello world`.
func tokenize(q string) []token {
	var out []token
	var cur strings.Builder
	inQuote, quoted, has := false, false, false
	flush := func() {
		if has {
			out = append(out, token{text: cur.String(), quotedTail: quoted})
		}
		cur.Reset()
		inQuote, quoted, has = false, false, false
	}
	for _, r := range q {
		switch {
		case r == '"':
			inQuote = !inQuote
			quoted = true
			has = true
		case unicode.IsSpace(r) && !inQuote:
			flush()
		default:
			cur.WriteRune(r)
			has = true
		}
	}
	flush()
	return out
}

func validateTerm(t *Term) error {
	switch t.Field {
	case "is":
		v := strings.ToLower(t.Value)
		if v != "read" && v != "unread" && v != "tagged" && v != "spam" {
			return fmt.Errorf("is: expects read, unread, tagged or spam")
		}
		t.Value = v
	case "has":
		v := strings.ToLower(t.Value)
		if v == "attachments" {
			v = "attachment"
		}
		if v != "attachment" && v != "inline" && v != "html" {
			return fmt.Errorf("has: expects attachment, inline or html")
		}
		t.Value = v
	case "larger", "smaller":
		if _, err := ParseSize(t.Value); err != nil {
			return fmt.Errorf("%s: %v", t.Field, err)
		}
	case "before", "after":
		if _, err := ParseDate(t.Value, time.Now()); err != nil {
			return fmt.Errorf("%s: %v", t.Field, err)
		}
	case "spam":
		if _, _, err := parseComparison(t.Value); err != nil {
			return fmt.Errorf("spam: %v", err)
		}
	}
	return nil
}

// ParseSize parses "500", "10K", "2M", "1.5MB" into bytes.
func ParseSize(v string) (int64, error) {
	s := strings.ToUpper(strings.TrimSpace(v))
	s = strings.TrimSuffix(s, "B")
	mult := 1.0
	switch {
	case strings.HasSuffix(s, "K"):
		mult, s = 1024, s[:len(s)-1]
	case strings.HasSuffix(s, "M"):
		mult, s = 1024*1024, s[:len(s)-1]
	case strings.HasSuffix(s, "G"):
		mult, s = 1024*1024*1024, s[:len(s)-1]
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0, fmt.Errorf("invalid size %q (use e.g. 500K, 2M)", v)
	}
	return int64(f * mult), nil
}

// ParseDate accepts yyyy/mm/dd, yyyy-mm-dd, RFC3339 or a relative age like 30m, 2h, 3d, 1w
// (meaning "that long ago").
func ParseDate(v string, now time.Time) (time.Time, error) {
	for _, layout := range []string{"2006/01/02", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, v, time.UTC); err == nil {
			return t, nil
		}
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	if len(v) >= 2 {
		n, err := strconv.Atoi(v[:len(v)-1])
		if err == nil && n >= 0 {
			unit := map[byte]time.Duration{'m': time.Minute, 'h': time.Hour, 'd': 24 * time.Hour, 'w': 7 * 24 * time.Hour}[v[len(v)-1]]
			if unit > 0 {
				return now.Add(-time.Duration(n) * unit), nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q (use yyyy/mm/dd or an age like 2h, 3d)", v)
}

// parseComparison parses ">5", ">=2.5", "<1", "=3" or "4" (meaning >=4).
func parseComparison(v string) (string, float64, error) {
	op := ">="
	for _, o := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(v, o) {
			op, v = o, v[len(o):]
			break
		}
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid number %q (use e.g. >5)", v)
	}
	return op, f, nil
}

// termSQL returns the SQL condition for one term. arg registers a bind parameter.
func termSQL(t Term, arg func(any) string, now time.Time) string {
	like := func() string { return arg(likeEscape(t.Value)) }
	var c string
	switch t.Field {
	case "":
		c = "m.search_text LIKE " + arg(likeEscape(strings.ToLower(t.Value)))
	case "from":
		p := like()
		c = "(m.from_header ILIKE " + p + " OR m.envelope_from ILIKE " + p + ")"
	case "to":
		p := like()
		c = "(m.to_header ILIKE " + p + " OR array_to_string(m.envelope_to, ' ') ILIKE " + p + ")"
	case "cc":
		c = "m.cc_header ILIKE " + like()
	case "bcc":
		// Envelope recipients that appear in neither To nor Cc.
		c = "EXISTS (SELECT 1 FROM unnest(m.envelope_to) r WHERE r ILIKE " + like() +
			" AND position(lower(r) in lower(m.to_header || ' ' || m.cc_header)) = 0)"
	case "reply-to":
		c = "COALESCE((m.headers->'Reply-To')::text, '') ILIKE " + like()
	case "addressed":
		p := like()
		c = "(m.from_header ILIKE " + p + " OR m.to_header ILIKE " + p + " OR m.cc_header ILIKE " + p +
			" OR m.envelope_from ILIKE " + p + " OR array_to_string(m.envelope_to, ' ') ILIKE " + p + ")"
	case "subject":
		c = "m.subject ILIKE " + like()
	case "message-id":
		c = "m.message_id ILIKE " + arg(likeEscape(strings.Trim(t.Value, "<>")))
	case "tag":
		c = "EXISTS (SELECT 1 FROM message_tags mt WHERE mt.message_id = m.id AND lower(mt.tag) = lower(" + arg(t.Value) + "))"
	case "is":
		switch t.Value {
		case "read":
			c = "m.is_read"
		case "unread":
			c = "NOT m.is_read"
		case "tagged":
			c = "EXISTS (SELECT 1 FROM message_tags mt WHERE mt.message_id = m.id)"
		case "spam":
			c = "COALESCE((m.spam_report->>'is_spam')::boolean, false)"
		}
	case "has":
		switch t.Value {
		case "attachment":
			c = "m.attachment_count > 0"
		case "inline":
			c = "EXISTS (SELECT 1 FROM attachments a WHERE a.message_id = m.id AND a.inline)"
		case "html":
			c = "m.html_body <> ''"
		}
	case "larger":
		n, _ := ParseSize(t.Value)
		c = "m.size > " + arg(n)
	case "smaller":
		n, _ := ParseSize(t.Value)
		c = "m.size < " + arg(n)
	case "after":
		d, _ := ParseDate(t.Value, now)
		c = "m.received_at >= " + arg(d)
	case "before":
		d, _ := ParseDate(t.Value, now)
		c = "m.received_at < " + arg(d)
	case "inbox":
		c = "EXISTS (SELECT 1 FROM inboxes ib WHERE ib.id = m.inbox_id AND (lower(ib.slug) = lower(" + arg(t.Value) +
			") OR ib.name ILIKE " + like() + "))"
	case "smtp-user":
		c = "m.smtp_user ILIKE " + like()
	case "ip":
		c = "m.client_ip LIKE " + arg(likePrefix(strings.TrimSuffix(t.Value, "*")))
	case "header":
		// header:Name (exists) or header:Name=value (contains).
		name, val, hasVal := strings.Cut(t.Value, "=")
		c = "EXISTS (SELECT 1 FROM jsonb_each(m.headers) h WHERE lower(h.key) = lower(" + arg(name) + ")"
		if hasVal {
			c += " AND h.value::text ILIKE " + arg(likeEscape(val))
		}
		c += ")"
	case "spam":
		op, f, _ := parseComparison(t.Value)
		c = "m.spam_score " + op + " " + arg(f)
	}
	if t.Neg {
		return "NOT COALESCE(" + c + ", false)"
	}
	return c
}

// likePrefix escapes s for LIKE and matches it as a prefix.
func likePrefix(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s) + "%"
}
