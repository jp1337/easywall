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

	cookie := makeAuthCookie(t, s)
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
	_ = w.Close()

	cookie := makeAuthCookie(t, s)
	req := httptest.NewRequest("POST", "/import", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	assertRedirect(t, rec, "/dashboard")
}

// A rule set big enough to matter still has to import. The handler documents a
// 512 KB ceiling, but the global MaxBodySize middleware had already wrapped the
// body at 64 KB, and the handler's own MaxBytesReader wrapped that limited
// reader rather than replacing it — so the inner limit won. A blacklist of a few
// thousand addresses is an ordinary export from a busy host and it came back as
// "no file uploaded", which blames the operator for a size problem.
func TestHandleImport_AcceptsAFileLargerThanTheGlobalBodyLimit(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdImportRules, shared.Response{Success: true})

	blacklist := make([]string, 6000)
	for i := range blacklist {
		blacklist[i] = fmt.Sprintf("198.51.%d.%d", i/256, i%256)
	}
	payload, err := json.Marshal(shared.Rules{Blacklist: blacklist})
	if err != nil {
		t.Fatalf("marshal rules: %v", err)
	}
	if len(payload) <= 64*1024 {
		t.Fatalf("test payload is %d bytes, which does not exceed the global limit", len(payload))
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("rules_file", "rules.json")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	_ = w.Close()

	cookie := makeAuthCookie(t, s)
	req := httptest.NewRequest("POST", "/import", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	assertRedirect(t, rec, "/dashboard")
	last := fc.LastCommand()
	if last == nil || last.Type != shared.CmdImportRules {
		t.Fatalf("import never reached the core; last command was %v", last)
	}
}

// Past the handler's own ceiling the operator has to be told it was the size.
func TestHandleImport_RejectsAnOversizedFileWithASizeMessage(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("rules_file", "rules.json")
	if _, err := fw.Write(bytes.Repeat([]byte("x"), maxImportBytes+1)); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	_ = w.Close()

	cookie := makeAuthCookie(t, s)
	req := httptest.NewRequest("POST", "/import", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	assertRedirect(t, rec, "/dashboard")
	if fc.LastCommand() != nil {
		t.Error("an oversized upload should not reach the core")
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "easywall") {
		t.Error("expected a flash cookie explaining the rejection")
	}
}

func TestHandleImport_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdImportRules, errorRespFor("invalid rules"))

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("rules_file", "rules.json")
	_, _ = io.WriteString(fw, `{}`)
	_ = w.Close()

	cookie := makeAuthCookie(t, s)
	req := httptest.NewRequest("POST", "/import", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	assertRedirect(t, rec, "/dashboard")
}
