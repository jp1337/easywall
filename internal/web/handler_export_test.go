package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleExport_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/export", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleExport_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	exportedRules := shared.Rules{
		TCP: []shared.PortRule{{Port: "22", Description: "SSH"}},
	}
	exportedData, _ := json.Marshal(exportedRules)
	wrappedData, _ := json.Marshal(json.RawMessage(exportedData))
	fc.SetResponse(shared.CmdExportRules, shared.Response{Success: true, Data: wrappedData})

	rec := doAuthRequest(t, s, "GET", "/export", nil)
	assertStatus(t, rec, http.StatusOK)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
	cd := rec.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Errorf("expected attachment content-disposition, got %s", cd)
	}
	if !strings.Contains(cd, "easywall-rules-") {
		t.Errorf("expected filename with easywall-rules-, got %s", cd)
	}
}

func TestHandleExport_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdExportRules, errorRespFor("export failed"))

	rec := doAuthRequest(t, s, "GET", "/export", nil)
	assertRedirect(t, rec, "/dashboard")
}

func TestHandleImport_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "POST", "/import", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleImport_NoFile(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	cookie := makeAuthCookie(t, s.store)
	req := httptest.NewRequest("POST", "/import", strings.NewReader("no_file_field=true"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	assertRedirect(t, rec, "/dashboard")
}

func TestHandleImport_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdImportRules, shared.Response{Success: true})

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("rules_file", "rules.json")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = fmt.Fprint(fw, `{"tcp":[],"udp":[],"blacklist":[],"whitelist":[],"forwarding":[],"custom":[]}`)
	w.Close()

	cookie := makeAuthCookie(t, s.store)
	req := httptest.NewRequest("POST", "/import", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	assertRedirect(t, rec, "/dashboard")
}

func TestHandleImport_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdImportRules, errorRespFor("invalid rules"))

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("rules_file", "rules.json")
	_, _ = io.WriteString(fw, `{}`)
	w.Close()

	cookie := makeAuthCookie(t, s.store)
	req := httptest.NewRequest("POST", "/import", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	assertRedirect(t, rec, "/dashboard")
}
