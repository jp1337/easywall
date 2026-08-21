package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// NewBundle loads all JSON message files from localesDir into a new i18n bundle.
// Falls back gracefully if directory or files don't exist.
func NewBundle(localesDir string) *i18n.Bundle {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	entries, err := os.ReadDir(localesDir)
	if err != nil {
		slog.Warn("locales directory not found", "dir", localesDir)
		return bundle
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == statusFile {
			continue
		}
		path := localesDir + "/" + e.Name()
		if _, err := bundle.LoadMessageFile(path); err != nil {
			slog.Warn("failed to load locale file", "file", path, "error", err)
		} else {
			slog.Debug("loaded locale", "file", e.Name())
		}
	}
	return bundle
}

// LangCookie holds an explicit language choice. It outranks Accept-Language,
// because a browser header describes the machine and this describes the person
// sitting at it — an operator on a borrowed laptop still gets their own language.
const LangCookie = "easywall_lang"

// langCandidates lists the languages to try, best first: the cookie the operator
// chose, then what their browser asks for, then the configured default, then
// English as the last resort.
func langCandidates(r *http.Request, defaultLang string) []string {
	var langs []string
	if c, err := r.Cookie(LangCookie); err == nil && c.Value != "" {
		langs = append(langs, c.Value)
	}
	langs = append(langs, parseAcceptLanguage(r.Header.Get("Accept-Language"))...)
	return append(langs, defaultLang, "en")
}

// NewLocalizer creates a localizer for the given request and default language.
func NewLocalizer(bundle *i18n.Bundle, r *http.Request, defaultLang string) *i18n.Localizer {
	return i18n.NewLocalizer(bundle, langCandidates(r, defaultLang)...)
}

// newLocalizerForLang builds a localizer pinned to one language, used to read a
// locale file's own name for the language switcher.
func newLocalizerForLang(bundle *i18n.Bundle, lang string) *i18n.Localizer {
	return i18n.NewLocalizer(bundle, lang)
}

// AvailableLangs lists the base language tags the bundle actually has messages
// for, sorted so the interface offers them in a stable order. It is derived from
// the locale files rather than hard-coded: dropping a fr.json into locales/ is
// all it should take to offer French.
func AvailableLangs(bundle *i18n.Bundle) []string {
	seen := make(map[string]bool)
	var out []string
	for _, t := range bundle.LanguageTags() {
		base, _ := t.Base()
		if b := base.String(); !seen[b] {
			seen[b] = true
			out = append(out, b)
		}
	}
	sort.Strings(out)
	return out
}

// ResolveLang reports the language the localizer will actually serve, so a page
// can declare it in <html lang>. go-i18n does not expose the tag it matched,
// and a page that renders German while declaring English is a WCAG 3.1.1
// failure: a screen reader pronounces it with English phonetics.
func ResolveLang(bundle *i18n.Bundle, r *http.Request, defaultLang string) string {
	available := make(map[string]bool)
	for _, l := range AvailableLangs(bundle) {
		available[l] = true
	}

	for _, c := range langCandidates(r, defaultLang) {
		tag, err := language.Parse(c)
		if err != nil {
			continue
		}
		if base, _ := tag.Base(); available[base.String()] {
			return base.String()
		}
	}
	return "en"
}

// T returns the translation for the given message ID, or the ID itself as fallback.
func T(l *i18n.Localizer, id string, args ...interface{}) string {
	cfg := &i18n.LocalizeConfig{MessageID: id}
	if len(args) == 1 {
		if data, ok := args[0].(map[string]interface{}); ok {
			cfg.TemplateData = data
		}
	}
	msg, err := l.Localize(cfg)
	if err != nil {
		return id
	}
	return msg
}

// parseAcceptLanguage extracts language tags from an Accept-Language header.
func parseAcceptLanguage(header string) []string {
	if header == "" {
		return nil
	}
	var langs []string
	for _, part := range strings.Split(header, ",") {
		lang := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if lang != "" {
			langs = append(langs, lang)
		}
	}
	return langs
}
