package core

import (
	"path/filepath"
	"testing"
)

func newTestUsageStore(t *testing.T) (*UsageStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "usage.json")
	return NewUsageStore(path), path
}

func allLive(ids ...string) map[string]bool {
	live := make(map[string]bool, len(ids))
	for _, id := range ids {
		live[id] = true
	}
	return live
}

// A missing file is an installation that has never collected, which is not a
// failure and must not be reported as one — the same reasoning applied-config.json
// runs on.
func TestUsageStore_AMissingFileIsAnEmptyResult(t *testing.T) {
	u, _ := newTestUsageStore(t)
	res, err := u.Read()
	if err != nil {
		t.Fatalf("Read on a missing file: %v", err)
	}
	if len(res.Usage) != 0 || !res.CollectedAt.IsZero() {
		t.Errorf("a missing file read as %+v, want an empty result", res)
	}
}

// The first collect books everything the kernel has counted so far, and the
// second books only what arrived since.
func TestUsageStore_CollectBooksTheDelta(t *testing.T) {
	u, _ := newTestUsageStore(t)
	live := allLive("aaaaaaaaaaaa")

	if err := u.Collect(map[string]RuleCounter{"aaaaaaaaaaaa": {Packets: 10, Bytes: 1000}}, live); err != nil {
		t.Fatal(err)
	}
	res, _ := u.Read()
	got := res.Usage["aaaaaaaaaaaa"]
	if got.Packets != 10 || got.Bytes != 1000 {
		t.Fatalf("after the first collect: %d packets / %d bytes, want 10/1000", got.Packets, got.Bytes)
	}
	if got.FirstSeen.IsZero() || got.LastSeen.IsZero() {
		t.Error("traffic was booked but FirstSeen/LastSeen were not set")
	}
	firstSeen, lastSeen := got.FirstSeen, got.LastSeen

	// Nothing new: the totals stand and LastSeen does not move. A column that
	// says "just now" on a port nobody has touched for a month is the failure
	// this whole release exists to prevent.
	if err := u.Collect(map[string]RuleCounter{"aaaaaaaaaaaa": {Packets: 10, Bytes: 1000}}, live); err != nil {
		t.Fatal(err)
	}
	res, _ = u.Read()
	got = res.Usage["aaaaaaaaaaaa"]
	if got.Packets != 10 {
		t.Errorf("an idle interval added %d packets", got.Packets-10)
	}
	if !got.LastSeen.Equal(lastSeen) {
		t.Error("LastSeen moved on an interval with no traffic")
	}

	// And four more packets are four more packets.
	if err := u.Collect(map[string]RuleCounter{"aaaaaaaaaaaa": {Packets: 14, Bytes: 1400}}, live); err != nil {
		t.Fatal(err)
	}
	res, _ = u.Read()
	got = res.Usage["aaaaaaaaaaaa"]
	if got.Packets != 14 || got.Bytes != 1400 {
		t.Errorf("after the third collect: %d/%d, want 14/1400", got.Packets, got.Bytes)
	}
	if !got.FirstSeen.Equal(firstSeen) {
		t.Error("FirstSeen moved; it is the first use, not the most recent one")
	}
	if !got.LastSeen.After(lastSeen) {
		t.Error("LastSeen did not advance on an interval that carried traffic")
	}
}

// The bug this field exists to prevent, and the reason the baseline is in the
// file rather than in a struct field.
//
// The kernel table outlives the daemon. A baseline held in memory starts at zero
// on the next start, so the first collect after a restart books the whole
// lifetime of every kernel rule a second time — on a machine that has been up for
// a month, a port that saw one packet in the first hour reports as busy today.
func TestUsageStore_ADaemonRestartDoesNotDoubleCount(t *testing.T) {
	u, path := newTestUsageStore(t)
	live := allLive("bbbbbbbbbbbb")

	if err := u.Collect(map[string]RuleCounter{"bbbbbbbbbbbb": {Packets: 500, Bytes: 50000}}, live); err != nil {
		t.Fatal(err)
	}

	// The daemon restarts: a brand new store over the same file, and a kernel
	// whose counters kept climbing meanwhile.
	restarted := NewUsageStore(path)
	if err := restarted.Collect(map[string]RuleCounter{"bbbbbbbbbbbb": {Packets: 507, Bytes: 50700}}, live); err != nil {
		t.Fatal(err)
	}
	res, _ := restarted.Read()
	if got := res.Usage["bbbbbbbbbbbb"].Packets; got != 507 {
		t.Errorf("after a restart the total is %d, want 507 — the baseline did not survive, "+
			"so the first collect booked the kernel's whole lifetime again", got)
	}
}

