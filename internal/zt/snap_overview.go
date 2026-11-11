package zt

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func snapCacheFile() string { return TmpDir + "/snapshot_overview.json" }

func snapInvalidate() {
	os.Remove(snapCacheFile())
}

type snapStats struct {
	Stamp   int64            `json:"stamp"`
	Key     string           `json:"key"`
	Auto    int              `json:"auto"`
	Total   int              `json:"total"`
	Held    int              `json:"held"`
	Used    int64            `json:"used"`
	Newest  int64            `json:"newest"`
	Keys    map[string]int64 `json:"keys"`
	ByType  map[string]int   `json:"by_type"`
	Missing []string         `json:"missing"`
}

func snapCovered(cfg []snapDS, ds string) bool {
	for _, d := range cfg {
		if ds == d.Name || (d.Recursive && strings.HasPrefix(ds, d.Name+"/")) {
			return true
		}
	}
	return false
}

func snapCollectStats(cfg []snapDS, key string) snapStats {
	st := snapStats{Stamp: now(), Key: key, Keys: map[string]int64{}, ByType: map[string]int{}, Missing: []string{}}
	pools := []string{}
	seen := map[string]bool{}
	top := map[string]bool{}
	for _, d := range cfg {
		top[d.Name] = true
		if p := snapPoolOf(d.Name); !seen[p] {
			seen[p] = true
			pools = append(pools, p)
		}
	}
	if len(pools) == 0 {
		return st
	}
	dr := zfs(append([]string{"list", "-Hp", "-o", "name,usedbysnapshots", "-t", "filesystem,volume", "-r"}, pools...)...)
	have := map[string]bool{}
	for _, l := range dr.Lines() {
		f := strings.Split(l, "\t")
		if len(f) < 2 {
			continue
		}
		have[f[0]] = true
		if snapCovered(cfg, f[0]) {
			st.Used += toInt64(f[1])
		}
	}
	for _, d := range cfg {
		if !have[d.Name] {
			st.Missing = append(st.Missing, d.Name)
		}
	}
	rows, _ := (snapZFS{}).snapshots(pools)
	for _, r := range rows {
		ds := r.dataset()
		if !snapCovered(cfg, ds) {
			continue
		}
		st.Total++
		if r.Refs > 0 {
			st.Held++
		}
		typ := snapTypeOf(r.snap())
		if typ == "" {
			continue
		}
		st.Auto++
		st.ByType[typ]++
		if r.Creation > st.Newest {
			st.Newest = r.Creation
		}
		if top[ds] {
			k := strings.TrimPrefix(r.snap(), "auto-"+typ+"-")
			if r.Creation >= st.Keys[typ+"|"+k] {
				st.Keys[typ+"|"+k] = r.Creation
			}
		}
	}
	return st
}

func snapStatsCached(cfg []snapDS) snapStats {
	key := fmt.Sprintf("%d|%d|%d", fileMtime(snapDatasetsFile()), fileMtime(snapStatusFile()), len(cfg))
	var st snapStats
	if jsonReadInto(snapCacheFile(), &st) && st.Key == key && now()-st.Stamp < 60 && now() >= st.Stamp {
		return st
	}
	st = snapCollectStats(cfg, key)
	jsonWrite(snapCacheFile(), st, 0o644)
	return st
}

func snapPeriodStart(typ string, t time.Time) time.Time {
	y, m, d := t.Date()
	loc := t.Location()
	switch typ {
	case "frequent":
		return t.Truncate(time.Minute)
	case "hourly":
		return time.Date(y, m, d, t.Hour(), 0, 0, 0, loc)
	case "daily":
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	case "weekly":
		wd := (int(t.Weekday()) + 6) % 7
		return time.Date(y, m, d-wd, 0, 0, 0, 0, loc)
	case "monthly":
		return time.Date(y, m, 1, 0, 0, 0, 0, loc)
	case "yearly":
		return time.Date(y, 1, 1, 0, 0, 0, 0, loc)
	}
	return t
}

