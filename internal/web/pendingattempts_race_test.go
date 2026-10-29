package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// serveConcurrently sends n copies of one form POST at once, each carrying the
// given cookies, and waits for every answer. doFormRequest is not usable here:
// its address counter is a plain int, and -race would report the helper rather
// than the handler.
func serveConcurrently(s *Server, n int, path string, form func(i int) string, cookies func(i int) []*http.Cookie) []*httptest.ResponseRecorder {
	recs := make([]*httptest.ResponseRecorder, n)
	start := make(chan struct{})
	var done sync.WaitGroup
	for i := 0; i < n; i++ {
		req := httptest.NewRequest("POST", path, strings.NewReader(form(i)))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, c := range cookies(i) {
			req.AddCookie(c)
		}
		recs[i] = httptest.NewRecorder()
		done.Add(1)
		go func(req *http.Request, rec *httptest.ResponseRecorder) {
			defer done.Done()
			<-start
			s.router.ServeHTTP(rec, req)
		}(req, recs[i])
	}
	close(start)
	done.Wait()
	return recs
}

// TestPending_ParallelCodesOnOneFrozenCookieSpendOnlyTheBudget is the frozen
// cookie again, sent all at once instead of one after another.
//
// readPending read the count and failVerifyAttempt charged it, with the code
// check and a flash write in between and no lock across the two: every request
// that arrived before the first charge landed saw 0 and was checked. 64 parallel
// posts of one cookie got 5–9 code checks against a budget of 3.
//
// What is counted is answers that carry a flash — verify_failed or login_again,
// written only after a code was checked — not the server's counter, which a
// capped implementation could hold at 3 while still checking every guess.
func TestPending_ParallelCodesOnOneFrozenCookieSpendOnlyTheBudget(t *testing.T) {
	s := newTestServer(t, newFakeCore(t))
	enrol(t, s)

	for round := 0; round < 20; round++ {
		login := doFormRequest(s, "POST", "/login",
			"username=admin&password="+url.QueryEscape(testPassword))
		frozen := pendingCookieOf(t, login)

		recs := serveConcurrently(s, 64, "/login/verify",
			func(int) string { return "code=000000" },
			func(int) []*http.Cookie { return []*http.Cookie{frozen} })

		checked := 0
		for _, rec := range recs {
			if sessionCookie(rec.Result()) != "" {
				checked++
			}
		}
		if checked > pendingMaxAttempts {
			t.Fatalf("round %d: one frozen cookie sent 64 times at once had %d codes checked, "+
				"budget %d — the attempt is charged after the check, not reserved before it",
				round, checked, pendingMaxAttempts)
		}
	}
}
