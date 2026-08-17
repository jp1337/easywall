package web

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gorilla/sessions"
	"golang.org/x/crypto/argon2"
)

const (
	SessionName     = "easywall"
	SessionLifetime = 600 // seconds
	SessionUserKey  = "user"

	// SessionCredentialKey holds a fingerprint of the password hash that was in
	// force when the session was created.
	SessionCredentialKey = "cred"

	// maxHashPart bounds the salt and the key read out of a stored hash. Both are
	// tens of bytes in anything easywall writes.
	maxHashPart = 1024

	// minPasswordLen is the shortest password accepted, by the first-run wizard
	// and by the change-password page. It was written as a bare 12 in both,
	// which is one place too many for a rule the interface also states in words.
	minPasswordLen = 12
)

// newSessionStore builds the cookie store every session is signed with.
//
// It exists so there is one place that gets the lifetime right, and because
// getting it right is not obvious. A session cookie carries a timestamp inside
// the signed value, and the server refuses one older than the codec's max age.
// That max age is set by NewCookieStore from its own default — thirty days —
// and assigning a fresh Options struct afterwards does not change it. Every
// caller here did exactly that, so the cookie the browser dropped after ten
// minutes stayed valid to the server for thirty days.
//
// It made logging out temporary rather than permanent. A logged-out session is
// remembered as revoked for one SessionLifetime and then forgotten, on the
// stated ground that the cookie expires on its own by then; it did not, so
// replaying the same cookie eleven minutes after logging out signed you back
// in. Measured before this change: /dashboard answered 200 with a cookie 29
// days old, and 200 again after a logout had been garbage-collected.
//
// store.MaxAge sets both halves — the Max-Age the browser sees and the age the
// server enforces — so use it rather than writing Options.MaxAge.
func newSessionStore(key string) *sessions.CookieStore {
	store := sessions.NewCookieStore([]byte(key))
	store.Options = &sessions.Options{
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	store.MaxAge(SessionLifetime)
	return store
}

// credentialFingerprint derives a short, non-reversible marker from the stored
// password hash.
//
// Sessions live in a signed cookie, so there is no server-side list to clear:
// changing the password left every existing session working until it timed out.
// Comparing this marker on each request ends them instead, which is what an
// operator changing the password because they suspect someone else is signed in
// is actually asking for. It is derived from the hash, never from the password,
// and it is only ever compared with itself.
//
// CodeQL reports this as password hashing with a weak algorithm
// (go/weak-sensitive-data-hashing, alert 197) and it is dismissed as a false
// positive. What goes in is an argon2id digest — 16 random salt bytes and a
// 32-byte key, already slow-hashed — not a password; what comes out is a marker
// compared on every request, which is precisely where the remedy the query asks
// for, an expensive KDF, cannot go. Read that before "fixing" it.
func credentialFingerprint(passwordHash string) string {
	sum := sha256.Sum256([]byte("easywall-session-v1:" + passwordHash))
	return base64.RawStdEncoding.EncodeToString(sum[:16])
}

type argon2Params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

var defaultArgon2Params = argon2Params{
	memory:      64 * 1024,
	iterations:  3,
	parallelism: 4,
	saltLength:  16,
	keyLength:   32,
}

// HashPassword returns an argon2id encoded hash of the given password.
func HashPassword(password string) (string, error) {
	p := defaultArgon2Params
	salt := make([]byte, p.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, p.keyLength)

	b64salt := base64.RawStdEncoding.EncodeToString(salt)
	b64hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.iterations, p.parallelism, b64salt, b64hash), nil
}

// Why a stored hash cannot be used, in words that carry nothing from the value
// itself.
//
// A fixed set rather than an error wrapped around the parser's, and that is the
// point: this text is written to the journal, the value it describes is the
// credential, and an error built by wrapping a parser can carry pieces of its
// input along with it — `fmt.Errorf("decode salt: %w", err)` is one library
// change away from printing what it failed to decode. Constants cannot.
var (
	errHashFormat      = errors.New("it is not the $argon2id$… form easywall writes")
	errHashVersion     = errors.New("it names an argon2 version this build cannot read")
	errHashParams      = errors.New("its cost parameters cannot be read")
	errHashSaltEncode  = errors.New("its salt is not valid base64")
	errHashKeyEncode   = errors.New("its key is not valid base64")
	errHashEmptySalt   = errors.New("it has an empty salt")
	errHashEmptyKey    = errors.New("it has an empty key; the value looks truncated")
	errHashParallelism = errors.New("its parallelism is below 1")
	errHashIterations  = errors.New("its iteration count is below 1")
	errHashMemory      = errors.New("its memory cost is below argon2's minimum for that parallelism")
	errHashOversized   = errors.New("its salt or key is longer than any hash easywall writes")
)

