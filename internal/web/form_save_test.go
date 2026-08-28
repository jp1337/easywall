package web

import (
	"regexp"
	"strings"
	"testing"
)

// The settings pages save on submit, and they can be submitted without a script.
//
// Both halves have failed. /options, /settings and /system posted over HTMX on
// every `change` *and* carried a Save button, so the button reported a write
// that had already happened two hundred milliseconds earlier — the same
// dishonesty as a save button for a value already saved. And the telemetry form
// on /system went the other way: hx-trigger was the only thing that ever
// submitted it, so it held a consent toggle that a script-free operator could
// move and never store it.
//
// Derived from the templates rather than from a list of four line numbers,
// because a fifth settings form is exactly the thing a hand-written list would
// not notice.
func TestSettingFormsSaveOnSubmitAndCanBeSubmittedWithoutAScript(t *testing.T) {
	for _, name := range []string{"options.html", "settings.html", "system.html"} {
		body := repoFile2(t, "web", "templates", name)
		for i, form := range postForms(body) {
			if strings.Contains(form, "hx-trigger") {
				t.Errorf("%s: form %d saves on its own:\n%s\n"+
					"  htmx falls back to a form's submit trigger; the Save button is "+
					"the only honest write", name, i, formTag(form))
			}
			if !strings.Contains(form, `type="submit"`) {
				t.Errorf("%s: form %d has no submit button, so it cannot be saved "+
					"without JavaScript:\n%s", name, i, formTag(form))
			}
		}
	}
}

// postForms returns the source of every <form method="POST"> in body.
func postForms(body string) []string {
	var out []string
	for _, chunk := range strings.Split(body, "<form")[1:] {
		end := strings.Index(chunk, "</form>")
		if end < 0 {
			continue
		}
		form := "<form" + chunk[:end]
		if strings.Contains(form, `method="POST"`) {
			out = append(out, form)
		}
	}
	return out
}

// formTag returns just the opening tag, so a failure names the form without
// printing the page.
var openTagRe = regexp.MustCompile(`(?s)^<form[^>]*>`)

func formTag(form string) string {
	if m := openTagRe.FindString(form); m != "" {
		return m
	}
	return form
}
