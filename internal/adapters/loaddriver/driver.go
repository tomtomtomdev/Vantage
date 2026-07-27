package loaddriver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/tomtomtomdev/vantage/internal/domain"
	"github.com/tomtomtomdev/vantage/internal/hist"
	"github.com/tomtomtomdev/vantage/internal/ports"
)

// Config parameterises the driver. Workers is the bounded pool size (it models
// the target's real connection limit); MaxRPS is the rate ceiling (SPEC §9, 0 ⇒
// none); AllowMutating is the explicit override a mutating Target requires;
// RequestTimeout caps every outbound request (CLAUDE §4 — no unbounded I/O).
type Config struct {
	Workers        int
	MaxRPS         int
	AllowMutating  bool
	RequestTimeout time.Duration
}

// Driver is the open-model load driver. It satisfies ports.LoadDriver.
type Driver struct {
	client        *http.Client
	workers       int
	maxRPS        int
	allowMutating bool
	reqTimeout    time.Duration
}

// New constructs a Driver. The HTTP transport reuses connections up to the pool
// size so the measurement is of the server, not of connection churn.
func New(cfg Config) *Driver {
	if cfg.Workers < 1 {
		cfg.Workers = 1
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 30 * time.Second
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = cfg.Workers
	tr.MaxIdleConnsPerHost = cfg.Workers
	tr.MaxConnsPerHost = cfg.Workers
	return &Driver{
		client:        &http.Client{Transport: tr}, // no client.Timeout: the per-request ctx owns the deadline
		workers:       cfg.Workers,
		maxRPS:        cfg.MaxRPS,
		allowMutating: cfg.AllowMutating,
		reqTimeout:    cfg.RequestTimeout,
	}
}

// CloseIdleConns releases keep-alive connections. Call it when the driver is done
// (also keeps goleak happy in tests).
func (d *Driver) CloseIdleConns() { d.client.CloseIdleConnections() }

// job is one scheduled arrival: its sequence, the time it was *intended* to
// dispatch (the anchor for latency), and whether it falls in the warmup window.
type job struct {
	intended time.Time
	warmup   bool
}

// outcome is one completed request as seen by a worker.
type outcome struct {
	latency    time.Duration // completion − intended dispatch (captures queueing)
	completion time.Time
	ok         bool
	class      string // error classification when !ok ("500", "timeout", "transport")
	warmup     bool
}

// Run measures one repetition. It refuses at the entry path on any blast-radius
// violation (SPEC §9) before generating load, then drives an open-model schedule
// through a bounded worker pool and returns the post-warmup, success-only result.
func (d *Driver) Run(ctx context.Context, t domain.Target, p domain.LoadProfile) (domain.RepResult, error) {
	if err := d.guard(t, p); err != nil {
		return domain.RepResult{}, err
	}

	start := time.Now()
	interval := time.Duration(float64(time.Second) / float64(p.RPS))
	n := int(math.Round(float64(p.RPS) * p.Duration.Seconds()))
	if n < 1 {
		n = 1
	}
	warmupCutoff := start.Add(p.Warmup)

	jobs := make(chan job, n) // buffered to n: the producer never blocks on slow
	//                           workers, so the schedule stays independent of
	//                           completion (the coordinated-omission fix).
	results := make(chan outcome, d.workers)

	// Producer: emits arrivals on the fixed schedule, recording how late it was
	// (scheduling delay = driver self-overhead). It owns and closes jobs.
	var (
		prodWG     sync.WaitGroup
		schedDelay time.Duration // summed lateness over emitted jobs
		emitted    int
	)
	prodWG.Add(1)
	go func() {
		defer prodWG.Done()
		defer close(jobs)
		for i := 0; i < n; i++ {
			intended := start.Add(time.Duration(i) * interval)
			if wait := time.Until(intended); wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			schedDelay += time.Since(intended) // ≥0; grows if the driver can't keep up
			emitted++
			jobs <- job{intended: intended, warmup: intended.Before(warmupCutoff)}
		}
	}()

	// Workers: bounded pool. Each ranges jobs, honours ctx as the kill switch, and
	// reports every completion on results.
	var workWG sync.WaitGroup
	for w := 0; w < d.workers; w++ {
		workWG.Add(1)
		go func() {
			defer workWG.Done()
			for j := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				results <- d.do(ctx, t, j)
			}
		}()
	}

	// Closer: once every worker has exited, no more outcomes can arrive — the sole
	// sender closes results so the collector's range terminates.
	go func() {
		workWG.Wait()
		close(results)
	}()

	// Collector (this goroutine): success-only histogram + separate error tally.
	h := hist.New()
	errs := map[string]int{}
	var success, failed int
	var lastCompletion time.Time
	for o := range results {
		if o.warmup {
			continue // steady-state only
		}
		if o.completion.After(lastCompletion) {
			lastCompletion = o.completion
		}
		if o.ok {
			success++
			if err := h.RecordDuration(o.latency); err != nil {
				return domain.RepResult{}, fmt.Errorf("recording latency: %w", err)
			}
		} else {
			failed++
			errs[o.class]++
		}
	}
	prodWG.Wait() // schedDelay/emitted are now safe to read

	if err := ctx.Err(); err != nil {
		return domain.RepResult{}, fmt.Errorf("run aborted: %w", err)
	}

	return d.summarise(p, h, errs, success, failed, warmupCutoff, lastCompletion, schedDelay, emitted), nil
}

