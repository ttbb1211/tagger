package server

import (
	"bytes"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/ut"
)

// fakeClock 让退避逻辑的测试不必真的等 5 分钟。
type fakeClock struct{ current time.Time }

func (f *fakeClock) now() time.Time { return f.current }

func (f *fakeClock) advance(d time.Duration) { f.current = f.current.Add(d) }

func newTestGuard(clock *fakeClock) *authGuard {
	guard := newAuthGuard()
	guard.now = clock.now
	return guard
}

func TestAuthGuardLocksAfterLimitFailures(t *testing.T) {
	clock := &fakeClock{current: time.Unix(1700000000, 0)}
	guard := newTestGuard(clock)

	for i := 1; i < authFailureLimit; i++ {
		if wait := guard.recordFailure("1.2.3.4"); wait != 0 {
			t.Fatalf("failure %d locked too early: %v", i, wait)
		}
		if locked := guard.lockedFor("1.2.3.4"); locked != 0 {
			t.Fatalf("failure %d should not be locked: %v", i, locked)
		}
	}
	// 第 limit 次失败触发锁定
	if wait := guard.recordFailure("1.2.3.4"); wait != authLockout {
		t.Fatalf("lockout = %v, want %v", wait, authLockout)
	}
	if locked := guard.lockedFor("1.2.3.4"); locked != authLockout {
		t.Fatalf("lockedFor = %v, want %v", locked, authLockout)
	}
	// 其他客户端不受影响
	if locked := guard.lockedFor("5.6.7.8"); locked != 0 {
		t.Fatalf("other client locked: %v", locked)
	}
}

func TestAuthGuardLockExpires(t *testing.T) {
	clock := &fakeClock{current: time.Unix(1700000000, 0)}
	guard := newTestGuard(clock)
	for i := 0; i < authFailureLimit; i++ {
		guard.recordFailure("1.2.3.4")
	}
	clock.advance(authLockout - time.Second)
	if locked := guard.lockedFor("1.2.3.4"); locked <= 0 {
		t.Fatalf("should still be locked, got %v", locked)
	}
	clock.advance(2 * time.Second)
	if locked := guard.lockedFor("1.2.3.4"); locked != 0 {
		t.Fatalf("lock should have expired, got %v", locked)
	}
	// 锁定过期后计数已清零：再失败一次不应立即重新锁定
	if wait := guard.recordFailure("1.2.3.4"); wait != 0 {
		t.Fatalf("counter should reset after lockout: %v", wait)
	}
}

func TestAuthGuardSuccessResetsCounter(t *testing.T) {
	clock := &fakeClock{current: time.Unix(1700000000, 0)}
	guard := newTestGuard(clock)
	for i := 0; i < authFailureLimit-1; i++ {
		guard.recordFailure("1.2.3.4")
	}
	guard.recordSuccess("1.2.3.4")
	for i := 0; i < authFailureLimit-1; i++ {
		if wait := guard.recordFailure("1.2.3.4"); wait != 0 {
			t.Fatalf("success did not reset counter (failure %d locked)", i+1)
		}
	}
}

func TestAuthGuardWindowExpiryResetsCounter(t *testing.T) {
	clock := &fakeClock{current: time.Unix(1700000000, 0)}
	guard := newTestGuard(clock)
	for i := 0; i < authFailureLimit-1; i++ {
		guard.recordFailure("1.2.3.4")
	}
	clock.advance(authFailureWindow + time.Second)
	if wait := guard.recordFailure("1.2.3.4"); wait != 0 {
		t.Fatalf("window should reset counter, got lock %v", wait)
	}
}

func TestAuthGuardRetryAfterHeader(t *testing.T) {
	guard := newAuthGuard()
	if got := guard.retryAfter(90 * time.Second); got != "91" {
		t.Fatalf("retryAfter = %q, want 91", got)
	}
	if got := guard.retryAfter(0); got != "1" {
		t.Fatalf("retryAfter(0) = %q, want 1", got)
	}
}

func TestAuthGuardKeyFrom(t *testing.T) {
	cases := []struct {
		forwarded string
		remote    string
		want      string
	}{
		{"203.0.113.7", "172.22.0.1", "203.0.113.7"},
		{"203.0.113.7, 10.0.0.1", "172.22.0.1", "203.0.113.7"},
		{" 203.0.113.7 ,10.0.0.1", "172.22.0.1", "203.0.113.7"},
		{"", "172.22.0.1", "172.22.0.1"},
		{" , ", "172.22.0.1", "172.22.0.1"},
	}
	for _, tc := range cases {
		if got := authGuardKeyFrom(tc.forwarded, tc.remote); got != tc.want {
			t.Fatalf("authGuardKeyFrom(%q, %q) = %q, want %q", tc.forwarded, tc.remote, got, tc.want)
		}
	}
}

// 端到端：连续猜错令牌达到阈值后返回 429，并带上 Retry-After；
// 另一个来源（不同 X-Forwarded-For）仍可用正确令牌正常访问。
func TestAuthLockoutAfterRepeatedFailures(t *testing.T) {
	s := newTestServer(t)
	s.SetAuthToken("correct-token")

	wrong := func() *ut.ResponseRecorder {
		return ut.PerformRequest(s.h.Engine, "GET", "/api/v1/system", nil,
			ut.Header{Key: "Authorization", Value: "Bearer wrong-token"})
	}

	for i := 1; i <= authFailureLimit; i++ {
		if resp := wrong(); resp.Code != 401 {
			t.Fatalf("attempt %d = %d, want 401", i, resp.Code)
		}
	}
	resp := wrong()
	if resp.Code != 429 {
		t.Fatalf("locked attempt = %d, want 429", resp.Code)
	}
	if retry := resp.Header().Peek("Retry-After"); len(retry) == 0 {
		t.Fatalf("missing Retry-After header")
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte("auth_locked")) {
		t.Fatalf("unexpected body: %s", resp.Body.String())
	}

	// 锁定按客户端生效：换一个来源，正确令牌应照常通过。
	other := ut.PerformRequest(s.h.Engine, "GET", "/api/v1/system", nil,
		ut.Header{Key: "Authorization", Value: "Bearer correct-token"},
		ut.Header{Key: "X-Forwarded-For", Value: "203.0.113.9"})
	if other.Code != 200 {
		t.Fatalf("valid token blocked for another client: %d %s", other.Code, other.Body.String())
	}
}

// 未携带令牌的请求不计入失败次数：否则页面首次加载就会把用户锁在门外。
func TestAuthWithoutTokenDoesNotLock(t *testing.T) {
	s := newTestServer(t)
	s.SetAuthToken("correct-token")
	for i := 0; i < authFailureLimit*2; i++ {
		resp := ut.PerformRequest(s.h.Engine, "GET", "/api/v1/system", nil)
		if resp.Code != 401 {
			t.Fatalf("anonymous attempt %d = %d, want 401", i+1, resp.Code)
		}
	}
	ok := ut.PerformRequest(s.h.Engine, "GET", "/api/v1/system", nil,
		ut.Header{Key: "Authorization", Value: "Bearer correct-token"})
	if ok.Code != 200 {
		t.Fatalf("valid token blocked: %d", ok.Code)
	}
}
