package web

import (
	"net/http"
	"strings"
	"testing"
)

func TestValidateIPListEntries_Empty(t *testing.T) {
	errs := validateIPListEntries("")
	if len(errs) != 0 {
		t.Errorf("expected no errors for empty input, got %v", errs)
	}
}

func TestValidateIPListEntries_BlankAndComments(t *testing.T) {
	errs := validateIPListEntries("\n  \n# comment\n  # indented comment\n")
	if len(errs) != 0 {
		t.Errorf("expected no errors for blank/comment lines, got %v", errs)
	}
}

func TestValidateIPListEntries_ValidIPv4(t *testing.T) {
	errs := validateIPListEntries("192.168.1.1\n10.0.0.5")
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateIPListEntries_ValidIPv6(t *testing.T) {
	errs := validateIPListEntries("2001:db8::1\nfe80::1234:5678:abcd:ef00")
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateIPListEntries_ValidCIDR(t *testing.T) {
	errs := validateIPListEntries("192.168.0.0/24\n10.0.0.0/8\n2001:db8::/32")
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateIPListEntries_InvalidIP(t *testing.T) {
	errs := validateIPListEntries("not-an-ip")
	if _, ok := errs[0]; !ok {
		t.Errorf("expected error at line 0, got %v", errs)
	}
}

func TestValidateIPListEntries_InvalidCIDR(t *testing.T) {
	errs := validateIPListEntries("192.168.1.1/99")
	if _, ok := errs[0]; !ok {
		t.Errorf("expected error at line 0 (invalid CIDR), got %v", errs)
	}
}

func TestValidateIPListEntries_MixedLineNumbers(t *testing.T) {
	// Each line index should match the position in the raw input,
	// including blank/comment lines (which get skipped, not removed).
	raw := strings.Join([]string{
		"# comment at line 0", // 0 — skip
		"192.168.1.1",         // 1 — valid
		"not-an-ip",           // 2 — INVALID
		"",                    // 3 — skip
		"10.0.0.5/24",         // 4 — valid
		"999.999.999.999",     // 5 — INVALID (octets out of range)
	}, "\n")
	errs := validateIPListEntries(raw)

	if _, ok := errs[2]; !ok {
		t.Errorf("expected error at line 2, got %v", errs)
	}
	if _, ok := errs[5]; !ok {
		t.Errorf("expected error at line 5, got %v", errs)
	}
	for _, ok := range []int{0, 1, 3, 4} {
		if _, exists := errs[ok]; exists {
			t.Errorf("did not expect error at line %d, got %q", ok, errs[ok])
		}
	}
}

func TestHandleIPListValidate_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/iplist/validate", "entries=192.168.1.1")
	assertRedirect(t, rec, "/login")
}

func TestHandleIPListValidate_AllValid(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doAuthFormRequest(t, s, "/iplist/validate", "entries=192.168.1.0%2F24%0A2001%3Adb8%3A%3A1")
	assertStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	if !strings.Contains(body, "alert-success") {
		t.Errorf("expected alert-success in response, got: %s", body)
	}
}

func TestHandleIPListValidate_WithErrors(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doAuthFormRequest(t, s, "/iplist/validate", "entries=not-an-ip%0A192.168.1.1")
	assertStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	if !strings.Contains(body, "alert-error") {
		t.Errorf("expected alert-error in response, got: %s", body)
	}
	if !strings.Contains(body, "Line 1") {
		t.Errorf("expected 'Line 1' in error list, got: %s", body)
	}
}

func TestHandleIPListValidate_EscapesErrorMessage(t *testing.T) {
	// XSS guard: any user-controlled content in the error output must be escaped.
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	// The bad input here contains <script> tags. Our error message ("not a
	// valid IP address") doesn't echo the input, but ParseCIDR's err.Error()
	// would for a CIDR with HTML. Use one to exercise the escape path.
	rec := doAuthFormRequest(t, s, "/iplist/validate", "entries=%3Cscript%3E%2F24")
	body := rec.Body.String()
	if strings.Contains(body, "<script>") {
		t.Errorf("response contains unescaped <script> tag: %s", body)
	}
}
