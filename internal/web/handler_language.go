package web

import (
	"log/slog"
	"net/http"
	"net/url"
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

	// Both redirects below carry #nosec G710: the taint analysis cannot see through
	// safeRedirect, which rebuilds the value from a parsed path and query and can
	// only return a local path. TestSafeRedirect and TestSafeRedirect_NeverLeavesTheSite
	// cover protocol-relative URLs, absolute URLs, backslashes and CRLF injection.
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
		// #nosec G710 -- target came from safeRedirect
		http.Redirect(w, r, target, http.StatusSeeOther) //nolint:gosec // G710
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
	// #nosec G710 -- target came from safeRedirect
	http.Redirect(w, r, target, http.StatusSeeOther) //nolint:gosec // G710
}

// safeRedirectFallback is where anything that is not a plain local path goes.
const safeRedirectFallback = "/dashboard"

// safeRedirect reduces a submitted return target to a local path. The form field
// is attacker-supplied and flows into a Location header, so without this the
// language switch would be an open redirect — "//evil.example" is a
// protocol-relative URL that a browser follows off-site.
//
// The target is parsed and the result rebuilt from the path and query alone.
// Returning a value constructed here, rather than the caller's string with a few
// prefixes ruled out, is what makes this checkable: there is no input that can
// reach the caller carrying a scheme, a host, or credentials, because none of
// those fields are ever copied into the output.
func safeRedirect(target string) string {
	if target == "" {
		return safeRedirectFallback
	}
	// Before parsing. url.Parse accepts a backslash inside a path, and browsers
	// have historically read "/\evil.example" as protocol-relative; a tab, CR or
	// LF would break the header apart. None of these belong in a path easywall
	// serves, so they are refused outright rather than normalised.
	if strings.ContainsAny(target, "\\\r\n\t") {
		return safeRedirectFallback
	}

	u, err := url.Parse(target)
	if err != nil || u.Scheme != "" || u.Opaque != "" || u.Host != "" || u.User != nil {
		return safeRedirectFallback
	}
	if !isLocalPath(u.Path) {
		return safeRedirectFallback
	}

	// Rebuilt from the two fields worth keeping. The query survives because on
	// /ports the open tab lives in ?type=, and dropping it would move an operator
	// from UDP back to TCP. A fragment never reaches the server.
	clean := (&url.URL{Path: u.Path, RawQuery: u.RawQuery}).String()
	if !isLocalPath(clean) {
		return safeRedirectFallback
	}
	return clean
}

// isLocalPath reports whether s is a path a browser resolves against this origin.
//
// All three conditions are here rather than spread across the caller, because
// this is the last thing standing between a form field and a Location header: a
// guard that depends on a check twenty lines earlier is one that survives a
// refactor by luck. It is also what CodeQL's go/bad-redirect-check looks for — a
// leading slash tested together with the character after it.
//
//   - a leading slash, or the browser treats it as relative to the current page
//   - not a second slash: "//evil.example" is a protocol-relative URL
//   - not a backslash: browsers have read "/\evil.example" the same way
func isLocalPath(s string) bool {
	return len(s) > 1 && s[0] == '/' && s[1] != '/' && s[1] != '\\'
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
