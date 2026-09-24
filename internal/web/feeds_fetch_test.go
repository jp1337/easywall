package web

import (
	"bytes"
	"compress/gzip"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Every rule of spec §3's client, against a real HTTP server.

func testRunner(t *testing.T) *feedRunner {
	t.Helper()
	return newFeedRunner(NewCoreClient(t.TempDir()+"/none.sock"), newFeedStore(t.TempDir()+"/feed_fetch.json"))
}

func wantFailure(t *testing.T, err error, reason string, status int) {
	t.Helper()
	var f *feedFailure
	if !errors.As(err, &f) {
		t.Fatalf("err = %v, want a feedFailure %q", err, reason)
	}
	if f.reason != reason || f.status != status {
		t.Fatalf("failure %q %d, want %q %d", f.reason, f.status, reason, status)
	}
}

func TestFeedRequestIdentifiesItselfAndAsksForGzip(t *testing.T) {
	var ua, ae, user, pass string
	var authed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua, ae = r.Header.Get("User-Agent"), r.Header.Get("Accept-Encoding")
		user, pass, authed = r.BasicAuth()
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte("192.0.2.1\n"))
		_ = gz.Close()
	}))
	defer srv.Close()

	res, err := testRunner(t).fetch(srv.URL, feedFetchState{}, false, feedSource{user: "u", password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if string(res.body) != "192.0.2.1\n" {
		t.Errorf("body %q: the gzip body was not decompressed", res.body)
	}
	if ua != feedUserAgent() || !strings.HasPrefix(ua, "easywall/") || !strings.HasSuffix(ua, " (+https://easywall-project.org)") {
		t.Errorf("User-Agent %q", ua)
	}
	if ae != "gzip" {
		t.Errorf("Accept-Encoding %q, want gzip: CrowdSec truncates an uncompressed body over 5 MB", ae)
	}
	if !authed || user != "u" || pass != "p" {
		t.Errorf("basic auth %v %q %q, want u/p", authed, user, pass)
	}
}

func TestFeedBodyIsCappedAfterDecompression(t *testing.T) {
	var size int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write(bytes.Repeat([]byte("1"), size)) // a few KiB on the wire
		_ = gz.Close()
	}))
	defer srv.Close()
	r := testRunner(t)

	size = feedMaxBody
	if res, err := r.fetch(srv.URL, feedFetchState{}, false, feedSource{}); err != nil || len(res.body) != feedMaxBody {
		t.Fatalf("exactly %d bytes: %d, %v — want accepted", feedMaxBody, len(res.body), err)
	}
	size = feedMaxBody + 1
	_, err := r.fetch(srv.URL, feedFetchState{}, false, feedSource{})
	wantFailure(t, err, feedErrTooLarge, 0)
}

func TestFeedRedirectIsRefusedNotFollowed(t *testing.T) {
	var followed atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followed.Store(true)
		_, _ = w.Write([]byte("192.0.2.1\n"))
	}))
	defer target.Close()
	srv := httptest.NewServer(http.RedirectHandler(target.URL, http.StatusFound))
	defer srv.Close()

	_, err := testRunner(t).fetch(srv.URL, feedFetchState{}, false, feedSource{user: "u", password: "p"})
	wantFailure(t, err, feedErrRedirected, 0)
	if followed.Load() {
		t.Error("the redirect was followed — with the own feed's credentials")
	}
}

func TestFeedConditionalRequestSendsBothValidators(t *testing.T) {
	var inm, ims string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inm, ims = r.Header.Get("If-None-Match"), r.Header.Get("If-Modified-Since")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()
	r := testRunner(t)
	st := feedFetchState{ETag: `"871-65c3a633a791b"`, LastModified: "Thu, 24 Sep 2026 13:15:02 GMT"}

	res, err := r.fetch(srv.URL, st, true, feedSource{})
	if err != nil || !res.notModified {
		t.Fatalf("304: %+v, %v — want not modified", res, err)
	}
	if inm != st.ETag || ims != st.LastModified {
		t.Errorf("If-None-Match %q, If-Modified-Since %q: want both (GitHub raw honours only the first)", inm, ims)
	}

	// Unconditional: no validators go out, and a 304 is not an answer to it.
	_, err = r.fetch(srv.URL, st, false, feedSource{})
	if inm != "" || ims != "" {
		t.Errorf("an unconditional request carried If-None-Match %q / If-Modified-Since %q", inm, ims)
	}
	wantFailure(t, err, feedErrHTTPStatus, http.StatusNotModified)
}

func TestFeedWebPageIsNotAList(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"content type": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("192.0.2.1\n"))
		},
		"first byte": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("\n  <!DOCTYPE html><title>Attention Required! | Cloudflare</title>"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			_, err := testRunner(t).fetch(srv.URL, feedFetchState{}, false, feedSource{})
			wantFailure(t, err, feedErrHTML, 0)
		})
	}
}

func TestFeedStatusCodesHaveTheirReasons(t *testing.T) {
	for code, want := range map[int]string{
		http.StatusTooManyRequests:     feedErrRateLimited,
		http.StatusForbidden:           feedErrHTTPStatus,
		http.StatusInternalServerError: feedErrHTTPStatus,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))
		_, err := testRunner(t).fetch(srv.URL, feedFetchState{}, false, feedSource{})
		srv.Close()
		status := code
		if want == feedErrRateLimited {
			status = 0
		}
		wantFailure(t, err, want, status)
	}
}

func TestFeedRequestIsBoundedInTime(t *testing.T) {
	old := feedFetchTimeout
	feedFetchTimeout = 100 * time.Millisecond
	t.Cleanup(func() { feedFetchTimeout = old })
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Headers at once, then a body that never ends: the bound must cover
		// the body, not only the connection.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("192.0.2.1\n"))
		w.(http.Flusher).Flush()
		<-release
	}))
	defer srv.Close()
	defer close(release)

	start := time.Now()
	_, err := testRunner(t).fetch(srv.URL, feedFetchState{}, false, feedSource{})
	wantFailure(t, err, feedErrTimeout, 0)
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %v", d)
	}
}

func TestFeedUnreachableHostIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_, err := testRunner(t).fetch(url, feedFetchState{}, false, feedSource{})
	wantFailure(t, err, feedErrUnreachable, 0)
}
