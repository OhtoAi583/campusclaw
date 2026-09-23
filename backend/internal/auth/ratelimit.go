package auth

import (
	"sync"
	"time"
)

// Limiter 是登录失败限流器：按 key（用户名 + 客户端 IP）计数。
// 状态保存在进程内存中，这也是本迭代只声明单实例部署的原因之一。
type Limiter struct {
	mu       sync.Mutex
	attempts map[string]*attempt
	max      int
	window   time.Duration
	lock     time.Duration
	now      func() time.Time
}

type attempt struct {
	failures    int
	firstAt     time.Time
	lockedUntil time.Time
}

// NewLimiter 构造限流器。max 为窗口内允许的失败次数。
func NewLimiter(max int, window, lock time.Duration) *Limiter {
	return &Limiter{
		attempts: make(map[string]*attempt),
		max:      max,
		window:   window,
		lock:     lock,
		now:      time.Now,
	}
}

// Allowed 返回该 key 当前是否还能尝试登录。
// 注意：被锁定时调用方仍然要返回与凭据错误一致的响应，不能用 429 暴露账号存在。
func (l *Limiter) Allowed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.attempts[key]
	if !ok {
		return true
	}
	now := l.now()
	if now.Before(a.lockedUntil) {
		return false
	}
	if a.lockedUntil.IsZero() && now.Sub(a.firstAt) > l.window {
		delete(l.attempts, key)
		return true
	}
	return true
}

// Fail 记录一次失败；达到阈值后进入锁定期。
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	a, ok := l.attempts[key]
	if !ok || now.Sub(a.firstAt) > l.window {
		l.attempts[key] = &attempt{failures: 1, firstAt: now}
		return
	}
	a.failures++
	if a.failures >= l.max {
		a.lockedUntil = now.Add(l.lock)
		a.failures = 0
		a.firstAt = now
	}
}

// Reset 在登录成功后清除计数。
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

// Sweep 清理过期条目，由后台协程定期调用，避免内存无界增长。
func (l *Limiter) Sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for key, a := range l.attempts {
		if now.Before(a.lockedUntil) {
			continue
		}
		if now.Sub(a.firstAt) > l.window+l.lock {
			delete(l.attempts, key)
		}
	}
}
