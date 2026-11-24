// Package linkcheck extracts the links and images of a message and checks that
// they resolve. Requests go through netguard (no internal addresses unless the
// inbox allows them) and results are cached in Postgres for a few minutes.
package linkcheck

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"github.com/matiasrohleder/PostDream/internal/netguard"
	"github.com/matiasrohleder/PostDream/internal/store"
)

const (
	MaxLinks    = 200
	cacheMaxAge = 15 * time.Minute
	concurrency = 8
)

type Link struct {
	URL      string   `json:"url"`
	Kind     string   `json:"kind"` // link | image
	Status   int      `json:"status"`
	OK       bool     `json:"ok"`
	Error    string   `json:"error,omitempty"`
	Cached   bool     `json:"cached"`
	Warnings []string `json:"warnings"` // localhost | insecure | unexpected-domain | blocked
}

type Report struct {
	Links     []*Link `json:"links"`
	Total     int     `json:"total"`
	Broken    int     `json:"broken"`
	Warnings  int     `json:"warnings"`
	Truncated bool    `json:"truncated"`
}

type Checker struct {
	Store     *store.Store
	Policy    netguard.Policy // global policy; inbox allowed hosts are appended
	UserAgent string
}

var textURLRe = regexp.MustCompile(`https?://[^\s<>"'()\[\]]+`)

// Extract returns the unique http(s) URLs of the HTML (links and images) and text bodies.
func Extract(htmlBody, textBody string) []*Link {
	seen := map[string]*Link{}
	var order []*Link
	add := func(raw, kind string) {
		u := strings.TrimSpace(raw)
		if !strings.HasPrefix(strings.ToLower(u), "http://") && !strings.HasPrefix(strings.ToLower(u), "https://") {
			return
		}
		u = strings.TrimRight(u, ".,;:!?")
		if l, ok := seen[u]; ok {
			if kind == "image" {
				l.Kind = "image"
			}
			return
		}
		l := &Link{URL: u, Kind: kind, Warnings: []string{}}
		seen[u] = l
		order = append(order, l)
	}
	z := html.NewTokenizer(strings.NewReader(htmlBody))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		name, hasAttr := z.TagName()
		for hasAttr {
			var k, v []byte
			k, v, hasAttr = z.TagAttr()
			key := string(k)
			switch {
			case key == "href" && (string(name) == "a" || string(name) == "area"):
				add(string(v), "link")
			case key == "src" && string(name) == "img", key == "background":
				add(string(v), "image")
			}
		}
	}
	for _, m := range textURLRe.FindAllString(textBody, -1) {
		add(m, "link")
	}
	return order
}

// Check verifies every link of a message. allowedHosts and expectedDomains come from the inbox.
func (c *Checker) Check(ctx context.Context, htmlBody, textBody string, allowedHosts, expectedDomains []string) *Report {
	links := Extract(htmlBody, textBody)
	rep := &Report{Links: links}
	if len(links) > MaxLinks {
		rep.Links, rep.Truncated = links[:MaxLinks], true
	}
	policy := c.Policy
	policy.Allowed = append(append([]string{}, policy.Allowed...), allowedHosts...)
	client := netguard.Client(policy, 10*time.Second)

	urls := make([]string, len(rep.Links))
	for i, l := range rep.Links {
		urls[i] = l.URL
		staticWarnings(l, expectedDomains)
	}
	cached, _ := c.Store.CachedLinks(ctx, urls, cacheMaxAge)

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, l := range rep.Links {
		if hit, ok := cached[l.URL]; ok {
			l.Status, l.Error, l.Cached = hit.Status, hit.Error, true
			l.OK = hit.Status >= 200 && hit.Status < 400
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(l *Link) {
			defer wg.Done()
			defer func() { <-sem }()
			c.probe(ctx, client, l)
		}(l)
	}
	wg.Wait()

	for _, l := range rep.Links {
		rep.Total++
		if !l.OK {
			rep.Broken++
		}
		if len(l.Warnings) > 0 {
			rep.Warnings++
		}
	}
	sort.SliceStable(rep.Links, func(i, j int) bool { return !rep.Links[i].OK && rep.Links[j].OK })
	return rep
}

func (c *Checker) probe(ctx context.Context, client *http.Client, l *Link) {
	status, err := c.request(ctx, client, http.MethodHead, l.URL)
	// Many servers reject or mishandle HEAD: retry with GET.
	if err == nil && (status == 405 || status == 403 || status == 501 || status == 404 || status >= 500) {
		status, err = c.request(ctx, client, http.MethodGet, l.URL)
	}
	l.Status = status
	if err != nil {
		l.Error = err.Error()
		if strings.Contains(l.Error, netguard.ErrBlocked.Error()) {
			l.Warnings = append(l.Warnings, "blocked")
			l.Error = "internal address blocked (allow the host in the inbox settings to check it)"
			return // not cached: depends on the inbox's policy
		}
	}
	l.OK = err == nil && status >= 200 && status < 400
	bg, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c.Store.CacheLink(bg, l.URL, status, l.Error)
}

func (c *Checker) request(ctx context.Context, client *http.Client, method, u string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "*/*")
	res, err := client.Do(req)
	if err != nil {
		// Drop the `Head "https://…":` prefix of *url.Error: the URL is already shown.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, err
	}
	res.Body.Close()
	return res.StatusCode, nil
}

func staticWarnings(l *Link, expectedDomains []string) {
	u, err := url.Parse(l.URL)
	if err != nil {
		return
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0" {
		l.Warnings = append(l.Warnings, "localhost")
	}
	if u.Scheme == "http" {
		l.Warnings = append(l.Warnings, "insecure")
	}
	if len(expectedDomains) > 0 && !matchesDomain(host, expectedDomains) {
		l.Warnings = append(l.Warnings, "unexpected-domain")
	}
}

// matchesDomain reports whether host is one of domains or a subdomain of one.
func matchesDomain(host string, domains []string) bool {
	for _, d := range domains {
		d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "*."))
		if d != "" && (host == d || strings.HasSuffix(host, "."+d)) {
			return true
		}
	}
	return false
}
