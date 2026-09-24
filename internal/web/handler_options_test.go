package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleOptions_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/options", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleOptions_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetOptions, successResp(shared.FirewallOptions{
		SSHBruteForce: true,
		ICMPFlood:     true,
		PortScan:      true,
	}))

	rec := doAuthRequest(t, s, "GET", "/options", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleOptions_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdGetOptions, errorRespFor("options unavailable"))

	rec := doAuthRequest(t, s, "GET", "/options", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleOptionsPOST_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/options", "ssh_brute_force=on")
	assertRedirect(t, rec, "/login")
}

func TestHandleOptionsPOST_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdSaveOptions, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/options",
		"ssh_brute_force=on&icmp_flood=on&syn_flood_limit=50")
	assertRedirect(t, rec, "/options")
}

func TestHandleOptionsPOST_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdSaveOptions, errorRespFor("save failed"))

	rec := doAuthFormRequest(t, s, "/options", "")
	assertRedirect(t, rec, "/options")
}

// Every field of FirewallOptions is read from the form, under its toml key.
// A switch added to the struct, the page and the core but not to
// handleOptionsPOST's literal is saved as off at every submit — the operator
// ticks it, "Options saved." appears, and the core is told the opposite.
// Derived from the struct, so the next option cannot be forgotten here either.
func TestHandleOptionsPOST_ReadsEveryOption(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	enrollFactor(t, s)
	fc.SetResponse(shared.CmdSaveOptions, shared.Response{Success: true})

	typ := reflect.TypeOf(shared.FirewallOptions{})
	form := url.Values{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		switch f.Type.Kind() {
		case reflect.Bool:
			form.Set(f.Tag.Get("toml"), "on")
		case reflect.Int:
			form.Set(f.Tag.Get("toml"), "7")
		default:
			t.Fatalf("%s is a %s; teach this test to fill it in", f.Name, f.Type.Kind())
		}
	}
	doAuthFormRequest(t, s, "/options", form.Encode())

	cmd := fc.LastCommand()
	var saved shared.FirewallOptions
	if cmd == nil || cmd.Type != shared.CmdSaveOptions || json.Unmarshal(cmd.Payload, &saved) != nil {
		t.Fatalf("no SAVE_OPTIONS reached the core: %+v", cmd)
	}
	v := reflect.ValueOf(saved)
	for i := 0; i < typ.NumField(); i++ {
		f := v.Field(i)
		if (f.Kind() == reflect.Bool && !f.Bool()) || (f.Kind() == reflect.Int && f.Int() != 7) {
			t.Errorf("%s (%s) was submitted and saved as %v", typ.Field(i).Name, typ.Field(i).Tag.Get("toml"), f.Interface())
		}
	}
}
