package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	loginWindow     = 60 * time.Second
	loginMaxFails   = 5
	loginBaseBlock  = 30 * time.Second
	loginMaxBlock   = 15 * time.Minute
	limiterGCPeriod = 5 * time.Minute
	limiterStaleAge = 30 * time.Minute
)

type limiterEntry struct {
	fails        int
	blocks       int
	firstAt      time.Time
	blockedUntil time.Time
}

type loginLimiter struct {
	mu sync.Mutex
	m  map[string]*limiterEntry
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{m: make(map[string]*limiterEntry)}
}

func NewLoginLimiter() *loginLimiter {
	l := newLoginLimiter()
	l.startGC()
	return l
}

func (l *loginLimiter) Allow(key string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	e, ok := l.m[key]
	if !ok {
		return true, 0
	}

	if now.Before(e.blockedUntil) {
		left := time.Until(e.blockedUntil)
		secs := int(left / time.Second)
		if left%time.Second != 0 {
			secs++
		}
		if secs < 1 {
			secs = 1
		}
		return false, secs
	}

	if !e.blockedUntil.IsZero() {
		e.blockedUntil = time.Time{}
	}

	if now.Sub(e.firstAt) > loginWindow {
		delete(l.m, key)
		return true, 0
	}

	return true, 0
}

func (l *loginLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	e, ok := l.m[key]
	if !ok {
		l.m[key] = &limiterEntry{fails: 1, firstAt: now}
		return
	}

	if now.Sub(e.firstAt) > loginWindow {
		e.fails = 1
		e.firstAt = now
		e.blockedUntil = time.Time{}
		return
	}

	e.fails++
	if e.fails < loginMaxFails {
		return
	}

	shift := e.blocks
	if shift > 20 {
		shift = 20
	}
	dur := loginBaseBlock << uint(shift)
	if dur > loginMaxBlock || dur <= 0 {
		dur = loginMaxBlock
	}
	e.blockedUntil = now.Add(dur)
	e.blocks++
	e.fails = 0
	e.firstAt = now
}

func (l *loginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, key)
}

func (l *loginLimiter) startGC() {
	go func() {
		ticker := time.NewTicker(limiterGCPeriod)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			l.mu.Lock()
			for k, e := range l.m {
				if now.Before(e.blockedUntil) {
					continue
				}
				if now.Sub(e.firstAt) > limiterStaleAge {
					delete(l.m, k)
				}
			}
			l.mu.Unlock()
		}
	}()
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func loginKey(r *http.Request, username string) string {
	return username + "|" + clientIP(r)
}
