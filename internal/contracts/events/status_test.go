package events_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

func TestStatusVocabularyIsPinned(t *testing.T) {
	cases := map[events.Status]string{
		events.StatusQueued:     "QUEUED",
		events.StatusProcessing: "PROCESSING",
		events.StatusSuccess:    "SUCCESS",
		events.StatusFailed:     "FAILED",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("status %q, want %q", got, want)
		}
	}
}

func TestStatusTerminalAndValid(t *testing.T) {
	cases := []struct {
		s        events.Status
		valid    bool
		terminal bool
	}{
		{events.StatusQueued, true, false},
		{events.StatusProcessing, true, false},
		{events.StatusSuccess, true, true},
		{events.StatusFailed, true, true},
		{"", false, false},
		{"queued", false, false}, // 大小写敏感：Redis 哈希里只会出现规范写法
		{"DONE", false, false},
	}
	for _, tc := range cases {
		if got := tc.s.Valid(); got != tc.valid {
			t.Errorf("%q.Valid() = %v, want %v", tc.s, got, tc.valid)
		}
		if got := tc.s.Terminal(); got != tc.terminal {
			t.Errorf("%q.Terminal() = %v, want %v", tc.s, got, tc.terminal)
		}
	}
}

func TestReasonVocabularyIsPinned(t *testing.T) {
	cases := map[events.Reason]string{
		events.ReasonOutOfStock:     "OUT_OF_STOCK",
		events.ReasonRepeat:         "REPEAT",
		events.ReasonTimeout:        "TIMEOUT",
		events.ReasonCouponDisabled: "COUPON_DISABLED",
		events.ReasonCouponNotFound: "COUPON_NOT_FOUND",
		events.ReasonLimitExceeded:  "LIMIT_EXCEEDED",
		events.ReasonEnqueueFailed:  "ENQUEUE_FAILED",
		events.ReasonSystemError:    "SYSTEM_ERROR",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("reason %q, want %q", got, want)
		}
		if !got.Valid() {
			t.Errorf("reason %q should be Valid", got)
		}
	}
	if events.Reason("whatever").Valid() || events.Reason("").Valid() {
		t.Error("unknown reasons must not be Valid")
	}
}

func TestReasonForError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want events.Reason
	}{
		{"out of stock", apperr.OutOfStock, events.ReasonOutOfStock},
		{"wrapped out of stock", fmt.Errorf("deduct: %w", apperr.OutOfStock), events.ReasonOutOfStock},
		{"repeat", apperr.RepeatSeckill, events.ReasonRepeat},
		{"limit exceeded", apperr.CouponLimitExceeded, events.ReasonLimitExceeded},
		{"coupon disabled", apperr.CouponDisabled, events.ReasonCouponDisabled},
		{"coupon not found", apperr.CouponNotFound, events.ReasonCouponNotFound},
		{"system busy", apperr.SystemBusy, events.ReasonSystemError},
		{"plain error", errors.New("boom"), events.ReasonSystemError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := events.ReasonForError(tc.err); got != tc.want {
				t.Fatalf("ReasonForError = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReasonAppErrorRoundTripsWithReasonForError(t *testing.T) {
	for _, r := range []events.Reason{
		events.ReasonOutOfStock, events.ReasonRepeat, events.ReasonCouponDisabled,
		events.ReasonCouponNotFound, events.ReasonLimitExceeded,
	} {
		if got := events.ReasonForError(r.AppError()); got != r {
			t.Errorf("ReasonForError(%q.AppError()) = %q", r, got)
		}
	}
	if got := events.ReasonTimeout.AppError(); got.Code != apperr.CodeSystemBusy {
		t.Errorf("TIMEOUT should surface as SystemBusy, got code %d", got.Code)
	}
	if got := events.Reason("bogus").AppError(); got.Code != apperr.CodeSysError {
		t.Errorf("unknown reason should surface as SysError, got code %d", got.Code)
	}
}

func TestSeckillResultWireFormat(t *testing.T) {
	success, _ := json.Marshal(events.SeckillResult{Status: events.StatusSuccess, OrderID: 1234567890123456789})
	if string(success) != `{"status":"SUCCESS","orderId":"1234567890123456789"}` {
		t.Errorf("success payload = %s", success)
	}
	failed, _ := json.Marshal(events.SeckillResult{Status: events.StatusFailed, Reason: events.ReasonOutOfStock})
	if string(failed) != `{"status":"FAILED","reason":"OUT_OF_STOCK"}` {
		t.Errorf("failed payload = %s", failed)
	}
	queued, _ := json.Marshal(events.SeckillResult{Status: events.StatusQueued})
	if string(queued) != `{"status":"QUEUED"}` {
		t.Errorf("queued payload = %s", queued)
	}

	var back events.SeckillResult
	if err := json.Unmarshal(success, &back); err != nil || back.OrderID != 1234567890123456789 {
		t.Errorf("round trip: %+v err=%v", back, err)
	}
}

func TestSeckillAcceptedWireFormat(t *testing.T) {
	data, _ := json.Marshal(events.SeckillAccepted{RequestID: "r1", Status: events.StatusQueued})
	if string(data) != `{"requestId":"r1","status":"QUEUED"}` {
		t.Errorf("accepted payload = %s", data)
	}
}
