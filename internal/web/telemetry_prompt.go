package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/jp1337/easywall/internal/shared"
)

// telemetryPrompt is what telemetry_prompt.json holds: the minor version whose
// question was last answered, and whether the operator asked never to be asked
// again (spec 2.28 D8). State, not configuration — web.toml is the operator's,
// often templated by Ansible, and "asked on 2.28" does not belong in it.
type telemetryPrompt struct {
	Asked string `json:"asked,omitempty"`
	Never bool   `json:"never,omitempty"`
}

// telemetryPromptMu serialises the read-modify-write of the file: two tabs
// answering at once must not lose a "never".
var telemetryPromptMu sync.Mutex

// TelemetryPromptPath returns the path for the after-an-update question's state.
func (c *Config) TelemetryPromptPath() string {
	if c.DataDir != "" {
		return c.StateDir() + "/telemetry_prompt.json"
	}
	return c.SSLDir + "/../telemetry_prompt.json"
}

// readTelemetryPrompt reads the file. Missing or unreadable reads as never
// asked: the worst a broken file costs is the question once more.
func readTelemetryPrompt(path string) telemetryPrompt {
	var p telemetryPrompt
	data, err := os.ReadFile(path)
	if err != nil {
		return p
	}
	if err := json.Unmarshal(data, &p); err != nil {
		slog.Warn("telemetry_prompt.json does not parse; the question will be asked again", "error", err)
		return telemetryPrompt{}
	}
	return p
}

// recordTelemetryAnswer marks the running minor as answered, and with never set
// also stops every later question. A failure is logged and otherwise ignored
// (D9): the consent is stored elsewhere and already saved, and a lost marker
// only means the question comes back.
func (s *Server) recordTelemetryAnswer(never bool) {
	minor, ok := shared.MinorVersion(shared.CurrentVersion)
	if !ok && !never {
		return
	}

	telemetryPromptMu.Lock()
	defer telemetryPromptMu.Unlock()

	path := s.cfg.TelemetryPromptPath()
	p := readTelemetryPrompt(path)
	if ok {
		p.Asked = minor
	}
	p.Never = p.Never || never
	data, err := json.Marshal(p)
	if err == nil {
		err = writeFileAtomic(path, data, 0600)
	}
	if err != nil {
		slog.Warn("could not record the answer to the telemetry question; it will be asked again", "error", err)
	}
}

// askTelemetry is D1: the dashboard asks only when counting is off, nobody
// decided elsewhere, and this minor version has no answer yet.
func (s *Server) askTelemetry() bool {
	if s.cfg.TelemetryEnabled() || s.client.IsDemo() {
		return false
	}
	// A set EASYWALL_WEB_TELEMETRY is the deployment's decision; an answer here
	// would be stored and beat it.
	if _, set := s.cfg.Provenance("telemetry"); set {
		return false
	}
	minor, ok := shared.MinorVersion(shared.CurrentVersion)
	if !ok {
		return false
	}
	telemetryPromptMu.Lock()
	p := readTelemetryPrompt(s.cfg.TelemetryPromptPath())
	telemetryPromptMu.Unlock()
	return !p.Never && p.Asked != minor
}

// handleTelemetryPromptPOST answers the dashboard's card: yes, no or never.
//
// Like /system/telemetry it writes web.toml and nothing else, so it works with
// the core gone.
func (s *Server) handleTelemetryPromptPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// A refusal must leave the card where it is: the card's outerHTML swap
	// would otherwise replace it with respondPartialError's empty 200, and an
	// operator who clicked Yes would see it gone as if it had worked.
	keepCard := func(flash string) {
		w.Header().Set("HX-Reswap", "none")
		s.respondPartialError(w, r, "/", flash)
	}
	if s.client.IsDemo() {
		keepCard("demo_readonly")
		return
	}

	var flash string
	switch r.FormValue("answer") {
	case "yes", "no":
		enabled := r.FormValue("answer") == "yes"
		// The consent first: a failure here must not leave a marker saying the
		// question was answered (D9).
		if err := s.cfg.SaveTelemetry(enabled); err != nil {
			slog.Warn("could not save the telemetry setting", "error", err)
			keepCard("save_error")
			return
		}
		slog.Info("telemetry setting changed", "enabled", enabled, "via", "dashboard question")
		s.recordTelemetryAnswer(false)
		flash = "telemetry_prompt_saved_" + r.FormValue("answer")
	case "never":
		s.recordTelemetryAnswer(true)
		flash = "telemetry_prompt_saved_never"
	default:
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if !isHTMX(r) {
		s.setFlash(w, r, flash)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	// 200 with an empty body, not respondPartialSave's 204: htmx swaps nothing
	// on a 204, and the card's outerHTML swap with nothing is what removes it.
	w.Header().Set("HX-Trigger", `{"easywall:saved":"`+flash+`"}`)
	w.WriteHeader(http.StatusOK)
}