func snapPeriodNext(typ string, t time.Time) time.Time {
	s := snapPeriodStart(typ, t)
	y, m, d := s.Date()
	loc := s.Location()
	switch typ {
	case "frequent":
		return s.Add(time.Minute)
	case "hourly":
		return time.Date(y, m, d, s.Hour()+1, 0, 0, 0, loc)
	case "daily":
		return time.Date(y, m, d+1, 0, 0, 0, 0, loc)
	case "weekly":
		return time.Date(y, m, d+7, 0, 0, 0, 0, loc)
	case "monthly":
		return time.Date(y, m+1, 1, 0, 0, 0, 0, loc)
	case "yearly":
		return time.Date(y+1, 1, 1, 0, 0, 0, 0, loc)
	}
	return t
}

func snapNextCreation(typ string, c *cronExpr, from time.Time, done func(key string) bool, dailyHour int) int64 {
	t := from.Unix()
	for i := 0; i < 1500; i++ {
		nt, ok := c.next(t)
		if !ok {
			return 0
		}
		tm := time.Unix(nt, 0).In(from.Location())
		if done(snapPeriodKey(typ, tm)) {
			t = snapPeriodNext(typ, tm).Unix() - 1
			continue
		}
		if snapGated(typ) && tm.Hour() < dailyHour {
			y, m, d := tm.Date()
			t = time.Date(y, m, d, dailyHour, 0, 0, 0, tm.Location()).Unix() - 1
			continue
		}
		return nt
	}
	return 0
}

func snapOverview(s Settings) any {
	enabled := s.Yes("snapshots_enabled")
	cfg, has, cfgErr := snapLoadDatasets()
	out := M{"enabled": enabled, "configured": has, "datasets": len(cfg), "running": false, "count": 0}
	var last *snapStatus
	var ls snapStatus
	if jsonReadInto(snapStatusFile(), &ls) {
		last = &ls
		out["last"] = ls
	} else if b, err := os.ReadFile(snapLastFile()); err == nil {
		out["last"] = M{"last_run": strings.TrimSpace(string(b))}
	}
	if r := snapRunning(); r != nil {
		out["running"] = true
		out["run"] = r
	}
	st := snapStats{Keys: map[string]int64{}, ByType: map[string]int{}, Missing: []string{}}
	if cfgErr == nil && len(cfg) > 0 {
		st = snapStatsCached(cfg)
	}
	out["count"] = st.Auto
	out["auto"] = st.Auto
	out["total"] = st.Total
	out["held"] = st.Held
	out["used"] = st.Used
	out["newest"] = st.Newest
	out["by_type"] = st.ByType
	out["missing"] = st.Missing

	active := map[string]bool{}
	for _, d := range cfg {
		keeps, _ := d.keeps(s)
		for t, n := range keeps {
			if n > 0 {
				active[t] = true
			}
		}
	}
	nowT := time.Now()
	next := M{}
	var tick int64
	expr := cronJobs[1].Expr(s)
	if c, err := cronParse(expr); err == nil && enabled {
		if t, ok := c.next(nowT.Unix()); ok {
			tick = t
		}
		cur := map[string]bool{}
		for k := range st.Keys {
			cur[k] = true
		}
		for _, t := range snapTypes {
			if !active[t] {
				continue
			}
			typ := t
			next[t] = snapNextCreation(t, c, nowT, func(key string) bool { return cur[typ+"|"+key] }, snapDailyHour(s))
		}
	}
	out["next"] = next
	out["tick"] = tick
	out["expr"] = expr

	low := []M{}
	minPct := snapMinFreePct(s)
	target := strings.TrimSpace(s.Get("snap_free_target", ""))
	seen := map[string]bool{}
	for _, d := range cfg {
		seen[snapPoolOf(d.Name)] = true
	}
	for _, p := range listPools() {
		if !seen[p.Name] {
			continue
		}
		if minPct > 0 && p.Capacity > 100-minPct {
			low = append(low, M{"pool": p.Name, "capacity": p.Capacity, "reason": "full"})
		} else if tb, ok := snapTargetBytes(target, p.Size); ok && tb > 0 && p.Free < tb {
			low = append(low, M{"pool": p.Name, "capacity": p.Capacity, "reason": "target", "free": p.Free, "target": tb})
		}
	}
	out["low"] = low

	cron := cronJobs[1].state()
	out["status"] = snapStatusOf(enabled, cfgErr, cfg, cron, last, out, low, st, next)
	return out
}

