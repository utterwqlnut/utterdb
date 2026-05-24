// utterdb load tester
//
// Connects a pool of workers to the NLB (or any proxy) over raw TCP and
// hammers it with a 70 % GET / 30 % WRITE mix, matching the old Locust ratio.
//
// Metrics reported every --interval seconds and at the end:
//   - Throughput  (req/s)
//   - Latency     p50 / p95 / p99  (µs)
//   - Failures    (count + rate)
//
// Usage:
//   go run src/test/benchmark.go [flags]
//
// Flags:
//   --host      target host          (default: localhost)
//   --port      target port          (default: 9000)
//   --workers   concurrent workers   (default: 50)
//   --duration  test duration        (default: 60s)
//   --interval  report interval      (default: 10s)
//   --seed      number of seed keys  (default: 500)
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ── CLI flags ─────────────────────────────────────────────────────────────────

var (
	host     = flag.String("host", "localhost", "Target host")
	port     = flag.Int("port", 9000, "Target port")
	workers  = flag.Int("workers", 50, "Number of concurrent workers")
	duration = flag.Duration("duration", 60*time.Second, "Total test duration (e.g. 60s, 2m)")
	interval = flag.Duration("interval", 10*time.Second, "Stats reporting interval")
	seedKeys = flag.Int("seed", 500, "Number of keys to pre-populate before the test")
)

// ── Shared state ──────────────────────────────────────────────────────────────

type sharedKeys struct {
	mu   sync.RWMutex
	keys []string
}

func (s *sharedKeys) add(k string) {
	s.mu.Lock()
	s.keys = append(s.keys, k)
	s.mu.Unlock()
}

func (s *sharedKeys) random() (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.keys) == 0 {
		return "", false
	}
	return s.keys[rand.Intn(len(s.keys))], true
}

// ── Per-interval sample bucket ────────────────────────────────────────────────

type bucket struct {
	mu       sync.Mutex
	latencies []int64 // µs
	failures  int64
}

func (b *bucket) record(latUS int64, failed bool) {
	b.mu.Lock()
	b.latencies = append(b.latencies, latUS)
	if failed {
		b.failures++
	}
	b.mu.Unlock()
}

func (b *bucket) drain() (latencies []int64, failures int64) {
	b.mu.Lock()
	latencies = b.latencies
	failures = b.failures
	b.latencies = nil
	b.failures = 0
	b.mu.Unlock()
	return
}

// ── Percentile helper ─────────────────────────────────────────────────────────

func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

// ── TCP worker ────────────────────────────────────────────────────────────────

type worker struct {
	addr   string
	conn   net.Conn
	reader *bufio.Reader
	rng    *rand.Rand
}

