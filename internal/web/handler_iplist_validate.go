package web

import (
	"net"
	"net/http"
	"strings"
)

// lineError is one rejected line from an editor. Key is a message id rather than
// prose: this fragment lands in the page an operator is reading and has to arrive
// in their language. Detail carries the parser's own words where they add
// something — a Go error is diagnostic output, not a sentence to translate, so it
// is shown verbatim beside the translated reason.
type lineError struct {
	Line   int // 1-based, as counted in the textarea
	Key    string
	Detail string
}

// validateIPListEntries returns the rejected lines of blacklist/whitelist editor
// input, in line order. Each non-blank, non-comment line must parse as either a
// single IP address (IPv4 or IPv6) or a CIDR network.
//
// Line numbers count every line of the raw input, blanks and comments included,
// so they match what the operator sees beside the textarea.
func validateIPListEntries(raw string) []lineError {
	var errs []lineError
	for i, line := range strings.Split(raw, "\n") {
		entry := strings.TrimSpace(line)
		if entry == "" || strings.HasPrefix(entry, "#") {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, _, err := net.ParseCIDR(entry); err != nil {
				errs = append(errs, lineError{
					Line: i + 1, Key: "validate_invalid_cidr", Detail: err.Error(),
				})
			}
			continue
		}
		if ip := net.ParseIP(entry); ip == nil {
			errs = append(errs, lineError{Line: i + 1, Key: "validate_invalid_ip"})
		}
	}
	return errs
}

// validationData is what the shared validation partial renders. TitleKey and
// OKKey let both editors use one template while still naming what they checked:
// addresses in one case, nftables syntax in the other.
type validationData struct {
	Errors   []lineError
	TitleKey string
	OKKey    string
	// NoticeKey replaces the result entirely when validation could not run.
	NoticeKey string
	// Mono renders each detail as code — right for an nftables statement, wrong
	// for a sentence about an address.
	Mono bool
}

// handleIPListValidate is an HTMX endpoint shared between the blacklist and
// whitelist editors. It accepts the textarea content via form POST and returns
// the fragment the page swaps into #iplist-errors.
//
// It renders a template rather than assembling HTML here. The inline version this
// replaced emitted `alert-success`, `alert-error` and `alert-soft` — daisyUI class
// names that stopped existing when the design system replaced daisyUI, so every
// validation response an operator ever saw was an unstyled box.
func (s *Server) handleIPListValidate(w http.ResponseWriter, r *http.Request) {
	s.renderPartial(w, r, "validation_result", validationData{
		Errors:   validateIPListEntries(r.FormValue("entries")),
		TitleKey: "validate_invalid_entries",
		OKKey:    "validate_iplist_ok",
	})
}
