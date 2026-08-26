package webauth

import (
	"strings"
	"sync"
	"time"
)

type attempt struct {
	count     int
	expiresAt time.Time
}

type LoginLimiter struct {
	mu      sync.Mutex
	entries map[string]attempt
	now     func() time.Time
	window  time.Duration
	account int
	address int
}

func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{
		entries: make(map[string]attempt), now: time.Now,
		window: 5 * time.Minute, account: 8, address: 30,
	}
}

func (l *LoginLimiter) Allowed(address, login string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	return l.countLocked("ip:"+address, now) < l.address &&
		l.countLocked("login:"+strings.ToLower(strings.TrimSpace(login)), now) < l.account
}

func (l *LoginLimiter) Failure(address, login string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for _, key := range []string{"ip:" + address, "login:" + strings.ToLower(strings.TrimSpace(login))} {
		value := l.entries[key]
		if !value.expiresAt.After(now) {
			value = attempt{expiresAt: now.Add(l.window)}
		}
		value.count++
		l.entries[key] = value
	}
}

func (l *LoginLimiter) Success(login string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, "login:"+strings.ToLower(strings.TrimSpace(login)))
}

func (l *LoginLimiter) countLocked(key string, now time.Time) int {
	value, found := l.entries[key]
	if !found || !value.expiresAt.After(now) {
		delete(l.entries, key)
		return 0
	}
	return value.count
}
