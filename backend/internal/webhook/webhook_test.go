package webhook

import (
	"testing"
	"time"
)

func TestSignVerify(t *testing.T) {
	body := []byte(`{"event":"ping"}`)
	now := time.Unix(1_700_000_000, 0)
	h := Sign("whsec_x", now.Unix(), body)
	if !Verify("whsec_x", h, body, 5*time.Minute, now) {
		t.Fatal("valid signature rejected")
	}
	if Verify("whsec_y", h, body, 5*time.Minute, now) {
		t.Fatal("wrong secret accepted")
	}
	if Verify("whsec_x", h, []byte(`{"event":"pong"}`), 5*time.Minute, now) {
		t.Fatal("tampered body accepted")
	}
	if Verify("whsec_x", h, body, 5*time.Minute, now.Add(10*time.Minute)) {
		t.Fatal("stale timestamp accepted")
	}
}
