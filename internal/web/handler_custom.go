package web

import (
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"sort"
	"strings"
)

type customData struct {
	Rules  []string
	Errors map[int]string // line index → error message
}

func (s *Server) handleCustomGET(w http.ResponseWriter, r *http.Request) {
	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("get rules error", "error", err)
		s.render(w, r, "custom.html", "custom", &customData{Errors: nil})
		return
	}
	s.render(w, r, "custom.html", "custom", &customData{Rules: state.Staged.Custom, Errors: nil})
}

func (s *Server) handleCustomPOST(w http.ResponseWriter, r *http.Request) {
	rules := parseIPList(r.FormValue("rules")) // reuses newline-parser

	// Validate syntax via core
	if errs, err := s.client.ValidateCustom(rules); err != nil {
		slog.Warn("validate custom rules error", "error", err)
		// Core unavailable — skip validation, let apply catch errors
	} else if len(errs) > 0 {
		s.render(w, r, "custom.html", "custom", &customData{Rules: rules, Errors: errs})
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
// content via form POST, runs syntax validation on the core, and responds
// with an HTML fragment that the page swaps into #custom-errors. No
// session state is mutated — this is read-only validation.
func (s *Server) handleCustomValidate(w http.ResponseWriter, r *http.Request) {
	rules := parseIPList(r.FormValue("rules"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	errs, err := s.client.ValidateCustom(rules)
	if err != nil {
		slog.Warn("validate custom rules error", "error", err)
		// Core unavailable — render a small notice so the user knows
		// validation isn't running, but don't block input.
		_, _ = fmt.Fprint(w, `<div role="alert" class="alert alert-info alert-soft mt-3">`+
			`<svg class="size-4" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a.75.75 0 000 1.5h.253a.25.25 0 01.244.304l-.459 2.066A1.75 1.75 0 0010.747 15H11a.75.75 0 000-1.5h-.253a.25.25 0 01-.244-.304l.459-2.066A1.75 1.75 0 009.253 9H9z" clip-rule="evenodd"/></svg>`+
			`<span>Live validation unavailable (core daemon offline). Errors will surface on save.</span>`+
			`</div>`)
		return
	}

	if len(errs) == 0 {
		_, _ = fmt.Fprint(w, `<div role="status" class="alert alert-success alert-soft mt-3">`+
			`<svg class="size-4" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.857-9.809a.75.75 0 00-1.214-.882l-3.483 4.79-1.88-1.88a.75.75 0 10-1.06 1.061l2.5 2.5a.75.75 0 001.137-.089l4-5.5z" clip-rule="evenodd"/></svg>`+
			`<span>Syntax OK — all rules are valid nftables.</span>`+
			`</div>`)
		return
	}

	// Render error list, sorted by line index for stable order.
	keys := make([]int, 0, len(errs))
	for k := range errs {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	var sb strings.Builder
	sb.WriteString(`<div role="alert" class="alert alert-error alert-soft mt-3">`)
	sb.WriteString(`<svg class="size-4" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd"/></svg>`)
	sb.WriteString(`<div class="flex-1"><strong class="font-medium">Syntax errors found:</strong>`)
	sb.WriteString(`<ul class="mt-1.5 ml-5 list-disc text-sm">`)
	for _, idx := range keys {
		fmt.Fprintf(&sb,
			`<li>Line %d: <code class="font-mono">%s</code></li>`,
			idx+1, html.EscapeString(errs[idx]),
		)
	}
	sb.WriteString(`</ul></div></div>`)
	_, _ = w.Write([]byte(sb.String()))
}
