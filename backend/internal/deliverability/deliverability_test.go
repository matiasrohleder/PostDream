package deliverability

import (
	"context"
	"testing"
	"time"
)

func statuses(r *Report) map[string]Status {
	m := map[string]Status{}
	for _, c := range r.Checks {
		m[c.ID] = c.Status
	}
	return m
}

func TestRunGoodMessage(t *testing.T) {
	raw := "From: Shop <news@mail.shop.test>\r\nTo: a@b.test\r\nSubject: Hi\r\nDate: Mon, 16 Nov 2026 10:00:00 +0000\r\n" +
		"Message-ID: <abc@shop.test>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain\r\n" +
		"List-Unsubscribe: <https://shop.test/u/1>, <mailto:u@shop.test>\r\nList-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n\r\nhello\r\n"
	r := Run(context.Background(), Input{Raw: []byte(raw), EnvelopeFrom: "bounce@shop.test",
		ReceivedAt: time.Date(2026, 11, 16, 10, 0, 1, 0, time.UTC)}, Options{})
	s := statuses(r)
	for id, want := range map[string]Status{"dkim": Info, "from": Pass, "alignment": Pass, "message-id": Pass, "date": Pass,
		"subject": Pass, "list-unsubscribe": Pass} {
		if s[id] != want {
			t.Errorf("%s: got %q want %q", id, s[id], want)
		}
	}
	if r.Fail != 0 {
		t.Errorf("unexpected failures: %+v", r.Checks)
	}
}

func TestRunBadMessage(t *testing.T) {
	raw := "From: nope\r\nSubject: Ünïcode\r\nContent-Type: text/html\r\nList-Unsubscribe: <https://x.test/u>\r\n" +
		"DKIM-Signature: v=1; a=rsa-sha256; d=x.test; s=s1; h=from; bh=AAAA; b=AAAA\r\n\r\n<p>x</p>"
	lookup := func(context.Context, string) ([]string, error) { return []string{"v=DKIM1; p="}, nil }
	r := Run(context.Background(), Input{Raw: []byte(raw), EnvelopeFrom: "a@other.test", HasHTML: true},
		Options{LookupTXT: lookup})
	s := statuses(r)
	for id, want := range map[string]Status{"dkim": Fail, "from": Fail, "message-id": Warn, "date": Warn,
		"mime-version": Warn, "text-part": Warn, "list-unsubscribe": Warn, "header-encoding": Warn} {
		if s[id] != want {
			t.Errorf("%s: got %q want %q", id, s[id], want)
		}
	}
}
