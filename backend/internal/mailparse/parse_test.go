package mailparse

import (
	"strings"
	"testing"
	"unicode/utf8"
)

const multipart = "From: \"Ana\" <ana@example.com>\r\n" +
	"To: bob@example.com\r\n" +
	"Subject: =?UTF-8?B?SG9sYSBtdW5kbyDwn5GL?=\r\n" +
	"Message-ID: <abc@test>\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=XX\r\n\r\n" +
	"--XX\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nhola texto\r\n" +
	"--XX\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>hola html</p>\r\n" +
	"--XX\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"doc.pdf\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\nJVBERi0xLjQ=\r\n--XX--\r\n"

func TestParseMultipart(t *testing.T) {
	p := Parse([]byte(multipart))
	if p.Subject != "Hola mundo 👋" {
		t.Errorf("subject decoded wrong: %q", p.Subject)
	}
	if p.MessageID != "abc@test" {
		t.Errorf("message id: %q", p.MessageID)
	}
	if !strings.Contains(p.Text, "hola texto") || !strings.Contains(p.HTML, "hola html") {
		t.Errorf("bodies: %q / %q", p.Text, p.HTML)
	}
	if len(p.Attachments) != 1 || p.Attachments[0].Filename != "doc.pdf" || string(p.Attachments[0].Data) != "%PDF-1.4" {
		t.Errorf("attachments: %+v", p.Attachments)
	}
}

func TestParseGarbageNeverFails(t *testing.T) {
	p := Parse([]byte("\x00\xff not a mail"))
	if strings.ContainsRune(p.Text, 0) {
		t.Error("NUL bytes must be stripped for Postgres")
	}
}

// FuzzParse checks the parser never panics and always returns storable text.
func FuzzParse(f *testing.F) {
	f.Add([]byte("Subject: hi\r\n\r\nbody"))
	f.Add([]byte("Content-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\nContent-Type: text/html\r\n\r\n<p>\x00</p>\r\n--x--"))
	f.Add([]byte{0xff, 0xfe, 0x00})
	f.Fuzz(func(t *testing.T, raw []byte) {
		p := Parse(raw)
		for _, s := range []string{p.Subject, p.Text, p.HTML, p.From, p.To} {
			if strings.Contains(s, "\x00") || !utf8.ValidString(s) {
				t.Fatalf("unsafe output for %q", raw)
			}
		}
	})
}
