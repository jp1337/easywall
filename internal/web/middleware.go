package web

import (
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/sessions"
	"golang.org/x/time/rate"
)

// SecurityHeaders adds hardened HTTP security headers to every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

// RequireAuth rejects unauthenticated requests with a redirect to /login.
func RequireAuth(store sessions.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, err := store.Get(r, SessionName)
			if err != nil || sess.Values[SessionUserKey] == nil {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// loginLimiter holds per-IP rate limiters for the login endpoint.
var loginLimiter = struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
}{buckets: make(map[string]*rateBucket)}

type rateBucket struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// LoginRateLimit limits login attempts to 5 requests per 10 minutes per source IP.
func LoginRateLimit(next http.Handler) http.Handler {
	// Background cleanup of stale limiters every 5 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			loginLimiter.mu.Lock()
			for ip, b := range loginLimiter.buckets {
				if time.Since(b.lastSeen) > 15*time.Minute {
					delete(loginLimiter.buckets, ip)
				}
			}
			loginLimiter.mu.Unlock()
		}
	}()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		loginLimiter.mu.Lock()
		b, ok := loginLimiter.buckets[ip]
		if !ok {
			// 5 tokens, refills at 1 token per 2 minutes (5 per 10 min)
			b = &rateBucket{lim: rate.NewLimiter(rate.Every(2*time.Minute), 5)}
			loginLimiter.buckets[ip] = b
		}
		b.lastSeen = time.Now()
		allowed := b.lim.Allow()
		loginLimiter.mu.Unlock()

		if !allowed {
			slog.Warn("login rate limit exceeded", "ip", ip)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// MaxBodySize limits request bodies to avoid memory exhaustion.
func MaxBodySize(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}
