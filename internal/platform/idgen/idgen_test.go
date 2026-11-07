package idgen_test

import (
	"encoding/json"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newGen(t *testing.T, worker int64, clk *fakeClock) *idgen.Generator {
	t.Helper()
	g, err := idgen.New(worker, idgen.WithClock(clk.Now))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

func TestNewRejectsOutOfRangeWorker(t *testing.T) {
	for _, w := range []int64{-1, 1024, 5000} {
		if _, err := idgen.New(w); err == nil {
			t.Errorf("New(%d) must fail", w)
		}
	}
	for _, w := range []int64{0, 1, 1023} {
		if _, err := idgen.New(w); err != nil {
			t.Errorf("New(%d): %v", w, err)
		}
	}
}

func TestIDsAreStrictlyIncreasingWithinOneMillisecond(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	g := newGen(t, 7, clk)
	prev := g.Next()
	for i := 0; i < 10000; i++ { // 同一毫秒内远超 4096 个：序列号溢出必须推进逻辑时间而不是重复
		id := g.Next()
		if id <= prev {
			t.Fatalf("id %d not greater than previous %d at i=%d", id, prev, i)
		}
		prev = id
	}
}

func TestClockRegressionNeverGoesBackwardsOrDuplicates(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 10, 0, time.UTC)}
	g := newGen(t, 3, clk)
	seen := map[int64]bool{}
	prev := int64(0)
	step := func() {
		id := g.Next()
		if id <= prev || seen[id] {
			t.Fatalf("id %d violates monotonic/unique (prev %d)", id, prev)
		}
		seen[id], prev = true, id
	}
	for i := 0; i < 100; i++ {
		step()
	}
	clk.Add(-5 * time.Second) // 时钟大幅回拨
	for i := 0; i < 5000; i++ {
		step()
	}
	clk.Add(20 * time.Second) // 恢复并超过之前
	for i := 0; i < 100; i++ {
		step()
	}
}

func TestEncodesWorkerAndTimestamp(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)}
	g := newGen(t, 513, clk)
	id := g.Next()
	if got := idgen.WorkerOf(id); got != 513 {
		t.Errorf("WorkerOf = %d, want 513", got)
	}
	if got := idgen.TimeOf(id); !got.Equal(clk.Now()) {
		t.Errorf("TimeOf = %v, want %v", got, clk.Now())
	}
	if id <= 0 {
		t.Errorf("id must be positive, got %d", id)
	}
}

func TestConcurrentUniqueness(t *testing.T) {
	g, err := idgen.New(11)
	if err != nil {
		t.Fatal(err)
	}
	const goroutines, per = 32, 4000
	out := make(chan []int64, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids := make([]int64, per)
			for j := range ids {
				ids[j] = g.Next()
			}
			out <- ids
		}()
	}
	wg.Wait()
	close(out)
	seen := make(map[int64]struct{}, goroutines*per)
	for ids := range out {
		prev := int64(0)
		for _, id := range ids {
			if id <= prev {
				t.Fatalf("per-goroutine order violated: %d after %d", id, prev)
			}
			prev = id
			if _, dup := seen[id]; dup {
				t.Fatalf("duplicate id %d", id)
			}
			seen[id] = struct{}{}
		}
	}
	if len(seen) != goroutines*per {
		t.Fatalf("got %d unique ids, want %d", len(seen), goroutines*per)
	}
}

func TestDifferentWorkersNeverCollide(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	a, b := newGen(t, 1, clk), newGen(t, 2, clk)
	seen := map[int64]bool{}
	for i := 0; i < 3000; i++ {
		for _, id := range []int64{a.Next(), b.Next()} {
			if seen[id] {
				t.Fatalf("collision across workers: %d", id)
			}
			seen[id] = true
		}
	}
}

func TestNextStringIsDecimal(t *testing.T) {
	g, _ := idgen.New(1)
	s := g.NextString()
	if _, err := strconv.ParseInt(s, 10, 64); err != nil {
		t.Fatalf("NextString %q is not decimal: %v", s, err)
	}
}

func TestResolveWorkerID(t *testing.T) {
	if got := idgen.ResolveWorkerID(5); got != 5 {
		t.Errorf("explicit worker id must win, got %d", got)
	}
	a, b := idgen.ResolveWorkerID(-1), idgen.ResolveWorkerID(-1)
	if a != b || a < 0 || a > 1023 {
		t.Errorf("hostname-derived id must be stable and in range, got %d / %d", a, b)
	}
	if idgen.WorkerIDFromName("order-7d9f8c6b5-abcde") == idgen.WorkerIDFromName("order-7d9f8c6b5-abcdf") {
		t.Log("hash collision on adjacent names is possible but unlikely; not a failure")
	}
	if got := idgen.WorkerIDFromName("x"); got < 0 || got > 1023 {
		t.Errorf("out of range: %d", got)
	}
}

func TestIDJSONRoundTrip(t *testing.T) {
	const big = int64(9007199254740993) // 超过 JS 安全整数
	b, err := json.Marshal(struct {
		ID idgen.ID `json:"id"`
	}{idgen.ID(big)})
	if err != nil || string(b) != `{"id":"9007199254740993"}` {
		t.Fatalf("Marshal = %s, %v (IDs must be strings)", b, err)
	}
	for in, want := range map[string]int64{
		`{"id":"9007199254740993"}`: big,
		`{"id":9007199254740993}`:   big,
		`{"id":"42"}`:               42,
		`{"id":null}`:               0,
	} {
		var out struct {
			ID idgen.ID `json:"id"`
		}
		if err := json.Unmarshal([]byte(in), &out); err != nil || int64(out.ID) != want {
			t.Errorf("Unmarshal(%s) = %d, %v; want %d", in, out.ID, err, want)
		}
	}
	for _, bad := range []string{`{"id":"abc"}`, `{"id":1.5}`, `{"id":true}`, `{"id":""}`, `{"id":"9223372036854775808"}`} {
		var out struct {
			ID idgen.ID `json:"id"`
		}
		if err := json.Unmarshal([]byte(bad), &out); err == nil {
			t.Errorf("Unmarshal(%s) must fail", bad)
		}
	}
	if idgen.ID(7).String() != "7" {
		t.Error("String")
	}
	if _, err := idgen.ParseID(" 5"); err == nil {
		t.Error("ParseID must be strict")
	}
}
