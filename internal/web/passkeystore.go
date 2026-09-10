package web

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// Where enrolled passkeys live.
//
// Under data_dir rather than in web.toml, unlike the TOTP secret, and the
// reason is the signature counter: an authenticator increments it on every
// assertion and the server stores the new value, so this file is written on
// every passkey login. Rewriting the operator's configuration file on every
// login is the wrong shape — a config file is something a human edits, and a
// program that rewrites it while they have it open in an editor will lose
// somebody's work.
//
// totpreplay.go is the model, down to the error handling: it is also written
// on every login, it also treats a missing or unparseable file as empty with a
// warning rather than as an error, and it does so because a file that will not
// parse must never be the reason an operator cannot get into their firewall.
type storedPasskey struct {
	// ID is the credential ID, and the handle for removal.
	ID []byte `json:"id"`

	// Name is what the operator called it. One passkey is a device; three are
	// three devices, and "the one I lost" has to be findable.
	Name string `json:"name"`

	AddedAt time.Time `json:"added_at"`

	Credential webauthn.Credential `json:"credential"`
}

type passkeyStoreFile struct {
	Passkeys []storedPasskey `json:"passkeys"`
}

// passkeyStore holds every enrolled passkey, on disk and in memory.
//
// Keyed by credential ID (as a string, so it can be a map key) rather than
// held as a slice: add, remove and updateCounter are all keyed lookups, and a
// map makes each O(1) instead of a scan. The consequence is that Go's map
// iteration is randomized per run, which is exactly why all() and
// fingerprintInput() below each impose their own explicit order rather than
// trusting range — an implementation that forgot to would still look correct
// in a single-entry test and misbehave the first time somebody enrolled two
// passkeys.
type passkeyStore struct {
	path string

	mu       sync.Mutex
	passkeys map[string]storedPasskey
}

// newPasskeyStore reads whatever is on disk. A missing or unparseable file
// means "no passkeys enrolled" rather than an error — the same direction
// newTOTPReplay takes, and for the same reason: a file that will not parse
// must never be the reason an operator cannot get into their firewall. The
// worst this does is fail to offer a passkey that was in fact enrolled; it
// never fails a login that would otherwise have succeeded.
func newPasskeyStore(path string) *passkeyStore {
	p := &passkeyStore{path: path, passkeys: map[string]storedPasskey{}}

	data, err := os.ReadFile(path) // #nosec G304 -- built from data_dir in the process's own config
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("could not read the passkey store; enrolled passkeys will not be offered "+
				"until it can be read again", "path", path, "error", err)
		}
		return p
	}
	var f passkeyStoreFile
	if err := json.Unmarshal(data, &f); err != nil {
		slog.Warn("the passkey store does not parse and is being ignored; enrolled passkeys will "+
			"not be offered until it is repaired", "path", path, "error", err)
		return p
	}
	for _, pk := range f.Passkeys {
		p.passkeys[string(pk.ID)] = pk
	}
	return p
}

// all returns every enrolled passkey, oldest enrolment first. The caller gets
// its own slice: nothing here hands out a reference into the store's own
// state, and the order is imposed explicitly rather than left to map
// iteration, which is randomized per run.
func (p *passkeyStore) all() []storedPasskey {
	p.mu.Lock()
	out := make([]storedPasskey, 0, len(p.passkeys))
	for _, pk := range p.passkeys {
		out = append(out, pk)
	}
	p.mu.Unlock()

	sort.Slice(out, func(i, j int) bool { return out[i].AddedAt.Before(out[j].AddedAt) })
	return out
}

// add enrolls a new passkey and persists the store.
//
// Unlike updateCounter and totpReplay.accept, a write failure here is
// returned rather than swallowed: this runs once, in response to an operator
// action, and an enrolment that silently does not survive a restart is worse
// than one the operator is told to retry.
func (p *passkeyStore) add(name string, c webauthn.Credential) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.passkeys[string(c.ID)] = storedPasskey{
		ID:         c.ID,
		Name:       name,
		AddedAt:    time.Now(),
		Credential: c,
	}
	return p.saveLocked()
}

// remove deletes a passkey by credential ID. Removing an ID that is not
// present is not an error — the end state the caller wants is already true.
func (p *passkeyStore) remove(id []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.passkeys, string(id))
	return p.saveLocked()
}

// updateCounter records the authenticator state — chiefly the signature
// counter — after a successful login, and persists it.
//
// Like totpReplay.accept, a write failure here is logged and swallowed rather
// than returned as fatal to the caller: the assertion that got here already
// succeeded, and the counter update is bookkeeping for the *next* login's
// clone detection, not a condition of this one. A disk-full server must not
// turn a successful passkey login into a failed one.
func (p *passkeyStore) updateCounter(id []byte, authenticator webauthn.Authenticator) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := string(id)
	pk, ok := p.passkeys[key]
	if !ok {
		return nil
	}
	pk.Credential.Authenticator = authenticator
	p.passkeys[key] = pk
	if err := p.saveLocked(); err != nil {
		slog.Warn("could not persist the updated passkey signature counter; a clone "+
			"may go undetected until the next successful write", "path", p.path, "error", err)
	}
	return nil
}

// fingerprintInput is a stable representation of the enrolled passkey set, for
// folding into credentialFingerprint (see auth.go) alongside the password hash
// and the TOTP secret.
//
// It must be stable in both directions that matter here:
//   - Adding or removing a passkey must change it, so every session opened
//     with a passkey that is now gone ends at that moment, exactly as a
//     password or TOTP change already does.
//   - Nothing else may change it. In particular it carries no signature
//     counter: an authenticator increments that on every assertion, and a
//     fingerprint that moved with it would invalidate the very session the
//     assertion had just opened.
//
// The credential IDs are sorted before joining because passkeys is a map: Go
// randomizes map iteration order per run, and without the sort this would be
// a different fingerprint every time the process restarted, for the exact
// same set of enrolled passkeys.
func (p *passkeyStore) fingerprintInput() string {
	p.mu.Lock()
	ids := make([][]byte, 0, len(p.passkeys))
	for _, pk := range p.passkeys {
		ids = append(ids, pk.ID)
	}
	p.mu.Unlock()

	sort.Slice(ids, func(i, j int) bool { return bytes.Compare(ids[i], ids[j]) < 0 })

	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = hex.EncodeToString(id)
	}
	// NUL-separated, matching credentialFingerprint's own domain separation
	// between fields — a byte that cannot appear in a hex string, so no join
	// of distinct ID sets can collide.
	return strings.Join(parts, "\x00")
}

// saveLocked writes the store atomically, as a JSON array sorted by credential
// ID so the file diffs sanely across saves instead of shuffling with map
// iteration order. Called with mu held.
func (p *passkeyStore) saveLocked() error {
	out := make([]storedPasskey, 0, len(p.passkeys))
	for _, pk := range p.passkeys {
		out = append(out, pk)
	}
	sort.Slice(out, func(i, j int) bool { return bytes.Compare(out[i].ID, out[j].ID) < 0 })

	data, err := json.Marshal(passkeyStoreFile{Passkeys: out})
	if err != nil {
		slog.Error("could not encode the passkey store", "error", err)
		return err
	}
	// The package's own atomic writer, from tlscert.go. See totpReplay.accept
	// for why no in-place fallback is needed here either: data_dir is
	// installed 0770 root:easywall with the web user in that group, so a temp
	// file always works.
	if err := writeFileAtomic(p.path, data, 0600); err != nil {
		return err
	}
	return nil
}
