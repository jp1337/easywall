package web

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// TestThePasskeyStoreSurvivesARestart — the counter and the credentials are on
// disk, because a passkey that is forgotten on restart is not a second factor.
func TestThePasskeyStoreSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "passkeys.json")

	p := newPasskeyStore(path)
	if err := p.add("YubiKey on the keyring", webauthn.Credential{ID: []byte("cred-1")}); err != nil {
		t.Fatalf("add: %v", err)
	}

	again := newPasskeyStore(path)
	got := again.all()
	if len(got) != 1 {
		t.Fatalf("after a restart the store holds %d passkeys, want 1", len(got))
	}
	if got[0].Name != "YubiKey on the keyring" {
		t.Errorf("the name did not survive: %q", got[0].Name)
	}
	if got[0].AddedAt.IsZero() {
		t.Error("no enrolment date was recorded; an operator with three passkeys cannot tell them apart")
	}
}

// TestACorruptStoreIsEmptyAndNotAnError is half of totpreplay.go's rule: a file
// that will not parse is not an error and not a lockout, and the store it
// leaves behind offers no passkey.
//
// Only half, because "offers no passkey" is not "the account has no second
// factor" — that conflation signed a passkey-only account in on its password
// alone. What the login does about it is passkeystore_corrupt_test.go.
func TestACorruptStoreIsEmptyAndNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "passkeys.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	p := newPasskeyStore(path)
	if n := len(p.all()); n != 0 {
		t.Errorf("a corrupt store reported %d passkeys", n)
	}
}

// TestTheFingerprintChangesWhenAPasskeyIsRemoved is what makes removing a lost
// device end every session it could still be used in.
func TestTheFingerprintChangesWhenAPasskeyIsRemoved(t *testing.T) {
	p := newPasskeyStore(filepath.Join(t.TempDir(), "passkeys.json"))
	_ = p.add("one", webauthn.Credential{ID: []byte("cred-1")})
	_ = p.add("two", webauthn.Credential{ID: []byte("cred-2")})

	before := p.fingerprintInput()
	if err := p.remove([]byte("cred-1")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if p.fingerprintInput() == before {
		t.Fatal("removing a passkey did not change the fingerprint input; " +
			"sessions opened with the lost device would keep working")
	}
}

// TestTheFingerprintIsStableAcrossReads — it must not change on its own, or
// every request would invalidate its own session.
func TestTheFingerprintIsStableAcrossReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "passkeys.json")
	p := newPasskeyStore(path)
	_ = p.add("b", webauthn.Credential{ID: []byte("cred-b")})
	_ = p.add("a", webauthn.Credential{ID: []byte("cred-a")})

	first := p.fingerprintInput()
	for i := 0; i < 5; i++ {
		if got := newPasskeyStore(path).fingerprintInput(); got != first {
			t.Fatalf("read %d gave a different fingerprint input; the set is not being ordered", i)
		}
	}
}

// TestAllIsStableWhenAddedAtTies guards against relying on Go's randomized map
// iteration order. all() is what Task 12 renders as the operator's list of
// enrolled passkeys, and a real (if narrow) window exists for two entries to
// share an identical AddedAt — the ordering must not fall back to map
// iteration the moment its primary sort key stops discriminating.
func TestAllIsStableWhenAddedAtTies(t *testing.T) {
	p := newPasskeyStore(filepath.Join(t.TempDir(), "passkeys.json"))

	same := time.Now()
	p.mu.Lock()
	for i := 0; i < 8; i++ {
		id := []byte{byte(i)}
		p.passkeys[string(id)] = storedPasskey{ID: id, Name: fmt.Sprintf("key-%d", i), AddedAt: same}
	}
	p.mu.Unlock()

	first := p.all()
	for i := 0; i < 30; i++ {
		got := p.all()
		if len(got) != len(first) {
			t.Fatalf("call %d: got %d entries, want %d", i, len(got), len(first))
		}
		for j := range got {
			if !bytes.Equal(got[j].ID, first[j].ID) {
				t.Fatalf("call %d gave a different order than call 0 at position %d; "+
					"all() is not stable when AddedAt ties", i, j)
			}
		}
	}
}

// TestTheFingerprintDoesNotChangeWithTheSignatureCounter — an authenticator
// increments its counter on every assertion; a fingerprint that moved with it
// would invalidate the session the assertion had just opened.
func TestTheFingerprintDoesNotChangeWithTheSignatureCounter(t *testing.T) {
	p := newPasskeyStore(filepath.Join(t.TempDir(), "passkeys.json"))
	_ = p.add("one", webauthn.Credential{ID: []byte("cred-1")})

	before := p.fingerprintInput()
	if err := p.updateCounter([]byte("cred-1"), webauthn.Authenticator{SignCount: 42}); err != nil {
		t.Fatalf("updateCounter: %v", err)
	}
	if got := p.fingerprintInput(); got != before {
		t.Fatal("the fingerprint changed when only the signature counter did")
	}
}

