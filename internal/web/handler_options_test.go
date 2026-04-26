package web

import (
	"net/http"
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
	fc.SetResponse(shared.CmdGetOptions, errorRespFor("options unavailable"))

	rec := doAuthRequest(t, s, "GET", "/options", nil)
	assertStatus(t, rec, http.StatusOK)
}
