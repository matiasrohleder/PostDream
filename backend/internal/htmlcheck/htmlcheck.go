// Package htmlcheck scores an email's HTML/CSS against the email-client support
// data from caniemail.com (vendored in caniemail.json, MIT licensed) and adds
// practical checks such as Gmail clipping, missing alt text and scripts.
package htmlcheck

import (
	_ "embed"
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

//go:embed caniemail.json
var rawData []byte

type dataset struct {
	Updated   string            `json:"updated"`
	Families  map[string]string `json:"families"`
	Platforms map[string]string `json:"platforms"`
	Features  []struct {
		Slug     string                       `json:"slug"`
		Title    string                       `json:"title"`
		URL      string                       `json:"url"`
		Category string                       `json:"category"`
		Support  map[string]map[string]string `json:"support"`
		Notes    map[string]string            `json:"notes"`
	} `json:"features"`
}

var (
	loadOnce sync.Once
	data     dataset
)

func load() *dataset {
	loadOnce.Do(func() {
		if err := json.Unmarshal(rawData, &data); err != nil {
			panic("htmlcheck: invalid caniemail.json: " + err.Error())
		}
	})
	return &data
}

type Result struct {
	Score       float64       `json:"score"` // 0..100, average support across all client platforms
	Clients     []ClientScore `json:"clients"`
	Features    []Feature     `json:"features"` // detected features, least supported first
	Warnings    []Warning     `json:"warnings"`
	HTMLBytes   int           `json:"html_bytes"`
	DataUpdated string        `json:"data_updated"`
}

type ClientScore struct {
	Family    string            `json:"family"`
	Name      string            `json:"name"`
	Score     float64           `json:"score"`
	Platforms map[string]string `json:"platforms"` // platform slug -> display name
}

type Feature struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Category    string   `json:"category"`
	Supported   float64  `json:"supported"` // % of platforms with full support
	Partial     float64  `json:"partial"`   // % with partial support
	Unsupported []string `json:"unsupported"`
	PartialIn   []string `json:"partial_in"`
	Notes       []string `json:"notes,omitempty"`
}

