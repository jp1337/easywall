package core

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// nftAvailable returns true if nft is on PATH AND can actually access the
// kernel (CAP_NET_ADMIN). A plain LookPath check is insufficient in WSL2 /
// unprivileged containers where the binary exists but netlink is blocked.
func nftAvailable() bool {
	if _, err := exec.LookPath("nft"); err != nil {
		return false
	}
	// Run an empty check — succeeds only when kernel access is granted.
	cmd := exec.Command("nft", "--check", "--file", "-")
	cmd.Stdin = strings.NewReader("table inet ew_probe {}\n")
	return cmd.Run() == nil
}

func TestValidateCustomRules_EmptyAndComments(t *testing.T) {
	// Empty lines and comment lines must be skipped without calling nft.
	// This works even without nft installed.
	errs := validateCustomRules([]string{"", "  ", "# this is a comment", "  # indented comment"})
	if len(errs) != 0 {
		t.Errorf("expected no errors for blank/comment lines, got %v", errs)
	}
}

func TestValidateCustomRules_ValidRule(t *testing.T) {
	if !nftAvailable() {
		t.Skip("nft not available")
	}
	errs := validateCustomRules([]string{"tcp dport 80 accept"})
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid rule, got %v", errs)
	}
}

func TestValidateCustomRules_InvalidRule(t *testing.T) {
	if !nftAvailable() {
		t.Skip("nft not available")
	}
	errs := validateCustomRules([]string{"this is not valid nftables syntax !!!"})
	if len(errs) == 0 {
		t.Error("expected an error for invalid rule")
	}
	if _, ok := errs[0]; !ok {
		t.Errorf("expected error at index 0, got %v", errs)
	}
}

func TestValidateCustomRules_MixedRules(t *testing.T) {
	if !nftAvailable() {
		t.Skip("nft not available")
	}
	rules := []string{
		"# comment at index 0",
		"tcp dport 443 accept", // valid, index 1
		"not valid syntax !!!", // invalid, index 2
		"",                     // blank, index 3
		"udp dport 53 accept",  // valid, index 4
	}
	errs := validateCustomRules(rules)
	if _, ok := errs[2]; !ok {
		t.Errorf("expected error at index 2 for invalid rule, got %v", errs)
	}
	if _, ok := errs[0]; ok {
		t.Error("index 0 is a comment — should not have an error")
	}
	if _, ok := errs[1]; ok {
		t.Error("index 1 is valid — should not have an error")
	}
	if _, ok := errs[3]; ok {
		t.Error("index 3 is blank — should not have an error")
	}
	if _, ok := errs[4]; ok {
		t.Error("index 4 is valid — should not have an error")
	}
}

func TestDaemonDispatch_ValidateCustom_Valid(t *testing.T) {
	if !nftAvailable() {
		t.Skip("nft not available")
	}
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	payload, _ := json.Marshal(shared.ValidateCustomPayload{
		Rules: []string{"tcp dport 22 accept"},
	})
	resp := d.dispatch(shared.Command{Type: shared.CmdValidateCustom, Payload: payload})
	if !resp.Success {
		t.Fatalf("expected success: %s", resp.Error)
	}
	var result shared.ValidateCustomResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}
}

func TestDaemonDispatch_ValidateCustom_Invalid(t *testing.T) {
	if !nftAvailable() {
		t.Skip("nft not available")
	}
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	payload, _ := json.Marshal(shared.ValidateCustomPayload{
		Rules: []string{"this is definitely not valid nft syntax !!!"},
	})
	resp := d.dispatch(shared.Command{Type: shared.CmdValidateCustom, Payload: payload})
	if !resp.Success {
		t.Fatalf("dispatch itself should succeed: %s", resp.Error)
	}
	var result shared.ValidateCustomResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one validation error")
	}
}

func TestDaemonDispatch_ValidateCustom_InvalidPayload(t *testing.T) {
	cfg := newTestConfig(t)
	fw := newTestFirewall(t, cfg)
	d := &Daemon{cfg: cfg, firewall: fw, quit: make(chan struct{})}

	resp := d.dispatch(shared.Command{Type: shared.CmdValidateCustom, Payload: []byte(`{not json`)})
	if resp.Success {
		t.Error("expected failure for invalid JSON payload")
	}
}
