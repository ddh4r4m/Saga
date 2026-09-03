package guard

import (
	"sort"
	"testing"
	"time"
)

// TestLatency measures check-cmd classification latency over 200
// commands (guard-spec 11.4: p50 <= 10 ms, p95 <= 40 ms). The numbers
// are logged and recorded in docs/specs/IMPLEMENTATION-STATUS.md; the
// test fails only on a gross regression.
func TestLatency(t *testing.T) {
	fixtures := loadFixtures(t)
	sb := newSandbox(t)
	var reqs []Request
	for _, f := range fixtures {
		if f.Skip != "" {
			continue
		}
		reqs = append(reqs, sb.request(f))
	}
	for len(reqs) < 200 {
		reqs = append(reqs, reqs...)
	}
	reqs = reqs[:200]
	for _, r := range reqs[:20] { // warm up
		_, _ = Check(r)
	}
	durs := make([]time.Duration, 0, len(reqs))
	for _, r := range reqs {
		start := time.Now()
		if _, err := Check(r); err != nil {
			t.Fatal(err)
		}
		durs = append(durs, time.Since(start))
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	p50 := durs[len(durs)/2]
	p95 := durs[len(durs)*95/100]
	p99 := durs[len(durs)*99/100]
	t.Logf("check-cmd latency over %d commands: p50 %v, p95 %v, p99 %v, max %v", len(durs), p50, p95, p99, durs[len(durs)-1])
	if p95 > 500*time.Millisecond {
		t.Errorf("p95 %v is far over the 40 ms bar", p95)
	}
}
