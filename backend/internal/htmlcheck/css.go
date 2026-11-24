package htmlcheck

import (
	"regexp"
	"strings"
)

type decl struct{ prop, value string }

// cssInfo collects what a message's CSS uses, from <style> blocks and style="" attributes.
type cssInfo struct {
	decls     []decl
	selectors []string
	atRules   []string // full preludes, e.g. "@media (prefers-color-scheme: dark)"
	comments  bool
	nesting   bool
}

func newCSSInfo() cssInfo { return cssInfo{} }

var commentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)

func (c *cssInfo) stripComments(s string) string {
	if strings.Contains(s, "/*") {
		c.comments = true
		s = commentRe.ReplaceAllString(s, " ")
	}
	return s
}

func (c *cssInfo) addInline(s string) {
	c.parseDecls(c.stripComments(s))
}

func (c *cssInfo) addSheet(s string) {
	s = c.stripComments(s)
	s = strings.NewReplacer("<!--", " ", "-->", " ").Replace(s)
	c.parseBlock(s, 0)
}

func (c *cssInfo) parseDecls(s string) {
	for _, part := range strings.Split(s, ";") {
		prop, val, ok := strings.Cut(part, ":")
		prop = strings.ToLower(strings.TrimSpace(prop))
		if !ok || prop == "" || strings.ContainsAny(prop, " {}") {
			continue
		}
		c.decls = append(c.decls, decl{prop: prop, value: strings.ToLower(strings.TrimSpace(val))})
	}
}

// parseBlock walks a sequence of CSS statements: at-rules and rule sets.
func (c *cssInfo) parseBlock(s string, depth int) {
	if depth > 8 {
		return
	}
	i := 0
	for i < len(s) {
		// Prelude up to '{' or ';'.
		j := strings.IndexAny(s[i:], "{;}")
		if j < 0 {
			return
		}
		prelude := strings.TrimSpace(s[i : i+j])
		ch := s[i+j]
		if ch == ';' || ch == '}' {
			if strings.HasPrefix(prelude, "@") {
				c.atRules = append(c.atRules, strings.ToLower(prelude)) // e.g. @import
			} else if depth > 0 && strings.Contains(prelude, ":") {
				c.parseDecls(prelude) // declaration inside a nested block
			}
			i += j + 1
			continue
		}
		// Find the matching closing brace.
		start := i + j + 1
		level, k := 1, start
		for k < len(s) && level > 0 {
			switch s[k] {
			case '{':
				level++
			case '}':
				level--
			}
			k++
		}
		body := s[start:max(start, k-1)]
		lp := strings.ToLower(prelude)
		switch {
		case strings.HasPrefix(lp, "@media"), strings.HasPrefix(lp, "@supports"), strings.HasPrefix(lp, "@layer"), strings.HasPrefix(lp, "@container"):
			c.atRules = append(c.atRules, lp)
			c.parseBlock(body, depth+1)
		case strings.HasPrefix(lp, "@keyframes"), strings.HasPrefix(lp, "@-webkit-keyframes"):
			c.atRules = append(c.atRules, lp)
		case strings.HasPrefix(lp, "@"):
			c.atRules = append(c.atRules, lp)
			c.parseDecls(body)
		default:
			c.selectors = append(c.selectors, prelude)
			if strings.Contains(body, "{") {
				c.nesting = true
				c.parseBlock(body, depth+1)
			} else {
				c.parseDecls(body)
			}
		}
		i = k
	}
}

var (
	pseudoRe      = regexp.MustCompile(`::?([a-z-]+)`)
	modernColorRe = regexp.MustCompile(`\b(hsla?|hwb|lab|lch|oklab|oklch|color|color-mix)\(`)
	typeSelRe     = regexp.MustCompile(`(^|[\s>+~,(])[a-z][a-z0-9]*`)
	chainRe       = regexp.MustCompile(`[a-z0-9\]\)][.#\[]|[.#][\w-]+[.#]`)
)

var pseudoElements = map[string]bool{"before": true, "after": true, "first-letter": true, "first-line": true, "marker": true, "placeholder": true}

// shorthand/longhand families that caniemail tracks under one slug.
var propFamilies = map[string][]string{
	"css-margin":                         {"margin", "margin-top", "margin-right", "margin-bottom", "margin-left"},
	"css-padding":                        {"padding", "padding-top", "padding-right", "padding-bottom", "padding-left"},
	"css-border":                         {"border", "border-top", "border-right", "border-bottom", "border-left", "border-width", "border-style", "border-color"},
	"css-left-right-top-bottom":          {"left", "right", "top", "bottom"},
	"css-margin-inline-block":            {"margin-inline", "margin-block"},
	"css-margin-block-start-end":         {"margin-block-start", "margin-block-end"},
	"css-margin-inline-start-end":        {"margin-inline-start", "margin-inline-end"},
	"css-padding-inline-block":           {"padding-inline", "padding-block"},
	"css-padding-block-start-end":        {"padding-block-start", "padding-block-end"},
	"css-padding-inline-start-end":       {"padding-inline-start", "padding-inline-end"},
	"css-border-inline-block":            {"border-inline", "border-block"},
	"css-border-inline-block-individual": {"border-inline-start", "border-inline-end", "border-block-start", "border-block-end"},
	"css-border-inline-block-longhand":   {"border-inline-width", "border-inline-style", "border-inline-color", "border-block-width", "border-block-style", "border-block-color"},
	"css-border-radius-logical":          {"border-start-start-radius", "border-start-end-radius", "border-end-start-radius", "border-end-end-radius"},
	"css-block-inline-size":              {"block-size", "inline-size"},
	"css-grid-template":                  {"grid-template", "grid-template-columns", "grid-template-rows", "grid-template-areas"},
	"css-column-layout-properties":       {"columns", "column-width", "column-gap", "column-rule", "column-span", "column-fill"},
	"css-intrinsic-size":                 {"contain-intrinsic-size"},
	"css-scroll-snap":                    {"scroll-snap-type", "scroll-snap-align", "scroll-snap-stop"},
	"css-text-emphasis":                  {"text-emphasis", "text-emphasis-style", "text-emphasis-color"},
}

