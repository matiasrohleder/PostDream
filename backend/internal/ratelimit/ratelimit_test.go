package ratelimit

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	l := New(3, time.Minute)
	for i := 0; i < 3; i++ {
		if l.Blocked("ip") {
			t.Fatal("blocked too early")
		}
		l.Fail("ip")
	}
	if !l.Blocked("ip") {
		t.Fatal("should be blocked")
	}
	l.Reset("ip")
	if l.Blocked("ip") {
		t.Fatal("reset should unblock")
	}
}
