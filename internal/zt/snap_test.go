package zt

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

type fakeZFS struct {
	ds        []string
	rows      []snapRow
	free      int64
	pending   int64
	size      int64
	capacity  int
	created   []string
	destroyed []string
	failOn    string
}

func (f *fakeZFS) ready() bool                 { return true }
func (f *fakeZFS) datasets() ([]string, error) { return f.ds, nil }
func (f *fakeZFS) snapshots(pools []string) ([]snapRow, error) {
	out := append([]snapRow{}, f.rows...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Creation < out[j].Creation })
	return out, nil
}
func (f *fakeZFS) create(name string, recursive bool) error {
	if f.failOn != "" && strings.Contains(name, f.failOn) {
		return errors.New("boom")
	}
	f.created = append(f.created, name)
	f.rows = append(f.rows, snapRow{Name: name, Creation: time.Now().Unix()})
	return nil
}
func (f *fakeZFS) destroy(name string, recursive bool) error {
	for i, r := range f.rows {
		if r.Name == name {
			if r.Refs > 0 {
				return errors.New("dataset is busy")
			}
			f.pending += r.Used
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			f.destroyed = append(f.destroyed, name)
			return nil
		}
	}
	return errors.New("could not find any snapshots to destroy")
}
func (f *fakeZFS) poolSpace(pool string) (int64, int64, int, bool) {
	return f.size, f.free, f.capacity, true
}
func (f *fakeZFS) sync(pool string) {
	f.free += f.pending
	f.pending = 0
}

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func settingsOf(kv ...string) Settings {
	s := Settings{}
	for _, k := range snapTypes {
		s["snap_"+k] = "0"
	}
	for i := 0; i+1 < len(kv); i += 2 {
		s[kv[i]] = kv[i+1]
	}
	return s
}

func engine(f *fakeZFS, s Settings, now time.Time, cfg ...snapDS) *snapEngine {
	return &snapEngine{s: s, be: f, now: now, config: cfg, hasConfig: true}
}

func TestSnapPeriodKeys(t *testing.T) {
	tm := at("2026-04-12 14:37")
	want := map[string]string{
		"frequent": "2026-04-12_14-37",
		"hourly":   "2026-04-12_14-00",
		"daily":    "2026-04-12",
		"weekly":   "2026-W15",
		"monthly":  "2026-04",
		"yearly":   "2026",
	}
	for typ, w := range want {
		if got := snapPeriodKey(typ, tm); got != w {
			t.Errorf("%s: got %s want %s", typ, got, w)
		}
	}
	if got := snapPeriodKey("weekly", at("2027-01-01 10:00")); got != "2026-W53" {
		t.Errorf("ISO week across the year: %s", got)
	}
	if got := snapPeriodKey("weekly", at("2024-12-30 10:00")); got != "2025-W01" {
		t.Errorf("ISO year ahead of calendar year: %s", got)
	}
}

