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
	// corrupt records that the file is *there* and could not be read or
	// parsed — which is not the same fact as "no passkeys enrolled", and the
	// difference is a whole login. See newPasskeyStore and factorCount.
	corrupt bool
}

// newPasskeyStore reads whatever is on disk, and distinguishes two states that
// leave an identical empty map behind them:
//
//   - The file is absent. That is every installation that has never enrolled a
//     passkey, and it means what it says: no passkeys. Nothing is flagged and
//     nothing about the login changes.
//   - The file is present and will not read or parse. The enrolled set is
//     *unknown*, and corrupt is set, for which factorCount counts one factor.
//
// The old rule — both states mean "no passkeys enrolled", the direction
// newTOTPReplay takes — was half right. It never failed a login that would
// otherwise have succeeded; it *passed* one that should have required a
// factor. For an account whose only factor is a passkey, factorCount fell to
// zero and handleLoginPOST granted a full session on the password alone, so a
// host restored without /var/lib/easywall, a moved data_dir or a truncated
// backup removed 2.18's whole mandate with one warning and no refusal.
//
// Counting it is not a lockout. It sends the login to the second step, where
// the TOTP secret and the eight recovery codes — both in web.toml, neither
// touched by whatever happened to this file — still open the door; every
// account gets eight codes at its first factor, passkey or not. An operator
// with neither has the remedy the release already documents, which names this
// very file: delete passkeys.json and restart, and the password alone signs in
// again.
func newPasskeyStore(path string) *passkeyStore {
	p := &passkeyStore{path: path, passkeys: map[string]storedPasskey{}}

	data, err := os.ReadFile(path) // #nosec G304 -- built from data_dir in the process's own config
	if err != nil {
		if !os.IsNotExist(err) {
			p.corrupt = true
			slog.Warn("the passkey store is there but cannot be read, so whether a passkey is "+
				"enrolled is unknown; the login will ask for a second factor — a TOTP code or a "+
				"recovery code — until it can be read again", "path", path, "error", err)
		}
		return p
	}
	var f passkeyStoreFile
	if err := json.Unmarshal(data, &f); err != nil {
		p.corrupt = true
		slog.Warn("the passkey store does not parse, so whether a passkey is enrolled is unknown; "+
			"the login will ask for a second factor — a TOTP code or a recovery code — until the "+
			"file is repaired or deleted", "path", path, "error", err)
		return p
	}
	for _, pk := range f.Passkeys {
		p.passkeys[string(pk.ID)] = pk
	}
	return p
}

// isCorrupt reports that the file exists and its contents are unknown, so the
// enrolled set cannot be trusted to be empty. factorCount is the only caller
// that matters; everything else about the store is honestly empty.
func (p *passkeyStore) isCorrupt() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.corrupt
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

	// AddedAt breaks ties by credential ID: two passkeys enrolled in the same
	// instant would otherwise leave the comparator returning false both ways,
	// sort.Slice would treat them as already in order, and the result would
	// fall through to map iteration order — randomized per range, and this is
	// the operator's list of enrolled passkeys, not an internal detail.
	sort.Slice(out, func(i, j int) bool {
		if !out[i].AddedAt.Equal(out[j].AddedAt) {
			return out[i].AddedAt.Before(out[j].AddedAt)
		}
		return bytes.Compare(out[i].ID, out[j].ID) < 0
	})
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
	// A file that did not parse is moved aside, never written over. Until this
	// it was overwritten by the next save — an enrolment, typically — and
	// whatever credentials the unreadable file held were gone for good, with
	// nothing left to repair by hand. Renaming costs one inode and keeps the
	// evidence; the flag clears with it, because once the bad file is out of
	// the way the enrolled set is known again and the phantom factor
	// factorCount counted must not outlive it.
	if p.corrupt {
		if err := os.Rename(p.path, p.path+".corrupt"); err != nil && !os.IsNotExist(err) {
			slog.Error("could not move the unreadable passkey store aside, so it will not be "+
				"written over either", "path", p.path, "error", err)
			return err
		}
		p.corrupt = false
	}

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
