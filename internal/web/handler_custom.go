package web

import (
	"log/slog"
	"net/http"
	"sort"
)

type customData struct {
	Rules []string
	// Validation is nil until a save was rejected. It is the same shape the HTMX
	// endpoint returns, so both paths render through one template: they used to be
	// two separate renderers, and only one of them was kept up to date.
	Validation *validationData
}

// customValidation turns the core's line→message map into the shape the shared
// validation partial renders.
func customValidation(errs map[int]string) *validationData {
	lines := make([]int, 0, len(errs))
	for k := range errs {
		lines = append(lines, k)
	}
	sort.Ints(lines)

	data := &validationData{
		TitleKey: "custom_errors_title",
		OKKey:    "validate_custom_ok",
		Mono:     true,
	}
	for _, idx := range lines {
		data.Errors = append(data.Errors, lineError{Line: idx + 1, Detail: errs[idx]})
	}
	return data
}

func (s *Server) handleCustomGET(w http.ResponseWriter, r *http.Request) {
	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("get rules error", "error", err)
		s.render(w, r, "custom.html", "custom", &customData{})
		return
	}
	s.render(w, r, "custom.html", "custom", &customData{Rules: state.Staged.Custom})
}

func (s *Server) handleCustomPOST(w http.ResponseWriter, r *http.Request) {
	rules := parseIPList(r.FormValue("rules")) // reuses newline-parser

	// Validate syntax via core
	if errs, err := s.client.ValidateCustom(rules); err != nil {
		slog.Warn("validate custom rules error", "error", err)
		// Core unavailable — skip validation, let apply catch errors
	} else if len(errs) > 0 {
		s.render(w, r, "custom.html", "custom", &customData{Rules: rules, Validation: customValidation(errs)})
		return
	}

	if err := s.client.SaveRules("custom", rules); err != nil {
		slog.Warn("save custom rules error", "error", err)
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, "/custom", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "saved")
	http.Redirect(w, r, "/custom", http.StatusSeeOther)
}

// handleCustomValidate is an HTMX endpoint: it accepts the rules textarea
// content via form POST, runs syntax validation on the core, and responds with
// the fragment the page swaps into #custom-errors. No session state is mutated —
// this is read-only validation.
func (s *Server) handleCustomValidate(w http.ResponseWriter, r *http.Request) {
	rules := parseIPList(r.FormValue("rules"))

	errs, err := s.client.ValidateCustom(rules)
	if err != nil {
		slog.Warn("validate custom rules error", "error", err)
		// Core unavailable: say that validation is not running rather than
		// implying the rules are fine, but do not block typing.
		s.renderPartial(w, r, "validation_result",
			&validationData{NoticeKey: "validate_unavailable"})
		return
	}

	s.renderPartial(w, r, "validation_result", customValidation(errs))
}
