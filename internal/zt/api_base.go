package zt

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type overviewPart func(s Settings) any

var overviewParts = map[string]overviewPart{}

func registerOverview(name string, fn overviewPart) {
	overviewParts[name] = fn
}

func pluginVersion() string {
	for _, p := range []string{"/var/log/plugins/" + Name + ".plg", "/boot/config/plugins/" + Name + ".plg"} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if m := regexp.MustCompile(`<!ENTITY\s+version\s+"([^"]+)"`).FindSubmatch(b); m != nil {
			return string(m[1])
		}
	}
	return ""
}

type poolInfo struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Alloc    int64  `json:"alloc"`
	Free     int64  `json:"free"`
	Capacity int    `json:"capacity"`
	Health   string `json:"health"`
}

func zfsReady() bool {
	r := zpool("list", "-H", "-o", "name")
	return r.OK() && len(r.Lines()) > 0
}

func listPools() []poolInfo {
	out := []poolInfo{}
	r := zpool("list", "-Hp", "-o", "name,size,alloc,free,capacity,health")
	if !r.OK() {
		return out
	}
	for _, l := range r.Lines() {
		f := strings.Split(l, "\t")
		if len(f) < 6 {
			continue
		}
		out = append(out, poolInfo{
			Name:     f[0],
			Size:     toInt64(f[1]),
			Alloc:    toInt64(f[2]),
			Free:     toInt64(f[3]),
			Capacity: phpInt(strings.TrimSuffix(f[4], "%")),
			Health:   f[5],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func settingsView(s Settings) M {
	out := M{}
	for _, k := range settingKeys {
		out[k] = s.Get(k, "")
		if v, ok := s[k]; ok && v == "" {
			if d := settingDefaults[k]; d == "" {
				out[k] = ""
			}
		}
	}
	for k, v := range s {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out
}

func apiOverview(p M) (M, error) {
	s := loadSettings()
	out := M{
		"version":  pluginVersion(),
		"server":   serverName(),
		"zfs":      zfsReady(),
		"pools":    listPools(),
		"settings": settingsView(s),
		"cron": M{
			"conversion":  cronJobs[0].state(),
			"snapshots":   cronJobs[1].state(),
			"replication": cronJobs[2].state(),
		},
		"array": arrayStarted(),
		"now":   now(),
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, fn := range overviewParts {
		wg.Add(1)
		go func(name string, fn overviewPart) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					out[name] = M{"error": r}
					mu.Unlock()
				}
			}()
			v := fn(s)
			mu.Lock()
			out[name] = v
			mu.Unlock()
		}(name, fn)
	}
	wg.Wait()
	return out, nil
}

var reFreeTarget = regexp.MustCompile(`(?i)^[0-9]+\s*([KMGTP]i?B?|%)$`)

func apiSettingsSave(p M) (M, error) {
	raw, err := pjsonMap(p, "settings")
	if err != nil {
		return nil, err
	}
	changes := map[string]string{}
	for k, v := range raw {
		sv, ok := scalarString(v)
		if !ok {
			continue
		}
		if b, isBool := v.(bool); isBool {
			sv = "no"
			if b {
				sv = "yes"
			}
		}
		changes[k] = sv
	}
	l := settingsLock()
	defer l.Unlock()
	merged := mergeSettings(loadSettings(), changes)
	warnings := []string{}
	for _, c := range []struct{ preset, custom, label string }{
		{"cron_preset", "cron_custom", "Conversion schedule"},
		{"snap_schedule_preset", "snap_schedule_custom", "Snapshot schedule"},
		{"send_schedule_preset", "send_schedule_custom", "Replication schedule"},
	} {
		if merged[c.preset] == "custom" {
			if prob := validCronProblem(merged[c.custom]); prob != "" {
				warnings = append(warnings, c.label+": \""+merged[c.custom]+"\" - "+prob+" The previous schedule is still active.")
			}
		}
	}
	if ft := strings.TrimSpace(merged["snap_free_target"]); ft != "" && !reFreeTarget.MatchString(ft) {
		warnings = append(warnings, "Free-space target: \""+ft+"\" is not understood - use something like 100G or 10%.")
	}
	if err := saveSettings(merged); err != nil {
		return nil, err
	}
	warnings = append(warnings, cronSyncAll()...)
	return M{"settings": settingsView(loadSettings()), "warnings": warnings}, nil
}

func init() {
	registerAction("overview", apiOverview)
	registerAction("settings_save", apiSettingsSave)
}
