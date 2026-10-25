//go:build nettest

// Package shared's country breadth guard. Not in the hermetic unit suite: it
// fetches live upstream data, so it is tagged `nettest` and run on a schedule
// (.github/workflows/security.yml), not on every push.
package shared

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"regexp"
	"sync"
	"testing"
	"time"
)

// FeedMaxBitsV4Country and FeedMaxBitsV6Country are justified by a point-in-time
// measurement of all 238 countries (see countries.go). The v6 limit has zero
// headroom — the broadest observed is /16 — so a single country prefix broader
// than that, published by upstream on its own schedule, makes validateFeedEntries
// refuse the whole country update with nothing louder than one feed_refused line.
//
// This re-measures upstream and fails if any country no longer fits, turning a
// constant justified by a measurement into one guarded by a repeatable one. A red
// Monday run is the signal to widen the limit deliberately, not to discover it in
// the field.
func TestCountryBreadthFitsUpstream(t *testing.T) {
	cidr := regexp.MustCompile(`[0-9a-fA-F.:]+/[0-9]{1,3}`)

	type bad struct {
		cc     string
		prefix netip.Prefix
	}
	var (
		mu       sync.Mutex
		breaches []bad
		fetchErr []string
	)

	codes := CountryCodes
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 30 * time.Second}

	for _, cc := range codes {
		wg.Add(1)
		sem <- struct{}{}
		go func(cc string) {
			defer wg.Done()
			defer func() { <-sem }()

			url := countryURLStem + cc + "/aggregated.json"
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			resp, err := client.Do(req)
			if err != nil {
				mu.Lock()
				fetchErr = append(fetchErr, cc+": "+err.Error())
				mu.Unlock()
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				mu.Lock()
				fetchErr = append(fetchErr, fmt.Sprintf("%s: HTTP %d", cc, resp.StatusCode))
				mu.Unlock()
				return
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				mu.Lock()
				fetchErr = append(fetchErr, cc+": "+err.Error())
				mu.Unlock()
				return
			}

			for _, m := range cidr.FindAllString(string(body), -1) {
				p, err := netip.ParsePrefix(m)
				if err != nil {
					continue
				}
				limit := FeedMaxBitsV4Country
				if p.Addr().Is6() {
					limit = FeedMaxBitsV6Country
				}
				if p.Bits() < limit {
					mu.Lock()
					breaches = append(breaches, bad{cc, p})
					mu.Unlock()
				}
			}
		}(cc)
	}
	wg.Wait()

	// A fetch that did not work is inconclusive, not a pass: say so loudly rather
	// than report "all fit" from a run that reached nothing.
	if len(fetchErr) > len(codes)/5 {
		t.Fatalf("too many countries could not be fetched (%d/%d); result inconclusive:\n  %v",
			len(fetchErr), len(codes), fetchErr)
	}
	for _, b := range breaches {
		t.Errorf("%s publishes %s, broader than the limit /%d (v4) or /%d (v6) — widen it in countries.go deliberately",
			b.cc, b.prefix, FeedMaxBitsV4Country, FeedMaxBitsV6Country)
	}
	if len(breaches) == 0 {
		t.Logf("all %d countries fit (%d unreachable)", len(codes)-len(fetchErr), len(fetchErr))
	}
}
