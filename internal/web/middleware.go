package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
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

// sessionUser returns the signed-in user a session represents, or "" if it
// represents nobody.
//
// One predicate for both sides. RequireAuth and the login page have to agree on
// what "signed in" means, and when they did not the result was a redirect loop:
// RequireAuth refused a revoked session and sent the browser to /login, which
// saw a user value in the same cookie, decided the visitor was already signed
// in, and sent them back. Chrome gave up with ERR_TOO_MANY_REDIRECTS. Both the
// logout revocation and the password-change check had that shape.
func sessionUser(sess *sessions.Session, currentCredential func() string) string {
	user, _ := sess.Values[SessionUserKey].(string)
	if user == "" {
		return ""
	}
	if id, _ := sess.Values[SessionIDKey].(string); sessionRevoked(id) {
		return ""
	}
	if currentCredential != nil {
		if fp, _ := sess.Values[SessionCredentialKey].(string); fp != currentCredential() {
			return ""
		}
	}
	return user
}

// RequireAuth rejects unauthenticated requests with a redirect to /login.
//
// currentCredential returns the fingerprint of the password in force right now.
// A session carrying a different one was issued under a password that has since
// been changed, and stops being accepted at that moment rather than when it
// happens to time out.
func RequireAuth(store sessions.Store, currentCredential func() string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, err := store.Get(r, SessionName)
			if err != nil || sessionUser(sess, currentCredential) == "" {
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

// MaxBodySize limits request bodies to avoid memory exhaustion. Paths listed in
// overrides get their own limit instead of the default.
//
// The override belongs here rather than in the handler because a handler cannot
// widen a limit that is already in place: http.MaxBytesReader has no way to
// unwrap, so a second, larger reader around the first one changes nothing and
// the smaller limit keeps applying. The only place a route can get more room is
// where the limit is first set.
func MaxBodySize(n int64, overrides map[string]int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := n
			if o, ok := overrides[r.URL.Path]; ok {
				limit = o
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// isBodyTooLarge reports whether err came from a body hitting its size limit,
// however deeply the multipart or form parser has wrapped it.
func isBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return true
	}
	// mime/multipart reports the limit as a plain string in some paths rather
	// than passing the typed error through.
	return err != nil && strings.Contains(err.Error(), "http: request body too large")
}
