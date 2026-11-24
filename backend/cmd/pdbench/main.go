// Command pdbench is an SMTP load generator for measuring PostDream's ingestion rate.
//
//	go run ./cmd/pdbench -addr localhost:1025 -n 2000 -c 20 -size 20000
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

func main() {
	addr := flag.String("addr", "localhost:1025", "SMTP address")
	n := flag.Int("n", 1000, "messages to send")
	c := flag.Int("c", 10, "concurrent connections")
	size := flag.Int("size", 5000, "approximate body size in bytes")
	user := flag.String("user", "", "SMTP username (optional)")
	pass := flag.String("pass", "", "SMTP password")
	perConn := flag.Int("per-conn", 50, "messages per SMTP connection before reconnecting")
	flag.Parse()

	body := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit.\r\n", max(1, *size/58))
	var sent, failed atomic.Int64
	latencies := make([]time.Duration, 0, *n)
	var mu sync.Mutex
	jobs := make(chan int, *n)
	for i := 0; i < *n; i++ {
		jobs <- i
	}
	close(jobs)

	start := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < *c; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var cl *smtp.Client
			used := 0
			for i := range jobs {
				if cl == nil || used >= *perConn {
					if cl != nil {
						cl.Quit()
					}
					var err error
					cl, err = smtp.Dial(*addr)
					if err == nil && *user != "" {
						err = cl.Auth(sasl.NewPlainClient("", *user, *pass))
					}
					if err != nil {
						failed.Add(1)
						cl = nil
						continue
					}
					used = 0
				}
				msg := fmt.Sprintf("From: bench@pdbench.test\r\nTo: user%d@pdbench.test\r\nSubject: Bench %d\r\n"+
					"Message-ID: <bench-%d-%d@pdbench.test>\r\n\r\n%s", i, i, start.UnixNano(), i, body)
				t := time.Now()
				err := cl.SendMail("bench@pdbench.test", []string{fmt.Sprintf("user%d@pdbench.test", i)}, strings.NewReader(msg))
				if err != nil {
					failed.Add(1)
					cl.Close()
					cl = nil
					continue
				}
				used++
				sent.Add(1)
				mu.Lock()
				latencies = append(latencies, time.Since(t))
				mu.Unlock()
			}
			if cl != nil {
				cl.Quit()
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	pct := func(p float64) time.Duration {
		if len(latencies) == 0 {
			return 0
		}
		return latencies[min(len(latencies)-1, int(float64(len(latencies))*p))]
	}
	fmt.Printf("sent=%d failed=%d elapsed=%s rate=%.1f msg/s  latency p50=%s p95=%s p99=%s\n",
		sent.Load(), failed.Load(), elapsed.Round(time.Millisecond), float64(sent.Load())/elapsed.Seconds(),
		pct(0.50).Round(time.Millisecond), pct(0.95).Round(time.Millisecond), pct(0.99).Round(time.Millisecond))
	if failed.Load() > 0 {
		os.Exit(1)
	}
}