// VerifyPassword returns true if password matches the argon2id encoded hash.
//
// A stored hash that cannot be used is reported, not just refused. This is the
// one place that discovers it, and the operator needs to be told: docs/_docs/security.md
// offers no password recovery beyond "editing web.toml on the host", so a hash
// that got mangled on the way into that file is reached by someone who is already
// locked out and has no other way in. The log line is what turns a login that
// will never work into something they can fix.
//
// Logging here is safe from the outside: the hash comes from the config file, so
// a wrong password never reaches this branch and nobody can make it noisy. And
// safe on the inside because the reason is one of the constants above — the
// stored credential does not reach the journal even in pieces.
func VerifyPassword(password, encodedHash string) bool {
	p, salt, hash, err := decodeArgon2Hash(encodedHash)
	if err != nil {
		slog.Error("the stored password hash cannot be used, so no password can match it; "+
			"replace the password line in web.toml with a hash easywall produced, or clear it "+
			"to reopen the first-run wizard", "reason", err.Error())
		return false
	}
	other := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, p.keyLength)
	return subtle.ConstantTimeCompare(hash, other) == 1
}

// decodeArgon2Hash parses a $argon2id$... encoded hash.
func decodeArgon2Hash(encoded string) (argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// expected: ["", "argon2id", "v=19", "m=65536,t=3,p=4", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argon2Params{}, nil, nil, errHashFormat
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return argon2Params{}, nil, nil, errHashVersion
	}

	var p argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.iterations, &p.parallelism); err != nil {
		return argon2Params{}, nil, nil, errHashParams
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2Params{}, nil, nil, errHashSaltEncode
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2Params{}, nil, nil, errHashKeyEncode
	}

	// Bounded before the conversion, not after: these two widths are handed
	// straight to argon2.IDKey, which allocates from them. easywall writes a
	// 16-byte salt and a 32-byte key; anything past a kilobyte is not a hash this
	// program produced, and there is no reason to let a config file ask for an
	// arbitrary allocation on every login attempt.
	if len(salt) > maxHashPart || len(hash) > maxHashPart {
		return argon2Params{}, nil, nil, errHashOversized
	}
	// #nosec G115 -- both lengths are bounded by maxHashPart immediately above.
	p.saltLength = uint32(len(salt))
	// #nosec G115 -- likewise.
	p.keyLength = uint32(len(hash))

	// The parameters were parsed but never checked for being usable, and
	// argon2.IDKey does not tolerate degenerate ones — it panics. Measured against
	// the running server with the trailing segment of the hash missing, which is
	// what a hash truncated while being pasted in looks like:
	//
	//	POST /login -> HTTP 500
	//	panic: runtime error: invalid memory address or nil pointer dereference
	//	  golang.org/x/crypto/blake2b.(*digest).Write
	//
	// p=0 and t=0 panic with their own messages. middleware.Recoverer turns each
	// into a 500 and a stack trace in the journal, so the only documented way back
	// into a locked-out installation answers with neither a login nor a reason.
	// Refused with a description instead, which VerifyPassword logs.
	switch {
	case len(salt) == 0:
		return argon2Params{}, nil, nil, errHashEmptySalt
	case len(hash) == 0:
		return argon2Params{}, nil, nil, errHashEmptyKey
	case p.parallelism < 1:
		return argon2Params{}, nil, nil, errHashParallelism
	case p.iterations < 1:
		return argon2Params{}, nil, nil, errHashIterations
	case p.memory < 8*uint32(p.parallelism):
		// argon2's own lower bound: fewer blocks than lanes has no meaning.
		return argon2Params{}, nil, nil, errHashMemory
	}

	return p, salt, hash, nil
}
