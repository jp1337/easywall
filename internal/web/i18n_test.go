package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseAcceptLanguage_Empty(t *testing.T) {
	if langs := parseAcceptLanguage(""); langs != nil {
		t.Errorf("expected nil for empty header, got: %v", langs)
	}
}

func TestParseAcceptLanguage_Single(t *testing.T) {
	langs := parseAcceptLanguage("de")
	if len(langs) != 1 || langs[0] != "de" {
		t.Errorf("unexpected: %v", langs)
	}
}

func TestParseAcceptLanguage_Multiple(t *testing.T) {
	langs := parseAcceptLanguage("de-DE,de;q=0.9,en;q=0.8")
	if len(langs) != 3 {
		t.Errorf("expected 3 languages, got %d: %v", len(langs), langs)
	}
	if langs[0] != "de-DE" {
		t.Errorf("expected de-DE first, got: %s", langs[0])
	}
}

func TestParseAcceptLanguage_StripsQuality(t *testing.T) {
	langs := parseAcceptLanguage("en;q=0.5")
	if len(langs) != 1 || langs[0] != "en" {
		t.Errorf("expected quality stripped, got: %v", langs)
	}
}

func TestNewBundle_MissingDir(t *testing.T) {
	bundle := NewBundle("/nonexistent/dir/locales")
	if bundle == nil {
		t.Error("expected non-nil bundle even when dir is missing")
	}
}

func TestNewBundle_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	bundle := NewBundle(dir)
	if bundle == nil {
		t.Error("expected non-nil bundle for empty dir")
	}
}

func TestNewBundle_LoadsJSON(t *testing.T) {
	dir := t.TempDir()
	content := `[{"id":"hello","translation":"Hello!"}]`
	_ = os.WriteFile(filepath.Join(dir, "en.json"), []byte(content), 0644)
	bundle := NewBundle(dir)
	if bundle == nil {
		t.Fatal("expected non-nil bundle")
	}
}

func TestNewBundle_SkipsNonJSON(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not json"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "subdir"), nil, 0755)
	// should not panic
	bundle := NewBundle(dir)
	if bundle == nil {
		t.Error("expected non-nil bundle")
	}
}

func TestT_FallbackToID(t *testing.T) {
	dir := t.TempDir()
	bundle := NewBundle(dir) // no locale files
	req := httptest.NewRequest("GET", "/", nil)
	l := NewLocalizer(bundle, req, "en")
	result := T(l, "some_nonexistent_key")
	if result != "some_nonexistent_key" {
		t.Errorf("expected fallback to key, got: %s", result)
	}
}

func TestT_ReturnsTranslation(t *testing.T) {
	dir := t.TempDir()
	content := `[{"id":"greeting","translation":"Hi there!"}]`
	_ = os.WriteFile(filepath.Join(dir, "en.json"), []byte(content), 0644)
	bundle := NewBundle(dir)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", "en")
	l := NewLocalizer(bundle, req, "en")
	result := T(l, "greeting")
	if result != "Hi there!" {
		t.Errorf("expected translation 'Hi there!', got: %s", result)
	}
}

func TestNewLocalizer_UsesFallback(t *testing.T) {
	dir := t.TempDir()
	bundle := NewBundle(dir)
	req := httptest.NewRequest("GET", "/", nil)
	// No Accept-Language header
	l := NewLocalizer(bundle, req, "en")
	if l == nil {
		t.Error("expected non-nil localizer")
	}
}

func TestT_WithTemplateData(t *testing.T) {
	dir := t.TempDir()
	bundle := NewBundle(dir) // no locale files
	req := httptest.NewRequest("GET", "/", nil)
	l := NewLocalizer(bundle, req, "en")
	// Pass map[string]interface{} as args[0] — should set TemplateData on config
	// Translation key doesn't exist, so result falls back to the key
	result := T(l, "some_key_with_data", map[string]interface{}{"Name": "World"})
	if result != "some_key_with_data" {
		t.Errorf("expected key fallback when no translation exists, got: %s", result)
	}
}