// guard enforces the blast-radius rules (SPEC §9) before any load.
func (d *Driver) guard(t domain.Target, p domain.LoadProfile) error {
	if !t.Allowlisted {
		return fmt.Errorf("%w: %q", domain.ErrTargetNotAllowlisted, t.Name)
	}
	if t.Mutating && !d.allowMutating {
		return fmt.Errorf("%w: %q", domain.ErrMutatingRefused, t.Name)
	}
	if d.maxRPS > 0 && p.RPS > d.maxRPS {
		return fmt.Errorf("%w: requested %d > ceiling %d", domain.ErrRateCeiling, p.RPS, d.maxRPS)
	}
	return nil
}

// do issues one request and measures latency from the intended dispatch time, so
// time the arrival spent queued in the client accrues to the tail.
func (d *Driver) do(ctx context.Context, t domain.Target, j job) outcome {
	reqCtx, cancel := context.WithTimeout(ctx, d.reqTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, t.BaseURL, nil)
	if err != nil {
		return outcome{completion: time.Now(), ok: false, class: "request", warmup: j.warmup}
	}

	resp, err := d.client.Do(req)
	completion := time.Now()
	if err != nil {
		class := "transport"
		if errors.Is(err, context.DeadlineExceeded) {
			class = "timeout"
		}
		return outcome{completion: completion, ok: false, class: class, warmup: j.warmup}
	}
	// Drain and close so the connection is reusable (keep-alive).
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	o := outcome{latency: completion.Sub(j.intended), completion: completion, ok: ok, warmup: j.warmup}
	if !ok {
		o.class = strconv.Itoa(resp.StatusCode)
	}
	return o
}

// summarise turns the collected samples into a RepResult.
func (d *Driver) summarise(
	p domain.LoadProfile, h *hist.Histogram, errs map[string]int,
	success, failed int, warmupCutoff, lastCompletion time.Time,
	schedDelay time.Duration, emitted int,
) domain.RepResult {
	measured := success + failed

	var errorRate float64
	if measured > 0 {
		errorRate = float64(failed) / float64(measured)
	}

	// Achieved throughput over the steady-state window: measured completions
	// divided by the wall time from the end of warmup to the last completion. Under
	// saturation this drain time exceeds the requested window, so achieved rps
	// falls below requested — the honest signal that the pool couldn't keep up.
	var achievedRPS float64
	if measured > 0 {
		elapsed := lastCompletion.Sub(warmupCutoff).Seconds()
		if elapsed <= 0 {
			elapsed = p.MeasuredWindow().Seconds()
		}
		if elapsed > 0 {
			achievedRPS = float64(measured) / elapsed
		}
	}

	var overheadMs float64
	if emitted > 0 {
		overheadMs = float64(schedDelay.Microseconds()) / float64(emitted) / 1000.0
	}

	blob, _ := h.Encode() // Encode only fails on a broken gob target, not on data

	return domain.RepResult{
		Histogram:        blob,
		P50Ms:            h.PercentileMillis(50),
		P90Ms:            h.PercentileMillis(90),
		P99Ms:            h.PercentileMillis(99),
		P999Ms:           h.PercentileMillis(99.9),
		MaxMs:            h.MaxMillis(),
		AchievedRPS:      achievedRPS,
		ErrorRate:        errorRate,
		Errors:           errs,
		DriverOverheadMs: overheadMs,
	}
}

// compile-time proof the adapter satisfies the consumer-owned port.
var _ ports.LoadDriver = (*Driver)(nil)
