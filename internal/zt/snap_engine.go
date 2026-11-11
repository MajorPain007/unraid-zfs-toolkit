package zt

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type snapRow struct {
	Name     string
	Creation int64
	Used     int64
	Refs     int
}

func (r snapRow) dataset() string {
	if i := strings.Index(r.Name, "@"); i >= 0 {
		return r.Name[:i]
	}
	return r.Name
}

func (r snapRow) snap() string {
	if i := strings.Index(r.Name, "@"); i >= 0 {
		return r.Name[i+1:]
	}
	return ""
}

type snapBackend interface {
	ready() bool
	datasets() ([]string, error)
	snapshots(pools []string) ([]snapRow, error)
	create(name string, recursive bool) error
	destroy(name string, recursive bool) error
	poolSpace(pool string) (size, free int64, capacity int, ok bool)
	sync(pool string)
}

type snapPlanItem struct {
	Op       string `json:"op"`
	Name     string `json:"name"`
	Dataset  string `json:"dataset"`
	Type     string `json:"type"`
	Used     int64  `json:"used"`
	Creation int64  `json:"creation"`
	Reason   string `json:"reason"`
}

type snapEngine struct {
	s        Settings
	be       snapBackend
	now      time.Time
	dry      bool
	forceNow bool
	forceAll bool
	out      func(line string)
	notify   func(key, subject, text, level string, every time.Duration)

	config    []snapDS
	hasConfig bool
	configErr string

	rows      map[string][]snapRow
	available []string

	Created   int
	Pruned    int
	Freed     int64
	Errors    int
	LastError string
	Warnings  []string
	DS        int
	Plan      []snapPlanItem
}

func snapPeriodKey(typ string, t time.Time) string {
	switch typ {
	case "frequent":
		return t.Format("2006-01-02_15-04")
	case "hourly":
		return t.Format("2006-01-02_15") + "-00"
	case "daily":
		return t.Format("2006-01-02")
	case "weekly":
		y, w := t.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", y, w)
	case "monthly":
		return t.Format("2006-01")
	case "yearly":
		return t.Format("2006")
	}
	return ""
}

func snapGated(typ string) bool {
	return typ == "daily" || typ == "weekly" || typ == "monthly" || typ == "yearly"
}

func (e *snapEngine) say(prefix, msg string) {
	if e.out == nil {
		return
	}
	tag := ""
	if e.dry {
		tag = "[dry-run] "
	}
	e.out("[" + e.now.Format("15:04:05") + "] " + tag + prefix + msg)
}

func (e *snapEngine) logf(format string, a ...any) { e.say("", fmt.Sprintf(format, a...)) }
func (e *snapEngine) okf(format string, a ...any)  { e.say("OK: ", fmt.Sprintf(format, a...)) }

func (e *snapEngine) warnf(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	e.Warnings = append(e.Warnings, msg)
	e.say("WARNING: ", msg)
}

func (e *snapEngine) errf(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	e.LastError = msg
	e.Errors++
	e.say("ERROR: ", msg)
}

func (e *snapEngine) sendNotify(key, subject, text, level string, every time.Duration) {
	if e.dry || e.notify == nil {
		return
	}
	e.notify(key, subject, text, level, every)
}

func (e *snapEngine) notifyErrors() {
	if e.Errors > 0 {
		e.sendNotify("errors", fmt.Sprintf("ZFS Snapshots - %d error(s)", e.Errors),
			fmt.Sprintf("Snapshot run finished with %d error(s). Last: %s", e.Errors, e.LastError), "warning", 6*time.Hour)
	}
}

func (e *snapEngine) plan(op, name, typ, reason string, used, creation int64) {
	e.Plan = append(e.Plan, snapPlanItem{Op: op, Name: name, Dataset: snapRow{Name: name}.dataset(), Type: typ, Used: used, Creation: creation, Reason: reason})
}

func (e *snapEngine) descendants(parent string) []string {
	out := []string{}
	for _, d := range e.available {
		if strings.HasPrefix(d, parent+"/") {
			out = append(out, d)
		}
	}
	return out
}