func newWorker(addr string) (*worker, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &worker{
		addr:   addr,
		conn:   conn,
		reader: bufio.NewReader(conn),
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

func (w *worker) reconnect() error {
	if w.conn != nil {
		w.conn.Close()
	}
	conn, err := net.DialTimeout("tcp", w.addr, 5*time.Second)
	if err != nil {
		return err
	}
	w.conn = conn
	w.reader = bufio.NewReader(conn)
	return nil
}

// sendCmd sends a newline-terminated command and reads one line back.
// Returns (response, latencyµs, error).
func (w *worker) sendCmd(cmd string) (string, int64, error) {
	start := time.Now()
	w.conn.SetDeadline(time.Now().Add(2 * time.Second))

	_, err := fmt.Fprintf(w.conn, "%s\n", cmd)
	if err != nil {
		return "", 0, err
	}

	resp, err := w.reader.ReadString('\n')
	latUS := time.Since(start).Microseconds()
	if err != nil {
		return "", latUS, err
	}
	return strings.TrimSpace(resp), latUS, nil
}

func (w *worker) doWrite(key, val string) (int64, bool) {
	cmd := fmt.Sprintf("WRITE|%s|string|%s|string", key, val)
	resp, lat, err := w.sendCmd(cmd)
	if err != nil || !strings.HasPrefix(resp, "OK") {
		w.reconnect() // best-effort
		return lat, true
	}
	return lat, false
}

func (w *worker) doGet(key string) (int64, bool) {
	cmd := fmt.Sprintf("GET|%s|string", key)
	resp, lat, err := w.sendCmd(cmd)
	if err != nil || strings.HasPrefix(resp, "ERR") {
		w.reconnect()
		return lat, true
	}
	return lat, false
}

// ── Seeding ───────────────────────────────────────────────────────────────────

func seedCluster(addr string, n int, keys *sharedKeys) error {
	w, err := newWorker(addr)
	if err != nil {
		return fmt.Errorf("seed connect: %w", err)
	}
	defer w.conn.Close()

	fmt.Printf("Seeding %d keys...\n", n)
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("seed_%d", i)
		val := fmt.Sprintf("value_%d", i)
		_, failed := w.doWrite(key, val)
		if failed {
			return fmt.Errorf("seed write failed at key %d", i)
		}
		keys.add(key)
	}
	fmt.Printf("Seeded %d keys.\n", n)
	return nil
}

// ── Stats printer ─────────────────────────────────────────────────────────────

func printStats(label string, elapsed time.Duration, lats []int64, failures int64) {
	total := int64(len(lats))
	if total == 0 {
		fmt.Printf("[%s] no requests recorded\n", label)
		return
	}

	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })

	throughput := float64(total) / elapsed.Seconds()
	failRate := float64(failures) / float64(total) * 100

	fmt.Printf(
		"[%s] reqs=%-6d  tput=%-8.1f req/s  "+
			"p50=%-6dµs  p95=%-6dµs  p99=%-6dµs  "+
			"failures=%d (%.1f%%)\n",
		label,
		total,
		throughput,
		percentile(lats, 0.50),
		percentile(lats, 0.95),
		percentile(lats, 0.99),
		failures,
		failRate,
	)
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", *host, *port)
	fmt.Printf("utterdb load test  target=%s  workers=%d  duration=%s\n",
		addr, *workers, *duration)

	// Seed keys shared across all workers
	keys := &sharedKeys{}
	if err := seedCluster(addr, *seedKeys, keys); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR seeding: %v\n", err)
		os.Exit(1)
	}

	// Global counters for the final summary
	var totalReqs int64
	var totalFails int64
	allLats := make([]int64, 0, int(*duration/time.Millisecond)**workers)
	var allLatsMu sync.Mutex

	// Per-interval bucket
	cur := &bucket{}

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// ── Spawn workers ─────────────────────────────────────────────────────────
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			w, err := newWorker(addr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "worker connect error: %v\n", err)
				return
			}
			defer w.conn.Close()

			for {
				select {
				case <-stop:
					return
				default:
				}

				var lat int64
				var failed bool

				// 70 % GET, 30 % WRITE
				if w.rng.Intn(10) < 7 {
					key, ok := keys.random()
					if !ok {
						continue
					}
					lat, failed = w.doGet(key)
				} else {
					key := fmt.Sprintf("bench_%d_%d", time.Now().UnixNano(), w.rng.Int63())
					val := "bench_val"
					lat, failed = w.doWrite(key, val)
					if !failed {
						keys.add(key)
					}
				}

				cur.record(lat, failed)
				atomic.AddInt64(&totalReqs, 1)
				if failed {
					atomic.AddInt64(&totalFails, 1)
				}

				allLatsMu.Lock()
				allLats = append(allLats, lat)
				allLatsMu.Unlock()
			}
		}()
	}

	// ── Interval reporter ─────────────────────────────────────────────────────
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	testEnd := time.After(*duration)
	start := time.Now()
	intervalStart := start

	fmt.Println(strings.Repeat("-", 90))

loop:
	for {
		select {
		case t := <-ticker.C:
			lats, fails := cur.drain()
			elapsed := t.Sub(intervalStart)
			intervalStart = t
			printStats(t.Format("15:04:05"), elapsed, lats, fails)

		case <-testEnd:
			break loop
		}
	}

	close(stop)
	wg.Wait()

	// ── Final summary ─────────────────────────────────────────────────────────
	fmt.Println(strings.Repeat("=", 90))
	printStats("TOTAL", time.Since(start), allLats, atomic.LoadInt64(&totalFails))
	fmt.Printf("Total requests: %d\n", atomic.LoadInt64(&totalReqs))
	fmt.Println(strings.Repeat("=", 90))
}
