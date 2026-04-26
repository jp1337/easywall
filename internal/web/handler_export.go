package web

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// handleExport streams the current rule set as a downloadable JSON file.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	data, err := s.client.ExportRules()
	if err != nil {
		slog.Warn("export rules error", "error", err)
		s.setFlash(w, r, "export_error")
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	filename := fmt.Sprintf("easywall-rules-%s.json", time.Now().UTC().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// handleImport reads an uploaded JSON file and passes it to the core for validation and import.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	// 512 KB max upload size for rule files
	r.Body = http.MaxBytesReader(w, r.Body, 512*1024)

	file, _, err := r.FormFile("rules_file")
	if err != nil {
		s.setFlash(w, r, "import_no_file")
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		s.setFlash(w, r, "import_read_error")
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	if err := s.client.ImportRules(data); err != nil {
		slog.Warn("import rules error", "error", err)
		s.setFlash(w, r, "import_error")
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "import_success")
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}
