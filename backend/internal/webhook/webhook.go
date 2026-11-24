// Package webhook delivers signed HTTP notifications for inbox events.
//
// Every request carries:
//
//	X-PostDream-Event:     message.received | ping
//	X-PostDream-Delivery:  unique id (stable across retries)
//	X-PostDream-Signature: t=<unix seconds>,v1=<hex HMAC-SHA256(secret, "<t>.<body>")>
//
// Receivers should recompute the HMAC, compare in constant time and reject
// timestamps older than a few minutes to prevent replays.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/matiasrohleder/PostDream/internal/jobs"
	"github.com/matiasrohleder/PostDream/internal/metrics"
	"github.com/matiasrohleder/PostDream/internal/netguard"
	"github.com/matiasrohleder/PostDream/internal/store"
)

const (
	JobKind       = "webhook.deliver"
	EventReceived = "message.received"
	EventPing     = "ping"
	MaxAttempts   = 6
)

// Events lists the subscribable events.
var Events = []string{EventReceived}

// Sign returns the X-PostDream-Signature header value.
func Sign(secret string, ts int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", ts)
	mac.Write(body)
	return "t=" + strconv.FormatInt(ts, 10) + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks a signature header (helper for receivers written in Go, and for tests).
func Verify(secret, header string, body []byte, maxAge time.Duration, now time.Time) bool {
	var ts int64
	var sig string
	for _, part := range strings.Split(header, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			ts, _ = strconv.ParseInt(v, 10, 64)
		case "v1":
			sig = v
		}
	}
	if ts == 0 || sig == "" || now.Sub(time.Unix(ts, 0)).Abs() > maxAge {
		return false
	}
	want := Sign(secret, ts, body)
	return hmac.Equal([]byte(want), []byte("t="+strconv.FormatInt(ts, 10)+",v1="+sig))
}

// JobPayload is what gets queued; the body is frozen at enqueue time so retries
// send exactly the same content even if the message is later deleted.
type JobPayload struct {
	WebhookID  string          `json:"webhook_id"`
	Event      string          `json:"event"`
	DeliveryID string          `json:"delivery_id"`
	Body       json.RawMessage `json:"body"`
}

type Deliverer struct {
	Store     *store.Store
	Policy    netguard.Policy // global outbound policy; the inbox's allowed hosts are added per request
	UserAgent string
}

// Result of one HTTP attempt.
type Result struct {
	StatusCode int    `json:"status_code"`
	DurationMS int    `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
	Response   string `json:"response,omitempty"`
}

// Send performs one signed POST and records it in the delivery log.
func (d *Deliverer) Send(ctx context.Context, w *store.Webhook, event, deliveryID string, body []byte, attempt int) Result {
	inbox, err := d.Store.GetInboxLite(ctx, w.InboxID)
	policy := d.Policy
	if err == nil {
		policy.Allowed = append(append([]string{}, policy.Allowed...), inbox.AllowedHosts...)
	}
	client := netguard.Client(policy, 15*time.Second)

	res := Result{}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", d.UserAgent)
		req.Header.Set("X-PostDream-Event", event)
		req.Header.Set("X-PostDream-Delivery", deliveryID)
		req.Header.Set("X-PostDream-Signature", Sign(w.Secret, time.Now().Unix(), body))
		var resp *http.Response
		resp, err = client.Do(req)
		if err == nil {
			snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			res.StatusCode = resp.StatusCode
			res.Response = strings.ToValidUTF8(string(snippet), "")
			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				err = fmt.Errorf("HTTP %d", resp.StatusCode)
			}
		}
	}
	res.DurationMS = int(time.Since(start).Milliseconds())
	if err != nil {
		res.Error = err.Error()
		metrics.Inc(metrics.WebhookDeliveries, "failure")
	} else {
		metrics.Inc(metrics.WebhookDeliveries, "success")
	}
	bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d.Store.RecordDelivery(bg, &store.WebhookDelivery{
		WebhookID: w.ID, Event: event, Attempt: attempt, StatusCode: res.StatusCode, Error: res.Error, DurationMS: res.DurationMS,
	})
	return res
}

// Handle is the job handler for queued deliveries.
func (d *Deliverer) Handle(ctx context.Context, raw json.RawMessage, attempt int) error {
	var p JobPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return jobs.Permanent{Err: err}
	}
	w, err := d.Store.GetWebhook(ctx, p.WebhookID)
	if errors.Is(err, store.ErrNotFound) {
		return nil // webhook deleted meanwhile
	}
	if err != nil {
		return err
	}
	if !w.Active {
		return nil
	}
	res := d.Send(ctx, w, p.Event, p.DeliveryID, p.Body, attempt)
	if res.Error == "" {
		return nil
	}
	// A blocked destination or 410 Gone will not fix itself: stop retrying.
	if strings.Contains(res.Error, netguard.ErrBlocked.Error()) || res.StatusCode == http.StatusGone {
		return jobs.Permanent{Err: errors.New(res.Error)}
	}
	return errors.New(res.Error)
}
