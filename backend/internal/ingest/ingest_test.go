package ingest

import (
	"reflect"
	"testing"

	"github.com/matiasrohleder/PostDream/internal/mailparse"
)

func TestAutoTags(t *testing.T) {
	raw := "From: a@x.test\r\nTo: \"QA\" <qa+signup@x.test>, other@x.test\r\nCc: b+Billing@x.test\r\n" +
		"X-Tags: urgent, Signup ,  \r\nSubject: hi\r\n\r\nbody\r\n"
	p := mailparse.Parse([]byte(raw))
	got := AutoTags(p, []string{"qa+signup@x.test", "<ops+alerts@x.test>", "plain@x.test"})
	want := []string{"alerts", "Billing", "signup", "urgent"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
