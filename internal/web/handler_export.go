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
	if _, writeErr := w.Write(data); writeErr != nil {
		slog.Warn("write export response", "error", writeErr)
	}
}

// maxImportBytes is the upload ceiling for a rules file. Every other form on
// the site is a handful of fields and lives under the global 64 KB cap; a rule
// set is the one thing an operator legitimately uploads by the hundred kilobyte.
const maxImportBytes = 512 * 1024

// handleImport reads an uploaded JSON file and passes it to the core for validation and import.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	// The body limit is set by the MaxBodySize middleware, which knows this
	// route needs more room than the rest of the site. Setting another
	// MaxBytesReader here would not widen anything — wrapping an already
	// limited reader leaves the inner, smaller limit in force, which is exactly
	// how a documented 512 KB ceiling silently behaved as 64 KB.
	file, _, err := r.FormFile("rules_file")
	if err != nil {
		// MaxBytesReader surfaces as a read error from the multipart parser, so
		// "no file" and "file too big" arrive at the same place. Telling the
		// operator to upload a file they did upload is the wrong answer.
		if isBodyTooLarge(err) {
			s.setFlash(w, r, "import_too_large")
		} else {
			s.setFlash(w, r, "import_no_file")
		}
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	defer file.Close() //nolint:errcheck // multipart file close errors are irrelevant after full read

	data, err := io.ReadAll(file)
	if err != nil {
		key := "import_read_error"
		if isBodyTooLarge(err) {
			key = "import_too_large"
		}
		s.setFlash(w, r, key)
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
