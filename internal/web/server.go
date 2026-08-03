package web

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"html/template"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
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

	// Asset is appended to static stylesheet URLs. Without it an operator who
	// upgrades easywall keeps the cached stylesheet from the previous version
	// and sees a broken interface until they force-reload.
	Asset string
	Data  interface{}
}

// Server is the easywall web frontend.
type Server struct {
	cfg     *Config
	client  *CoreClient
	store   sessions.Store
	bundle  *i18n.Bundle
	tmpl    *template.Template
	router  chi.Router
	httpSrv *http.Server
}

// NewServer initialises the web server with all dependencies.
func NewServer(cfg *Config) (*Server, error) {
	if err := os.MkdirAll(cfg.SSLDir, 0750); err != nil {
		return nil, fmt.Errorf("create ssl dir: %w", err)
	}

	// TLS certificate — generate self-signed if needed
	if cfg.TLS.CertFile == "" {
		if certNeedsRenewal(cfg.CertPath()) {
			slog.Info("generating self-signed TLS certificate", "dir", cfg.SSLDir)
			if err := generateSelfSignedCert(cfg.SSLDir); err != nil {
				return nil, fmt.Errorf("generate TLS cert: %w", err)
			}
		}
	}

	var client *CoreClient
	if cfg.DemoMode {
		slog.Info("demo mode active — using in-memory mock instead of core socket")
		client = NewDemoClient()
	} else {
		client = NewCoreClient(cfg.SocketPath)
	}

	store := sessions.NewCookieStore([]byte(cfg.SessionKey))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   SessionLifetime,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}

	bundle := NewBundle(cfg.LocalesDir())

	s := &Server{
		cfg:    cfg,
		client: client,
		store:  store,
		bundle: bundle,
	}

	// Load templates — non-fatal if not yet created (Phase 4)
	tmpl, err := loadTemplates(cfg.TemplatesDir())
	if err != nil {
		slog.Warn("templates not loaded", "dir", cfg.TemplatesDir(), "error", err)
	} else {
		s.tmpl = tmpl
	}

	s.router = s.buildRouter(cfg)
	s.httpSrv = &http.Server{
		Addr:         cfg.BindAddr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s, nil
}

// Start begins serving HTTPS traffic. Blocks until Stop() is called.
func (s *Server) Start() error {
	slog.Info("easywall-web listening", "addr", s.cfg.BindAddr)
	if err := s.httpSrv.ListenAndServeTLS(s.cfg.CertPath(), s.cfg.KeyPath()); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTPS server: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.httpSrv.Shutdown(ctx)
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
	r.Use(MaxBodySize(64 * 1024))

	// CSRF protection via Go 1.25 net/http.CrossOriginProtection (Origin/Sec-Fetch-Site header check)
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Cross-origin request rejected", http.StatusForbidden)
	}))
	r.Use(func(next http.Handler) http.Handler { return cop.Handler(next) })

	// Static assets
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir(cfg.StaticDir()))))

	// Public routes
	r.Group(func(r chi.Router) {
		r.Get("/login", s.handleLoginGET)
		r.With(LoginRateLimit).Post("/login", s.handleLoginPOST)
		r.Get("/logout", s.handleLogout)
		// Outside RequireAuth on purpose: someone who cannot read the login page
		// has no way to sign in and change the language afterwards.
		r.Post("/language", s.handleLanguage)

		if cfg.IsFirstRun() {
			r.Get("/firstrun", s.handleFirstRunGET)
			r.Post("/firstrun", s.handleFirstRunPOST)
		}
	})

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(s.store))

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

		r.Get("/system", s.handleSystemGET)
		r.Post("/system", s.handleSystemPOST)

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

// render executes a named template with common page data.
func (s *Server) render(w http.ResponseWriter, r *http.Request, name, page string, data interface{}) {
	if s.tmpl == nil {
		http.Error(w, "Web interface not ready (templates missing — run Phase 4)", http.StatusServiceUnavailable)
		return
	}

	sess, _ := s.store.Get(r, SessionName)
	flash, _ := sess.Values["flash"].(string)
	if flash != "" {
		delete(sess.Values, "flash")
		_ = sess.Save(r, w)
	}

	user, _ := sess.Values[SessionUserKey].(string)
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
		Data:    data,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, pd); err != nil {
		slog.Error("template render error", "template", name, "error", err)
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("partial render error", "template", name, "error", err)
	}
}

// setFlash stores a one-time flash message in the session.
func (s *Server) setFlash(w http.ResponseWriter, r *http.Request, msg string) {
	sess, _ := s.store.Get(r, SessionName)
	sess.Values["flash"] = msg
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
// keys are exactly the strings written by internal/core (see firewall.go and
// daemon.go); anything unknown falls back to a humanised form of the identifier
// itself, so a new action added in the core still renders sensibly before this
// map catches up.
var auditActionLabels = map[string]string{
	"apply_started":    "audit_apply_started",
	"apply_accepted":   "audit_apply_accepted",
	"apply_rolledback": "audit_apply_rolledback",
	"apply_failed":     "audit_apply_failed",
	"rules_saved":      "audit_rules_saved",
	"rules_imported":   "audit_rules_imported",
	"options_saved":    "audit_options_saved",
	"settings_saved":   "audit_settings_saved",
	"system_saved":     "audit_system_saved",
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
	return template.HTML(b.String()), nil //nolint:gosec // G203: every interpolated value is escaped above
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
// clock time for today, day and month before that. The full value stays in the
// element's title attribute where the templates use it, so nothing is lost.
//
// The offset carried in the stored string is preserved rather than converted —
// it is the host's own local time, which is the frame an operator correlating
// this against syslog is working in.
func shortTime(v string) string {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return v // not a timestamp we recognise; show it untouched
	}
	now := time.Now().In(t.Location())
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
var clientStringKeys = []string{
	"saved", "options_saved", "settings_saved", "system_saved",
	"save_error", "system_invalid_duration",
	"state_idle", "state_pending", "state_accepted", "state_rolled_back",
	"apply_rolled_back_toast",
	"ports_port", "ports_port_hint", "ports_description", "ports_desc_hint",
	"ports_ssh", "action_remove_rule", "port_range_hint",
	"forwarding_protocol", "forwarding_source_port", "forwarding_dest_port",
	"count_entry_one", "count_entry_many", "count_rule_one", "count_rule_many",
	"count_filtered",
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
	}
	warningKeys := map[string]bool{
		"password_too_short": true, "password_mismatch": true, "username_required": true,
		"system_invalid_duration": true,
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

// certNeedsRenewal returns true if the cert doesn't exist or expires within 30 days.
func certNeedsRenewal(certPath string) bool {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return true
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return true
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return true
	}
	return time.Until(cert.NotAfter) < 30*24*time.Hour
}

// generateSelfSignedCert creates a new ECDSA P-256 self-signed certificate valid 1 year.
func generateSelfSignedCert(dir string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"easywall"},
			CommonName:   "easywall",
		},
		NotBefore:   time.Now().Add(-time.Minute),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		// SANs are required by modern browsers — CN alone is not trusted since Chrome 58.
		DNSNames:              []string{"localhost", "easywall"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}

	certPath := dir + "/cert.pem"
	certOut, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create cert file: %w", err)
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return err
	}

	keyPath := dir + "/key.pem"
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create key file: %w", err)
	}
	defer keyOut.Close()

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	return pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}
