package testkit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/kafkax"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

func TestAsUserProducesHeadersAcceptedByGatewayVerification(t *testing.T) {
	gs, err := authx.NewGatewaySigner(testkit.GatewayKey)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Authenticated", "spoofed")
	testkit.AsUser(req, 42, "user", "admin")
	id, err := gs.Verify(req.Header)
	if err != nil || id.UserID != 42 || !id.IsAdmin() {
		t.Fatalf("AsUser headers must verify: %+v %v", id, err)
	}
	if req.Header.Get("X-Authenticated") != "" {
		t.Fatal("AsUser must strip foreign identity headers")
	}
	// 没有角色也合法。
	if id, err := gs.Verify(testkit.AsUser(httptest.NewRequest(http.MethodGet, "/x", nil), 7).Header); err != nil || id.UserID != 7 || len(id.Roles) != 0 {
		t.Fatalf("role-less user: %+v %v", id, err)
	}
}

func TestSecretsAreLongEnoughForTheirConsumers(t *testing.T) {
	if _, err := authx.NewGatewaySigner(testkit.GatewayKey); err != nil {
		t.Error(err)
	}
	if _, err := authx.NewTokens(authx.TokenConfig{Secret: testkit.JWTSecret, Issuer: "i", Audience: "a"}); err != nil {
		t.Error(err)
	}
	if len(testkit.InternalToken) < 32 {
		t.Error("internal token too short")
	}
}

func TestDoJSONParsesEnvelopeAndData(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen-User", r.Header.Get("X-User-Id"))
		w.Header().Set("X-Seen-Internal", r.Header.Get("X-Internal-Token"))
		w.Header().Set("X-Seen-Custom", r.Header.Get("X-Custom"))
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"requestId":"r-1","n":3}}`))
	})
	res := testkit.DoJSON(t, h, http.MethodPost, "/x", map[string]int{"a": 1},
		testkit.User(9, "admin"), testkit.Internal(), testkit.Header("X-Custom", "yes"))
	if res.Status != http.StatusAccepted || res.Code != 200 || res.Message != "ok" {
		t.Fatalf("envelope not parsed: %+v", res)
	}
	var data struct {
		RequestID string `json:"requestId"`
		N         int    `json:"n"`
	}
	res.Decode(&data)
	if data.RequestID != "r-1" || data.N != 3 || res.DataIsNull() {
		t.Fatalf("Decode wrong: %+v", data)
	}
	if res.Header.Get("X-Seen-User") != "9" || res.Header.Get("X-Seen-Internal") != testkit.InternalToken || res.Header.Get("X-Seen-Custom") != "yes" {
		t.Fatalf("request options not applied: %v", res.Header)
	}
}

func TestDoJSONHandlesFailureEnvelopeAndNonJSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			_, _ = w.Write([]byte("# HELP x\n"))
			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":10002,"message":"库存不足","data":null,"requestId":"abc"}`))
	})
	res := testkit.DoJSON(t, h, http.MethodGet, "/fail", nil)
	if res.Status != 409 || res.Code != 10002 || res.Message != "库存不足" || res.RequestID != "abc" || !res.DataIsNull() {
		t.Fatalf("failure envelope wrong: %+v", res)
	}
	plain := testkit.DoJSON(t, h, http.MethodGet, "/metrics", nil)
	if plain.Code != 0 || string(plain.Body) != "# HELP x\n" {
		t.Fatalf("non-envelope body wrong: %+v", plain)
	}
}

func TestDoJSONSendsRawStringBodiesVerbatim(t *testing.T) {
	var got string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 64)
		n, _ := r.Body.Read(buf)
		got = string(buf[:n])
		w.WriteHeader(http.StatusOK)
	})
	testkit.DoJSON(t, h, http.MethodPost, "/x", `{"broken":`)
	if got != `{"broken":` {
		t.Fatalf("string body must be sent verbatim, got %q", got)
	}
}

func TestClock(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := testkit.NewClock(start)
	now := c.Now
	c.Advance(time.Minute)
	if !now().Equal(start.Add(time.Minute)) {
		t.Fatal("Advance not visible through the method value")
	}
	c.Advance(-2 * time.Minute)
	if !c.Now().Equal(start.Add(-time.Minute)) {
		t.Fatal("negative advance simulates clock regression")
	}
	c.Set(start)
	if !c.Now().Equal(start) {
		t.Fatal("Set")
	}
}

