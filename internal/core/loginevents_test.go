package core

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

type recordedEntry struct{ action, ruleType, detail, user string }

func newRecordingEvents() (*loginEvents, *[]recordedEntry) {
	var got []recordedEntry
	l := newLoginEvents(func(action, ruleType, detail, user string) {
		got = append(got, recordedEntry{action, ruleType, detail, user})
	})
	return l, &got
}

// Two lines per burst, never more. The first immediately, so an operator who is
// watching sees it; the summary afterwards, so a scanner cannot fill the 200
// entries the viewer shows.
func TestLoginEvents_FourteenFailuresProduceExactlyTwoLines(t *testing.T) {
	l, got := newRecordingEvents()
	start := time.Unix(1755600000, 0).UTC()

	for i := 0; i < 14; i++ {
		if err := l.record(shared.LogEventPayload{
			Event: shared.EvLoginFailed, Addr: "203.0.113.7",
		}, start.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if len(*got) != 1 {
		t.Fatalf("during the window %d lines were written, want exactly 1 (the first)", len(*got))
	}
	if (*got)[0].detail != "from 203.0.113.7" {
		t.Errorf("first line detail is %q, want %q", (*got)[0].detail, "from 203.0.113.7")
	}

	l.sweep(start.Add(loginEventWindow + time.Second))

	if len(*got) != 2 {
		t.Fatalf("after the window closed there are %d lines, want exactly 2", len(*got))
	}
	if want := "from 203.0.113.7, 13 more within 60s"; (*got)[1].detail != want {
		t.Errorf("summary detail is %q, want %q", (*got)[1].detail, want)
	}
	for i, e := range *got {
		if e.action != string(shared.EvLoginFailed) {
			t.Errorf("line %d action is %q", i, e.action)
		}
		if e.user != "web" {
			t.Errorf("line %d user is %q, want web", i, e.user)
		}
	}
}

// A single failure is one line and no summary. Two lines for one event would be
// noise, and the summary exists for bursts.
func TestLoginEvents_OneFailureIsOneLine(t *testing.T) {
	l, got := newRecordingEvents()
	start := time.Unix(1755600000, 0).UTC()

	_ = l.record(shared.LogEventPayload{Event: shared.EvLoginFailed, Addr: "203.0.113.7"}, start)
	l.sweep(start.Add(loginEventWindow + time.Second))

	if len(*got) != 1 {
		t.Errorf("one failure produced %d lines, want 1", len(*got))
	}
}

// An event is debounced exactly when an unauthenticated request can cause it.
// Logout belongs on that list: POST /logout is in the public route group, so a
// replayed session cookie in a loop reaches the same path a failed login does.
// Successes and the three enrolment events cannot be produced by an anonymous
// caller at all, so they stay immediate.
//
// This is the regression for the audit-log erasure: it fails if EvLogout is
// removed from debouncedEvents, because logout would then produce three lines
// for three requests instead of one.
func TestLoginEvents_OnlyTheStrangerTriggerableOnesAreDebounced(t *testing.T) {
	debounced := []shared.LoginEvent{
		shared.EvLoginFailed, shared.Ev2FAFailed, shared.EvRateLimited, shared.EvLogout,
	}
	for _, ev := range shared.AllLoginEvents {
		l, got := newRecordingEvents()
		start := time.Unix(1755600000, 0).UTC()
		for i := 0; i < 3; i++ {
			_ = l.record(shared.LogEventPayload{Event: ev, Addr: "203.0.113.7"}, start.Add(time.Duration(i)*time.Second))
		}
		isDebounced := false
		for _, d := range debounced {
			if d == ev {
				isDebounced = true
			}
		}
		want := 3
		if isDebounced {
			want = 1
		}
		if len(*got) != want {
			t.Errorf("%s: three events produced %d lines, want %d", ev, len(*got), want)
		}
	}
}

// The web process cannot invent a line.
func TestLoginEvents_AnUnknownEventIsRefusedAndWritesNothing(t *testing.T) {
	l, got := newRecordingEvents()
	if err := l.record(shared.LogEventPayload{Event: "definitely_not_an_event"}, time.Now()); err == nil {
		t.Error("an unknown event was accepted")
	}
	if len(*got) != 0 {
		t.Errorf("a refused event wrote %d line(s)", len(*got))
	}
}

// The entry is the record; the address is the annotation.
func TestLoginEvents_AnUnparseableAddressYieldsAnEntryWithoutOne(t *testing.T) {
	l, got := newRecordingEvents()
	if err := l.record(shared.LogEventPayload{
		Event: shared.EvLoginFailed, Addr: "<script>alert(1)</script>",
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 {
		t.Fatalf("the entry was dropped along with its address (%d lines)", len(*got))
	}
	if (*got)[0].detail != "" {
		t.Errorf("detail is %q; an address that does not parse must not reach the record", (*got)[0].detail)
	}
}

// The address is normalised in the core rather than trusted as written, so
// "203.0.113.007" and an IPv6 address in two spellings do not become two keys.
func TestLoginEvents_TheAddressIsNormalisedHere(t *testing.T) {
	l, got := newRecordingEvents()
	start := time.Unix(1755600000, 0).UTC()

	_ = l.record(shared.LogEventPayload{Event: shared.EvLoginFailed, Addr: "2001:0db8:0000:0000:0000:0000:0000:0001"}, start)
	if len(*got) != 1 {
		t.Fatalf("got %d lines", len(*got))
	}
	if want := "from 2001:db8::1"; (*got)[0].detail != want {
		t.Errorf("detail is %q, want %q", (*got)[0].detail, want)
	}
}

// This runs in the root process on input from outside, so the table has a
// ceiling. Beyond it, further addresses fold into one aggregate bucket: the
// record still shows the volume while the memory does not hang off a number a
// stranger chooses.
func TestLoginEvents_TheTableStopsGrowingAtTheCeiling(t *testing.T) {
	l, got := newRecordingEvents()
	start := time.Unix(1755600000, 0).UTC()

	// Fill the table exactly, then push well past it.
	for i := 0; i < loginEventMaxAddrs; i++ {
		addr := fmt.Sprintf("10.%d.%d.%d", i/65025, (i/255)%255, i%255+1)
		_ = l.record(shared.LogEventPayload{Event: shared.EvLoginFailed, Addr: addr}, start)
	}
	tracked := len(*got)
	if tracked != loginEventMaxAddrs {
		t.Fatalf("%d distinct addresses produced %d immediate lines, want %d", loginEventMaxAddrs, tracked, loginEventMaxAddrs)
	}

	for i := 0; i < 500; i++ {
		addr := fmt.Sprintf("198.51.100.%d", i%255+1)
		_ = l.record(shared.LogEventPayload{Event: shared.EvLoginFailed, Addr: addr}, start.Add(time.Second))
	}
	if len(*got) != tracked {
		t.Errorf("addresses past the ceiling wrote %d immediate line(s); they must fold into "+
			"the aggregate instead", len(*got)-tracked)
	}
	if n := l.trackedAddrs(); n > loginEventMaxAddrs {
		t.Errorf("the table holds %d addresses, past the ceiling of %d", n, loginEventMaxAddrs)
	}

	l.sweep(start.Add(loginEventWindow + 2*time.Second))

	var aggregate string
	for _, e := range (*got)[tracked:] {
		if strings.Contains(e.detail, "more than") {
			aggregate = e.detail
		}
	}
	if aggregate == "" {
		t.Fatal("the window closed and the overflow was never recorded; the volume is invisible")
	}
	if !strings.Contains(aggregate, fmt.Sprintf("more than %d addresses", loginEventMaxAddrs)) ||
		!strings.Contains(aggregate, "500 within 60s") {
		t.Errorf("aggregate detail is %q, want it to name the ceiling and the count", aggregate)
	}
}

// login_recovery_used carries how many are left, because that is the number the
// operator has to act on.
func TestLoginEvents_RecoveryUsedNamesWhatIsLeft(t *testing.T) {
	l, got := newRecordingEvents()
	_ = l.record(shared.LogEventPayload{Event: shared.EvRecoveryUsed, Addr: "203.0.113.7", Left: 7}, time.Now())

	if len(*got) != 1 {
		t.Fatalf("got %d lines", len(*got))
	}
	if !strings.Contains((*got)[0].detail, "7") {
		t.Errorf("detail is %q and does not say how many are left", (*got)[0].detail)
	}
}
