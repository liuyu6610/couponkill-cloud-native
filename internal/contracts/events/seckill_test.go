package events_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
)

func validMsg() events.SeckillOrderCreate {
	return events.SeckillOrderCreate{
		RequestID:    "req-0001",
		UserID:       1234567890123456789, // 超过 2^53，验证 ID 以字符串传输不丢精度
		CouponID:     1001,
		EnqueuedAtMs: 1760000000123,
	}
}

func TestTopicAndGroupConstantsArePinned(t *testing.T) {
	if events.TopicSeckillOrderCreate != "seckill_order_create" {
		t.Errorf("TopicSeckillOrderCreate = %q", events.TopicSeckillOrderCreate)
	}
	if events.TopicSeckillOrderCreateDLT != "seckill_order_create_dlt" {
		t.Errorf("TopicSeckillOrderCreateDLT = %q", events.TopicSeckillOrderCreateDLT)
	}
	if events.GroupSeckillOrderCreate != "seckill-order-create-group" {
		t.Errorf("GroupSeckillOrderCreate = %q", events.GroupSeckillOrderCreate)
	}
}

func TestMarshalWireFormatPinsFieldNamesAndStringIDs(t *testing.T) {
	data, err := events.MarshalSeckillOrderCreate(validMsg())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	want := map[string]any{
		"requestId":    "req-0001",
		"userId":       "1234567890123456789",
		"couponId":     "1001",
		"enqueuedAtMs": float64(1760000000123),
	}
	if len(raw) != len(want) {
		t.Fatalf("payload fields = %v, want exactly %v", raw, want)
	}
	for k, v := range want {
		if raw[k] != v {
			t.Errorf("field %s = %#v, want %#v", k, raw[k], v)
		}
	}
}

func TestRoundTripPreservesLargeIDs(t *testing.T) {
	in := validMsg()
	data, err := events.MarshalSeckillOrderCreate(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := events.UnmarshalSeckillOrderCreate(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Fatalf("round trip mismatch: got %+v want %+v", out, in)
	}
}

func TestUnmarshalIgnoresUnknownFields(t *testing.T) {
	payload := `{"requestId":"req-1","userId":"7","couponId":"1001","enqueuedAtMs":5,"traceId":"abc","v2":{"x":1}}`
	out, err := events.UnmarshalSeckillOrderCreate([]byte(payload))
	if err != nil {
		t.Fatalf("unknown fields must be ignored for forward compatibility: %v", err)
	}
	if out.RequestID != "req-1" || out.UserID != 7 || out.CouponID != 1001 || out.EnqueuedAtMs != 5 {
		t.Fatalf("unexpected decode: %+v", out)
	}
}

func TestValidationRejectsBadMessages(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*events.SeckillOrderCreate)
	}{
		{"empty request id", func(m *events.SeckillOrderCreate) { m.RequestID = "" }},
		{"zero user id", func(m *events.SeckillOrderCreate) { m.UserID = 0 }},
		{"negative user id", func(m *events.SeckillOrderCreate) { m.UserID = -1 }},
		{"zero coupon id", func(m *events.SeckillOrderCreate) { m.CouponID = 0 }},
		{"zero enqueued time", func(m *events.SeckillOrderCreate) { m.EnqueuedAtMs = 0 }},
		{"request id with space", func(m *events.SeckillOrderCreate) { m.RequestID = "a b" }},
		{"request id with newline", func(m *events.SeckillOrderCreate) { m.RequestID = "a\nb" }},
		{"request id too long", func(m *events.SeckillOrderCreate) { m.RequestID = strings.Repeat("a", 129) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validMsg()
			tc.mutate(&m)
			if err := m.Validate(); !errors.Is(err, events.ErrInvalidEvent) {
				t.Fatalf("Validate() = %v, want ErrInvalidEvent", err)
			}
			if _, err := events.MarshalSeckillOrderCreate(m); !errors.Is(err, events.ErrInvalidEvent) {
				t.Fatalf("Marshal err = %v, want ErrInvalidEvent", err)
			}
		})
	}
}

func TestUnmarshalRejectsBadPayloads(t *testing.T) {
	cases := map[string]string{
		"not json":            `nope`,
		"empty":               ``,
		"json null":           `null`,
		"numeric user id":     `{"requestId":"r","userId":7,"couponId":"1","enqueuedAtMs":1}`,
		"zero user id":        `{"requestId":"r","userId":"0","couponId":"1","enqueuedAtMs":1}`,
		"missing request id":  `{"userId":"7","couponId":"1","enqueuedAtMs":1}`,
		"non-numeric coupon":  `{"requestId":"r","userId":"7","couponId":"abc","enqueuedAtMs":1}`,
		"missing coupon":      `{"requestId":"r","userId":"7","enqueuedAtMs":1}`,
		"trailing garbage":    `{"requestId":"r","userId":"7","couponId":"1","enqueuedAtMs":1} x`,
		"array instead of {}": `[]`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := events.UnmarshalSeckillOrderCreate([]byte(payload))
			if !errors.Is(err, events.ErrInvalidEvent) {
				t.Fatalf("err = %v, want ErrInvalidEvent", err)
			}
		})
	}
}

func TestPartitionKeyIsUserID(t *testing.T) {
	m := validMsg()
	if got := string(m.Key()); got != "1234567890123456789" {
		t.Fatalf("Key() = %q, want user id", got)
	}
	if got := string(events.SeckillOrderCreateKey(42)); got != "42" {
		t.Fatalf("SeckillOrderCreateKey(42) = %q", got)
	}
	// 同一用户的所有消息必须落在同一分区，且与 requestId 无关。
	a, b := validMsg(), validMsg()
	b.RequestID = "another"
	if string(a.Key()) != string(b.Key()) {
		t.Fatal("key must depend on userID only")
	}
}

func TestNewSeckillOrderCreateStampsEnqueueTime(t *testing.T) {
	now := time.UnixMilli(1760000000999)
	m := events.NewSeckillOrderCreate("req-9", 7, 1001, now)
	if m.EnqueuedAtMs != 1760000000999 || m.RequestID != "req-9" || m.UserID != 7 || m.CouponID != 1001 {
		t.Fatalf("unexpected message: %+v", m)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("constructed message must be valid: %v", err)
	}
}
