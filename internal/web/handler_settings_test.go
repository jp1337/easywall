package web

import (
	"net/http"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestHandleSettingsGET_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doRequest(s, "GET", "/settings", nil)
	assertRedirect(t, rec, "/login")
}

func TestHandleSettingsGET_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{
		IPv6:   shared.IPv6Config{Enabled: true},
		Docker: shared.DockerConfig{Enabled: false},
	}))

	rec := doAuthRequest(t, s, "GET", "/settings", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleSettingsGET_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdGetSettings, errorRespFor("core unavailable"))

	rec := doAuthRequest(t, s, "GET", "/settings", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestHandleSettingsPOST_RequiresAuth(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/settings", "ipv6_enabled=on")
	assertRedirect(t, rec, "/login")
}

func TestHandleSettingsPOST_Success(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveSettings, shared.Response{Success: true})

	rec := doAuthFormRequest(t, s, "/settings",
		"ipv6_enabled=on&docker_enabled=on&allow_bridge_networks=on&custom_networks=172.20.0.0%2F16")
	assertRedirect(t, rec, "/settings")
}

func TestHandleSettingsPOST_CoreError(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	fc.SetResponse(shared.CmdSaveSettings, errorRespFor("save failed"))

	rec := doAuthFormRequest(t, s, "/settings", "")
	assertRedirect(t, rec, "/settings")
}
