package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
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

// redirectRefused sends a refused request to `to`. A plain navigation gets a
// 303. An htmx request gets a 401 with HX-Redirect instead: its XHR would
// follow a 303 without htmx seeing it and swap the target page into the
// element it was updating — /login inside /blocked's table, every five
// seconds, after a session expired. htmx reads HX-Redirect before it looks at
// the status and navigates the whole window.
func redirectRefused(w http.ResponseWriter, r *http.Request, to string) {
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", to)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
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
				redirectRefused(w, r, "/login")
				return
			}
			if backgroundRequest(r) {
				next.ServeHTTP(w, r)
				return
			}
			rw := &renewingWriter{ResponseWriter: w, r: r, sess: sess}
			next.ServeHTTP(rw, r)
			rw.renew() // a handler that wrote nothing at all
		})
	}
}

// renewSession re-signs an authenticated session, so its lifetime counts from
// now: privacy.md and security.md say a session ends ten minutes after last
// use. Only flash-writing actions used to save it, so a session browsed with
// GETs ended ten minutes after sign-in — and an operator watching a long
// acceptance window was sent to /login at the moment they went to confirm.
//
// Only for a session sessionUser has accepted. A revoked one never gets here,
// which is what keeps the revocation record's one-lifetime retention sound.
func renewSession(w http.ResponseWriter, r *http.Request, sess *sessions.Session) {
	if err := sess.Save(r, w); err != nil {
		slog.Warn("could not renew the session", "error", err)
	}
}

// renewingWriter renews the session as the response starts, unless the
// handler already saved it — a flash, a re-stamp after a password change. Two
// Set-Cookie headers for one name is a response whose meaning depends on the
// reader keeping the last one; one header is not.
type renewingWriter struct {
	http.ResponseWriter
	r    *http.Request
	sess *sessions.Session
	done bool
}

func (w *renewingWriter) renew() {
	if w.done {
		return
	}
	w.done = true
	for _, c := range w.Header().Values("Set-Cookie") {
		if strings.HasPrefix(c, SessionName+"=") {
			return
		}
	}
	renewSession(w.ResponseWriter, w.r, w.sess)
}

func (w *renewingWriter) WriteHeader(code int) {
	w.renew()
	w.ResponseWriter.WriteHeader(code)
}

func (w *renewingWriter) Write(b []byte) (int, error) {
	w.renew()
	return w.ResponseWriter.Write(b)
}

func (w *renewingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// backgroundRequest reports a GET a page makes on its own — htmx polling
// /blocked/rows, app.js fetching /apply/status — rather than one the operator
// made. Those do not renew a session, or an open tab would keep it alive for
// ever. The apply screen renews explicitly while a window is open.
func backgroundRequest(r *http.Request) bool {
	return r.Method == http.MethodGet && (isHTMX(r) || r.Header.Get("Sec-Fetch-Dest") == "empty")
}

// RequireSecondFactor sends an authenticated operator with no second factor to
// the page where they enrol one, and lets nothing else through.
//
// It runs after RequireAuth, never instead of it: the session is real, the
// operator is who they say they are, and what is missing is the factor. That
// ordering is why an upgrade cannot lock anyone out — handleLoginPOST still
// grants the session when no factor is enrolled, and this redirects afterwards.
// A gate that refused the login would strand an operator whose only route to
// the host is this interface, which is the direction 2.7 exists to prevent.
//
// hasFactor and isDemo are callbacks rather than a *Server for the reason
// LoginRateLimit's resolve is: a middleware that reaches for Config is a
// middleware that cannot be tested without one.
//
// There is no deferral. A gate with a "remind me later" is a gate nobody
// passes through, and the operators who most need the factor are exactly the
// ones who would press it.
func RequireSecondFactor(hasFactor func() bool, isDemo func() bool) func(http.Handler) http.Handler {
	// The routes enrolment itself needs. Exact matches, not a prefix: a prefix
	// of "/password" would also admit anything a later release mounts below it,
	// and the gate would widen without anyone deciding that it should. Nothing
	// the router registers today tells the two apart, so
	// TestTheGateAllowlistIsExactAndNotAPrefix drives this middleware directly
	// and asks about paths no route has — without it, the sentence above is a
	// description of a guarantee nothing holds.
	//
	// "/logout" is the one entry the router does not exercise: server.go
	// registers POST /logout in the public group, which RequireSecondFactor is
	// not mounted on, so the gate never meets it and this entry is inert today.
	// It stays because a way out must never need the factor it is gating, and
	// the day /logout moves inside the authenticated group is not the day to
	// rediscover that. Same test holds it.
	allowed := map[string]bool{
		"/password":                      true,
		"/password/2fa/begin":            true,
		"/password/2fa/confirm":          true,
		"/password/2fa/enrol-unverified": true,
		"/password/2fa/disable":          true,
		"/password/2fa/recovery":         true,
		"/password/passkey/begin":        true,
		"/password/passkey/finish":       true,
		"/password/passkey/remove":       true,
		"/logout":                        true,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isDemo() || hasFactor() || allowed[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			// The enrolment routes themselves also redirect to /password on a
			// wrong or missing current_password, an expired setup, and other
			// ordinary validation failures — same status, same target, for a
			// completely different reason. This header is how
			// TestTheGateCannotBeWalkedPast tells "the gate stopped this
			// request" apart from "the handler it reached sent it back here",
			// without which the walk cannot tell an enrolment route working
			// as designed from one the gate incorrectly blocked.
			w.Header().Set("X-Easywall-Gate", "second-factor-required")
			redirectRefused(w, r, "/password")
		})
	}
}