func TestRedisHelperGivesWorkingLuaCapableClient(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	ctx := context.Background()
	if err := rdb.Set(ctx, "k", "v", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(2 * time.Minute)
	if rdb.Exists(ctx, "k").Val() != 0 {
		t.Fatal("FastForward must expire keys")
	}
	if n, err := rdb.Eval(ctx, `return redis.call('INCR', KEYS[1])`, []string{"c"}).Int64(); err != nil || n != 1 {
		t.Fatalf("Lua must work: %d %v", n, err)
	}
}

// ---------- FakeBroker 自身的行为 ----------

func TestFakeBrokerPartitionsByKeyAndAssignsOffsets(t *testing.T) {
	b := testkit.NewFakeBroker(testkit.WithPartitions(4))
	p := b.Producer()
	ctx := context.Background()
	partOf := map[string]int32{}
	for i := 0; i < 20; i++ {
		for _, k := range []string{"a", "b", "c"} {
			if err := p.Publish(ctx, "t", []byte(k), []byte{byte(i)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	perPartOffsets := map[int32]int64{}
	for _, m := range b.Messages("t") {
		if prev, ok := partOf[string(m.Key)]; ok && prev != m.Partition {
			t.Fatalf("key %s moved partitions", m.Key)
		}
		partOf[string(m.Key)] = m.Partition
		if m.Offset != perPartOffsets[m.Partition] {
			t.Fatalf("offsets must be contiguous per partition: got %d want %d", m.Offset, perPartOffsets[m.Partition])
		}
		perPartOffsets[m.Partition]++
	}
}

func TestFakeBrokerInjectedPublishFailures(t *testing.T) {
	b := testkit.NewFakeBroker()
	p := b.Producer()
	b.FailPublishes(2)
	for i := 0; i < 2; i++ {
		if err := p.Publish(context.Background(), "t", nil, []byte("x")); err != testkit.ErrInjectedPublish {
			t.Fatalf("publish %d: want injected failure, got %v", i, err)
		}
	}
	if err := p.Publish(context.Background(), "t", nil, []byte("x")); err != nil {
		t.Fatalf("failures must be exhausted after n calls: %v", err)
	}
	if len(b.Messages("t")) != 1 || b.PublishCalls() != 3 {
		t.Fatalf("failed publishes must not be stored: msgs=%d calls=%d", len(b.Messages("t")), b.PublishCalls())
	}
}

func TestFakeBrokerPublishHonoursCancelledContext(t *testing.T) {
	b := testkit.NewFakeBroker()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.Producer().Publish(ctx, "t", nil, []byte("x")); err == nil {
		t.Fatal("publish with a cancelled ctx must fail")
	}
}

func TestFakeBrokerMessagesAreIsolatedCopies(t *testing.T) {
	b := testkit.NewFakeBroker()
	v := []byte("original")
	_ = b.Producer().Publish(context.Background(), "t", nil, v, kafkax.WithHeader("h", "1"))
	v[0] = 'X'
	got := b.Messages("t")[0]
	got.Value[0] = 'Y'
	if string(b.Messages("t")[0].Value) != "original" {
		t.Fatal("stored messages must not alias caller buffers")
	}
}

func TestFakeSourcePollBlocksUntilPublishAndRespectsCancel(t *testing.T) {
	b := testkit.NewFakeBroker()
	cfg := b.ConsumerConfig("g", "t")
	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		msgs []kafkax.Message
		err  error
	}
	out := make(chan result, 1)
	go func() {
		m, err := cfg.Source.Poll(ctx)
		out <- result{m, err}
	}()
	select {
	case <-out:
		t.Fatal("Poll must block while there is nothing to read")
	case <-time.After(30 * time.Millisecond):
	}
	_ = b.Producer().Publish(context.Background(), "t", nil, []byte("hello"))
	select {
	case r := <-out:
		if r.err != nil || len(r.msgs) != 1 || string(r.msgs[0].Value) != "hello" {
			t.Fatalf("unexpected: %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Poll did not wake on publish")
	}
	go func() {
		m, err := cfg.Source.Poll(ctx)
		out <- result{m, err}
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case r := <-out:
		if r.err == nil {
			t.Fatal("cancelled Poll must return an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Poll ignored cancellation")
	}
}
