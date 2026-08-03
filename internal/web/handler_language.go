package web

import (
	"log/slog"
	"net/http"
	"strings"
)

// langCookieMaxAge keeps the choice for a year. It is a display preference, not a
// session: signing out should not put the interface back into a language the
// operator does not read.
const langCookieMaxAge = 365 * 24 * 60 * 60

// handleLanguage records an explicit language choice and returns to the page the
// operator was on.
//
// POST rather than GET: it writes a cookie, and the CrossOriginProtection
// middleware only guards unsafe methods. A plain form means the switcher works
// with JavaScript disabled.
func (s *Server) handleLanguage(w http.ResponseWriter, r *http.Request) {
	lang := r.FormValue("lang")

	// gosec G710 flags http.Redirect with a request-derived target and cannot see
	// through the sanitiser. safeRedirect reduces this to a local path — see
	// TestSafeRedirect and TestSafeRedirect_NeverLeavesTheSite, which cover
	// protocol-relative URLs, absolute URLs, backslashes and CRLF injection.
	target := safeRedirect(r.FormValue("return"))

	// Only a language the bundle actually has. Anything else would be written
	// into a cookie and reflected back on every request afterwards.
	valid := false
	for _, available := range AvailableLangs(s.bundle) {
		if lang == available {
			valid = true
			break
		}
	}
	if !valid {
		slog.Debug("rejected unknown language", "lang", lang)
		http.Redirect(w, r, target, http.StatusSeeOther) //nolint:gosec // G710: target is sanitised by safeRedirect
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     LangCookie,
		Value:    lang,
		Path:     "/",
		MaxAge:   langCookieMaxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, target, http.StatusSeeOther) //nolint:gosec // G710: target is sanitised by safeRedirect
}

// safeRedirect reduces a submitted return target to a local path. Without this
// the switcher would be an open redirect: the form field is attacker-supplied,
// and "//evil.example" is a protocol-relative URL that browsers follow off-site.
func safeRedirect(target string) string {
	if target == "" || !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") {
		return "/dashboard"
	}
	// Reject anything that could carry a scheme or credentials past the prefix
	// check, or split the Location header.
	if strings.ContainsAny(target, ":\\\r\n") {
		return "/dashboard"
	}
	// The query is kept: on /ports the open tab lives in ?type=, so dropping it
	// would silently move an operator from UDP back to TCP. A fragment never
	// reaches the server, so there is nothing to preserve there.
	if i := strings.IndexByte(target, '#'); i >= 0 {
		target = target[:i]
	}
	if target == "" || target == "/" {
		return "/dashboard"
	}
	return target
}

// languageOption is one entry in the switcher. Label is the language's own name,
// read from that language's own locale file, so German reads "Deutsch" rather
// than "German" whatever the current interface language is.
type languageOption struct {
	Code    string
	Label   string
	Current bool
}

func (s *Server) languageOptions(r *http.Request, current string) []languageOption {
	codes := AvailableLangs(s.bundle)
	if len(codes) < 2 {
		return nil // nothing to switch between; the control would be noise
	}
	opts := make([]languageOption, 0, len(codes))
	for _, code := range codes {
		loc := newLocalizerForLang(s.bundle, code)
		label := T(loc, "language_name")
		if label == "language_name" {
			label = strings.ToUpper(code) // locale file forgot to name itself
		}
		opts = append(opts, languageOption{Code: code, Label: label, Current: code == current})
	}
	return opts
}
