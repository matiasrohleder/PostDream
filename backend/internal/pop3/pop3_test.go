package pop3

import (
	"bufio"
	"strings"
	"testing"
)

func TestWriteDotStuffed(t *testing.T) {
	raw := "Subject: x\nFrom: a@b\n\n.line one\nline two\nline three\n"
	var b strings.Builder
	w := bufio.NewWriter(&b)
	writeDotStuffed(w, []byte(raw), -1)
	w.Flush()
	want := "Subject: x\r\nFrom: a@b\r\n\r\n..line one\r\nline two\r\nline three\r\n"
	if b.String() != want {
		t.Fatalf("got %q", b.String())
	}

	b.Reset()
	writeDotStuffed(w, []byte(raw), 1)
	w.Flush()
	if want := "Subject: x\r\nFrom: a@b\r\n\r\n..line one\r\n"; b.String() != want {
		t.Fatalf("TOP 1: got %q", b.String())
	}
}
