package smtpd

import (
	"testing"

	"github.com/emersion/go-smtp"

	"github.com/matiasrohleder/PostDream/internal/store"
)

func TestLoginServer(t *testing.T) {
	var gotUser, gotPass string
	l := &loginServer{authenticate: func(u, p string) error { gotUser, gotPass = u, p; return nil }}
	if c, done, _ := l.Next(nil); done || string(c) != "Username:" {
		t.Fatalf("step0: %q %v", c, done)
	}
	if c, done, _ := l.Next([]byte("inbox")); done || string(c) != "Password:" {
		t.Fatalf("step1: %q %v", c, done)
	}
	if _, done, err := l.Next([]byte("secret")); !done || err != nil {
		t.Fatalf("step2: %v %v", done, err)
	}
	if gotUser != "inbox" || gotPass != "secret" {
		t.Fatalf("got %q/%q", gotUser, gotPass)
	}
}

func TestLoginServerInitialResponse(t *testing.T) {
	var gotUser string
	l := &loginServer{authenticate: func(u, p string) error { gotUser = u; return nil }}
	if c, done, _ := l.Next([]byte("inbox")); done || string(c) != "Password:" {
		t.Fatalf("step0: %q %v", c, done)
	}
	if _, done, _ := l.Next([]byte("pw")); !done || gotUser != "inbox" {
		t.Fatal("expected auth with initial response")
	}
}

func TestChaos(t *testing.T) {
	b := &Backend{rand: func(int) int { return 50 }}
	inbox := &store.Inbox{}
	inbox.Chaos.Rcpt = &store.ChaosRule{Code: 451, Probability: 60}
	inbox.Chaos.Data = &store.ChaosRule{Code: 552, Probability: 40}
	inbox.Chaos.Mail = &store.ChaosRule{Code: 200, Probability: 100} // invalid code: ignored
	if err := b.chaos(inbox, "rcpt"); err == nil {
		t.Fatal("rcpt: roll 50 < 60 must fail")
	} else if se, ok := err.(*smtp.SMTPError); !ok || se.Code != 451 || se.EnhancedCode[0] != 4 {
		t.Fatalf("unexpected error %#v", err)
	}
	if err := b.chaos(inbox, "data"); err != nil {
		t.Fatal("data: roll 50 >= 40 must pass")
	}
	if err := b.chaos(inbox, "mail"); err != nil {
		t.Fatal("invalid code must be ignored")
	}
	if err := b.chaos(nil, "auth"); err != nil {
		t.Fatal("nil inbox must pass")
	}
}
