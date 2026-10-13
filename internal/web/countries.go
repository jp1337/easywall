package web

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"unicode"

	"github.com/jp1337/easywall/internal/shared"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
	"golang.org/x/text/unicode/norm"
)

// The Countries card on /blocklist (2.26, spec §2). A country is a feed with
// the id country-<cc>; the card stages the country subset of Rules.Feeds and
// leaves every other id where it is.

// countryName is what country cc is called in lang — "de" is "Deutschland"
// under de and "Germany" under en (spec D7): CLDR's names through x/text, so
// no locale key per country. The code in capitals when x/text has none.
func countryName(lang, cc string) string {
	if r, err := language.ParseRegion(cc); err == nil {
		if n := display.Regions(language.Make(lang)).Name(r); n != "" {
			return n
		}
	}
	return strings.ToUpper(cc)
}

// nameSortKey orders the countries' names the way a reader looks one up:
// "Ägypten" beside "Afghanistan", "États-Unis" among the E's — the accents
// taken off, the case folded. Not x/text/collate, which adds 1.5 MB to the
// web binary for an order that differs from this one in no name x/text has
// in en, de or fr (TestCountriesSortTheWayAReaderLooksThemUp).
func nameSortKey(s string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, norm.NFD.String(s)))
}

// countriesNext is what a save of the Countries card stages: before with add
// appended, or with remove taken out. Every id that is not a country keeps
// its place. The second value is a flash key when nothing is staged.
func countriesNext(before []string, add, remove string) ([]string, string) {
	next := append([]string{}, before...)
	switch {
	case remove != "" && shared.IsCountryFeedID(remove):
		return slices.DeleteFunc(next, func(id string) bool { return id == remove }), ""
	case remove == "" && shared.IsCountryFeedID(add):
		if slices.Contains(next, add) {
			return next, ""
		}
		n := 0
		for _, id := range next {
			if shared.IsCountryFeedID(id) {
				n++
			}
		}
		if n >= shared.MaxCountries {
			return nil, "feeds_error_too_many_countries"
		}
		return append(next, add), ""
	}
	return nil, "save_error" // an id that is not a country: a forged form
}

// handleCountriesPOST adds or removes one country. A form that carries
// remove only removes, whatever else it carries, and fetches nothing.
func (s *Server) handleCountriesPOST(w http.ResponseWriter, r *http.Request) {
	back := "/blocklist#countries"
	state, err := s.client.GetRules()
	if err != nil {
		slog.Warn("get rules error", "error", err)
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	add, remove := r.PostFormValue("add"), r.PostFormValue("remove")
	next, refused := countriesNext(state.Staged.Feeds, add, remove)
	if refused != "" {
		slog.Info("countries not staged", "reason", refused)
		s.setFlash(w, r, refused)
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if err := s.client.SaveRules("feeds", next); err != nil {
		slog.Warn("save countries error", "error", err)
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	// After the save, as the Feeds card does: the runner's own GET_RULES
	// already sees it staged. Only for a country that was not staged before.
	if remove == "" && !slices.Contains(state.Staged.Feeds, add) {
		s.refreshFeedNow(add)
	}
	s.setFlash(w, r, "saved")
	http.Redirect(w, r, back, http.StatusSeeOther)
}