func (e *snapEngine) addRow(ds, snap string, recursive bool) {
	targets := []string{ds}
	if recursive {
		targets = append(targets, e.descendants(ds)...)
	}
	for _, t := range targets {
		name := t + "@" + snap
		if e.hasRow(t, name) {
			continue
		}
		e.rows[t] = append(e.rows[t], snapRow{Name: name, Creation: e.now.Unix()})
	}
}

func (e *snapEngine) hasRow(ds, name string) bool {
	for _, r := range e.rows[ds] {
		if r.Name == name {
			return true
		}
	}
	return false
}

func (e *snapEngine) dropRow(ds, snap string, recursive bool) {
	targets := []string{ds}
	if recursive {
		targets = append(targets, e.descendants(ds)...)
	}
	for _, t := range targets {
		name := t + "@" + snap
		keep := e.rows[t][:0]
		for _, r := range e.rows[t] {
			if r.Name != name {
				keep = append(keep, r)
			}
		}
		e.rows[t] = keep
	}
}

func (e *snapEngine) create(d snapDS, typ string) {
	snap := "auto-" + typ + "-" + snapPeriodKey(typ, e.now)
	name := d.Name + "@" + snap
	if e.hasRow(d.Name, name) {
		return
	}
	if e.dry {
		e.logf("Would create %s", name)
		e.Created++
		e.plan("create", name, typ, "", 0, e.now.Unix())
		e.addRow(d.Name, snap, d.Recursive)
		return
	}
	if err := e.be.create(name, d.Recursive); err != nil {
		e.errf("Failed to create %s: %s", name, err.Error())
		e.plan("fail", name, typ, err.Error(), 0, e.now.Unix())
		return
	}
	e.okf("Created %s", name)
	e.Created++
	e.plan("create", name, typ, "", 0, e.now.Unix())
	e.addRow(d.Name, snap, d.Recursive)
}

func (e *snapEngine) destroy(r snapRow, recursive bool, typ, reason string) bool {
	if e.dry {
		e.logf("Would prune %s (%s)", r.Name, humanBytes(float64(r.Used)))
		e.Pruned++
		e.Freed += r.Used
		e.plan("prune", r.Name, typ, reason, r.Used, r.Creation)
		e.dropRow(r.dataset(), r.snap(), recursive)
		return true
	}
	if err := e.be.destroy(r.Name, recursive); err != nil {
		e.warnf("Could not prune %s: %s", r.Name, err.Error())
		e.plan("fail", r.Name, typ, err.Error(), r.Used, r.Creation)
		return false
	}
	e.logf("Pruned %s (%s)", r.Name, humanBytes(float64(r.Used)))
	e.Pruned++
	e.Freed += r.Used
	e.plan("prune", r.Name, typ, reason, r.Used, r.Creation)
	e.dropRow(r.dataset(), r.snap(), recursive)
	return true
}

func snapPruneSelect(rows []snapRow, typ string, keep, maxAgeDays int, now int64) (del []snapRow, reasons []string, held []snapRow) {
	if keep == 0 && maxAgeDays == 0 {
		return
	}
	prefix := "auto-" + typ + "-"
	own := []snapRow{}
	for _, r := range rows {
		if strings.HasPrefix(r.snap(), prefix) {
			own = append(own, r)
		}
	}
	sort.SliceStable(own, func(i, j int) bool { return own[i].Creation < own[j].Creation })
	total := len(own)
	var cutoff int64
	if maxAgeDays > 0 {
		cutoff = now - int64(maxAgeDays)*86400
	}
	for i, r := range own {
		reason := ""
		if keep > 0 && i < total-keep {
			reason = "count"
		}
		if cutoff > 0 && r.Creation < cutoff && reason == "" {
			reason = "age"
		}
		if keep > 0 && i == total-1 {
			reason = ""
		}
		if reason == "" {
			continue
		}
		if r.Refs > 0 {
			held = append(held, r)
			continue
		}
		del = append(del, r)
		reasons = append(reasons, reason)
	}
	return
}

