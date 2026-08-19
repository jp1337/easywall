package web

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
)

// totpReplay remembers the last TOTP step that was accepted, so a code cannot be
// used twice inside its own validity window.
//
// It lives in data_dir rather than in web.toml, and that split is the whole
// decision: the secret and the recovery codes change eight times in the life of
// an enrolment and belong in the credential file, but this changes once per
// login. web.toml is rewritten in place on a directory the packaged layout does
// not let the web user create temp files in — that is not a file to rewrite on
// every sign-in.
type totpReplay struct {
	path string

	mu   sync.Mutex
	step uint64
}

type totpReplayFile struct {
	Step uint64 `json:"step"`
}

// newTOTPReplay reads whatever is on disk. A missing or unreadable file means
// "nothing accepted yet", which is the safe direction: the worst it does is
// allow one code that a previous process had already seen.
func newTOTPReplay(path string) *totpReplay {
	r := &totpReplay{path: path}

	data, err := os.ReadFile(path) // #nosec G304 -- built from data_dir in the process's own config
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("could not read the TOTP replay store; a code accepted by a previous "+
				"process may be accepted once more", "path", path, "error", err)
		}
		return r
	}
	var f totpReplayFile
	if err := json.Unmarshal(data, &f); err != nil {
		slog.Warn("the TOTP replay store does not parse and is being ignored",
			"path", path, "error", err)
		return r
	}
	r.step = f.Step
	return r
}

// last returns the last accepted step.
func (r *totpReplay) last() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.step
}

// accept records a step and reports whether it was new.
//
// A write that fails is logged and the login proceeds. The replay guard narrows
// a thirty-second window; refusing to sign anybody in because the disk is full
// would be the same trade 2.7 refused when it decided that a restore which
// cannot be carried out must not become a lockout.
func (r *totpReplay) accept(step uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if step <= r.step {
		return false
	}
	r.step = step

	data, err := json.Marshal(totpReplayFile{Step: step})
	if err != nil {
		slog.Error("could not encode the TOTP replay store", "error", err)
		return true
	}
	if err := writeFileAtomic(r.path, data, 0600); err != nil {
		slog.Warn("could not persist the last accepted TOTP step; a code used now may be "+
			"accepted again after a restart", "path", r.path, "error", err)
	}
	return true
}