func snapNextSummary(next M) string {
	best := ""
	var bt int64
	for _, t := range snapTypes {
		v := toInt64(next[t])
		if v > 0 && (bt == 0 || v < bt) {
			bt, best = v, t
		}
	}
	if bt == 0 {
		return ""
	}
	d := bt - now()
	if d < 60 {
		return "next " + best + " snapshot within a minute"
	}
	return "next " + best + " in " + humanDuration(float64(d))
}

func snapAgo(ts int64) string {
	d := now() - ts
	if d < 60 {
		return "just now"
	}
	return humanDuration(float64(d)) + " ago"
}

func snapStatusOf(enabled bool, cfgErr error, cfg []snapDS, cron cronState, last *snapStatus, out M, low []M, st snapStats, next M) M {
	n := len(cfg)
	if !enabled {
		sub := "Turn them on in the Snapshots tab."
		if n > 0 {
			sub = fmt.Sprintf("%d dataset(s) configured, nothing is taken automatically.", n)
		}
		return M{"level": "idle", "text": "Automatic snapshots are off", "sub": sub}
	}
	if toBool(out["running"], false) {
		return M{"level": "run", "text": "Taking snapshots", "sub": "The snapshot manager is running."}
	}
	if cfgErr != nil {
		return M{"level": "err", "text": "The snapshot dataset list cannot be read", "sub": cfgErr.Error()}
	}
	if n == 0 {
		return M{"level": "warn", "text": "No dataset is selected for snapshots", "sub": "Add a dataset in the Snapshots tab."}
	}
	if !cron.Live {
		return M{"level": "err", "text": "The snapshot schedule is not installed", "sub": "No cron entry is active. Save the snapshot settings again."}
	}
	if len(st.Missing) > 0 {
		return M{"level": "err", "text": fmt.Sprintf("%d snapshot dataset(s) do not exist", len(st.Missing)), "sub": strings.Join(st.Missing, ", ")}
	}
	if last != nil && last.Errors > 0 {
		return M{"level": "err", "text": fmt.Sprintf("Last snapshot run had %d error(s)", last.Errors), "sub": last.LastError}
	}
	if len(low) > 0 {
		p := low[0]
		if p["reason"] == "full" {
			return M{"level": "warn", "text": fmt.Sprintf("Pool %s is %d%% full", p["pool"], p["capacity"]), "sub": "Snapshot retention may be keeping too much data."}
		}
		return M{"level": "warn", "text": fmt.Sprintf("Pool %s is below its free-space target", p["pool"]), "sub": "Every snapshot run prunes the oldest auto snapshots until the target is met."}
	}
	parts := []string{}
	if last != nil && last.Ended > 0 {
		parts = append(parts, "last run "+snapAgo(last.Ended))
		if expr := str(out["expr"]); expr != "" {
			if c, err := cronParse(expr); err == nil {
				if pt, ok := c.prev(now() - 300); ok && last.Ended < pt-60 && fileMtime(cronJobs[1].file()) < pt {
					return M{"level": "warn", "text": "Snapshot runs are overdue", "sub": "The last run was " + snapAgo(last.Ended) + ", the schedule expected one at " + time.Unix(pt, 0).Format("15:04") + "."}
				}
			}
		}
	} else {
		parts = append(parts, "no run yet")
	}
	if s := snapNextSummary(next); s != "" {
		parts = append(parts, s)
	}
	return M{"level": "ok", "text": fmt.Sprintf("Snapshots protect %d dataset(s)", n), "sub": strings.Join(parts, " · ")}
}

func init() {
	registerOverview("snapshots", snapOverview)
}
