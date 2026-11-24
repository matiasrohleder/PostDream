// Package metrics is a tiny Prometheus text-format registry (counters only;
// gauges are computed at scrape time by the caller). It avoids pulling the full
// client library for a handful of series.
package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

type counter struct {
	help   string
	labels []string
	mu     sync.Mutex
	values map[string]float64 // key = joined label values
}

var (
	mu       sync.RWMutex
	counters = map[string]*counter{}
)

// Register declares a counter. Calling Inc on an unregistered name panics.
func Register(name, help string, labels ...string) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := counters[name]; !ok {
		counters[name] = &counter{help: help, labels: labels, values: map[string]float64{}}
	}
}

// Add increases a counter by v for the given label values.
func Add(name string, v float64, labelValues ...string) {
	mu.RLock()
	c := counters[name]
	mu.RUnlock()
	if c == nil {
		panic("metrics: unregistered counter " + name)
	}
	key := strings.Join(labelValues, "\x00")
	c.mu.Lock()
	c.values[key] += v
	c.mu.Unlock()
}

// Inc increases a counter by one.
func Inc(name string, labelValues ...string) { Add(name, 1, labelValues...) }

// Gauge is a value computed at scrape time.
type Gauge struct {
	Name   string
	Help   string
	Labels map[string]string
	Value  float64
}

func escape(v string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`).Replace(v)
}

func labelString(names, values []string) string {
	if len(names) == 0 {
		return ""
	}
	parts := make([]string, len(names))
	for i, n := range names {
		v := ""
		if i < len(values) {
			v = values[i]
		}
		parts[i] = fmt.Sprintf(`%s="%s"`, n, escape(v))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// Write renders all counters plus the given gauges in Prometheus text format.
func Write(w io.Writer, gauges []Gauge) {
	mu.RLock()
	names := make([]string, 0, len(counters))
	for n := range counters {
		names = append(names, n)
	}
	mu.RUnlock()
	sort.Strings(names)
	for _, n := range names {
		mu.RLock()
		c := counters[n]
		mu.RUnlock()
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", n, c.help, n)
		c.mu.Lock()
		keys := make([]string, 0, len(c.values))
		for k := range c.values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			var vals []string
			if len(c.labels) > 0 {
				vals = strings.Split(k, "\x00")
			}
			fmt.Fprintf(w, "%s%s %g\n", n, labelString(c.labels, vals), c.values[k])
		}
		c.mu.Unlock()
	}
	seen := map[string]bool{}
	for _, g := range gauges {
		if !seen[g.Name] {
			fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n", g.Name, g.Help, g.Name)
			seen[g.Name] = true
		}
		var ln, lv []string
		for k := range g.Labels {
			ln = append(ln, k)
		}
		sort.Strings(ln)
		for _, k := range ln {
			lv = append(lv, g.Labels[k])
		}
		fmt.Fprintf(w, "%s%s %g\n", g.Name, labelString(ln, lv), g.Value)
	}
}

// Names of the counters used across PostDream.
const (
	MessagesReceived   = "postdream_messages_received_total"
	BytesReceived      = "postdream_received_bytes_total"
	SMTPRejected       = "postdream_smtp_rejected_total"
	SMTPAuthFailures   = "postdream_smtp_auth_failures_total"
	WebhookDeliveries  = "postdream_webhook_deliveries_total"
	RetentionDeleted   = "postdream_retention_deleted_total"
	HTTPRequests       = "postdream_http_requests_total"
	MessagesReleased   = "postdream_messages_released_total"
	POP3Logins         = "postdream_pop3_logins_total"
	SpamChecks         = "postdream_spam_checks_total"
	ChaosFaultsApplied = "postdream_chaos_faults_total"
)

func init() {
	Register(MessagesReceived, "Messages captured, by inbox.", "inbox")
	Register(BytesReceived, "Bytes captured, by inbox.", "inbox")
	Register(SMTPRejected, "SMTP transactions rejected, by reason.", "reason")
	Register(SMTPAuthFailures, "Failed SMTP AUTH attempts.")
	Register(WebhookDeliveries, "Webhook delivery attempts, by result (success|failure).", "result")
	Register(RetentionDeleted, "Messages deleted by the retention janitor.")
	Register(HTTPRequests, "HTTP API requests, by status class.", "code")
	Register(MessagesReleased, "Messages released to a real SMTP server, by result.", "result")
	Register(POP3Logins, "POP3 login attempts, by result.", "result")
	Register(SpamChecks, "Spam analyses, by engine and result.", "engine", "result")
	Register(ChaosFaultsApplied, "SMTP faults injected by chaos rules, by stage.", "stage")
}
