package web

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/sessions"
	"github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/jp1337/easywall/internal/shared"
)

// PageData is passed to every template render call.
type PageData struct {
	Flash string
	User  string
	Page  string // current page for nav active state
	Nonce string // CSP nonce for the theme-init inline script
	Demo  bool   // true when running with the in-memory mock (banner)
	Lang  string // language actually served, for <html lang>

	// Langs lists the languages on offer, with the current one marked. Empty when
	// only one locale file is installed, in which case the switcher is not drawn.
	Langs []languageOption

	// Path is where the language switcher returns to, query included: on /ports the
	// open tab is ?type=, and losing it moves the operator to a different tab.
	Path string

	// Strings carries the translations app.js needs. It builds some text in the
	// browser — toast bodies, the polled apply state, the placeholders on a row
	// added client-side — which is not in the rendered HTML to read back out.
	Strings map[string]string

	// Asset is appended to every versioned static URL — the stylesheet and both
	// scripts. Without it an operator who upgrades easywall keeps the cached
	// copy from the previous version and sees a broken interface until they
	// force-reload. It was on the stylesheet alone for a while, which was the
	// worse of the two states: the new stylesheet arrived and app.js did not,
	// so the upgraded page ran the previous release's JavaScript.
	Asset string

	// Version is the release this binary is, for the operator to read. Asset
	// above holds the same value today and is not the same field: Asset hangs
	// off asset URLs and may become a build hash tomorrow without anyone
	// thinking about it, and displaying the version through it is the coupling
	// that produces "easywall v3f9a1c" on the first build hash.
	Version string
	Data    interface{}

	// FlashN is the one number a flash may carry: how many recovery codes are
	// left. A flash is a message id, not a sentence, so the count travels beside
	// it rather than inside it.
	FlashN int

	// Panic is true while the core reports that this installation is
	// deliberately unfiltered. It is on PageData rather than on one handler's
	// data because the banner it drives belongs on every page: a warning only
	// the dashboard carries is one an operator does not see, and not seeing it
	// is the whole problem with panic mode.
	Panic bool
}

// Server is the easywall web frontend.
type Server struct {
	cfg    *Config
	client *CoreClient
	store  sessions.Store
	// pending holds the login's intermediate state. Separate from store on
	// purpose — see newPendingStore.
	pending sessions.Store
	// replay remembers the last accepted TOTP step, so a code cannot be used
	// twice inside its own thirty-second validity window.
	replay  *totpReplay
	bundle  *i18n.Bundle
	tmpl    *template.Template
	router  chi.Router
	httpSrv *http.Server
	version *shared.Checker
	certs   *certManager

	// telemetry is nil in demo mode. The public demo is reset every few hours,
	// which would give it a fresh identifier each time and manufacture several
	// installations a day — in a count whose whole value is being small enough
	// to mean something.
	telemetry     *shared.Reporter
	telemetryStop chan struct{}
	telemetryOnce sync.Once

	// events carries login events to the core without a request waiting on one.
	events     *auditEvents
	eventsStop chan struct{}
	eventsOnce sync.Once
}

// NewServer initialises the web server with all dependencies.
func NewServer(cfg *Config) (*Server, error) {
	if err := os.MkdirAll(cfg.SSLDir, 0750); err != nil {
		return nil, fmt.Errorf("create ssl dir: %w", err)
	}

	// TLS certificate — generated on first start, and kept current from here on
	// by the manager rather than only at process start.
	certs := newCertManager(cfg)
	if err := certs.ensure(); err != nil {
		return nil, fmt.Errorf("generate TLS cert: %w", err)
	}

	var client *CoreClient
	if cfg.DemoMode {
		slog.Info("demo mode active — using in-memory mock instead of core socket")
		client = NewDemoClient()
	} else {
		client = NewCoreClient(cfg.SocketPath)
	}

	store := newSessionStore(cfg.SessionKey)
	pending := newPendingStore(cfg.SessionKey)

	bundle := NewBundle(cfg.LocalesDir())

	s := &Server{
		cfg:     cfg,
		client:  client,
		store:   store,
		pending: pending,
		replay:  newTOTPReplay(cfg.TOTPReplayPath()),
		bundle:  bundle,
		version: shared.NewChecker(cfg.VersionCachePath(), cfg.UpdateCheckEnabled()),
		certs:   certs,
	}

	if !cfg.DemoMode {
		s.telemetry = shared.NewReporter(cfg.TelemetryStatePath(), cfg.TelemetryEnabled)
		s.telemetryStop = make(chan struct{})
	}

	s.events = newAuditEvents(client)
	s.eventsStop = make(chan struct{})

	// Non-fatal here so tests can build a Server without the asset tree; Start()
	// refuses to serve without it.
	tmpl, err := loadTemplates(cfg.TemplatesDir())
	if err != nil {
		slog.Warn("templates not loaded", "dir", cfg.TemplatesDir(), "error", err)
	} else {
		s.tmpl = tmpl
	}

	s.router = s.buildRouter(cfg)
	s.httpSrv = &http.Server{
		Addr:    cfg.BindAddr,
		Handler: s.router,
		// A request that waits on the core synchronously — POST /import and
		// POST /validate — cannot be cut off before shared.CommandTimeout gives
		// up on the same command, or a reply the core finishes writing never
		// gets written: the client would report a dropped connection for an
		// import that had already succeeded. 10s of margin on top covers this
		// handler's own work assembling the response.
		ReadTimeout:  15 * time.Second,
		WriteTimeout: writeTimeout(),
		IdleTimeout:  60 * time.Second,
		// The certificate comes from the manager on every handshake instead of
		// being read once from disk, so a renewed or replaced certificate takes
		// effect without restarting the service.
		TLSConfig: &tls.Config{
			GetCertificate: certs.GetCertificate,
			MinVersion:     tls.VersionTLS12,
		},
	}

	return s, nil
}

