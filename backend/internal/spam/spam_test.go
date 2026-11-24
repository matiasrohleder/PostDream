package spam

import (
	"strings"
	"testing"
)

const spamdReply = "SPAMD/1.1 0 EX_OK\r\nContent-length: 600\r\nSpam: True ; 7.3 / 5.0\r\n\r\n" +
	"Spam detection software, running on the system \"x\", has\r\n" +
	"identified this incoming email as possible spam.\r\n\r\n" +
	"Content analysis details:   (7.3 points, 5.0 required)\r\n\r\n" +
	" pts rule name              description\r\n" +
	"---- ---------------------- --------------------------------------------------\r\n" +
	" 0.0 URIBL_BLOCKED          ADMINISTRATOR NOTICE: The query to URIBL was\r\n" +
	"                            blocked.\r\n" +
	" 1.4 MISSING_DATE           Missing Date: header\r\n" +
	" 5.9 GTUBE                  BODY: Generic Test for Unsolicited Bulk Email\r\n" +
	"-0.1 DKIM_VALID             Message has at least one valid DKIM signature\r\n"

func TestParseSpamd(t *testing.T) {
	r, err := parseSpamd(strings.NewReader(spamdReply))
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsSpam || r.Score != 7.3 || r.Threshold != 5 {
		t.Fatalf("bad header parse: %+v", r)
	}
	if len(r.Rules) != 4 || r.Rules[0].Name != "GTUBE" {
		t.Fatalf("rules not parsed/sorted: %+v", r.Rules)
	}
	for _, rule := range r.Rules {
		if rule.Name == "URIBL_BLOCKED" && !strings.HasSuffix(rule.Description, "blocked.") {
			t.Errorf("continuation line not joined: %q", rule.Description)
		}
	}
}

func TestParseSpamdError(t *testing.T) {
	if _, err := parseSpamd(strings.NewReader("SPAMD/1.0 76 Bad header line\r\n")); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseRspamd(t *testing.T) {
	body := `{"is_skipped":false,"score":16.5,"required_score":15,"action":"reject",
	"symbols":{"GTUBE":{"name":"GTUBE","score":15,"description":"GTUBE test"},
	"MISSING_DATE":{"name":"MISSING_DATE","score":1.5,"options":["x"]}}}`
	r, err := parseRspamd([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsSpam || r.Score != 16.5 || r.Action != "reject" || len(r.Rules) != 2 || r.Rules[0].Name != "GTUBE" {
		t.Fatalf("unexpected: %+v", r)
	}
	if r.Rules[1].Description != "[x]" {
		t.Errorf("options not appended: %q", r.Rules[1].Description)
	}
}