// TestACredentialRoundTripsFieldByField guards against silent data loss from
// an unexported field: encoding/json drops what it cannot see, and a passkey
// that lost a field to a restart would look enrolled while failing every
// assertion.
func TestACredentialRoundTripsFieldByField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "passkeys.json")
	want := webauthn.Credential{
		ID:                []byte{0x01, 0x02, 0x03, 0xff},
		PublicKey:         []byte{0xaa, 0xbb, 0xcc},
		AttestationType:   "basic_full",
		AttestationFormat: "packed",
		Transport:         []protocol.AuthenticatorTransport{protocol.USB, protocol.Internal},
		Flags: webauthn.CredentialFlags{
			UserPresent:    true,
			UserVerified:   true,
			BackupEligible: true,
			BackupState:    false,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:       []byte{0x10, 0x20},
			SignCount:    7,
			CloneWarning: false,
			Attachment:   protocol.Platform,
		},
		Attestation: webauthn.CredentialAttestation{
			ClientDataJSON:     []byte(`{"type":"webauthn.create"}`),
			ClientDataHash:     []byte{0xde, 0xad},
			AuthenticatorData:  []byte{0xbe, 0xef},
			PublicKeyAlgorithm: -7,
			Object:             []byte{0x01},
		},
	}

	p := newPasskeyStore(path)
	if err := p.add("round-trip", want); err != nil {
		t.Fatalf("add: %v", err)
	}

	again := newPasskeyStore(path)
	got := again.all()
	if len(got) != 1 {
		t.Fatalf("got %d passkeys, want 1", len(got))
	}
	gotCred := got[0].Credential

	if string(gotCred.ID) != string(want.ID) {
		t.Errorf("ID = %v, want %v", gotCred.ID, want.ID)
	}
	if string(gotCred.PublicKey) != string(want.PublicKey) {
		t.Errorf("PublicKey = %v, want %v", gotCred.PublicKey, want.PublicKey)
	}
	if gotCred.AttestationType != want.AttestationType {
		t.Errorf("AttestationType = %q, want %q", gotCred.AttestationType, want.AttestationType)
	}
	if gotCred.AttestationFormat != want.AttestationFormat {
		t.Errorf("AttestationFormat = %q, want %q", gotCred.AttestationFormat, want.AttestationFormat)
	}
	if len(gotCred.Transport) != 2 || gotCred.Transport[0] != protocol.USB || gotCred.Transport[1] != protocol.Internal {
		t.Errorf("Transport = %v, want [usb internal]", gotCred.Transport)
	}
	if gotCred.Flags != want.Flags {
		// CredentialFlags carries an unexported `raw` field that does not
		// round-trip through JSON — see the package comment. Compare the
		// four exported bits explicitly rather than the whole struct.
		if gotCred.Flags.UserPresent != want.Flags.UserPresent ||
			gotCred.Flags.UserVerified != want.Flags.UserVerified ||
			gotCred.Flags.BackupEligible != want.Flags.BackupEligible ||
			gotCred.Flags.BackupState != want.Flags.BackupState {
			t.Errorf("Flags = %+v, want %+v (exported bits)", gotCred.Flags, want.Flags)
		}
	}
	if gotCred.Authenticator.SignCount != want.Authenticator.SignCount {
		t.Errorf("SignCount = %d, want %d", gotCred.Authenticator.SignCount, want.Authenticator.SignCount)
	}
	if string(gotCred.Authenticator.AAGUID) != string(want.Authenticator.AAGUID) {
		t.Errorf("AAGUID = %v, want %v", gotCred.Authenticator.AAGUID, want.Authenticator.AAGUID)
	}
	if gotCred.Authenticator.Attachment != want.Authenticator.Attachment {
		t.Errorf("Attachment = %q, want %q", gotCred.Authenticator.Attachment, want.Authenticator.Attachment)
	}
	if string(gotCred.Attestation.ClientDataJSON) != string(want.Attestation.ClientDataJSON) {
		t.Errorf("ClientDataJSON = %s, want %s", gotCred.Attestation.ClientDataJSON, want.Attestation.ClientDataJSON)
	}
	if gotCred.Attestation.PublicKeyAlgorithm != want.Attestation.PublicKeyAlgorithm {
		t.Errorf("PublicKeyAlgorithm = %d, want %d", gotCred.Attestation.PublicKeyAlgorithm, want.Attestation.PublicKeyAlgorithm)
	}
	if string(gotCred.Attestation.Object) != string(want.Attestation.Object) {
		t.Errorf("Object = %v, want %v", gotCred.Attestation.Object, want.Attestation.Object)
	}
}
