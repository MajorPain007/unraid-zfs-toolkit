package zt

import (
	"os"
	"path/filepath"
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