func (e *snapEngine) prune(d snapDS, typ string, keep, maxAge int) {
	del, reasons, held := snapPruneSelect(e.rows[d.Name], typ, keep, maxAge, e.now.Unix())
	for _, r := range held {
		e.warnf("Keeping held snapshot %s (userrefs=%d) - release the hold to allow pruning.", r.Name, r.Refs)
		e.plan("held", r.Name, typ, "held", r.Used, r.Creation)
	}
	for i, r := range del {
		e.destroy(r, d.Recursive, typ, reasons[i])
	}
}

func (e *snapEngine) process(d snapDS, keeps, ages map[string]int) {
	hour := e.now.Hour()
	daily := snapDailyHour(e.s)
	for _, typ := range snapTypes {
		want := keeps[typ] > 0 || (typ == "frequent" && e.forceAll)
		if !want {
			continue
		}
		if snapGated(typ) && !e.forceNow && hour < daily {
			continue
		}
		e.create(d, typ)
	}
	for _, typ := range snapTypes {
		e.prune(d, typ, keeps[typ], ages[typ])
	}
}

func snapFreeCandidates(rows []snapRow) []snapRow {
	sorted := append([]snapRow{}, rows...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Creation < sorted[j].Creation })
	newest := map[string]int{}
	for i, r := range sorted {
		typ := snapTypeOf(r.snap())
		if typ == "" {
			continue
		}
		newest[r.dataset()+"@"+typ] = i
	}
	out := []snapRow{}
	for i, r := range sorted {
		typ := snapTypeOf(r.snap())
		if typ == "" || r.Refs > 0 || newest[r.dataset()+"@"+typ] == i {
			continue
		}
		out = append(out, r)
	}
	return out
}

func (e *snapEngine) freeSpace(pool string) {
	spec := strings.TrimSpace(e.s.Get("snap_free_target", ""))
	if spec == "" {
		return
	}
	size, free, _, ok := e.be.poolSpace(pool)
	if !ok {
		e.warnf("Cannot read free space for pool '%s'.", pool)
		return
	}
	target, ok := snapTargetBytes(spec, size)
	if !ok {
		e.warnf("Cannot parse snap_free_target='%s' (use e.g. 100G or 10%%).", spec)
		return
	}
	if target <= 0 || free >= target {
		return
	}
	e.warnf("Pool '%s': %s free, target is %s - pruning oldest auto snapshots.", pool, humanBytes(float64(free)), humanBytes(float64(target)))
	all := []snapRow{}
	for ds, rows := range e.rows {
		if snapPoolOf(ds) == pool {
			all = append(all, rows...)
		}
	}
	cands := snapFreeCandidates(all)
	if len(cands) == 0 {
		e.warnf("Pool '%s': no prunable auto snapshots left, still below target.", pool)
		e.sendNotify("space-none-"+pool, "ZFS pool "+pool+" low on space",
			"Free space is below the configured target and no further auto snapshots can be pruned.", "alert", 12*time.Hour)
		return
	}
	deleted := 0
	expected := free
	for _, r := range cands {
		if !e.destroy(r, false, snapTypeOf(r.snap()), "space") {
			continue
		}
		deleted++
		expected += r.Used
		if e.dry {
			free = expected
		} else {
			e.be.sync(pool)
			_, measured, _, ok := e.be.poolSpace(pool)
			if !ok {
				break
			}
			free = measured
			if expected > free {
				free = expected
			}
		}
		if free >= target {
			break
		}
		if deleted >= 500 {
			e.warnf("Pool '%s': stopped after 500 deletions in one run.", pool)
			break
		}
	}
	if free < target {
		e.warnf("Pool '%s': still %s free after pruning %d snapshot(s), target %s.", pool, humanBytes(float64(free)), deleted, humanBytes(float64(target)))
		e.sendNotify("space-short-"+pool, "ZFS pool "+pool+" still below free-space target",
			fmt.Sprintf("Pruned %d snapshot(s), free space is %s, target %s.", deleted, humanBytes(float64(free)), humanBytes(float64(target))), "warning", 12*time.Hour)
	} else {
		e.okf("Pool '%s': free space target reached after pruning %d snapshot(s).", pool, deleted)
	}
}

