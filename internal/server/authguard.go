package server

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
)

const (
	// authFailureLimit 是窗口内允许的连续失败次数，超过即锁定。
	authFailureLimit = 10
	// authFailureWindow 是失败计数的统计窗口。
	authFailureWindow = 5 * time.Minute
	// authLockout 是触发后的锁定时长。
	authLockout = 5 * time.Minute
	// authEntrySweepThreshold 超过这个条目数时顺手清理过期记录，
	// 防止大量不同来源 IP 把表撑大。
	authEntrySweepThreshold = 1024
)

// authGuard 限制单个客户端连续猜错访问令牌的次数。
//
// 背景：访问令牌由用户自己设置，可能是短口令，而服务本身没有任何限速，
// 公网上就等于允许无限次尝试。这里按客户端做「N 次失败 → 锁定一段时间」，
// 认证成功立即清零，正常使用完全不受影响（只有失败路径才会累加）。
type authGuard struct {
	mu      sync.Mutex
	now     func() time.Time
	limit   int
	window  time.Duration
	lockout time.Duration
	entries map[string]*authEntry
}

type authEntry struct {
	fails     int
	windowEnd time.Time
	lockedTo  time.Time
}

func newAuthGuard() *authGuard {
	return &authGuard{
		now:     time.Now,
		limit:   authFailureLimit,
		window:  authFailureWindow,
		lockout: authLockout,
		entries: make(map[string]*authEntry),
	}
}

// lockedFor 返回该客户端剩余锁定时间，0 表示当前允许尝试认证。
func (g *authGuard) lockedFor(key string) time.Duration {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	entry := g.entries[key]
	if entry == nil {
		return 0
	}
	now := g.now()
	if entry.lockedTo.After(now) {
		return entry.lockedTo.Sub(now)
	}
	if !entry.lockedTo.IsZero() {
		// 锁定到期，计数重新开始。
		delete(g.entries, key)
	}
	return 0
}

// recordFailure 记一次失败。达到阈值时进入锁定并返回锁定时长。
func (g *authGuard) recordFailure(key string) time.Duration {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	entry := g.entries[key]
	if entry == nil || now.After(entry.windowEnd) {
		entry = &authEntry{windowEnd: now.Add(g.window)}
		g.entries[key] = entry
	}
	entry.fails++
	if entry.fails >= g.limit {
		entry.fails = 0
		entry.lockedTo = now.Add(g.lockout)
		entry.windowEnd = now.Add(g.window)
		g.sweepLocked(now)
		return g.lockout
	}
	return 0
}

// recordSuccess 在认证成功后清零该客户端的失败计数。
func (g *authGuard) recordSuccess(key string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.entries, key)
}

// retryAfter 返回 Retry-After 头的取值（秒，至少 1）。
func (g *authGuard) retryAfter(wait time.Duration) string {
	seconds := int(wait.Seconds()) + 1
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}

// sweepLocked 清理已经过期且未锁定的条目。调用方必须已持有锁。
func (g *authGuard) sweepLocked(now time.Time) {
	if len(g.entries) < authEntrySweepThreshold {
		return
	}
	for key, entry := range g.entries {
		if entry.lockedTo.After(now) {
			continue
		}
		if now.After(entry.windowEnd) {
			delete(g.entries, key)
		}
	}
}

// authGuardKey 取客户端标识：经反向代理时用 X-Forwarded-For 的第一跳，
// 否则退回连接来源 IP。同一 NAT 后的多个客户端会共享同一个 key，
// 这是刻意的 —— 宁可少给一次机会，也不要让同一来源无限尝试。
func authGuardKey(c *app.RequestContext) string {
	return authGuardKeyFrom(string(c.Request.Header.Peek("X-Forwarded-For")), c.ClientIP())
}

// authGuardKeyFrom 把 X-Forwarded-For 与来源 IP 折算成限速键，便于单独测试。
func authGuardKeyFrom(forwardedFor, remoteIP string) string {
	if forwardedFor != "" {
		if index := strings.IndexByte(forwardedFor, ','); index >= 0 {
			forwardedFor = forwardedFor[:index]
		}
		if ip := strings.TrimSpace(forwardedFor); ip != "" {
			return ip
		}
	}
	return strings.TrimSpace(remoteIP)
}