type Warning struct {
	ID      string `json:"id"`
	Level   string `json:"level"` // error | warning | info
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// GmailClipBytes is the HTML size beyond which Gmail clips the message.
const GmailClipBytes = 102 * 1024

// Check analyses htmlBody. hasText tells whether the message also has a text/plain part.
func Check(htmlBody string, hasText bool) *Result {
	d := load()
	res := &Result{HTMLBytes: len(htmlBody), DataUpdated: d.Updated, Features: []Feature{}, Warnings: []Warning{}, Clients: []ClientScore{}}
	if strings.TrimSpace(htmlBody) == "" {
		res.Score = 100
		return res
	}
	doc := scan(htmlBody)
	found := detect(doc)

	type acc struct{ sum, n float64 }
	platformAcc := map[string]*acc{} // "family/platform"
	for _, f := range d.Features {
		if !found[f.Slug] {
			continue
		}
		feat := Feature{Slug: f.Slug, Title: f.Title, URL: f.URL, Category: f.Category, Unsupported: []string{}, PartialIn: []string{}}
		var yes, part, total float64
		noteSet := map[string]bool{}
		for fam, plats := range f.Support {
			for plat, v := range plats {
				level, notes := parseSupport(v)
				if level < 0 {
					continue // unknown
				}
				total++
				key := fam + "/" + plat
				if platformAcc[key] == nil {
					platformAcc[key] = &acc{}
				}
				platformAcc[key].sum += level
				platformAcc[key].n++
				label := clientLabel(d, fam, plat)
				switch level {
				case 1:
					yes++
				case 0.5:
					part++
					feat.PartialIn = append(feat.PartialIn, label)
				default:
					feat.Unsupported = append(feat.Unsupported, label)
				}
				for _, n := range notes {
					if txt := f.Notes[n]; txt != "" && level < 1 {
						noteSet[txt] = true
					}
				}
			}
		}
		if total > 0 {
			feat.Supported = round1(100 * yes / total)
			feat.Partial = round1(100 * part / total)
		}
		for n := range noteSet {
			feat.Notes = append(feat.Notes, n)
		}
		sort.Strings(feat.Notes)
		sort.Strings(feat.Unsupported)
		sort.Strings(feat.PartialIn)
		res.Features = append(res.Features, feat)
	}
	sort.SliceStable(res.Features, func(i, j int) bool {
		a, b := res.Features[i], res.Features[j]
		sa, sb := a.Supported+a.Partial/2, b.Supported+b.Partial/2
		if sa != sb {
			return sa < sb
		}
		return a.Slug < b.Slug
	})

	// Per family and overall scores.
	famAcc := map[string]*acc{}
	famPlats := map[string]map[string]string{}
	var overall acc
	for key, a := range platformAcc {
		fam, plat, _ := strings.Cut(key, "/")
		s := a.sum / a.n
		if famAcc[fam] == nil {
			famAcc[fam] = &acc{}
			famPlats[fam] = map[string]string{}
		}
		famAcc[fam].sum += s
		famAcc[fam].n++
		famPlats[fam][plat] = d.Platforms[plat]
		overall.sum += s
		overall.n++
	}
	for fam, a := range famAcc {
		name := d.Families[fam]
		if name == "" {
			name = fam
		}
		res.Clients = append(res.Clients, ClientScore{Family: fam, Name: name, Score: round1(100 * a.sum / a.n), Platforms: famPlats[fam]})
	}
	sort.Slice(res.Clients, func(i, j int) bool {
		if res.Clients[i].Score != res.Clients[j].Score {
			return res.Clients[i].Score < res.Clients[j].Score
		}
		return res.Clients[i].Name < res.Clients[j].Name
	})
	res.Score = 100
	if overall.n > 0 {
		res.Score = round1(100 * overall.sum / overall.n)
	}
	res.Warnings = warnings(doc, len(htmlBody), hasText)
	return res
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// parseSupport maps "y", "a #1", "n", "u" to 1, 0.5, 0, -1 plus note numbers.
func parseSupport(v string) (float64, []string) {
	fields := strings.Fields(v)
	if len(fields) == 0 {
		return -1, nil
	}
	var notes []string
	for _, f := range fields[1:] {
		notes = append(notes, strings.TrimPrefix(f, "#"))
	}
	switch fields[0] {
	case "y":
		return 1, notes
	case "a":
		return 0.5, notes
	case "n":
		return 0, notes
	}
	return -1, nil
}

func clientLabel(d *dataset, fam, plat string) string {
	f, p := d.Families[fam], d.Platforms[plat]
	if f == "" {
		f = fam
	}
	if p == "" {
		p = plat
	}
	return f + " (" + p + ")"
}

// ---------- document scan ----------

type document struct {
	elements   map[string]int
	attrs      map[string]int
	inputTypes map[string]bool
	btnTypes   map[string]bool
	hrefs      []string
	imgs       []img
	css        cssInfo
	doctype    bool
	comments   bool
	htmlLang   bool
	metaColor  bool
	viewport   bool
	scripts    int
	forms      int
	extCSS     int
}

type img struct{ src, alt string }

func scan(body string) *document {
	d := &document{elements: map[string]int{}, attrs: map[string]int{}, inputTypes: map[string]bool{}, btnTypes: map[string]bool{}}
	d.css = newCSSInfo()
	z := html.NewTokenizer(strings.NewReader(body))
	inStyle := false
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return d
		case html.DoctypeToken:
			d.doctype = true
		case html.CommentToken:
			d.comments = true
		case html.TextToken:
			if inStyle {
				d.css.addSheet(string(z.Text()))
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if string(name) == "style" {
				inStyle = false
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			nameB, hasAttr := z.TagName()
			name := string(nameB)
			d.elements[name]++
			attrs := map[string]string{}
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				key := strings.ToLower(string(k))
				attrs[key] = string(v)
				d.attrs[key]++
			}
			if st, ok := attrs["style"]; ok {
				d.css.addInline(st)
			}
			switch name {
			case "style":
				inStyle = tt == html.StartTagToken
			case "html":
				if attrs["lang"] != "" {
					d.htmlLang = true
				}
			case "img":
				alt, hasAlt := attrs["alt"]
				if !hasAlt {
					alt = "\x00" // marker: attribute missing (alt="" is valid for decorative images)
				}
				d.imgs = append(d.imgs, img{src: attrs["src"], alt: alt})
			case "a":
				d.hrefs = append(d.hrefs, attrs["href"])
			case "input":
				d.inputTypes[strings.ToLower(attrs["type"])] = true
			case "button":
				t := strings.ToLower(attrs["type"])
				if t == "" {
					t = "submit"
				}
				d.btnTypes[t] = true
			case "meta":
				n := strings.ToLower(attrs["name"])
				if n == "color-scheme" || n == "supported-color-schemes" {
					d.metaColor = true
				}
				if n == "viewport" {
					d.viewport = true
				}
			case "script":
				d.scripts++
			case "form":
				d.forms++
			case "link":
				if strings.Contains(strings.ToLower(attrs["rel"]), "stylesheet") {
					d.extCSS++
				}
			}
		}
	}
}

var semanticTags = []string{"article", "aside", "details", "figcaption", "figure", "footer", "header", "main", "mark", "nav", "section", "summary", "time"}

var imageExt = map[string][]string{
	"image-apng": {".apng"}, "image-avif": {".avif"}, "image-bmp": {".bmp"}, "image-gif": {".gif"}, "image-hdr": {".hdr"},
	"image-heif": {".heif", ".heic"}, "image-ico": {".ico"}, "image-jpg": {".jpg", ".jpeg"}, "image-mp4": {".mp4"},
	"image-png": {".png"}, "image-svg": {".svg"}, "image-tiff": {".tif", ".tiff"}, "image-webp": {".webp"},
}

// detect returns the set of caniemail slugs present in the document.
func detect(d *document) map[string]bool {
	f := map[string]bool{}
	set := func(slug string, cond bool) {
		if cond {
			f[slug] = true
		}
	}
	// HTML: generic element/attribute names, then special cases.
	for el := range d.elements {
		f["html-"+el] = true
	}
	for a := range d.attrs {
		if a != "style" { // the style attribute is CSS, "html-style" means the <style> element
			f["html-"+a] = true
		}
	}
	for i := 1; i <= 6; i++ {
		set("html-h1-h6", d.elements["h"+string(rune('0'+i))] > 0)
	}
	set("html-lists", d.elements["ul"]+d.elements["ol"]+d.elements["dl"] > 0)
	for _, t := range semanticTags {
		set("html-semantics", d.elements[t] > 0)
	}
	set("html-image-maps", d.elements["map"]+d.elements["area"] > 0)
	for t := range d.inputTypes {
		f["html-input-"+t] = true
	}
	for t := range d.btnTypes {
		f["html-button-"+t] = true
	}
	set("html-doctype", d.doctype)
	set("html-comments", d.comments)
	set("html-meta-color-scheme", d.metaColor)
	set("html-loading-attribute", d.attrs["loading"] > 0)
	set("html-command-attribute", d.attrs["command"]+d.attrs["commandfor"] > 0)
	for _, h := range d.hrefs {
		h = strings.TrimSpace(strings.ToLower(h))
		set("html-anchor-links", strings.HasPrefix(h, "#"))
		set("html-mailto-links", strings.HasPrefix(h, "mailto:"))
	}
	// Images by format.
	for _, im := range d.imgs {
		src := strings.ToLower(strings.TrimSpace(im.src))
		if strings.HasPrefix(src, "data:") {
			f["image-base64"] = true
			if mt, _, ok := strings.Cut(strings.TrimPrefix(src, "data:image/"), ";"); ok {
				switch mt {
				case "jpeg":
					mt = "jpg"
				case "svg+xml":
					mt = "svg"
				}
				f["image-"+mt] = true
			}
			continue
		}
		if i := strings.IndexAny(src, "?#"); i >= 0 {
			src = src[:i]
		}
		for slug, exts := range imageExt {
			for _, e := range exts {
				set(slug, strings.HasSuffix(src, e))
			}
		}
	}
	// CSS.
	for slug := range d.css.slugs() {
		f[slug] = true
	}
	return f
}

var (
	unitRe    = regexp.MustCompile(`(?i)\d(px|em|rem|ex|ch|cm|mm|in|pt|pc|vh|vw|vmin|vmax|%)`)
	unitSlugs = map[string]string{"%": "percent"}
)

func warnings(d *document, size int, hasText bool) []Warning {
	w := []Warning{}
	if size > GmailClipBytes {
		w = append(w, Warning{ID: "gmail-clipping", Level: "error", Message: "HTML is larger than 102 KB: Gmail will clip the message",
			Detail: "Gmail hides everything after the first 102 KB behind a \"View entire message\" link, including tracking pixels and unsubscribe links."})
	} else if size > 90*1024 {
		w = append(w, Warning{ID: "gmail-clipping", Level: "warning", Message: "HTML is close to Gmail's 102 KB clipping limit"})
	}
	missingAlt := 0
	insecure := 0
	for _, im := range d.imgs {
		if im.alt == "\x00" {
			missingAlt++
		}
		if strings.HasPrefix(strings.ToLower(im.src), "http://") {
			insecure++
		}
	}
	if missingAlt > 0 {
		w = append(w, Warning{ID: "img-alt", Level: "warning", Message: "Images without an alt attribute",
			Detail: strconv.Itoa(missingAlt) + " image(s). Many clients block images by default; alt text keeps the email readable and accessible."})
	}
	if insecure > 0 {
		w = append(w, Warning{ID: "img-http", Level: "warning", Message: "Images loaded over plain HTTP",
			Detail: strconv.Itoa(insecure) + " image(s). Some clients block or warn about non-HTTPS content."})
	}
	if !hasText {
		w = append(w, Warning{ID: "no-text-part", Level: "warning", Message: "No plain-text alternative",
			Detail: "A text/plain part improves deliverability and accessibility."})
	}
	if d.scripts > 0 {
		w = append(w, Warning{ID: "script", Level: "error", Message: "Contains <script> tags",
			Detail: "Email clients strip scripts; some spam filters penalise them."})
	}
	if d.forms > 0 {
		w = append(w, Warning{ID: "form", Level: "warning", Message: "Contains forms", Detail: "Forms are unsupported or disabled in many email clients."})
	}
	if d.extCSS > 0 {
		w = append(w, Warning{ID: "external-css", Level: "warning", Message: "Uses external stylesheets (<link rel=stylesheet>)",
			Detail: "Most clients ignore external CSS; inline the styles instead."})
	}
	if !d.htmlLang {
		w = append(w, Warning{ID: "html-lang", Level: "info", Message: "Missing lang attribute on <html>",
			Detail: "Screen readers use it to pick the right pronunciation."})
	}
	if !d.viewport {
		w = append(w, Warning{ID: "viewport", Level: "info", Message: "No viewport meta tag",
			Detail: "Without <meta name=\"viewport\" content=\"width=device-width\"> some mobile clients render the email zoomed out."})
	}
	return w
}
