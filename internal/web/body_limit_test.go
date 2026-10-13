package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// A form the server could not read must not be read as a form that is empty.
// Every one of these handlers takes a missing field for "the operator cleared
// the list": a body over the size limit, or one with a broken escape, made
// ParseForm fail, FormValue answer "", and the blocklist was saved as nothing —
// measured, 6 000 imported entries saved back unchanged became 0, flashed
// "saved". The validation endpoints answered "ok" for the same reason.
func TestAFormTheServerCouldNotReadChangesNothing(t *testing.T) {
	routes := map[string]string{
		"/blocklist":            "entries",
		"/allowlist":            "entries",
		"/custom":               "rules",
		"/ports":                "rules",
		"/forwarding":           "rules",
		"/iplist/validate":      "entries",
		"/custom/validate":      "rules",
		"/settings":             "custom_networks",
		"/blocklist/feeds":      "n",
		"/blocked/stage":        "addr",
		"/options":              "ssh_brute_force",
		"/notify":               "url",
		"/system":               "acceptance_duration",
		"/password/2fa/confirm": "code",
	}
	bodies := map[string]func(field string) string{
		"over the limit": func(field string) string {
			return field + "=" + strings.Repeat("10.0.0.1%0D%0A", maxListFormBytes/14+1)
		},
		"a broken escape": func(field string) string { return field + "=10.0.0.1%zz" },
	}
	for route, field := range routes {
		for name, body := range bodies {
			t.Run(route+" "+name, func(t *testing.T) {
				fc := newFakeCore(t)
				s := newTestServer(t, fc)
				enrollFactor(t, s)
				var writes atomic.Int32
				for _, c := range []shared.CommandType{shared.CmdSaveRules, shared.CmdValidateCustom} {
					fc.OnCommand(c, func(shared.Command) { writes.Add(1) })
				}

				rec := doAuthFormRequest(t, s, route, body(field))

				if rec.Code != http.StatusRequestEntityTooLarge && rec.Code != http.StatusBadRequest {
					t.Errorf("answered %d, want 413 or 400", rec.Code)
				}
				if n := writes.Load(); n != 0 {
					t.Errorf("%d commands reached the core from a body nobody could read", n)
				}
			})
		}
	}
}

// What /import accepts has to be saveable through the editor it lands in. The
// two limits contradicted each other: a 512 KiB import fit, and the same list
// sent back from the blocklist page did not fit 64 KiB.
func TestAnImportedListCanBeSavedBackFromItsEditor(t *testing.T) {
	// The largest blocklist a maxImportBytes rules file can carry, in the
	// address form whose punctuation the browser escapes most.
	var entries []string
	size := len(`{"blocklist":[]}`)
	for i := 0; ; i++ {
		e := fmt.Sprintf("::%x/128", i)
		if size+len(e)+3 > maxImportBytes {
			break
		}
		entries = append(entries, e)
		size += len(e) + 3
	}
	file, _ := json.Marshal(map[string][]string{"blocklist": entries})
	if len(file) > maxImportBytes {
		t.Fatalf("the sample is %d bytes, over the import limit", len(file))
	}

	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	var saved atomic.Int32
	fc.OnCommand(shared.CmdSaveRules, func(c shared.Command) {
		var p struct {
			Rules []string `json:"rules"`
		}
		_ = json.Unmarshal(c.Payload, &p)
		saved.Store(int32(len(p.Rules)))
	})

	// A browser submits a textarea's line breaks as CRLF.
	form := url.Values{"entries": {strings.Join(entries, "\r\n")}}.Encode()
	rec := doAuthFormRequest(t, s, "/blocklist", form)

	assertRedirect(t, rec, "/blocklist")
	if got := int(saved.Load()); got != len(entries) {
		t.Errorf("saved %d of %d imported entries (form body %d bytes)", got, len(entries), len(form))
	}
}
