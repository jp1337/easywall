package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/sessions"
	"golang.org/x/time/rate"
)

type ctxKey string

const nonceCtxKey ctxKey = "csp-nonce"

func generateNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// SecurityHeaders adds hardened HTTP security headers to every response.
// A per-request CSP nonce is generated and stored in the request context so
// templates can reference it for the theme-init inline script.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := generateNonce()
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("Content-Security-Policy", fmt.Sprintf(
			// Fonts are self-hosted (see DESIGN.md § Typography), so no external
			// origin is permitted anywhere: easywall frequently runs on machines
			// without outbound internet access, and an administrative interface
			// has no business making third-party requests.
			"default-src 'self'; script-src 'self' 'nonce-%s'; style-src 'self'; font-src 'self'; img-src 'self' data:; connect-src 'self'",
			nonce,
		))
		ctx := context.WithValue(r.Context(), nonceCtxKey, nonce)
		next.ServeHTTP(w, r.WithContext(ctx))
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
	once    sync.Once
	buckets map[string]*rateBucket
}{buckets: make(map[string]*rateBucket)}

type rateBucket struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// LoginRateLimit limits login attempts to 5 requests per 10 minutes per source IP.
func LoginRateLimit(next http.Handler) http.Handler {
	// Start the cleanup goroutine exactly once for the process lifetime,
	// regardless of how many times this middleware factory is called (e.g. in tests).
	loginLimiter.once.Do(func() {
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
	})

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