func (e *snapEngine) capacityCheck(pool string) {
	pct := snapMinFreePct(e.s)
	if pct <= 0 {
		return
	}
	_, _, capacity, ok := e.be.poolSpace(pool)
	if !ok || capacity <= 100-pct {
		return
	}
	e.warnf("Pool '%s' is %d%% full (threshold %d%% free). Review your retention settings.", pool, capacity, pct)
	e.sendNotify("full-"+pool, fmt.Sprintf("ZFS pool %s is %d%% full", pool, capacity),
		fmt.Sprintf("Snapshot retention may be keeping too much data. Threshold: %d%% free.", pct), "warning", 24*time.Hour)
}

var errSnapNoZFS = errors.New("no zfs")

func (e *snapEngine) run() error {
	if e.rows == nil {
		e.rows = map[string][]snapRow{}
	}
	if !e.be.ready() {
		e.warnf("ZFS is not available or no pool is imported (array not started?). Nothing to do.")
		return errSnapNoZFS
	}
	if e.configErr != "" {
		e.errf("%s", e.configErr)
		return errors.New(e.configErr)
	}
	if !e.hasConfig {
		e.warnf("No dataset config found (%s). Add a dataset in the GUI.", snapDatasetsFile())
		return nil
	}
	avail, err := e.be.datasets()
	if err != nil {
		e.errf("Cannot list ZFS datasets: %s", err.Error())
		return err
	}
	e.available = avail
	have := map[string]bool{}
	for _, a := range avail {
		have[a] = true
	}
	type item struct {
		d     snapDS
		keeps map[string]int
	}
	items := []item{}
	pools := []string{}
	seenPool := map[string]bool{}
	seen := map[string]bool{}
	for _, d := range snapSortedNames(e.config) {
		if seen[d.Name] {
			continue
		}
		seen[d.Name] = true
		if !validDataset(d.Name) {
			e.errf("Skipping invalid dataset name from config: '%s'", d.Name)
			continue
		}
		if !have[d.Name] {
			e.errf("Dataset does not exist: %s", d.Name)
			e.errf("Available ZFS datasets: %s", snapShortList(avail, 40))
			continue
		}
		keeps, warns := d.keeps(e.s)
		for _, w := range warns {
			e.warnf("%s", w)
		}
		items = append(items, item{d, keeps})
		if p := snapPoolOf(d.Name); !seenPool[p] {
			seenPool[p] = true
			pools = append(pools, p)
		}
	}
	sort.Strings(pools)
	if len(pools) > 0 {
		rows, err := e.be.snapshots(pools)
		if err != nil {
			e.errf("Cannot list snapshots: %s", err.Error())
			return err
		}
		for _, r := range rows {
			e.rows[r.dataset()] = append(e.rows[r.dataset()], r)
		}
	}
	ages := map[string]int{}
	for _, t := range snapTypes {
		n, w := snapGlobal(e.s, "snap_age_"+t)
		if w != "" {
			e.warnf("%s", w)
		}
		ages[t] = n
	}
	for _, it := range items {
		e.logf("Processing dataset: %s (recursive=%s)", it.d.Name, map[bool]string{true: "1", false: "0"}[it.d.Recursive])
		e.process(it.d, it.keeps, ages)
		e.DS++
	}
	for _, p := range pools {
		e.freeSpace(p)
		e.capacityCheck(p)
	}
	if e.DS == 0 {
		e.warnf("No datasets configured.")
	} else if e.Created == 0 && e.Pruned == 0 && e.Errors == 0 {
		e.logf("Nothing due - all snapshots for the current period already exist.")
	}
	return nil
}

func snapShortList(l []string, max int) string {
	if len(l) <= max {
		return strings.Join(l, " ")
	}
	return strings.Join(l[:max], " ") + fmt.Sprintf(" ... (%d more)", len(l)-max)
}
