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

// hasLine reports whether a line was rejected, and returns its entry.
func hasLine(errs []lineError, line int) (lineError, bool) {
	for _, e := range errs {
		if e.Line == line {
			return e, true
		}
	}
	return lineError{}, false
}

func TestValidateIPListEntries_InvalidIP(t *testing.T) {
	errs := validateIPListEntries("not-an-ip")
	e, ok := hasLine(errs, 1)
	if !ok {
		t.Fatalf("expected an error on line 1, got %v", errs)
	}
	if e.Key != "validate_invalid_ip" {
		t.Errorf("key = %q, want validate_invalid_ip", e.Key)
	}
}

func TestValidateIPListEntries_InvalidCIDR(t *testing.T) {
	errs := validateIPListEntries("192.168.1.1/99")
	e, ok := hasLine(errs, 1)
	if !ok {
		t.Fatalf("expected an error on line 1, got %v", errs)
	}
	if e.Key != "validate_invalid_cidr" {
		t.Errorf("key = %q, want validate_invalid_cidr", e.Key)
	}
	// The parser's own words are diagnostic output, not a sentence to translate,
	// so they ride along beside the translated reason.
	if e.Detail == "" {
		t.Error("expected the parser error to be carried in Detail")
	}
}

func TestValidateIPListEntries_MixedLineNumbers(t *testing.T) {
	// Each line index should match the position in the raw input,
	// including blank/comment lines (which get skipped, not removed).
	raw := strings.Join([]string{
		"# comment on line 1", // skip
		"192.168.1.1",         // valid
		"not-an-ip",           // line 3 — INVALID
		"",                    // skip
		"10.0.0.5/24",         // valid
		"999.999.999.999",     // line 6 — INVALID (octets out of range)
	}, "\n")
	errs := validateIPListEntries(raw)

	// 1-based, counting blanks and comments, so the number matches the textarea.
	for _, want := range []int{3, 6} {
		if _, ok := hasLine(errs, want); !ok {
			t.Errorf("expected an error on line %d, got %v", want, errs)
		}
	}
	for _, unwanted := range []int{1, 2, 4, 5} {
		if e, ok := hasLine(errs, unwanted); ok {
			t.Errorf("did not expect an error on line %d, got %+v", unwanted, e)
		}
	}
	// Reported in line order, so the list reads top to bottom like the textarea.
	for i := 1; i < len(errs); i++ {
		if errs[i-1].Line > errs[i].Line {
			t.Errorf("errors are not in line order: %v", errs)
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
	assertAlertVariant(t, body, "alert-ok")
}

func TestHandleIPListValidate_WithErrors(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doAuthFormRequest(t, s, "/iplist/validate", "entries=not-an-ip%0A192.168.1.1")
	assertStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	assertAlertVariant(t, body, "alert-crit")
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