// writeTimeout is the HTTP server's WriteTimeout: long enough to outlast the
// slowest command a handler can be waiting on when it writes its reply.
//
// POST /import and POST /validate call the core synchronously, and both carry
// shared.CommandTimeout's longest budget — CmdImportRules and CmdValidateCustom
// both wait out shared.NftTimeout plus a margin. The two are equal today; the
// max is taken anyway so this does not silently fall behind if one of them
// changes without the other. 10s on top of that is this handler's own room to
// marshal the response and write it out.
func writeTimeout() time.Duration {
	longest := shared.CommandTimeout(shared.CmdImportRules)
	if v := shared.CommandTimeout(shared.CmdValidateCustom); v > longest {
		longest = v
	}
	return longest + 10*time.Second
}

// Start begins serving HTTPS traffic. Blocks until Stop() is called.
func (s *Server) Start() error {
	// Templates before binding, for the same reason as the certificate below.
	// Without them the process started, systemd reported the unit active, the
	// port answered, and every page returned 503 "Web interface not ready
	// (templates missing — run Phase 4)" — a phase number from this project's
	// own development that means nothing to whoever is reading it. The only
	// other signal was one WARN line at startup.
	//
	// The directories are resolved relative to the working directory, so this
	// is what a wrong or missing WorkingDirectory= looks like, and it is worth
	// naming both in the message.
	if s.tmpl == nil {
		wd, _ := os.Getwd()
		return fmt.Errorf("no templates in %s (working directory %s): easywall-web "+
			"serves its interface from files installed beside it, and cannot serve "+
			"anything without them", s.cfg.TemplatesDir(), wd)
	}

	// Load the certificate before binding. Serving the port and failing every
	// handshake looks, from the outside, like a broken network rather than a
	// missing file; refusing to start says which.
	if _, err := s.certs.GetCertificate(nil); err != nil {
		return fmt.Errorf("TLS certificate: %w", err)
	}

	slog.Info("easywall-web listening", "addr", s.cfg.BindAddr)
	go s.certs.maintain()
	if s.telemetry != nil {
		go s.telemetry.Run(s.telemetryStop)
	}
	go s.events.run(s.eventsStop)
	// Empty paths: the certificate is supplied by TLSConfig.GetCertificate.
	if err := s.httpSrv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTPS server: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() {
	s.certs.close()
	if s.telemetryStop != nil {
		s.telemetryOnce.Do(func() { close(s.telemetryStop) })
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.httpSrv.Shutdown(ctx)

	// Closed only after Shutdown returns, not before. auditEvents.run exits the
	// instant eventsStop closes, and Record only warns when its buffer is full —
	// so closing this first meant every login and logout in flight during the
	// shutdown's up-to-ten-second grace period queued into a channel nobody was
	// draining anymore, and the loss was silent on both ends. Handlers still
	// running have finished writing to the channel by the time Shutdown
	// returns, so nothing more is enqueued after this point.
	if s.eventsStop != nil {
		s.eventsOnce.Do(func() { close(s.eventsStop) })
	}
}

func (s *Server) buildRouter(cfg *Config) chi.Router {
	r := chi.NewRouter()

	// Global middleware
	//
	// Deliberately NOT using middleware.RealIP: easywall-web terminates TLS
	// itself and isn't assumed to sit behind a trusted reverse proxy, so
	// X-Forwarded-For/X-Real-IP/True-Client-IP are attacker-controlled.
	// Trusting them would let a client spoof its IP and bypass the
	// per-IP login rate limiter. r.RemoteAddr (the actual TCP peer) stays
	// authoritative — see GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp, GHSA-9g5q-2w5x-hmxf.
	r.Use(middleware.Recoverer)
	r.Use(SecurityHeaders)
	r.Use(MaxBodySize(64*1024, map[string]int64{"/import": maxImportBytes}))

	// CSRF protection via Go 1.25 net/http.CrossOriginProtection (Origin/Sec-Fetch-Site header check)
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Cross-origin request rejected", http.StatusForbidden)
	}))
	r.Use(func(next http.Handler) http.Handler { return cop.Handler(next) })

	// Static assets
	r.Handle("/static/*", staticCacheHeaders(
		http.StripPrefix("/static/", http.FileServer(http.Dir(cfg.StaticDir())))))

	// Public routes
	r.Group(func(r chi.Router) {
		r.Get("/login", s.handleLoginGET)
		r.With(LoginRateLimit(s.onLoginBlocked)).Post("/login", s.handleLoginPOST)
		// The second step. Outside RequireAuth because nobody is signed in yet,
		// and with no rate limit of its own — see handleLoginVerifyPOST for the
		// arithmetic that makes the password step's limit cover it.
		r.Get("/login/verify", s.handleLoginVerifyGET)
		r.Post("/login/verify", s.handleLoginVerifyPOST)
		// POST, so CrossOriginProtection covers it. It was a GET, and that
		// middleware exempts safe methods by design — measured: a request
		// carrying Origin: https://evil.example and Sec-Fetch-Site: cross-site
		// answered 303 and ended the session, while the same request to a POST
		// route answered 403. Any page the operator had open could sign them out
		// of their firewall's interface with an <img> tag.
		r.Post("/logout", s.handleLogout)
		// Outside RequireAuth on purpose: someone who cannot read the login page
		// has no way to sign in and change the language afterwards.
		r.Post("/language", s.handleLanguage)

		if cfg.IsFirstRun() {
			r.Get("/firstrun", s.handleFirstRunGET)
			r.Post("/firstrun", s.handleFirstRunPOST)

			// Inside this block on purpose: these write credentials, and they
			// must stop existing the moment an account does. That is also why
			// they are not in credentialWritingRoutes — the demo ships with a
			// password set, so they are never registered there at all.
			r.Post("/firstrun/confirm", s.handleFirstRunConfirm)
			r.Post("/firstrun/skip", s.handleFirstRunSkip)
		}
	})

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(s.store, s.currentCredential()))

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		})
		r.Get("/dashboard", s.handleDashboard)

		r.Get("/ports", s.handlePortsGET)
		r.Post("/ports", s.handlePortsPOST)

		r.Get("/blacklist", s.handleBlacklistGET)
		r.Post("/blacklist", s.handleBlacklistPOST)

		r.Get("/whitelist", s.handleWhitelistGET)
		r.Post("/whitelist", s.handleWhitelistPOST)

		// Shared HTMX validation endpoint for both blacklist and whitelist.
		r.Post("/iplist/validate", s.handleIPListValidate)

		r.Get("/forwarding", s.handleForwardingGET)
		r.Post("/forwarding", s.handleForwardingPOST)

		r.Get("/custom", s.handleCustomGET)
		r.Post("/custom", s.handleCustomPOST)
		r.Post("/custom/validate", s.handleCustomValidate)

		r.Get("/options", s.handleOptionsGET)
		r.Post("/options", s.handleOptionsPOST)

		r.Get("/settings", s.handleSettingsGET)
		r.Post("/settings", s.handleSettingsPOST)

		r.Get("/password", s.handlePasswordGET)
		r.Post("/password", s.handlePasswordPOST)

		// All four POST, all inside this group, and therefore all under the
		// existing http.NewCrossOriginProtection. begin, confirm and recovery
		// render their result in place rather than redirecting to a GET, so a
		// reload cannot mint a second secret and the eight codes have no URL.
		r.Post("/password/2fa/begin", s.handle2FABegin)
		r.Post("/password/2fa/confirm", s.handle2FAConfirm)
		r.Post("/password/2fa/disable", s.handle2FADisable)
		r.Post("/password/2fa/recovery", s.handle2FARecovery)

		r.Get("/system", s.handleSystemGET)
		r.Post("/system", s.handleSystemPOST)
		r.Post("/system/telemetry", s.handleTelemetryPOST)

		r.Get("/log", s.handleLog)
		r.Get("/log/filter", s.handleLogFilter)

		r.Get("/apply", s.handleApplyGET)
		r.Post("/apply/start", s.handleApplyStart)
		r.Post("/apply/confirm", s.handleApplyConfirm)
		r.Get("/apply/status", s.handleApplyStatus)

		r.Get("/export", s.handleExport)
		r.Post("/import", s.handleImport)
	})

	return r
}

