package htmlcheck

import (
	"strings"
	"testing"
)

const sample = `<!doctype html><html><head>
<meta name="viewport" content="width=device-width">
<style>
/* comment */
@media (prefers-color-scheme: dark) { .box { background-color: #000 !important; } }
a:hover { color: red; }
.wrap > p { display: flex; margin: 0 auto; padding: 1em; }
</style></head>
<body><table cellpadding="0"><tr><td align="center" style="border-radius: 4px; width: calc(100% - 10px)">
<img src="https://x.test/logo.png"><img src="data:image/gif;base64,R0lGOD" alt="">
<a href="mailto:a@b.c">mail</a><script>alert(1)</script>
</td></tr></table></body></html>`

func TestCheckDetectsFeatures(t *testing.T) {
	r := Check(sample, false)
	got := map[string]bool{}
	for _, f := range r.Features {
		got[f.Slug] = true
	}
	for _, want := range []string{
		"css-at-media", "css-at-media-prefers-color-scheme", "css-important", "css-pseudo-class-hover",
		"css-display-flex", "css-margin", "css-padding", "css-border-radius", "css-unit-calc", "css-unit-em",
		"css-unit-percent", "css-unit-px", "css-selector-class", "css-selector-child", "css-comments",
		"css-background-color", "css-width", "html-cellpadding", "html-align", "html-doctype", "html-mailto-links",
		"html-table", "image-png", "image-gif", "image-base64", "html-style",
	} {
		if !got[want] {
			t.Errorf("feature %s not detected", want)
		}
	}
	if r.Score <= 0 || r.Score >= 100 {
		t.Errorf("unexpected score %v", r.Score)
	}
	if len(r.Clients) == 0 {
		t.Fatal("no client scores")
	}
	ids := map[string]bool{}
	for _, w := range r.Warnings {
		ids[w.ID] = true
	}
	for _, want := range []string{"img-alt", "no-text-part", "script", "html-lang"} {
		if !ids[want] {
			t.Errorf("warning %s missing (got %v)", want, ids)
		}
	}
	if ids["viewport"] {
		t.Error("viewport warning should not fire")
	}
	// Least supported first.
	for i := 1; i < len(r.Features); i++ {
		a, b := r.Features[i-1], r.Features[i]
		if a.Supported+a.Partial/2 > b.Supported+b.Partial/2 {
			t.Fatalf("features not sorted at %d", i)
		}
	}
}

func TestGmailClipping(t *testing.T) {
	r := Check("<p>"+strings.Repeat("x", GmailClipBytes+10)+"</p>", true)
	if len(r.Warnings) == 0 || r.Warnings[0].ID != "gmail-clipping" || r.Warnings[0].Level != "error" {
		t.Fatalf("expected clipping error, got %+v", r.Warnings)
	}
}

func TestEmptyHTML(t *testing.T) {
	if r := Check("  ", true); r.Score != 100 || len(r.Features) != 0 {
		t.Fatalf("unexpected %+v", r)
	}
}
