package web

import (
	"encoding/json"
	"log/slog"
	"net/http"

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
		s.setFlash(w, r, "apply_error")
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