func TestT_WithTemplateDataAndTranslation(t *testing.T) {
	dir := t.TempDir()
	// go-i18n v2 format with template variable
	content := `[{"id":"greeting","translation":"Hello {{.Name}}!"}]`
	_ = os.WriteFile(filepath.Join(dir, "en.json"), []byte(content), 0644)
	bundle := NewBundle(dir)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", "en")
	l := NewLocalizer(bundle, req, "en")
	result := T(l, "greeting", map[string]interface{}{"Name": "World"})
	if result != "Hello World!" {
		t.Errorf("expected 'Hello World!', got: %s", result)
	}
}

func TestT_NonMapArg_NotUsedAsTemplateData(t *testing.T) {
	dir := t.TempDir()
	bundle := NewBundle(dir)
	req := httptest.NewRequest("GET", "/", nil)
	l := NewLocalizer(bundle, req, "en")
	// A non-map arg — the type assertion fails, TemplateData stays nil
	result := T(l, "some_key", "not a map")
	if result != "some_key" {
		t.Errorf("expected key fallback, got: %s", result)
	}
}

func TestNewBundle_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	// Valid filename suffix .json, but invalid content for go-i18n
	_ = os.WriteFile(filepath.Join(dir, "en.json"), []byte("this is not json at all"), 0644)
	bundle := NewBundle(dir)
	if bundle == nil {
		t.Error("expected non-nil bundle even with invalid locale JSON")
	}
	// The bundle should be usable (falls back gracefully)
}

func TestNewLocalizer_ReadsHeader(t *testing.T) {
	dir := t.TempDir()
	content := `[{"id":"hello","translation":"Hallo!"}]`
	_ = os.WriteFile(filepath.Join(dir, "de.json"), []byte(content), 0644)
	bundle := NewBundle(dir)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", "de")
	l := NewLocalizer(bundle, req, "en")
	if l == nil {
		t.Error("expected non-nil localizer")
	}
	result := T(l, "hello")
	if result != "Hallo!" {
		t.Errorf("expected German translation, got: %s", result)
	}
}

func TestResolveLang(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"en.json": `[{"id":"hello","translation":"Hello"}]`,
		"de.json": `[{"id":"hello","translation":"Hallo"}]`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bundle := NewBundle(dir)

	cases := []struct {
		name, header, dflt, want string
	}{
		{"no header falls back to config", "", "en", "en"},
		{"config default wins when no header", "", "de", "de"},
		{"regional tag matches its base", "de-DE,de;q=0.9", "en", "de"},
		{"first available candidate wins", "fr-FR,de;q=0.8", "en", "de"},
		{"unavailable language falls through", "fr-FR,fr;q=0.9", "en", "en"},
		{"garbage tag is skipped, not fatal", "!!!,de", "en", "de"},
		{"unknown config default still yields a tag", "", "xx", "en"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if c.header != "" {
				req.Header.Set("Accept-Language", c.header)
			}
			if got := ResolveLang(bundle, req, c.dflt); got != c.want {
				t.Errorf("ResolveLang(header=%q, default=%q) = %q, want %q",
					c.header, c.dflt, got, c.want)
			}
		})
	}
}

// ResolveLang exists only so <html lang> matches the text actually rendered. If
// the two ever disagree the page lies to a screen reader, so assert them
// together rather than trusting the helper in isolation.
func TestResolveLang_AgreesWithRenderedText(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"en.json": `[{"id":"hello","translation":"Hello"}]`,
		"de.json": `[{"id":"hello","translation":"Hallo"}]`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bundle := NewBundle(dir)

	for header, want := range map[string]string{"de-DE,de;q=0.9": "Hallo", "en-GB": "Hello"} {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Language", header)

		lang := ResolveLang(bundle, req, "en")
		text := T(NewLocalizer(bundle, req, "en"), "hello")
		if text != want {
			t.Fatalf("header %q rendered %q, want %q", header, text, want)
		}
		wantLang := map[string]string{"Hallo": "de", "Hello": "en"}[want]
		if lang != wantLang {
			t.Errorf("header %q rendered %q but declared lang=%q, want %q",
				header, text, lang, wantLang)
		}
	}
}
