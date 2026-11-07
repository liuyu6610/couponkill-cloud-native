package platforms

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

func TestMaskKey(t *testing.T) {
	for in, want := range map[string]string{"": "", "abcd": "****", "ab": "****", "abcde": "ab****de", "0123456789": "01****89"} {
		if got := MaskKey(in); got != want {
			t.Errorf("MaskKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// 传输层错误（连接被拒）不得把带签名/凭证的请求内容带进错误文案；配置的结构体被格式化时同样不得泄露。
func TestTransportErrorsAndFormattingDoNotLeakCredentials(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // 端口已关闭：连接必被拒绝

	clk := testkit.NewClock(time.Now())
	jd := NewJD(JDConfig{
		ServerURL: "http://" + addr + "/routerjson?app_key=LEAKME",
		AppKey:    "LEAK-KEY-123", AppSecret: "LEAK-SECRET-456", AccessToken: "LEAK-TOKEN-789",
		Caller: CallerConfig{Retries: -1, Sleep: func(context.Context, time.Duration) error { return nil }},
		Now:    clk.Now,
	})
	_, err = jd.Product(context.Background(), "1")
	if err == nil {
		t.Fatal("want connection error")
	}
	for _, secret := range []string{"LEAK-KEY-123", "LEAK-SECRET-456", "LEAK-TOKEN-789", "LEAKME"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaks %q: %v", secret, err)
		}
	}
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if out := fmt.Sprintf(format, jd.cfg); strings.Contains(out, "LEAK-SECRET-456") || strings.Contains(out, "LEAK-TOKEN-789") || strings.Contains(out, "LEAK-KEY-123") {
			t.Fatalf("config formatted with %s leaks credentials: %s", format, out)
		}
	}
}