// staticCacheHeaders says how long a static file may be reused, instead of
// leaving it to the browser to guess.
//
// http.FileServer sends Last-Modified and nothing else, and a response with no
// freshness information is cached heuristically: browsers reuse it for roughly
// a tenth of its age without asking the server. dpkg preserves the build mtime,
// so on a packaged installation "its age" is however long ago the release was
// built — days to weeks of serving a file the server has already replaced.
//
// A URL carrying ?v= names one release's copy of the file and can be kept for
// as long as the browser likes; the upgrade changes the URL. Everything else —
// fonts, icons, the manifest — must be revalidated, which costs one conditional
// request answered with 304.
func staticCacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

// currentCredential returns the fingerprint of the password in force now, as a
// function so callers see the value at the moment they ask rather than at wiring
// time.
func (s *Server) currentCredential() func() string {
	return func() string { return credentialFingerprint(s.cfg.PasswordHash(), s.cfg.TOTPSecret()) }
}

// recordLoginEvent is the shape every handler calls: it takes the address off
// the request and hands the event to the buffered dispatcher.
func (s *Server) recordLoginEvent(r *http.Request, ev shared.LoginEvent, left int) {
	if s.events == nil {
		return // a Server built by a test that does not care about events
	}
	s.events.Record(ev, clientIP(r), left)
}

