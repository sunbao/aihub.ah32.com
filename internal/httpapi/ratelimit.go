package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ipRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*ipEntry
	limit   int
	window  time.Duration
}

type ipEntry struct {
	resetAt time.Time
	count   int
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		entries: map[string]*ipEntry{},
		limit:   limit,
		window:  window,
	}
}

func (l *ipRateLimiter) allow(ip string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	e := l.entries[ip]
	if e == nil || now.After(e.resetAt) {
		l.entries[ip] = &ipEntry{resetAt: now.Add(l.window), count: 1}
		return true
	}
	if e.count >= l.limit {
		return false
	}
	e.count++
	return true
}

func (l *ipRateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip SSE: it's long-lived and should not be rate-limited per-request.
		if strings.Contains(r.URL.Path, "/stream") {
			next.ServeHTTP(w, r)
			return
		}
		ip := clientIP(r)
		if ip == "" {
			ip = "unknown"
		}
		if !l.allow(ip) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	// NOTE: for MVP we trust the direct remote address only.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
