package core

import (
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// A network the daemon cannot interpret must stop it with the key named.
//
// Nothing looked at these two lists when they arrived in the file — only
// SaveNetworkSettings, on the way in from the interface — and
// features/system-settings.md tells operators to edit easywall.toml and send
// SIGHUP. So a mistyped network was accepted at startup and reached the kernel
// as nothing. Measured against a real kernel, with
// routing.networks = ["10.8.0.0/24", "10.9.0.0-24"] and mode = "networks":
//
//	daemon startup:  INFO easywall-core started / INFO daemon listening   ← no warning
//	nft list chain inet easywall forward:
//	        policy drop;
//	        ct state established,related accept
//	        ip saddr 10.8.0.0/24 accept
//	        ip daddr 10.8.0.0/24 accept          ← and nothing for 10.9.0.0
//
// configuration.md's rule is that a value which cannot be interpreted stops the
// daemon with the key named. This is that kind of value.
func TestABadNetworkInTheConfigFileStopsTheDaemon(t *testing.T) {
	for _, tc := range []struct{ name, section, want string }{
		{"routing", "[routing]\nmode = \"networks\"\nnetworks = [\"10.8.0.0/24\", \"10.9.0.0-24\"]\n", "routing network"},
		{"docker", "[docker]\nenabled = true\ncustom_networks = [\"172.17.0.0/16\", \"192.168.1.5\"]\n", "docker custom network"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := LoadConfig(writeCoreConfig(t, tc.section))
			if err != nil {
				t.Fatal(err)
			}
			err = cfg.Validate()
			if err == nil {
				t.Fatal("Validate accepted a network the apply step turns into no rule")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the message does not name the key: %v", err)
			}
		})
	}
}

// And the same lists must still accept what an operator legitimately writes into
// them, both in the file and through the interface. Refusing a comment here
// would turn this fix into a daemon that will not start.
func TestNetworkListsInTheConfigFileMayCarryComments(t *testing.T) {
	cfg, err := LoadConfig(writeCoreConfig(t,
		"[routing]\nmode = \"networks\"\nnetworks = [\"10.8.0.0/24\", \"# wireguard peers\", \"\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("a commented network list stopped the daemon: %v", err)
	}
}

// The save path and the file path have to agree, or the interface writes a file
// the daemon then refuses to reload.
func TestSaveNetworkSettingsAcceptsWhatTheEditorSends(t *testing.T) {
	cfg, err := LoadConfig(writeCoreConfig(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}

	// Exactly what handler_settings.go sends after parseIPList on a textarea
	// holding a blank separator line and a note.
	err = cfg.SaveNetworkSettings(shared.NetworkSettings{
		IPv6: shared.IPv6Config{Mode: shared.IPv6Filter},
		Docker: shared.DockerConfig{Enabled: true,
			CustomNetworks: []string{"172.17.0.0/16", "", "# the office VPN", "172.18.0.0/16"}},
		Routing: shared.RoutingConfig{Mode: shared.RoutingNetworks,
			Networks: []string{"10.8.0.0/24", "# wireguard peers"}},
	})
	if err != nil {
		t.Fatalf("the Network page could not save what its own editor accepts: %v", err)
	}
	if got := cfg.NetworkSettings().Routing.Networks; len(got) != 2 {
		t.Errorf("the note was not kept: %q", got)
	}
}
