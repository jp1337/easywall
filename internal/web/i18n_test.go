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
