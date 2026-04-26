package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
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
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
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

// NewLocalizer creates a localizer for the given request and default language.
// It reads the Accept-Language header and falls back to defaultLang.
func NewLocalizer(bundle *i18n.Bundle, r *http.Request, defaultLang string) *i18n.Localizer {
	accept := r.Header.Get("Accept-Language")
	langs := parseAcceptLanguage(accept)
	langs = append(langs, defaultLang, "en")
	return i18n.NewLocalizer(bundle, langs...)
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