// loginLimiter holds per-IP rate limiters for the login endpoint.
var loginLimiter = struct {
	mu      sync.Mutex
	once    sync.Once
	buckets map[string]*rateBucket
}{buckets: make(map[string]*rateBucket)}

// The login budget: loginBudget attempts, one returned every loginBudgetRefill.
const (
	loginBudget       = 5
	loginBudgetRefill = 10 * time.Minute
)

// loginMaxBuckets bounds the limiter's map, whose size is the number of
// addresses a stranger chooses to send from. It grew without limit: 20,000
// /64s made 20,000 buckets. ~190 bytes each, so ~12 MB at the ceiling.
//
// At the ceiling a new address is refused, never let in by evicting another:
// random eviction was tried, and cycling through more addresses than the map
// holds reset an exhausted budget every time. Only sweepLoginBuckets frees a
// slot, and only a bucket that limits nobody. The cost, documented in
// security.md: while 65,536 sources are live, a source not among them waits.
const loginMaxBuckets = 65536

// sweepLoginBuckets drops the buckets that limit nobody: refilled to the full
// budget, so indistinguishable from a new one, or unused for 15 minutes —
// longer than the ten-minute window, so a fresh bucket after that still lets
// no window hold more than loginBudget.
func sweepLoginBuckets(now time.Time) {
	loginLimiter.mu.Lock()
	defer loginLimiter.mu.Unlock()
	for k, b := range loginLimiter.buckets {
		if now.Sub(b.lastSeen) > 15*time.Minute || b.lim.TokensAt(now) >= loginBudget {
			delete(loginLimiter.buckets, k)
		}
	}
}

// limiterKey is the address a login budget belongs to: an IPv4 address, or
// the /64 an IPv6 one is in. A single subscriber is routed a whole /64 and
// picks any address in it, so keying on the full address let one host send
// 200 attempts from 200 addresses, every one of them let through — and each
// a 64 MiB Argon2id run.
func limiterKey(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	a = a.Unmap()
	if a.Is4() {
		return a.String()
	}
	p, _ := a.WithZone("").Prefix(64)
	return p.String()
}

// loginClock is the limiter's clock, so a test can walk it through ten minutes.
var loginClock = time.Now

type rateBucket struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// LoginRateLimit limits login attempts to 5 requests per 10 minutes per client,
// and calls onBlocked with the address and whether that address stands in for
// a client it could not name, each time it refuses one.
//
// resolve answers who a request is from. It is a parameter rather than a call
// into the configuration because a middleware that reaches for Config is a
// middleware that cannot be tested without one — the same separation the
// two-process design rests on, and the reason onBlocked is a callback too. In
// the running server it is (*Server).clientAddr, so the bucket key, the audit
// address and the apply screen's verdict cannot disagree about who a caller is.
//
// Until 2.13 this keyed on r.RemoteAddr's host directly. Behind a reverse proxy
// that is one bucket for everybody, so one attacker exhausted the budget for
// every operator — the cost docs-tech/threat-model.md documented and this
// release removes. It is removed only for peers on the trusted list: with an
// empty list the key is the peer, exactly as before.
//
// onBlocked may be nil. It is also where a refusal is logged: the server knows
// whether it is the public demo, which must not write the address down.
func LoginRateLimit(resolve func(*http.Request) (string, bool),
	onBlocked func(ip string, proxied bool)) func(http.Handler) http.Handler {
	// Start the cleanup goroutine exactly once for the process lifetime,
	// regardless of how many times this middleware factory is called (e.g. in tests).
	loginLimiter.once.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				sweepLoginBuckets(loginClock())
			}
		}()
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, proxied := resolve(r)

			now := loginClock()
			key := limiterKey(ip)
			loginLimiter.mu.Lock()
			b, ok := loginLimiter.buckets[key]
			allowed := false
			switch {
			case !ok && len(loginLimiter.buckets) >= loginMaxBuckets:
				// Refused, not let in by evicting a bucket — see loginMaxBuckets.
			case !ok:
				// 5 tokens, one back every 10 minutes: at most 5 in any ten
				// minutes, which is what security.md's fifteen code attempts
				// rest on. It refilled one every 2 minutes, which is 9 password
				// rounds in ten minutes — 27 code attempts, not 15.
				b = &rateBucket{lim: rate.NewLimiter(rate.Every(loginBudgetRefill), loginBudget)}
				loginLimiter.buckets[key] = b
				fallthrough
			default:
				b.lastSeen = now
				allowed = b.lim.AllowN(now, 1)
			}
			loginLimiter.mu.Unlock()

			if !allowed {
				if onBlocked != nil {
					onBlocked(ip, proxied)
				}
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
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