// slugs maps what was collected to caniemail CSS feature slugs.
func (c *cssInfo) slugs() map[string]bool {
	f := map[string]bool{}
	props := map[string]bool{}
	for _, d := range c.decls {
		props[d.prop] = true
		f["css-"+d.prop] = true
		v := d.value
		if strings.HasPrefix(d.prop, "--") || strings.Contains(v, "var(") {
			f["css-variables"] = true
		}
		if strings.Contains(v, "!important") {
			f["css-important"] = true
		}
		if d.prop == "display" {
			switch strings.TrimSpace(strings.TrimSuffix(v, "!important")) {
			case "flex", "inline-flex":
				f["css-display-flex"] = true
			case "grid", "inline-grid":
				f["css-display-grid"] = true
			case "none":
				f["css-display-none"] = true
			}
		}
		for _, fn := range []string{"clamp", "max", "min", "light-dark"} {
			if strings.Contains(v, fn+"(") {
				f["css-function-"+fn] = true
			}
		}
		for _, g := range []string{"linear-gradient", "radial-gradient", "conic-gradient"} {
			if strings.Contains(v, g+"(") {
				f["css-"+g] = true
			}
		}
		if strings.Contains(v, "calc(") {
			f["css-unit-calc"] = true
		}
		if strings.Contains(v, "rgba(") {
			f["css-rgba"] = true
		} else if strings.Contains(v, "rgb(") {
			f["css-rgb"] = true
		}
		if modernColorRe.MatchString(v) {
			f["css-modern-color"] = true
		}
		if v == "initial" {
			f["css-unit-initial"] = true
		}
		if d.prop == "font-family" && strings.Contains(v, "system-ui") {
			f["css-sytem-ui"] = true // sic: caniemail's slug
		}
		for _, m := range unitRe.FindAllStringSubmatch(v, -1) {
			u := strings.ToLower(m[1])
			if s, ok := unitSlugs[u]; ok {
				u = s
			}
			f["css-unit-"+u] = true
		}
	}
	for slug, family := range propFamilies {
		for _, p := range family {
			if props[p] {
				f[slug] = true
			}
		}
	}
	if c.comments {
		f["css-comments"] = true
	}
	if c.nesting {
		f["css-nesting"] = true
	}
	for _, a := range c.atRules {
		name := strings.Fields(a)[0]
		switch name {
		case "@media":
			f["css-at-media"] = true
			for _, mf := range []struct{ needle, slug string }{
				{"prefers-color-scheme", "css-at-media-prefers-color-scheme"},
				{"prefers-reduced-motion", "css-at-media-prefers-reduced-motion"},
				{"hover", "css-at-media-hover"},
				{"orientation", "css-at-media-orientation"},
				{"device-pixel-ratio", "css-at-media-device-pixel-ratio"},
				{"resolution", "css-at-media-device-pixel-ratio"},
			} {
				if strings.Contains(a, mf.needle) {
					f[mf.slug] = true
				}
			}
		case "@font-face":
			f["css-at-font-face"] = true
		case "@import":
			f["css-at-import"] = true
		case "@keyframes", "@-webkit-keyframes":
			f["css-at-keyframes"] = true
		case "@supports":
			f["css-at-supports"] = true
		}
	}
	for _, sel := range c.selectors {
		s := strings.ToLower(sel)
		for _, m := range pseudoRe.FindAllStringSubmatch(s, -1) {
			name := m[1]
			if strings.HasPrefix(m[0], "::") || pseudoElements[name] {
				f["css-pseudo-element-"+name] = true
			} else {
				f["css-pseudo-class-"+name] = true
			}
		}
		if strings.Contains(s, ",") {
			f["css-selector-grouping"] = true
		}
		for _, part := range strings.Split(s, ",") {
			part = strings.TrimSpace(pseudoRe.ReplaceAllString(part, ""))
			if part == "" {
				continue
			}
			if strings.Contains(part, ".") {
				f["css-selector-class"] = true
			}
			if strings.Contains(part, "#") {
				f["css-selector-id"] = true
			}
			if strings.Contains(part, "[") {
				f["css-selector-attribute"] = true
			}
			if strings.Contains(part, ">") {
				f["css-selector-child"] = true
			}
			if strings.Contains(part, "+") {
				f["css-selector-adjacent-sibling"] = true
			}
			if strings.Contains(part, "~") {
				f["css-selector-general-sibling"] = true
			}
			if strings.Contains(part, "*") {
				f["css-selector-universal"] = true
			}
			if typeSelRe.MatchString(part) {
				f["css-selector-type"] = true
			}
			if chainRe.MatchString(part) {
				f["css-selector-chaining"] = true
			}
			if len(strings.Fields(strings.NewReplacer(">", " ", "+", " ", "~", " ").Replace(part))) > 1 && !strings.ContainsAny(part, ">+~") {
				f["css-selector-descendant"] = true
			}
		}
	}
	return f
}