// onLoginBlocked is what LoginRateLimit calls when it refuses a request. It is
// supplied by the server when it builds the router, so middleware.go stays free
// of the client.
func (s *Server) onLoginBlocked(ip string) {
	if s.events == nil {
		return
	}
	s.events.Record(shared.EvRateLimited, ip, 0)
}

// render executes a named template with common page data.
func (s *Server) render(w http.ResponseWriter, r *http.Request, name, page string, data interface{}) {
	if s.tmpl == nil {
		http.Error(w, "Web interface not ready: its templates are not installed", http.StatusServiceUnavailable)
		return
	}

	sess, _ := s.store.Get(r, SessionName)
	flash, _ := sess.Values["flash"].(string)
	flashN, _ := sess.Values["flash_n"].(int)
	if flash != "" {
		delete(sess.Values, "flash")
		delete(sess.Values, "flash_n")
		_ = sess.Save(r, w)
	}

	user, _ := sess.Values[SessionUserKey].(string)

	// One GET_STATUS per authenticated render, over a local Unix socket. The
	// cost is deliberate and the alternative was worse: reading the marker file
	// from here would mean guessing where the core's data directory is, and the
	// two processes may be configured apart.
	//
	// Never for a signed-out visitor. /login and /firstrun are served before
	// there is a session and have to work with no core at all.
	panicMode := false
	if user != "" {
		if status, err := s.client.GetStatus(); err == nil {
			panicMode = status.Panic
		}
	}

	loc := NewLocalizer(s.bundle, r, s.cfg.Language)
	tFunc := func(id string, args ...interface{}) string { return T(loc, id, args...) }

	// Clone template and inject the per-request T function.
	// The clone shares the parsed AST but gets its own FuncMap entry for T.
	tmpl, err := s.tmpl.Clone()
	if err != nil {
		slog.Error("template clone error", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	tmpl.Funcs(template.FuncMap{
		"T": tFunc,
		"actionLabel": func(action string) string {
			return actionLabel(tFunc, action)
		},
		"detailLabel": func(detail string) string {
			return detailLabel(tFunc, detail)
		},
	})

	nonce, _ := r.Context().Value(nonceCtxKey).(string)
	lang := ResolveLang(s.bundle, r, s.cfg.Language)
	pd := PageData{
		Flash:   flash,
		User:    user,
		Page:    page,
		Nonce:   nonce,
		Demo:    s.client.IsDemo(),
		Lang:    lang,
		Langs:   s.languageOptions(r, lang),
		Path:    r.URL.RequestURI(),
		Strings: clientStrings(tFunc),
		Asset:   shared.CurrentVersion,
		Version: shared.CurrentVersion,
		Data:    data,
		Panic:   panicMode,
		FlashN:  flashN,
	}

	// Render into a buffer first. Executing straight into the ResponseWriter
	// commits a 200 and whatever was produced before the error — for a template
	// that does not exist at all, that is an empty page reported as success, and
	// for one that fails halfway, half a form. Neither is something an operator
	// should have to recognise as a failure.
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, pd); err != nil {
		slog.Error("template render error", "template", name, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		slog.Warn("could not write rendered page", "template", name, "error", err)
	}
}

// renderPartial executes a defined template block with raw data (no PageData
// wrapper). Used for HTMX fragment endpoints — the response is just the
// inner HTML the client will swap into the target. The per-request T func
// is injected the same way render() does it.
func (s *Server) renderPartial(w http.ResponseWriter, r *http.Request, name string, data interface{}) {
	if s.tmpl == nil {
		http.Error(w, "templates missing", http.StatusServiceUnavailable)
		return
	}
	loc := NewLocalizer(s.bundle, r, s.cfg.Language)
	tFunc := func(id string, args ...interface{}) string { return T(loc, id, args...) }
	tmpl, err := s.tmpl.Clone()
	if err != nil {
		slog.Error("template clone error", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tmpl.Funcs(template.FuncMap{
		"T": tFunc,
		"actionLabel": func(action string) string {
			return actionLabel(tFunc, action)
		},
		"detailLabel": func(detail string) string {
			return detailLabel(tFunc, detail)
		},
	})
	// Buffered for the same reason as render: htmx swaps the response body into
	// the page, so a half-rendered fragment is swapped in as though it were the
	// finished one.
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		slog.Error("partial render error", "template", name, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		slog.Warn("could not write rendered fragment", "template", name, "error", err)
	}
}

// setFlash stores a one-time flash message in the session.
func (s *Server) setFlash(w http.ResponseWriter, r *http.Request, msg string) {
	sess, _ := s.store.Get(r, SessionName)
	sess.Values["flash"] = msg
	_ = sess.Save(r, w)
}

// setFlashN is setFlash with the one number a flash may carry.
func (s *Server) setFlashN(w http.ResponseWriter, r *http.Request, msg string, n int) {
	sess, _ := s.store.Get(r, SessionName)
	sess.Values["flash"] = msg
	sess.Values["flash_n"] = n
	_ = sess.Save(r, w)
}

// isHTMX reports whether the request was issued by HTMX.
// HTMX always sets the HX-Request header on AJAX requests.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// respondPartialSave returns a 204 + HX-Trigger header for HTMX partial-save
// requests (so the client toast listener can react), or a flash + redirect
// for regular form POSTs. flashKey is the i18n message key (e.g. "options_saved").
func (s *Server) respondPartialSave(w http.ResponseWriter, r *http.Request, redirect, flashKey string) {
	if isHTMX(r) {
		w.Header().Set("HX-Trigger", `{"easywall:saved":"`+flashKey+`"}`)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.setFlash(w, r, flashKey)
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// respondPartialError returns a 200 + HX-Trigger error event for HTMX,
// or a flash + redirect for regular form POSTs. We use 200 (not 5xx) so
// HTMX fires htmx:afterRequest cleanly; the client distinguishes via the
// custom event name.
func (s *Server) respondPartialError(w http.ResponseWriter, r *http.Request, redirect, flashKey string) {
	if isHTMX(r) {
		w.Header().Set("HX-Trigger", `{"easywall:error":"`+flashKey+`"}`)
		w.WriteHeader(http.StatusOK)
		return
	}
	s.setFlash(w, r, flashKey)
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// loadTemplates parses all .html files in dir as a single template set.
// A stub T function is registered so that the HTML escape context is established at parse time;
// the actual per-request T is injected via a cloned template in render().
func loadTemplates(dir string) (*template.Template, error) {
	pattern := dir + "/*.html"
	stub := func(id string, args ...interface{}) string { return id }
	funcs := templateFuncs()
	funcs["T"] = stub
	tmpl := template.New("").Funcs(funcs)
	_, err := tmpl.ParseGlob(pattern)
	if err != nil {
		return nil, err
	}
	return tmpl, nil
}

// auditActionLabels maps the core's audit action identifiers to message ids. The
// keys are exactly the strings four files write — internal/core's firewall.go,
// daemon.go and restore.go, plus cmd/easywall-core/subcommands.go, which writes
// panic_engaged and panic_resumed itself on the path where there is no daemon to
// write them. Anything unknown falls back to a humanised form of the identifier
// itself, so a new action added in the core still renders sensibly before this
// map catches up.
var auditActionLabels = map[string]string{
	"apply_started":    "audit_apply_started",
	"apply_accepted":   "audit_apply_accepted",
	"apply_rolledback": "audit_apply_rolledback",
	"apply_failed":     "audit_apply_failed",
	"rollback_failed":  "audit_rollback_failed",
	"rules_saved":      "audit_rules_saved",
	"rules_imported":   "audit_rules_imported",
	"options_saved":    "audit_options_saved",
	"settings_saved":   "audit_settings_saved",
	"system_saved":     "audit_system_saved",

	// Panic mode, and the boot-time enforcement it sits beside: the four from
	// the original panic-mode work plus three a later fix round added once the
	// edge cases around a pending apply and a contested resume turned up.
	//
	// Not all seven come from internal/core, which is what this note used to
	// claim. restore.go writes boot_enforced, boot_enforce_failed, panic_engaged,
	// panic_resumed and resume_restore_skipped; firewall.go writes
	// apply_refused_panic and rollback_skipped; daemon.go writes
	// boot_enforce_failed for a panic marker it cannot read at all; and
	// cmd/easywall-core/subcommands.go writes panic_engaged and panic_resumed
	// from the console when no daemon is running — the same actions from a
	// different process, distinguishable only by the user column.
	// Left unregistered, each one still renders — actionLabel humanises an
	// unknown identifier — but in whatever the raw snake_case says, in no
	// language a translator chose.
	"boot_enforced":          "audit_boot_enforced",
	"boot_enforce_failed":    "audit_boot_enforce_failed",
	"panic_engaged":          "audit_panic_engaged",
	"panic_resumed":          "audit_panic_resumed",
	"apply_refused_panic":    "audit_apply_refused_panic",
	"rollback_skipped":       "audit_rollback_skipped",
	"resume_restore_skipped": "audit_resume_restore_skipped",

	// The nine login events, new in 2.8. Where there were none at all before:
	// features/audit-log.md sent an operator to `journalctl -u easywall-web` for
	// a failed login, which is not where anybody looks for "who has been at the
	// door". None of them is in auditActionTones, deliberately — see the note
	// there.
	"login_ok":                   "audit_login_ok",
	"login_failed":               "audit_login_failed",
	"login_2fa_failed":           "audit_login_2fa_failed",
	"login_recovery_used":        "audit_login_recovery_used",
	"login_ratelimited":          "audit_login_ratelimited",
	"logout":                     "audit_logout",
	"totp_enabled":               "audit_totp_enabled",
	"totp_disabled":              "audit_totp_disabled",
	"recovery_codes_regenerated": "audit_recovery_codes_regenerated",
}

// auditActionTones maps an action to a firewall state, and only to a firewall
// state. DESIGN.md reserves green, amber and red for what the firewall is
// doing; saving a rule stages it and changes nothing that is live, so it stays
// neutral however important it feels.
//
// This previously lived in the stylesheet, keyed on `rules_applied` and
// `rules_rolled_back` — action names only the demo client ever produced. In
// production the real names never matched, so a rolled-back apply, the single
// most consequential line in the log, rendered neutral grey.
var auditActionTones = map[string]string{
	"apply_accepted":   "ok",
	"apply_started":    "warn",
	"apply_rolledback": "crit",
	"apply_failed":     "crit",
	// The worst outcome there is: the new rules did not take and the old ones
	// did not come back either.
	"rollback_failed": "crit",

	// boot_enforced / boot_enforce_failed: whether the stored rules made it
	// back into the kernel at startup is exactly "is the firewall doing its
	// job", the same question apply_accepted/apply_failed answer for a live
	// apply. boot_enforce_failed is the one case on this whole list where
	// nothing is worse: the machine came up and is not filtering, with no
	// operator watching a page to notice.
	"boot_enforced":       "ok",
	"boot_enforce_failed": "crit",

	// panic_engaged / panic_resumed: deliberate does not make it neutral. The
	// rule is what the firewall is doing, not whether a human meant it —
	// engaging panic mode is the machine going unfiltered, same as any other
	// path that gets there, and resuming is it filtering again.
	"panic_engaged": "crit",
	"panic_resumed": "ok",

	// resume_restore_skipped: the marker says panic mode ended, but the
	// restore an apply was holding the slot for never ran — the machine is
	// left exactly as unfiltered as boot_enforce_failed describes, just
	// reached from Resume instead of startup. Same question, same answer.
	"resume_restore_skipped": "crit",

	// apply_refused_panic and rollback_skipped are deliberately absent, for
	// the same reason rules_saved and the other staging actions above have no
	// entry: neither one leaves the firewall doing something new. A refused
	// apply leaves the kernel as it was, and an apply whose write raced a
	// `panic` has that write taken down again, ending where the console's own
	// teardown had already put it. rollback_skipped covers a rollback that
	// reverted the rules file and skipped only the kernel, which is the same
	// answer: the file is not what this colour describes, and the table it did
	// not write to is one the console had already emptied. Coloured, either
	// would look like news about the firewall's state when the actual news —
	// panic_engaged is already crit, resume_restore_skipped already crit if
	// resume failed to undo it — is elsewhere in the same log.
	//
	// The one case that is *not* neutral goes elsewhere on purpose:
	// Firewall.panicLandedDuringWrite writes boot_enforce_failed instead of the
	// action the caller asked for when its teardown failed, because a machine
	// that is filtering while the marker, the banner and `easywall-core status`
	// all report panic mode is the same state boot_enforce_failed already
	// describes, and it must not be rendered in two colours depending on which
	// code path reached it.
	//
	// None of the nine login events is here either, and for the same reason as
	// apply_refused_panic and rollback_skipped above: a login does not change
	// what the firewall is doing. It is read, not signalled. That 2.13 will push
	// a notification on repeated login_failed is not a contradiction — a
	// notification is not a colour.
}

// actionLabel resolves an action to its translated label. tFunc is the
// per-request T; passing it in rather than reaching for a package-level
// localizer keeps the audit log in the same language as the page around it,
// including the filter, which searches the label an operator can actually see.
func actionLabel(tFunc func(string, ...interface{}) string, action string) string {
	if key, ok := auditActionLabels[action]; ok {
		return tFunc(key)
	}
	// An action the core has learned to write but this map has not caught up
	// with. Humanising the identifier beats printing a raw snake_case token,
	// and it is deliberately not translated: there is nothing to translate yet.
	s := strings.ReplaceAll(action, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// wrapPairs renders text delimited by marker into element, escaping everything.
// An unterminated marker keeps its own character and the tail stays plain text:
// a typo in a translation must not blank the panel it sits in.
func wrapPairs(s, marker, element string, inner func(string) string) string {
	segs := strings.Split(s, marker)
	var b strings.Builder
	for i, seg := range segs {
		if i%2 == 1 && i < len(segs)-1 {
			b.WriteString("<" + element + ">")
			b.WriteString(template.HTMLEscapeString(seg))
			b.WriteString("</" + element + ">")
			continue
		}
		if i%2 == 1 {
			b.WriteString(marker)
		}
		b.WriteString(inner(seg))
	}
	return b.String()
}

// inlineMarkup renders the two inline forms a translation may carry: `literal`
// becomes <code>, *word* becomes <em>. Emphasis is not decoration here — "this
// list is evaluated *before* the whitelist" is the whole point of the sentence —
// so a translator needs to be able to move it.
func inlineMarkup(s string) string {
	return wrapPairs(s, "`", "code", func(seg string) string {
		return wrapPairs(seg, "*", "em", template.HTMLEscapeString)
	})
}

// richText renders a translated sentence that carries inline markup — `code`,
// *emphasis*, and "{}" for each link, filled from the href/label pairs that
// follow.
//
// Sentences like these used to be split into before/after fragments around the
// anchor, which does not survive translation: German writes "Die Blacklist wird
// zuerst ausgewertet" with the link first where English has it third. Keeping the
// sentence whole leaves word order to the translator.
//
// Everything from the locale is HTML-escaped; the only markup reaching the page
// is the <code> and <a> elements built here.
func richText(text string, hrefLabelPairs ...string) (template.HTML, error) {
	if len(hrefLabelPairs)%2 != 0 {
		return "", fmt.Errorf("richText: got %d arguments after the text, want href/label pairs",
			len(hrefLabelPairs))
	}
	parts := strings.Split(text, "{}")
	if want := len(parts) - 1; want != len(hrefLabelPairs)/2 {
		return "", fmt.Errorf("richText: %q has %d link slots but %d links were given",
			text, want, len(hrefLabelPairs)/2)
	}

	var b strings.Builder
	for i, part := range parts {
		b.WriteString(inlineMarkup(part))
		if i == len(parts)-1 {
			break
		}
		href, label := hrefLabelPairs[2*i], hrefLabelPairs[2*i+1]
		b.WriteString(`<a class="link" href="`)
		b.WriteString(template.HTMLEscapeString(href))
		b.WriteString(`">`)
		b.WriteString(inlineMarkup(label))
		b.WriteString(`</a>`)
	}
	// gosec G203: template.HTML bypasses auto-escaping, which is the point — this
	// function builds the markup. Everything that came from the locale went
	// through template.HTMLEscapeString above (inlineMarkup for the text and the
	// labels, explicitly for the href), so the only tags here are the <code>,
	// <em> and <a> written above. TestRichText_EscapesEverything guards it.
	// #nosec G203 -- every interpolated value is escaped above; this function is
	// the one that builds the markup. Covered by TestRichText_EscapesEverything.
	return template.HTML(b.String()), nil //nolint:gosec // G203 — see above
}

// auditDetailKeys maps the fixed detail strings the core writes to message ids.
// Almost every entry the core records carries an empty detail; the exceptions are
// this one token and, for a failed apply, the nftables error, which is diagnostic
// output rather than a sentence and is shown verbatim.
var auditDetailKeys = map[string]string{
	"timeout": "audit_detail_timeout",
}

// detailLabel translates a detail the core wrote from a known vocabulary, and
// leaves anything else exactly as stored. An audit record is evidence: what is not
// a recognised token is passed through rather than guessed at.
func detailLabel(tFunc func(string, ...interface{}) string, detail string) string {
	if key, ok := auditDetailKeys[detail]; ok {
		return tFunc(key)
	}
	return detail
}

// actionTone returns "ok", "warn", "crit" or "" for a neutral action.
func actionTone(action string) string { return auditActionTones[action] }

// shortTime renders a stored RFC 3339 timestamp the way someone reads a log:
// clock time for today, day and month before that. The full stored value stays
// in the element's title attribute where the templates use it, so the exact
// instant is never lost.
//
// Converted into the zone this process runs in, which is a correction. It used
// to preserve whatever offset the string carried, on the stated grounds that the
// offset "is the host's own local time, which is the frame an operator
// correlating this against syslog is working in". The reasoning was sound and
// the premise was not: the core writes UTC and nothing else — rules.go:319 for
// every audit entry, firewall.go:286 for the last apply — so preserving the
// offset meant displaying UTC to everybody. journalctl beside it shows local
// time, so the two disagreed by the host's whole offset, and
// features/audit-log.md told anyone who noticed to run `timedatectl
// set-timezone`, which could not affect it.
//
// Storage stays UTC. One unambiguous instant on disk is right for an audit log,
// and it is the only form that survives a daylight-saving boundary without
// argument. This is a display decision and it belongs here.
func shortTime(v string) string {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return v // not a timestamp we recognise; show it untouched
	}
	t = t.Local()
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04:05")
	}
	if t.Year() == now.Year() {
		return t.Format("2 Jan 15:04")
	}
	return t.Format("2 Jan 2006 15:04")
}

// clientStringKeys are the message ids app.js needs. Kept as an explicit list
// rather than shipping every translation: the blob is inlined into every page.
// The column labels are not here: a row added in the browser reads them out of
// the table header, so it is labelled in the interface's own language without
// shipping them twice. Six of them were in this list anyway, inlined into every
// page and asked for by nothing — TestClientStringsCoverWhatAppJSAsksFor now
// checks both directions.
var clientStringKeys = []string{
	"saved", "options_saved", "settings_saved", "system_saved",
	"save_error", "system_invalid_duration", "settings_invalid_network",
	"options_invalid_limit",
	"state_idle", "state_pending", "state_accepted", "state_rolled_back",
	"state_unknown",
	"apply_rolled_back_toast",
	"ports_port_hint", "ports_desc_hint", "action_remove_rule", "port_range_hint",
	"count_entry_one", "count_entry_many", "count_rule_one", "count_rule_many",
	"count_filtered",
	"totp_copy", "totp_copied", "totp_copy_failed",
}

func clientStrings(tFunc func(string, ...interface{}) string) map[string]string {
	m := make(map[string]string, len(clientStringKeys))
	for _, k := range clientStringKeys {
		m[k] = tFunc(k)
	}
	return m
}

// templateFuncs returns the shared FuncMap used across all templates.
func templateFuncs() template.FuncMap {
	successKeys := map[string]bool{
		"saved": true, "rules_accepted": true, "import_success": true,
		"options_saved": true, "password_changed": true, "settings_saved": true,
		"system_saved": true,
		// A recovery code did exactly what it exists to do.
		"recovery_left": true,
		// The second factor is now doing what it was set up to do.
		"totp_enabled": true, "totp_disabled": true, "totp_recovery_renewed": true,
		// The account was created and the choices staged — nothing failed here.
		// Found by rendering the first-run flow for a screenshot: without this,
		// the one flash every install sees renders alert-crit, in red, for a
		// message that says everything worked.
		"firstrun_done": true,
	}
	warningKeys := map[string]bool{
		"password_too_short": true, "password_mismatch": true, "username_required": true,
		"system_invalid_duration": true,
		// A network the operator has to correct, not a failure of anything. Amber
		// here and amber in app.js's toast map, which is the path this one
		// actually takes: the Network page saves itself over HTMX.
		"settings_invalid_network": true,
		"options_invalid_limit":    true,
		// Nothing went wrong here: the core declined a second apply while a
		// window was open, which is the safety mechanism working.
		"apply_already_running": true,
		"demo_readonly":         true,
		// The code was accepted and let the operator in; the disk is what
		// failed. Amber, not red: signing in did work.
		"recovery_not_consumed": true,
		// The code is right; the fault is the server's clock, not an attack —
		// and the setup timing out is a wait, not a wrong answer. Two ids per
		// direction: clockSkewKey picks _one or _many so "1 minute" is never
		// "1 minutes".
		"totp_clock_behind_one": true, "totp_clock_behind_many": true,
		"totp_clock_ahead_one": true, "totp_clock_ahead_many": true,
		"totp_setup_expired": true,
	}

	checkSVG := template.HTML(`<svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.857-9.809a.75.75 0 00-1.214-.882l-3.483 4.79-1.88-1.88a.75.75 0 10-1.06 1.061l2.5 2.5a.75.75 0 001.137-.089l4-5.5z" clip-rule="evenodd"/></svg>`)
	warnSVG := template.HTML(`<svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd"/></svg>`)
	errorSVG := template.HTML(`<svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd"/></svg>`)

	return template.FuncMap{
		"add1": func(i int) int { return i + 1 },
		// The list editors hold raw lines, comments and blanks included. A
		// counter that reports those as entries is simply wrong, and it is the
		// number an operator uses to sanity-check a paste.
		"countEntries": func(lines []string) int {
			n := 0
			for _, l := range lines {
				l = strings.TrimSpace(l)
				if l != "" && !strings.HasPrefix(l, "#") {
					n++
				}
			}
			return n
		},
		// "1 entry" / "12 entries", so the templates do not each carry an if.
		"plural": func(n int, one, many string) string {
			if n == 1 {
				return one
			}
			return many
		},
		// actionLabel is rebound per request in render()/renderPartial(), where the
		// localizer exists. This entry only keeps ParseGlob happy at startup.
		"actionLabel": func(action string) string { return action },
		"detailLabel": func(detail string) string { return detail },
		"actionTone":  actionTone,
		"richText":    richText,
		"shortTime":   shortTime,
		// dict lets a template pass named values into a translation that carries
		// its own {{.Placeholder}} — the only way a sentence with an interpolated
		// value stays one message for the translator.
		"dict": func(pairs ...interface{}) (map[string]interface{}, error) {
			if len(pairs)%2 != 0 {
				return nil, fmt.Errorf("dict: got %d arguments, want key/value pairs", len(pairs))
			}
			m := make(map[string]interface{}, len(pairs)/2)
			for i := 0; i < len(pairs); i += 2 {
				k, ok := pairs[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: key %d is %T, want string", i, pairs[i])
				}
				m[k] = pairs[i+1]
			}
			return m, nil
		},
		// Class names come from DESIGN.md § Components — only the three firewall
		// states carry colour, so there is no informational variant here.
		"flashClass": func(key string) string {
			if successKeys[key] {
				return "alert-ok"
			}
			if warningKeys[key] {
				return "alert-warn"
			}
			return "alert-crit"
		},
		"flashIcon": func(key string) template.HTML {
			if successKeys[key] {
				return checkSVG
			}
			if warningKeys[key] {
				return warnSVG
			}
			return errorSVG
		},
	}
}
