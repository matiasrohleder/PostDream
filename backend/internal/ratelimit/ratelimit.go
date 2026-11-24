// Package ratelimit implements a small fixed-window failure counter used to
// slow down brute-force attempts against UI logins and SMTP AUTH.
package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string]*entry
}

type entry struct {
	count int
	reset time.Time
}

func New(max int, window time.Duration) *Limiter {
	l := &Limiter{max: max, window: window, hits: map[string]*entry{}}
	go func() {
		for range time.Tick(window) {
			l.mu.Lock()
			now := time.Now()
			for k, e := range l.hits {
				if now.After(e.reset) {
					delete(l.hits, k)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

// Blocked reports whether key exceeded the allowed failures in the window.
func (l *Limiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.hits[key]
	return ok && time.Now().Before(e.reset) && e.count >= l.max
}

func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.hits[key]
	if !ok || time.Now().After(e.reset) {
		e = &entry{reset: time.Now().Add(l.window)}
		l.hits[key] = e
	}
	e.count++
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.hits, key)
	l.mu.Unlock()
}
