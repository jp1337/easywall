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
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/sessions"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// PageData is passed to every template render call.
type PageData struct {
	T     func(id string, args ...interface{}) string
	Flash string
	User  string
	Page  string // current page for nav active state
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

	client := NewCoreClient(cfg.SocketPath)

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
	r.Use(middleware.RealIP)
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

		r.Get("/forwarding", s.handleForwardingGET)
		r.Post("/forwarding", s.handleForwardingPOST)

		r.Get("/custom", s.handleCustomGET)
		r.Post("/custom", s.handleCustomPOST)

		r.Get("/options", s.handleOptions)

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

	pd := PageData{
		T:     func(id string, args ...interface{}) string { return T(loc, id, args...) },
		Flash: flash,
		User:  user,
		Page:  page,
		Data:  data,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, pd); err != nil {
		slog.Error("template render error", "template", name, "error", err)
	}
}

// setFlash stores a one-time flash message in the session.
func (s *Server) setFlash(w http.ResponseWriter, r *http.Request, msg string) {
	sess, _ := s.store.Get(r, SessionName)
	sess.Values["flash"] = msg
	_ = sess.Save(r, w)
}

// loadTemplates parses all .html files in dir as a single template set.
func loadTemplates(dir string) (*template.Template, error) {
	pattern := dir + "/*.html"
	tmpl := template.New("").Funcs(templateFuncs())
	_, err := tmpl.ParseGlob(pattern)
	if err != nil {
		return nil, err
	}
	return tmpl, nil
}

// templateFuncs returns the shared FuncMap used across all templates.
func templateFuncs() template.FuncMap {
	successKeys := map[string]bool{
		"saved": true, "rules_accepted": true, "import_success": true,
	}
	warningKeys := map[string]bool{
		"password_too_short": true, "password_mismatch": true, "username_required": true,
	}

	checkSVG := template.HTML(`<svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.857-9.809a.75.75 0 00-1.214-.882l-3.483 4.79-1.88-1.88a.75.75 0 10-1.06 1.061l2.5 2.5a.75.75 0 001.137-.089l4-5.5z" clip-rule="evenodd"/></svg>`)
	warnSVG := template.HTML(`<svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd"/></svg>`)
	errorSVG := template.HTML(`<svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd"/></svg>`)

	return template.FuncMap{
		"flashClass": func(key string) string {
			if successKeys[key] {
				return "alert-success"
			}
			if warningKeys[key] {
				return "alert-warning"
			}
			return "alert-error"
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
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
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