// A table rebuilt behind the collector's back — `nft flush ruleset` typed by
// hand, or a restore easywall did not drive. The kernel value is below the
// baseline, which can only mean the counter restarted, so the whole of it is new.
func TestUsageStore_ARebuiltTableIsNotNegativeTraffic(t *testing.T) {
	u, _ := newTestUsageStore(t)
	live := allLive("cccccccccccc")

	_ = u.Collect(map[string]RuleCounter{"cccccccccccc": {Packets: 900, Bytes: 90000}}, live)
	_ = u.Collect(map[string]RuleCounter{"cccccccccccc": {Packets: 4, Bytes: 400}}, live)

	res, _ := u.Read()
	got := res.Usage["cccccccccccc"]
	if got.Packets != 904 {
		t.Errorf("total = %d, want 904 (900 booked, then 4 on the rebuilt table)", got.Packets)
	}
	if got.KernelPackets != 4 {
		t.Errorf("baseline = %d, want 4 — it has to follow the new table, not the old one", got.KernelPackets)
	}
}

// ResetBaselines is what apply() calls once the table has been rebuilt under it:
// the totals stand, the baselines go to zero, because the table about to be read
// next has none of the old counts in it.
func TestUsageStore_ResetBaselinesKeepsTheTotals(t *testing.T) {
	u, _ := newTestUsageStore(t)
	live := allLive("dddddddddddd")
	_ = u.Collect(map[string]RuleCounter{"dddddddddddd": {Packets: 77, Bytes: 7700}}, live)

	if err := u.ResetBaselines(); err != nil {
		t.Fatal(err)
	}
	res, _ := u.Read()
	got := res.Usage["dddddddddddd"]
	if got.Packets != 77 {
		t.Errorf("the total was reset too: %d, want 77", got.Packets)
	}
	if got.KernelPackets != 0 || got.KernelBytes != 0 {
		t.Errorf("baseline = %d/%d, want 0/0", got.KernelPackets, got.KernelBytes)
	}

	// And the next collect on the fresh table books its counts once, not twice.
	_ = u.Collect(map[string]RuleCounter{"dddddddddddd": {Packets: 3, Bytes: 300}}, live)
	res, _ = u.Read()
	if got := res.Usage["dddddddddddd"].Packets; got != 80 {
		t.Errorf("after the reset and one collect: %d, want 80", got)
	}
}

// A rule the operator deleted stops being tracked, or usage.json grows for the
// life of the installation. Liveness comes from the stored rules, never from
// what the kernel happens to hold: panic mode empties the table, and pruning on
// that would throw every history away the moment somebody runs `panic`.
func TestUsageStore_ForgetsRulesThatAreGone(t *testing.T) {
	u, _ := newTestUsageStore(t)
	_ = u.Collect(map[string]RuleCounter{
		"eeeeeeeeeeee": {Packets: 5}, "ffffffffffff": {Packets: 5},
	}, allLive("eeeeeeeeeeee", "ffffffffffff"))

	// The second rule is deleted; panic mode then empties the kernel entirely.
	_ = u.Collect(map[string]RuleCounter{}, allLive("eeeeeeeeeeee"))

	res, _ := u.Read()
	if _, still := res.Usage["ffffffffffff"]; still {
		t.Error("a deleted rule is still tracked; usage.json grows without bound")
	}
	if _, gone := res.Usage["eeeeeeeeeeee"]; !gone {
		t.Error("a rule that is still configured lost its history because the kernel was empty — " +
			"that is what happens under panic mode, and it must not")
	}
}
