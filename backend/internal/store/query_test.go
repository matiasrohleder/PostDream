package store

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseSearch(t *testing.T) {
	got, err := ParseSearch(`from:alice "password reset" -is:read subject:"hello world" !has:attachment https://x.test/a:b tag:Welcome larger:1.5M`)
	if err != nil {
		t.Fatal(err)
	}
	want := []Term{
		{Field: "from", Value: "alice"},
		{Value: "password reset"},
		{Field: "is", Value: "read", Neg: true},
		{Field: "subject", Value: "hello world"},
		{Field: "has", Value: "attachment", Neg: true},
		{Value: "https://x.test/a:b"},
		{Field: "tag", Value: "Welcome"},
		{Field: "larger", Value: "1.5M"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestParseSearchErrors(t *testing.T) {
	for _, q := range []string{"is:maybe", "has:pdf", "larger:huge", "before:yesterday", "spam:lots"} {
		if _, err := ParseSearch(q); err == nil {
			t.Errorf("%q: expected an error", q)
		}
	}
	// A lone "-" or an empty field value is ignored, not an error.
	if terms, err := ParseSearch(`- from: "" plain`); err != nil || len(terms) != 1 || terms[0].Value != "plain" {
		t.Errorf("got %+v %v", terms, err)
	}
}

func TestParseSizeAndDate(t *testing.T) {
	for in, want := range map[string]int64{"500": 500, "10K": 10240, "2M": 2 << 20, "1.5MB": 1572864, "1g": 1 << 30} {
		if got, err := ParseSize(in); err != nil || got != want {
			t.Errorf("ParseSize(%q)=%d,%v want %d", in, got, err, want)
		}
	}
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Time{
		"2026/11/01": time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		"2026-11-01": time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		"2h":         now.Add(-2 * time.Hour),
		"3d":         now.Add(-72 * time.Hour),
		"1w":         now.Add(-7 * 24 * time.Hour),
	} {
		if got, err := ParseDate(in, now); err != nil || !got.Equal(want) {
			t.Errorf("ParseDate(%q)=%v,%v want %v", in, got, err, want)
		}
	}
}

func TestTermSQLIsParameterized(t *testing.T) {
	terms, _ := ParseSearch(`from:"x'; DROP TABLE messages; --" header:X-Env=prod%_ -tag:a spam:>5 ip:10.0.* inbox:qa`)
	f := MessageFilter{InboxIDs: []string{"a"}, Terms: terms}
	where, args := f.where()
	if strings.Contains(where, "DROP") || strings.Contains(where, "prod") {
		t.Fatalf("user input leaked into SQL: %s", where)
	}
	if !strings.Contains(where, "NOT COALESCE(EXISTS (SELECT 1 FROM message_tags") {
		t.Errorf("negated tag not generated: %s", where)
	}
	want := map[string]bool{`%prod\%\_%`: false, `10.0.%`: false}
	for _, a := range args {
		if s, ok := a.(string); ok {
			if _, tracked := want[s]; tracked {
				want[s] = true
			}
		}
	}
	for v, seen := range want {
		if !seen {
			t.Errorf("expected escaped arg %q in %v", v, args)
		}
	}
	if n := maxPlaceholder(where); n != len(args) {
		t.Errorf("highest placeholder $%d but %d args: %s", n, len(args), where)
	}
}

func TestNormalizeTags(t *testing.T) {
	got := NormalizeTags([]string{" b ", "A", "a", "", "x\x00y", strings.Repeat("z", 101)})
	if !reflect.DeepEqual(got, []string{"A", "b", "xy"}) {
		t.Fatalf("got %v", got)
	}
}

func FuzzParseSearch(f *testing.F) {
	for _, s := range []string{`from:a "b c" -is:read`, `larger:2M before:2026/01/01`, `"unterminated`, `!`, `header:a=b`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, q string) {
		terms, err := ParseSearch(q)
		if err != nil {
			return
		}
		mf := MessageFilter{Terms: terms}
		where, args := mf.where()
		if n := maxPlaceholder(where); n != len(args) {
			t.Fatalf("placeholder mismatch for %q: $%d vs %d args: %s", q, n, len(args), where)
		}
	})
}

var placeholderRe = regexp.MustCompile(`\$(\d+)`)

func maxPlaceholder(sql string) int {
	n := 0
	for _, m := range placeholderRe.FindAllStringSubmatch(sql, -1) {
		if v, _ := strconv.Atoi(m[1]); v > n {
			n = v
		}
	}
	return n
}
