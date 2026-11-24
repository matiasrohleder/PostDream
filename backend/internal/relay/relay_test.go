package relay

import "testing"

func TestParse(t *testing.T) {
	c, err := Parse("smtp://user:p%40ss@mail.test", "")
	if err != nil || c.Addr != "mail.test:587" || c.Mode != "starttls" || c.Username != "user" || c.Password != "p@ss" {
		t.Fatalf("got %+v %v", c, err)
	}
	if c, _ := Parse("smtps://mail.test", ""); c.Addr != "mail.test:465" || c.Mode != "tls" {
		t.Fatalf("smtps: %+v", c)
	}
	if c, _ := Parse("smtp://mail.test:25?tls=none", ""); c.Mode != "none" || c.Addr != "mail.test:25" {
		t.Fatalf("none: %+v", c)
	}
	if c, _ := Parse("", ""); c != nil {
		t.Fatal("empty URL must disable relay")
	}
	for _, bad := range []string{"http://x", "smtp://x?tls=maybe", "smtp://:25"} {
		if _, err := Parse(bad, ""); err == nil {
			t.Errorf("%s: expected error", bad)
		}
	}
}

func TestCheckRecipients(t *testing.T) {
	pat := `@(acme\.test|example\.org)$`
	if err := CheckRecipients(pat, []string{"qa@acme.test", "x@example.org"}); err != nil {
		t.Fatal(err)
	}
	for _, to := range [][]string{{"x@gmail.com"}, {}, {"bad\r\n@acme.test"}} {
		if err := CheckRecipients(pat, to); err == nil {
			t.Errorf("%v: expected error", to)
		}
	}
	if err := CheckRecipients("", []string{"qa@acme.test"}); err == nil {
		t.Error("empty pattern must disable releasing")
	}
}
