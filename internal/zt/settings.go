package zt

import (
	"os"
	"regexp"
	"strings"
)

var settingKeys = []string{
	"dry_run", "cleanup", "replace_spaces", "send_notifications", "show_in_menu", "ui_layout",
	"should_process_containers", "appdata_pool", "appdata_dataset",
	"should_process_vms", "vm_pool", "vm_dataset", "vm_forceshutdown_wait",
	"buffer_zone", "validation_tolerance", "extra_datasets",

	"cron_enabled", "cron_preset", "cron_hour", "cron_minute", "cron_weekday", "cron_custom",

	"snapshots_enabled", "snap_frequent", "snap_hourly", "snap_daily", "snap_weekly",
	"snap_monthly", "snap_yearly", "snap_daily_hour", "snap_min_free_pct",
	"snap_schedule_preset", "snap_schedule_custom",

	"snap_age_frequent", "snap_age_hourly", "snap_age_daily", "snap_age_weekly",
	"snap_age_monthly", "snap_age_yearly", "snap_free_target",

	"send_enabled", "send_schedule_preset", "send_schedule_hour", "send_schedule_custom",
}

var settingDefaults = map[string]string{
	"dry_run":                   "yes",
	"cleanup":                   "yes",
	"replace_spaces":            "no",
	"send_notifications":        "yes",
	"show_in_menu":              "yes",
	"ui_layout":                 "cards",
	"should_process_containers": "yes",
	"appdata_pool":              "cache",
	"appdata_dataset":           "appdata",
	"should_process_vms":        "no",
	"vm_pool":                   "cache",
	"vm_dataset":                "domains",
	"vm_forceshutdown_wait":     "90",
	"buffer_zone":               "11",
	"validation_tolerance":      "5",
	"extra_datasets":            "",
	"cron_enabled":              "no",
	"cron_preset":               "daily",
	"cron_hour":                 "2",
	"cron_minute":               "0",
	"cron_weekday":              "0",
	"cron_custom":               "0 2 * * *",
	"snapshots_enabled":         "no",
	"snap_frequent":             "0",
	"snap_hourly":               "24",
	"snap_daily":                "30",
	"snap_weekly":               "4",
	"snap_monthly":              "3",
	"snap_yearly":               "0",
	"snap_daily_hour":           "2",
	"snap_min_free_pct":         "5",
	"snap_schedule_preset":      "15min",
	"snap_schedule_custom":      "*/15 * * * *",
	"snap_free_target":          "",
	"snap_age_frequent":         "0",
	"snap_age_hourly":           "0",
	"snap_age_daily":            "0",
	"snap_age_weekly":           "0",
	"snap_age_monthly":          "0",
	"snap_age_yearly":           "0",
	"send_enabled":              "no",
	"send_schedule_preset":      "daily",
	"send_schedule_hour":        "4",
	"send_schedule_custom":      "0 4 * * *",
}

type Settings map[string]string

var reSettingKey = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func parseSettings(data string) Settings {
	s := Settings{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimRight(line, "\r")
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || !strings.Contains(t, "=") {
			continue
		}
		i := strings.Index(line, "=")
		k := strings.TrimSpace(line[:i])
		v := strings.TrimSpace(line[i+1:])
		if !reSettingKey.MatchString(k) {
			continue
		}
		s[k] = v
	}
	return s
}

func loadSettings() Settings {
	b, err := os.ReadFile(SettingsCfg)
	if err != nil {
		return Settings{}
	}
	return parseSettings(string(b))
}

func (s Settings) Get(key, def string) string {
	if v, ok := s[key]; ok && v != "" {
		return v
	}
	if def == "" {
		if d, ok := settingDefaults[key]; ok {
			return d
		}
	}
	return def
}

func (s Settings) Raw(key string) string {
	return s[key]
}

func (s Settings) Yes(key string) bool {
	return yes(s.Get(key, ""))
}

func yes(v string) bool {
	return strings.EqualFold(strings.TrimSpace(v), "yes")
}

func (s Settings) Render() string {
	var b strings.Builder
	b.WriteString("# ZFS Toolkit settings\n\n")
	for _, k := range settingKeys {
		b.WriteString(k + "=" + s[k] + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

func saveSettings(s Settings) error {
	if err := os.MkdirAll(dirOf(SettingsCfg), 0o755); err != nil {
		return err
	}
	if !fileWrite(SettingsCfg, []byte(s.Render()), 0o644) {
		return apiFailf("Cannot write %s", SettingsCfg)
	}
	return nil
}

func mergeSettings(old Settings, changes map[string]string) Settings {
	out := Settings{}
	for _, k := range settingKeys {
		if v, ok := changes[k]; ok {
			out[k] = strings.NewReplacer("\r", "", "\n", "").Replace(v)
		} else {
			out[k] = old[k]
		}
	}
	return out
}

func settingsLock() *fileLock {
	return lockFile(TmpDir+"/settings.lock", true)
}
