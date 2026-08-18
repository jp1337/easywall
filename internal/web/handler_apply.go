package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jp1337/easywall/internal/shared"
)

type applyData struct {
	Status  *shared.FirewallStatus
	CoreErr string
}

func (s *Server) handleApplyGET(w http.ResponseWriter, r *http.Request) {
	status, err := s.client.GetStatus()
	if err != nil {
		slog.Warn("get status error", "error", err)
		s.render(w, r, "apply.html", "apply", &applyData{CoreErr: err.Error()})
		return
	}
	s.render(w, r, "apply.html", "apply", &applyData{Status: status})
}

// handleApplyStart triggers an asynchronous rule application on the core.
// The core applies rules and enters the acceptance window.
func (s *Server) handleApplyStart(w http.ResponseWriter, r *http.Request) {
	if err := s.client.ApplyRules(); err != nil {
		slog.Warn("apply rules error", "error", err)
		// The core refuses a second apply while a window is open, and that is
		// not a failure to report as one: it is the safety mechanism doing its
		// job, and the operator's next move is to confirm the apply they already
		// started. The page hides the Start button in that state, so getting
		// here means a second tab, a double submit, or the back button.
		flash := "apply_error"
		switch {
		case strings.Contains(err.Error(), shared.ErrApplyInProgressText):
			flash = "apply_already_running"
		// A human took the firewall down at the console. The web interface may
		// not be the thing that re-arms it — that refusal is core.ErrPanicEngaged,
		// not a bug — so the operator needs to be told why, not handed the same
		// text a broken socket would produce.
		case strings.Contains(err.Error(), shared.ErrPanicEngagedText):
			flash = "apply_panic_engaged"
		}
		s.setFlash(w, r, flash)
		http.Redirect(w, r, "/apply", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/apply", http.StatusSeeOther)
}

// handleApplyConfirm sends the acceptance signal to the core.
// Must be called while the acceptance window is pending.
func (s *Server) handleApplyConfirm(w http.ResponseWriter, r *http.Request) {
	accepted, err := s.client.Accept()
	if err != nil {
		slog.Warn("accept error", "error", err)
		s.setFlash(w, r, "accept_error")
		http.Redirect(w, r, "/apply", http.StatusSeeOther)
		return
	}

	// A confirmation that arrives after the window closed changes nothing: the
	// rules were rolled back when it expired. Saying "accepted and applied
	// successfully" here told the operator their change was live at the one
	// moment it was not, and sent them to the dashboard to admire it.
	if !accepted {
		slog.Info("confirmation arrived after the acceptance window closed")
		s.setFlash(w, r, "accept_too_late")
		http.Redirect(w, r, "/apply", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "rules_accepted")
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// handleApplyStatus returns the current acceptance status as JSON for HTMX polling.
func (s *Server) handleApplyStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.client.GetStatus()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		if encErr := json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}); encErr != nil {
			slog.Warn("encode status error response", "error", encErr)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if encErr := json.NewEncoder(w).Encode(status); encErr != nil {
		slog.Warn("encode status response", "error", encErr)
	}
}
