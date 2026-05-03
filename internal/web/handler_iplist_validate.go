package web

import (
	"fmt"
	"html"
	"net"
	"net/http"
	"sort"
	"strings"
)

// validateIPListEntries returns a per-line error map for blacklist/whitelist
// editor input. Each non-blank, non-comment line must parse as either a single
// IP address (IPv4 or IPv6) or a CIDR network. The returned map's keys are
// 0-based line indices into the raw input — keeping line numbers aligned with
// the textarea makes it trivial for the UI to highlight the bad row.
func validateIPListEntries(raw string) map[int]string {
	errs := map[int]string{}
	for i, line := range strings.Split(raw, "\n") {
		entry := strings.TrimSpace(line)
		if entry == "" || strings.HasPrefix(entry, "#") {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, _, err := net.ParseCIDR(entry); err != nil {
				errs[i] = "invalid CIDR: " + err.Error()
			}
			continue
		}
		if ip := net.ParseIP(entry); ip == nil {
			errs[i] = "not a valid IP address"
		}
	}
	return errs
}

// handleIPListValidate is an HTMX endpoint shared between the blacklist and
// whitelist editors. It accepts the textarea content via form POST and
// returns an HTML fragment describing per-line parse errors (or a soft
// success notice when everything parses cleanly).
func (s *Server) handleIPListValidate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	errs := validateIPListEntries(r.FormValue("entries"))

	if len(errs) == 0 {
		fmt.Fprint(w, `<div role="status" class="alert alert-success alert-soft mt-3">`+
			`<svg class="size-4" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.857-9.809a.75.75 0 00-1.214-.882l-3.483 4.79-1.88-1.88a.75.75 0 10-1.06 1.061l2.5 2.5a.75.75 0 001.137-.089l4-5.5z" clip-rule="evenodd"/></svg>`+
			`<span>All entries are valid IPv4/IPv6 addresses or CIDR networks.</span>`+
			`</div>`)
		return
	}

	keys := make([]int, 0, len(errs))
	for k := range errs {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	var sb strings.Builder
	sb.WriteString(`<div role="alert" class="alert alert-error alert-soft mt-3">`)
	sb.WriteString(`<svg class="size-4" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd"/></svg>`)
	sb.WriteString(`<div class="flex-1"><strong class="font-medium">Invalid entries:</strong>`)
	sb.WriteString(`<ul class="mt-1.5 ml-5 list-disc text-sm">`)
	for _, idx := range keys {
		fmt.Fprintf(&sb,
			`<li>Line %d — <span>%s</span></li>`,
			idx+1, html.EscapeString(errs[idx]),
		)
	}
	sb.WriteString(`</ul></div></div>`)
	_, _ = w.Write([]byte(sb.String()))
}