func TestSnapParseDatasets(t *testing.T) {
	l, err := snapParseDatasets([]byte(`{"datasets":[
		{"name":"cache\/appdata","recursive":true,"use_template":true,"hourly":"","daily":""},
		{"name":"cache/TV Shows","recursive":"true","use_template":false,"hourly":"","daily":14,"weekly":"4"},
		{"name":"","recursive":true},
		{"name":"cache/old","use_template":false,"daily":"3"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != 3 || l[0].Name != "cache/appdata" || !l[0].Recursive {
		t.Fatalf("parsed %+v", l)
	}
	s := settingsOf("snap_hourly", "24", "snap_daily", "30", "snap_weekly", "4", "snap_frequent", "8")
	k, _ := l[0].keeps(s)
	if k["hourly"] != 24 || k["frequent"] != 8 {
		t.Errorf("template keeps %v", k)
	}
	k, _ = l[1].keeps(s)
	if !l[1].Recursive || k["hourly"] != 0 || k["daily"] != 14 || k["weekly"] != 4 || k["frequent"] != 0 {
		t.Errorf("own keeps %v", k)
	}
	k, _ = l[2].keeps(s)
	if k["hourly"] != 24 || k["daily"] != 3 || k["frequent"] != 0 || k["weekly"] != 4 {
		t.Errorf("missing fields fall back to the template, frequent to none: %v", k)
	}
	l[1].Hourly = strPtr("x")
	if _, w := l[1].keeps(s); len(w) != 1 {
		t.Errorf("invalid retention warns: %v", w)
	}
	enc := string(jsonEncode(M{"datasets": l}, false))
	if strings.Contains(enc, `\/`) || !strings.Contains(enc, `"cache/appdata"`) {
		t.Errorf("slashes are escaped: %s", enc)
	}
	if strings.Contains(enc, `"frequent"`) {
		t.Errorf("absent fields stay absent: %s", enc)
	}
	if _, err := snapParseDatasets([]byte(`{"datasets":[`)); err == nil {
		t.Error("broken JSON is reported")
	}
}

func strPtr(s string) *string { return &s }

func TestSnapValidNames(t *testing.T) {
	if !snapValidSnapshot("cache/TV Shows@auto-daily-2026-01-01") || snapValidSnapshot("cache@") || snapValidSnapshot("@x") || snapValidSnapshot("cache@a/b") {
		t.Error("snapshot names")
	}
}

func TestSnapTargetBytes(t *testing.T) {
	cases := map[string]int64{"100G": 100 << 30, "100 gib": 100 << 30, "5M": 5 << 20, "1T": 1 << 40, "10%": 100, "123": 123, "2KB": 2048}
	for in, want := range cases {
		got, ok := snapTargetBytes(in, 1000)
		if !ok || got != want {
			t.Errorf("%s: %d %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "abc", "10%%", "-5G", "5X"} {
		if _, ok := snapTargetBytes(in, 1000); ok {
			t.Errorf("%q accepted", in)
		}
	}
	if _, ok := snapTargetBytes("10%", 0); ok {
		t.Error("percent without a size")
	}
}

func rowsOf(ds string, snaps ...string) []snapRow {
	out := []snapRow{}
	for i, s := range snaps {
		out = append(out, snapRow{Name: ds + "@" + s, Creation: int64(1000 + i*3600), Used: 10})
	}
	return out
}

func TestSnapPruneSelect(t *testing.T) {
	rows := rowsOf("c/a", "auto-hourly-1", "auto-hourly-2", "manual", "auto-hourly-3", "auto-daily-1", "auto-hourly-4", "zdc-send-x-1", "autosnap_hourly")
	del, reasons, held := snapPruneSelect(rows, "hourly", 2, 0, 1e9)
	if len(del) != 2 || del[0].snap() != "auto-hourly-1" || del[1].snap() != "auto-hourly-2" || reasons[0] != "count" || len(held) != 0 {
		t.Errorf("count pass %v %v", del, reasons)
	}
	rows[1].Refs = 1
	del, _, held = snapPruneSelect(rows, "hourly", 2, 0, 1e9)
	if len(del) != 1 || len(held) != 1 || held[0].snap() != "auto-hourly-2" {
		t.Errorf("held snapshot kept %v %v", del, held)
	}
	now := int64(1000 + 5*3600)
	del, reasons, _ = snapPruneSelect(rowsOf("c/a", "auto-hourly-1", "auto-hourly-2", "auto-hourly-3"), "hourly", 10, 1, now+86400)
	if len(del) != 2 || reasons[0] != "age" {
		t.Errorf("age pass deletes all but the newest of an active type: %v", del)
	}
	del, _, _ = snapPruneSelect(rowsOf("c/a", "auto-hourly-1", "auto-hourly-2"), "hourly", 0, 1, now+86400)
	if len(del) != 2 {
		t.Errorf("an inactive type ages out completely: %v", del)
	}
	if del, _, _ := snapPruneSelect(rows, "hourly", 0, 0, now); len(del) != 0 {
		t.Error("no retention, no pruning")
	}
	if del, _, _ := snapPruneSelect(rowsOf("c/a", "auto-hourly-1"), "hourly", 1, 0, now); len(del) != 0 {
		t.Error("the only snapshot stays")
	}
}

func TestSnapFreeCandidates(t *testing.T) {
	rows := append(rowsOf("c/a", "auto-hourly-1", "manual", "auto-hourly-2", "auto-daily-1", "zdc-send-j-1", "auto-hourly-3"), rowsOf("c/b", "auto-hourly-1")...)
	rows[2].Refs = 2
	got := []string{}
	for _, r := range snapFreeCandidates(rows) {
		got = append(got, r.Name)
	}
	if strings.Join(got, ",") != "c/a@auto-hourly-1" {
		t.Errorf("candidates %v", got)
	}
}

func TestSnapEngineDueBased(t *testing.T) {
	f := &fakeZFS{ds: []string{"c", "c/a"}}
	s := settingsOf("snap_hourly", "24", "snap_daily", "7", "snap_weekly", "4", "snap_daily_hour", "2")
	d := snapDS{Name: "c/a", UseTemplate: true}
	e := engine(f, s, at("2026-04-12 01:15"), d)
	e.run()
	if strings.Join(f.created, ",") != "c/a@auto-hourly-2026-04-12_01-00" {
		t.Errorf("before the daily hour only hourly: %v", f.created)
	}
	f.created = nil
	engine(f, s, at("2026-04-12 01:45"), d).run()
	if len(f.created) != 0 {
		t.Errorf("the same period is never taken twice: %v", f.created)
	}
	engine(f, s, at("2026-04-12 07:10"), d).run()
	want := "c/a@auto-hourly-2026-04-12_07-00,c/a@auto-daily-2026-04-12,c/a@auto-weekly-2026-W15"
	if strings.Join(f.created, ",") != want {
		t.Errorf("missed daily is caught up later the same day: %v", f.created)
	}
	f.created = nil
	e = engine(f, s, at("2026-04-13 00:30"), d)
	e.forceNow = true
	e.run()
	if strings.Join(f.created, ",") != "c/a@auto-hourly-2026-04-13_00-00,c/a@auto-daily-2026-04-13,c/a@auto-weekly-2026-W16" {
		t.Errorf("--now ignores the daily hour: %v", f.created)
	}
}

func TestSnapEngineRecursiveChildConfigured(t *testing.T) {
	f := &fakeZFS{ds: []string{"c", "c/a", "c/a/x"}}
	s := settingsOf("snap_hourly", "2")
	e := engine(f, s, at("2026-04-12 10:00"), snapDS{Name: "c/a/x", UseTemplate: true}, snapDS{Name: "c/a", Recursive: true, UseTemplate: true})
	e.run()
	if strings.Join(f.created, ",") != "c/a@auto-hourly-2026-04-12_10-00" || e.Errors != 0 {
		t.Errorf("the parent runs first and covers the child: %v %d", f.created, e.Errors)
	}
}

func TestSnapEngineRetentionAndProtection(t *testing.T) {
	base := at("2026-04-12 10:00").Unix()
	f := &fakeZFS{ds: []string{"c", "c/a"}}
	for i, n := range []string{"auto-hourly-2026-04-12_05-00", "auto-hourly-2026-04-12_06-00", "auto-hourly-2026-04-12_07-00", "auto-hourly-2026-04-12_08-00", "before-upgrade", "zdc-send-j1-1", "auto-hourly-2026-04-12_09-00"} {
		f.rows = append(f.rows, snapRow{Name: "c/a@" + n, Creation: base - int64(7-i)*3600})
	}
	f.rows[0].Refs = 1
	s := settingsOf("snap_hourly", "2")
	e := engine(f, s, at("2026-04-12 10:00"), snapDS{Name: "c/a", UseTemplate: true})
	e.run()
	got := strings.Join(f.destroyed, ",")
	want := "c/a@auto-hourly-2026-04-12_06-00,c/a@auto-hourly-2026-04-12_07-00,c/a@auto-hourly-2026-04-12_08-00"
	if got != want {
		t.Errorf("destroyed %s", got)
	}
	left := []string{}
	for _, r := range f.rows {
		left = append(left, r.snap())
	}
	if strings.Join(left, ",") != "auto-hourly-2026-04-12_05-00,before-upgrade,zdc-send-j1-1,auto-hourly-2026-04-12_09-00,auto-hourly-2026-04-12_10-00" {
		t.Errorf("left %v", left)
	}
}

func TestSnapEngineFreeSpaceCountsUsed(t *testing.T) {
	base := at("2026-04-12 10:00").Unix()
	f := &fakeZFS{ds: []string{"c", "c/a"}, size: 2000, free: 100}
	for i := 0; i < 6; i++ {
		f.rows = append(f.rows, snapRow{Name: "c/a@auto-hourly-2026-04-11_0" + string(rune('0'+i)) + "-00", Creation: base - int64(10-i)*3600, Used: 100})
	}
	s := settingsOf("snap_hourly", "50", "snap_free_target", "250")
	e := engine(f, s, at("2026-04-12 10:30"), snapDS{Name: "c/a", UseTemplate: true})
	e.run()
	if len(f.destroyed) != 2 {
		t.Errorf("exactly two snapshots of 100 go for 150 more: %v", f.destroyed)
	}
	if !strings.HasSuffix(f.destroyed[0], "_00-00") {
		t.Errorf("oldest first: %v", f.destroyed)
	}
}

func TestSnapEngineDryRunChangesNothing(t *testing.T) {
	base := at("2026-04-12 10:00").Unix()
	f := &fakeZFS{ds: []string{"c", "c/a"}}
	for i := 0; i < 5; i++ {
		f.rows = append(f.rows, snapRow{Name: "c/a@auto-hourly-x" + string(rune('a'+i)), Creation: base - int64(10-i)*3600, Used: 7})
	}
	e := engine(f, settingsOf("snap_hourly", "2"), at("2026-04-12 10:30"), snapDS{Name: "c/a", UseTemplate: true})
	e.dry = true
	e.run()
	if len(f.created) != 0 || len(f.destroyed) != 0 {
		t.Fatal("dry run touched zfs")
	}
	if e.Created != 1 || e.Pruned != 4 || e.Freed != 28 {
		t.Errorf("plan created=%d pruned=%d freed=%d", e.Created, e.Pruned, e.Freed)
	}
}

func TestSnapEngineErrors(t *testing.T) {
	f := &fakeZFS{ds: []string{"c", "c/a"}, failOn: "hourly"}
	e := engine(f, settingsOf("snap_hourly", "2"), at("2026-04-12 10:30"), snapDS{Name: "c/a", UseTemplate: true}, snapDS{Name: "c/missing", UseTemplate: true}, snapDS{Name: "c/Bad(name)", UseTemplate: true})
	e.run()
	if e.Errors != 4 || !strings.Contains(e.LastError, "boom") {
		t.Errorf("errors=%d last=%s", e.Errors, e.LastError)
	}
}

func TestSnapNextCreation(t *testing.T) {
	c, _ := cronParse("*/15 * * * *")
	from := at("2026-04-12 10:07")
	none := func(string) bool { return false }
	if got := snapNextCreation("hourly", c, from, none, 2); got != at("2026-04-12 10:15").Unix() {
		t.Errorf("due hourly at the next tick: %v", time.Unix(got, 0))
	}
	doneHour := func(k string) bool { return k == "2026-04-12_10-00" }
	if got := snapNextCreation("hourly", c, from, doneHour, 2); got != at("2026-04-12 11:00").Unix() {
		t.Errorf("next hour: %v", time.Unix(got, 0))
	}
	doneDay := func(k string) bool { return k == "2026-04-12" }
	if got := snapNextCreation("daily", c, from, doneDay, 2); got != at("2026-04-13 02:00").Unix() {
		t.Errorf("next daily at the daily hour: %v", time.Unix(got, 0))
	}
	doneYear := func(k string) bool { return k == "2026" }
	if got := snapNextCreation("yearly", c, from, doneYear, 2); got != at("2027-01-01 02:00").Unix() {
		t.Errorf("next yearly: %v", time.Unix(got, 0))
	}
	h, _ := cronParse("0 * * * *")
	if got := snapNextCreation("frequent", h, from, func(k string) bool { return false }, 2); got != at("2026-04-12 11:00").Unix() {
		t.Errorf("frequent follows the cron: %v", time.Unix(got, 0))
	}
}

func fakeNotify(t *testing.T) func() int {
	d := t.TempDir()
	rec := filepath.Join(d, "notify.args")
	for name, body := range map[string]string{"notify": "echo sent >> " + rec, "setsid": "exec \"$@\""} {
		if err := os.WriteFile(filepath.Join(d, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	old := notifyBin
	notifyBin = filepath.Join(d, "notify")
	t.Cleanup(func() { notifyBin = old })
	count := func() int {
		b, _ := os.ReadFile(rec)
		return strings.Count(string(b), "sent")
	}
	return func() int {
		for i := 0; i < 100 && count() == 0; i++ {
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(300 * time.Millisecond)
		return count()
	}
}

func TestSnapFailingSnapshotNotifiesOncePerSixHours(t *testing.T) {
	notified := fakeNotify(t)
	oldTmp := TmpDir
	TmpDir = t.TempDir()
	defer func() { TmpDir = oldTmp }()
	f := &fakeZFS{ds: []string{"c", "c/a"}, failOn: "frequent"}
	s := settingsOf("snap_frequent", "4")
	start := at("2026-04-12 10:00")
	clean := engine(&fakeZFS{ds: []string{"c", "c/a"}}, s, start, snapDS{Name: "c/a", UseTemplate: true})
	clean.notify = snapNotifyLimited
	clean.run()
	clean.notifyErrors()
	if seen, _ := jsonRead(snapNotifyFile()); len(seen) != 0 {
		t.Fatalf("a run without errors notified: %v", seen)
	}
	names := map[string]bool{}
	for i := 0; i < 72; i++ {
		e := engine(f, s, start.Add(time.Duration(i)*5*time.Minute), snapDS{Name: "c/a", UseTemplate: true})
		e.notify = snapNotifyLimited
		e.run()
		if e.Errors == 0 {
			t.Fatalf("tick %d: the snapshot did not fail", i)
		}
		names[e.LastError] = true
		e.notifyErrors()
	}
	if len(names) != 72 {
		t.Fatalf("the failing snapshot names do not change: %d", len(names))
	}
	if n := notified(); n != 1 {
		t.Errorf("%d notifications in 6 hours of failing snapshots every 5 minutes, want 1", n)
	}
	if seen, _ := jsonRead(snapNotifyFile()); len(seen) != 1 {
		t.Errorf("snapshot_notify.json grows per tick: %d keys", len(seen))
	}
}
