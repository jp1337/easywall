package core

import (
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// loadValidated goes the way the daemon goes: LoadConfig parses, Validate fills
// in and refuses. cmd/easywall-core/main.go calls both, and the defaulting under
// test lives in the second.
func loadValidated(t *testing.T, content string) *Config {
	t.Helper()
	cfg, err := LoadConfig(writeTempCoreConfig(t, content))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return cfg
}

// A config written before [routing] existed must come out closed.
//
// Every installation upgrading into this release is that config. Anything other
// than closed would change what a host routes on the strength of a key its
// operator has never seen.
func TestRouting_AbsentSectionIsClosed(t *testing.T) {
	cfg := loadValidated(t, validCoreConfig)
	if cfg.Routing.Mode != shared.RoutingClosed {
		t.Errorf("routing.mode = %q, want %q", cfg.Routing.Mode, shared.RoutingClosed)
	}
	if got := cfg.NetworkSettings().Routing.Mode; got != shared.RoutingClosed {
		t.Errorf("NetworkSettings routing.mode = %q, want %q", got, shared.RoutingClosed)
	}
}

// A mode that is set and wrong is refused, naming the three that are not.
// Guessing between them would open or close a router quietly.
func TestRouting_UnknownModeIsRefused(t *testing.T) {
	cfg, err := LoadConfig(writeTempCoreConfig(t, validCoreConfig+"\n[routing]\nmode = \"sometimes\"\n"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	err = cfg.Validate()
	if err == nil {
		t.Fatal("expected an unknown routing.mode to be refused")
	}
	for _, want := range []string{"routing.mode", "closed", "networks", "open", "sometimes"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %q: %v", want, err)
		}
	}
}

func TestRouting_ModesLoad(t *testing.T) {
	for _, mode := range []shared.RoutingMode{shared.RoutingClosed, shared.RoutingNetworks, shared.RoutingOpen} {
		cfg := loadValidated(t, validCoreConfig+
			"\n[routing]\nmode = \""+string(mode)+"\"\nnetworks = [\"10.8.0.0/24\"]\n")
		if cfg.Routing.Mode != mode {
			t.Errorf("routing.mode = %q, want %q", cfg.Routing.Mode, mode)
		}
		if len(cfg.Routing.Networks) != 1 || cfg.Routing.Networks[0] != "10.8.0.0/24" {
			t.Errorf("routing.networks = %v", cfg.Routing.Networks)
		}
	}
}

// An entry the apply step could not turn into a rule is refused on save.
//
// The alternative is what the Docker list used to do before it was checked: the
// interface lists a network as routed, addCIDRAccept skips it silently, and the
// operator finds out because traffic they expected to pass does not.
func TestRouting_SaveRefusesSomethingThatIsNotANetwork(t *testing.T) {
	cfg := loadValidated(t, validCoreConfig)
	err := cfg.SaveNetworkSettings(shared.NetworkSettings{
		IPv6:    shared.IPv6Config{Mode: shared.IPv6Filter},
		Routing: shared.RoutingConfig{Mode: shared.RoutingNetworks, Networks: []string{"10.8.0.0"}},
	})
	if err == nil {
		t.Fatal("expected a bare address to be refused as a routing network")
	}
	if !strings.Contains(err.Error(), "routing network") {
		t.Errorf("the error does not say which list it came from: %v", err)
	}
}

func TestRouting_SaveRefusesAnUnknownMode(t *testing.T) {
	cfg := loadValidated(t, validCoreConfig)
	err := cfg.SaveNetworkSettings(shared.NetworkSettings{
		IPv6:    shared.IPv6Config{Mode: shared.IPv6Filter},
		Routing: shared.RoutingConfig{Mode: "wide-open"},
	})
	if err == nil {
		t.Fatal("expected an unknown routing mode to be refused on save")
	}
}

// Saved, written to the file, and adopted by a reload — the path the settings
// page and SIGHUP actually take.
func TestRouting_SurvivesSaveAndReload(t *testing.T) {
	cfg := loadValidated(t, validCoreConfig)
	path := cfg.configPath

	want := shared.RoutingConfig{Mode: shared.RoutingNetworks, Networks: []string{"10.8.0.0/24", "fd00::/64"}}
	if err := cfg.SaveNetworkSettings(shared.NetworkSettings{
		IPv6:    shared.IPv6Config{Mode: shared.IPv6Filter},
		Routing: want,
	}); err != nil {
		t.Fatalf("SaveNetworkSettings: %v", err)
	}

	reread, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("re-reading the saved config: %v", err)
	}
	if reread.Routing.Mode != want.Mode || len(reread.Routing.Networks) != 2 {
		t.Errorf("after save the file holds %+v, want %+v", reread.Routing, want)
	}

	// Reload adopts it in the running daemon, which is what SIGHUP does.
	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if cfg.NetworkSettings().Routing.Mode != want.Mode {
		t.Errorf("Reload did not adopt [routing]: %+v", cfg.NetworkSettings().Routing)
	}
}

// NetworkSettings hands out a copy. Sharing the backing array is the race the
// Docker list already had to be fixed for.
func TestRouting_NetworkSettingsCopiesTheList(t *testing.T) {
	cfg := loadValidated(t, validCoreConfig+
		"\n[routing]\nmode = \"networks\"\nnetworks = [\"10.8.0.0/24\"]\n")
	handed := cfg.NetworkSettings().Routing.Networks
	handed[0] = "0.0.0.0/0"
	if cfg.Routing.Networks[0] != "10.8.0.0/24" {
		t.Errorf("the caller's write reached the config: %v", cfg.Routing.Networks)
	}
}
