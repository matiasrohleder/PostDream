package linkcheck

import (
	"reflect"
	"testing"
)

func TestExtract(t *testing.T) {
	html := `<a href="https://a.test/x">a</a><a href="mailto:x@y">m</a><a href="#top">t</a>
<img src="https://cdn.test/i.png"><td background="http://cdn.test/bg.jpg"></td><a href="https://a.test/x">dup</a>`
	text := "See https://a.test/x. and http://b.test/path?q=1, also (https://c.test)"
	var got []string
	kinds := map[string]string{}
	for _, l := range Extract(html, text) {
		got = append(got, l.URL)
		kinds[l.URL] = l.Kind
	}
	want := []string{"https://a.test/x", "https://cdn.test/i.png", "http://cdn.test/bg.jpg", "http://b.test/path?q=1", "https://c.test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	if kinds["https://cdn.test/i.png"] != "image" || kinds["https://a.test/x"] != "link" {
		t.Errorf("kinds: %v", kinds)
	}
}

func TestStaticWarnings(t *testing.T) {
	l := &Link{URL: "http://localhost:3000/reset", Warnings: []string{}}
	staticWarnings(l, []string{"staging.acme.test"})
	if !reflect.DeepEqual(l.Warnings, []string{"localhost", "insecure", "unexpected-domain"}) {
		t.Fatalf("got %v", l.Warnings)
	}
	ok := &Link{URL: "https://app.staging.acme.test/x", Warnings: []string{}}
	staticWarnings(ok, []string{"*.staging.acme.test"})
	if len(ok.Warnings) != 0 {
		t.Fatalf("unexpected warnings %v", ok.Warnings)
	}
}
