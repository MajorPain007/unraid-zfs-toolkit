package zt

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSettingsSavedTwiceIsWrittenOnce(t *testing.T) {
	old := SettingsCfg
	SettingsCfg = filepath.Join(t.TempDir(), "settings.cfg")
	defer func() { SettingsCfg = old }()
	changes := map[string]string{"snapshots_enabled": "yes", "snap_hourly": "24", "snap_schedule_custom": "*/15 * * * *"}
	if err := saveSettings(mergeSettings(loadSettings(), changes)); err != nil {
		t.Fatal(err)
	}
	first, err := os.Stat(SettingsCfg)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := os.ReadFile(SettingsCfg)
	time.Sleep(1100 * time.Millisecond)
	if err := saveSettings(mergeSettings(loadSettings(), changes)); err != nil {
		t.Fatal(err)
	}
	second, err := os.Stat(SettingsCfg)
	if err != nil {
		t.Fatal(err)
	}
	if b2, _ := os.ReadFile(SettingsCfg); string(b1) != string(b2) {
		t.Errorf("the same values give different content:\n%s\n%s", b1, b2)
	}
	if !os.SameFile(first, second) || !first.ModTime().Equal(second.ModTime()) {
		t.Errorf("saving unchanged settings rewrote the file")
	}
	if got := loadSettings(); got["snapshots_enabled"] != "yes" || got["snap_hourly"] != "24" {
		t.Errorf("values lost: %v", got)
	}
	changes["snap_hourly"] = "12"
	if err := saveSettings(mergeSettings(loadSettings(), changes)); err != nil {
		t.Fatal(err)
	}
	if loadSettings()["snap_hourly"] != "12" {
		t.Errorf("a real change was not written")
	}
}

func TestSettingNumbersAreClampedToTheirRange(t *testing.T) {
	field := func(job int, preset, key string, i int) func(Settings) int {
		return func(s Settings) int {
			s[key] = preset
			return digitsInt(strings.Fields(cronJobs[job].Expr(s))[i])
		}
	}
	reads := []struct {
		key         string
		def, lo, hi int
		read        func(Settings) int
	}{
		{"cron_hour", 2, 0, 23, field(0, "weekly", "cron_preset", 1)},
		{"cron_minute", 0, 0, 59, field(0, "weekly", "cron_preset", 0)},
		{"cron_weekday", 0, 0, 7, field(0, "weekly", "cron_preset", 4)},
		{"send_schedule_hour", 4, 0, 23, field(2, "daily", "send_schedule_preset", 1)},
		{"snap_daily_hour", 2, 0, 23, snapDailyHour},
		{"snap_min_free_pct", 5, 0, 90, snapMinFreePct},
		{"vm_forceshutdown_wait", 90, 10, 600, func(s Settings) int { return convConfigFrom(s).VMWait }},
		{"buffer_zone", 11, 0, 100, func(s Settings) int { return convConfigFrom(s).Buffer }},
		{"validation_tolerance", 5, 0, 20, func(s Settings) int { return convConfigFrom(s).Tolerance }},
	}
	for _, r := range reads {
		cases := map[string]int{
			"":                               r.def,
			"abc":                            r.def,
			"-1":                             r.def,
			"1.5":                            r.def,
			strconv.Itoa(r.lo):               r.lo,
			strconv.Itoa(r.hi):               r.hi,
			" " + strconv.Itoa(r.hi-1) + " ": r.hi - 1,
			strconv.Itoa(r.hi + 1):           r.hi,
			"99999999999999999999":           r.hi,
		}
		if r.lo > 0 {
			cases[strconv.Itoa(r.lo-1)] = r.lo
		}
		for v, want := range cases {
			if got := r.read(Settings{r.key: v}); got != want {
				t.Errorf("%s=%q: got %d, want %d", r.key, v, got, want)
			}
		}
	}
}
