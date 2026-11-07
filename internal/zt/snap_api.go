package zt

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const snapHoldTag = "zdc-keep"

func snapDatasetsLock() *fileLock {
	return lockFile(TmpDir+"/snap_datasets.lock", true)
}

func snapAvailable() []M {
	out := []M{}
	r := zfs("list", "-Hp", "-o", "name,type,mountpoint,used,usedbysnapshots", "-t", "filesystem,volume")
	if !r.OK() {
		return out
	}
	for _, l := range r.Lines() {
		f := strings.Split(l, "\t")
		if len(f) < 5 {
			continue
		}
		out = append(out, M{"name": f[0], "type": f[1], "mountpoint": f[2], "used": toInt64(f[3]), "snapused": toInt64(f[4])})
	}
	return out
}

func apiSnapDatasets(p M) (M, error) {
	cfg, has, err := snapLoadDatasets()
	out := M{"datasets": cfg, "exists": has, "available": snapAvailable()}
	if err != nil {
		return nil, apiFail("Could not read the dataset list: "+err.Error()+". Nothing is saved over it until it can be read.", err.Error())
	}
	return out, nil
}

func snapDatasetsFromParam(p M) ([]snapDS, []string, error) {
	raw, err := pjsonList(p, "datasets")
	if err != nil {
		return nil, nil, err
	}
	out := []snapDS{}
	problems := []string{}
	seen := map[string]bool{}
	for _, it := range raw {
		m, ok := it.(M)
		if !ok {
			continue
		}
		d := snapDSFromMap(m)
		if d.Name == "" {
			continue
		}
		if !snapValidDataset(d.Name) {
			problems = append(problems, fmt.Sprintf("\"%s\" is not a valid dataset name.", d.Name))
			continue
		}
		if seen[d.Name] {
			continue
		}
		seen[d.Name] = true
		for _, t := range snapTypes {
			fp := d.field(t)
			if *fp == nil {
				continue
			}
			v := **fp
			if v != "" && (!isDigits(v) || len(v) > 5) {
				problems = append(problems, fmt.Sprintf("%s: %s retention \"%s\" must be a whole number (empty = none).", d.Name, t, v))
			}
		}
		out = append(out, d)
	}
	return out, problems, nil
}

func apiSnapDatasetsSave(p M) (M, error) {
	l, problems, err := snapDatasetsFromParam(p)
	if err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		return nil, apiFail("The dataset list was not saved.", problems...)
	}
	lk := snapDatasetsLock()
	defer lk.Unlock()
	if err := snapSaveDatasets(l); err != nil {
		return nil, err
	}
	snapInvalidate()
	return M{"datasets": l}, nil
}

func snapInfoOf(r snapRow, refer int64) M {
	sn := r.snap()
	return M{
		"name":       r.Name,
		"dataset":    r.dataset(),
		"snapshot":   sn,
		"type":       snapTypeOf(sn),
		"managed":    strings.HasPrefix(sn, "auto-"),
		"checkpoint": strings.HasPrefix(sn, "zdc-send-"),
		"used":       r.Used,
		"refer":      refer,
		"creation":   r.Creation,
		"held":       r.Refs > 0,
	}
}

func apiSnapList(p M) (M, error) {
	ds := strings.TrimSpace(pstr(p, "dataset", ""))
	onlyAuto := toBool(param(p, "only_auto", false))
	if ds != "" && !snapValidDataset(ds) {
		return nil, apiFail("Invalid dataset name")
	}
	scope := []string{}
	if ds != "" {
		scope = append(scope, ds)
	}
	datasets := []M{}
	dr := zfs(append([]string{"list", "-Hp", "-o", "name,used,usedbysnapshots,available,type", "-t", "filesystem,volume", "-r"}, scope...)...)
	if !dr.OK() && ds != "" {
		return nil, apiFail("Dataset not found: " + ds)
	}
	for _, l := range dr.Lines() {
		f := strings.Split(l, "\t")
		if len(f) < 5 {
			continue
		}
		datasets = append(datasets, M{"name": f[0], "used": toInt64(f[1]), "usedbysnapshots": toInt64(f[2]), "available": toInt64(f[3]), "type": f[4]})
	}
	r := zfs(append([]string{"list", "-Hp", "-t", "snapshot", "-o", "name,used,referenced,creation,userrefs", "-s", "creation", "-r"}, scope...)...)
	if !r.OK() && strings.TrimSpace(r.Stdout) == "" && zfsReady() && ds != "" {
		return nil, apiFail("zfs list failed: " + r.Message())
	}
	snaps := []M{}
	var total int64
	for _, l := range r.Lines() {
		f := strings.Split(l, "\t")
		if len(f) < 5 || !strings.Contains(f[0], "@") {
			continue
		}
		refs := 0
		if f[4] != "-" {
			refs = toInt(f[4])
		}
		row := snapRow{Name: f[0], Used: toInt64(f[1]), Creation: toInt64(f[3]), Refs: refs}
		if onlyAuto && !strings.HasPrefix(row.snap(), "auto-") {
			continue
		}
		total += row.Used
		snaps = append(snaps, snapInfoOf(row, toInt64(f[2])))
	}
	sort.SliceStable(snaps, func(i, j int) bool { return toInt64(snaps[i]["creation"]) > toInt64(snaps[j]["creation"]) })
	return M{"snapshots": snaps, "datasets": datasets, "count": len(snaps), "total_used": total}, nil
}

func snapNamesParam(p M) ([]string, error) {
	names := []string{}
	if l, err := pjsonList(p, "snapshots"); err == nil {
		for _, v := range l {
			if s := strings.TrimSpace(str(v)); s != "" {
				names = append(names, s)
			}
		}
	} else if s := strings.TrimSpace(pstr(p, "snapshot", "")); s != "" {
		names = append(names, s)
	}
	if len(names) == 0 {
		return nil, apiFail("No snapshots selected")
	}
	if len(names) > 500 {
		return nil, apiFail("Refusing to change more than 500 snapshots in one request")
	}
	return names, nil
}

func snapUserrefs(name string) int {
	r := zfs("list", "-H", "-o", "userrefs", name)
	if !r.OK() || len(r.Lines()) == 0 {
		return 0
	}
	v := strings.TrimSpace(r.Lines()[0])
	if v == "-" {
		return 0
	}
	return toInt(v)
}

func apiSnapDestroy(p M) (M, error) {
	names, err := snapNamesParam(p)
	if err != nil {
		return nil, err
	}
	forceCheckpoint := toBool(param(p, "force_checkpoint", false))
	results := []M{}
	okCount := 0
	for _, n := range names {
		res := M{"name": n, "ok": false}
		switch {
		case !snapValidSnapshot(n):
			res["error"] = "Invalid snapshot name"
		case strings.Contains(n, "@zdc-send-") && !forceCheckpoint:
			res["error"] = "replication checkpoint - deleting it breaks the incremental chain"
		case snapUserrefs(n) > 0:
			res["error"] = "Snapshot is held - release the hold first"
		default:
			r := zfs("destroy", n)
			if r.OK() {
				res["ok"] = true
				okCount++
			} else {
				res["error"] = r.Message()
			}
		}
		results = append(results, res)
	}
	snapInvalidate()
	snapAudit(fmt.Sprintf("Manager: destroyed %d snapshot(s)%s", okCount, snapNamesSuffix(results)))
	return M{"deleted": okCount, "failed": len(results) - okCount, "results": results}, nil
}

func snapNamesSuffix(results []M) string {
	n := []string{}
	for _, r := range results {
		if toBool(r["ok"]) {
			n = append(n, str(r["name"]))
		}
	}
	if len(n) == 0 {
		return ""
	}
	return ": " + snapShortList(n, 10)
}

func snapAudit(line string) {
	logAppend(snapLogFile(), "["+time.Now().Format("15:04:05")+"] "+line)
}

func snapHoldAction(hold bool) apiAction {
	return func(p M) (M, error) {
		names, err := snapNamesParam(p)
		if err != nil {
			return nil, err
		}
		verb := "release"
		if hold {
			verb = "hold"
		}
		results := []M{}
		okCount := 0
		for _, n := range names {
			res := M{"name": n, "ok": false}
			if !snapValidSnapshot(n) {
				res["error"] = "Invalid snapshot name"
				results = append(results, res)
				continue
			}
			r := zfs(verb, snapHoldTag, n)
			msg := strings.ToLower(r.Message())
			switch {
			case r.OK():
				res["ok"] = true
				okCount++
			case !hold && strings.Contains(msg, "no such tag"):
				res["ok"] = true
				res["note"] = "no plugin hold was set"
				okCount++
			case hold && strings.Contains(msg, "tag already exists"):
				res["ok"] = true
				res["note"] = "already held"
				okCount++
			default:
				res["error"] = r.Message()
			}
			results = append(results, res)
		}
		snapInvalidate()
		return M{"held": hold, "changed": okCount, "failed": len(results) - okCount, "results": results}, nil
	}
}

func snapNewer(name string) ([]M, error) {
	ds := name[:strings.Index(name, "@")]
	r := zfs("list", "-Hp", "-t", "snapshot", "-o", "name,used,referenced,creation,userrefs", "-s", "creation", "-d", "1", ds)
	if !r.OK() {
		return nil, apiFail("Cannot list the snapshots of " + ds + ": " + r.Message())
	}
	found := false
	out := []M{}
	for _, l := range r.Lines() {
		f := strings.Split(l, "\t")
		if len(f) < 5 {
			continue
		}
		if f[0] == name {
			found = true
			continue
		}
		if !found {
			continue
		}
		refs := 0
		if f[4] != "-" {
			refs = toInt(f[4])
		}
		out = append(out, snapInfoOf(snapRow{Name: f[0], Used: toInt64(f[1]), Creation: toInt64(f[3]), Refs: refs}, toInt64(f[2])))
	}
	if !found {
		return nil, apiFail("Snapshot not found on dataset " + ds)
	}
	return out, nil
}

func apiSnapRollbackCheck(p M) (M, error) {
	name := strings.TrimSpace(pstr(p, "snapshot", ""))
	if !snapValidSnapshot(name) {
		return nil, apiFail("Invalid snapshot name")
	}
	newer, err := snapNewer(name)
	if err != nil {
		return nil, err
	}
	held, checkpoints := 0, 0
	for _, n := range newer {
		if toBool(n["held"]) {
			held++
		}
		if toBool(n["checkpoint"]) {
			checkpoints++
		}
	}
	return M{"snapshot": name, "dataset": name[:strings.Index(name, "@")], "newer": newer, "held": held, "checkpoints": checkpoints}, nil
}

func apiSnapRollback(p M) (M, error) {
	name := strings.TrimSpace(pstr(p, "snapshot", ""))
	confirm := strings.TrimSpace(pstr(p, "confirm", ""))
	if !snapValidSnapshot(name) {
		return nil, apiFail("Invalid snapshot name")
	}
	if confirm != name {
		return nil, apiFail("Confirmation does not match the snapshot name")
	}
	ds := name[:strings.Index(name, "@")]
	newer, err := snapNewer(name)
	if err != nil {
		return nil, err
	}
	lost := []string{}
	checkpoints := 0
	heldNames := []string{}
	for _, n := range newer {
		if toBool(n["held"]) {
			heldNames = append(heldNames, str(n["name"]))
		}
	}
	if len(heldNames) > 0 {
		return nil, apiFail("Rollback refused: newer snapshots are held. Release the hold first - otherwise ZFS destroys some newer snapshots and then stops halfway.", heldNames...)
	}
	for _, n := range newer {
		lost = append(lost, str(n["name"]))
		if toBool(n["checkpoint"]) {
			checkpoints++
		}
	}
	r := zfs("rollback", "-r", name)
	if !r.OK() {
		return nil, apiFail("Rollback failed: " + r.Message())
	}
	snapInvalidate()
	msg := "Rolled back " + ds + " to " + name
	if len(lost) > 0 {
		msg += fmt.Sprintf(" (%d newer snapshot(s) destroyed)", len(lost))
	}
	if checkpoints > 0 {
		msg += fmt.Sprintf(". %d replication checkpoint(s) went with them - the next run of those jobs will need a full send.", checkpoints)
	}
	snapAudit("Manager: " + msg)
	return M{"dataset": ds, "destroyed": lost, "message": msg}, nil
}

func apiSnapCreate(p M) (M, error) {
	ds := strings.TrimSpace(pstr(p, "dataset", ""))
	label := strings.TrimSpace(pstr(p, "name", ""))
	recursive := toBool(param(p, "recursive", false))
	if !snapValidDataset(ds) {
		return nil, apiFail("Invalid dataset name")
	}
	if label == "" {
		label = "manual-" + time.Now().Format("2006-01-02_15-04-05")
	}
	if strings.HasPrefix(label, "auto-") || strings.HasPrefix(label, "zdc-send-") {
		return nil, apiFail("Names starting with auto- or zdc-send- are reserved for the scheduler and replication.")
	}
	if !reSnapName.MatchString(label) || len(ds)+1+len(label) > 255 {
		return nil, apiFail("Use letters, digits, space and _ . : - for the snapshot name.")
	}
	name := ds + "@" + label
	if err := (snapZFS{}).create(name, recursive); err != nil {
		return nil, apiFail("Could not create " + name + ": " + err.Error())
	}
	snapInvalidate()
	snapAudit("Manager: created " + name + map[bool]string{true: " (recursive)", false: ""}[recursive])
	return M{"name": name}, nil
}

func apiSnapRun(p M) (M, error) {
	if !zfsReady() {
		return nil, apiFail("ZFS is not available or no pool is imported.")
	}
	if st := snapRunning(); st != nil {
		return nil, apiFail("A snapshot run is already in progress.")
	}
	args := []string{"--now"}
	if toBool(param(p, "all", false)) {
		args = []string{"--all"}
	}
	if err := snapSpawn(args...); err != nil {
		return nil, apiFail("Could not start the snapshot run: " + err.Error())
	}
	return M{"started": true}, nil
}

func apiSnapPreview(p M) (M, error) {
	s := loadSettings()
	if raw, err := pjsonMap(p, "settings"); err == nil {
		changes := map[string]string{}
		for k, v := range raw {
			if sv, ok := scalarString(v); ok {
				changes[k] = sv
			}
		}
		s = mergeSettings(s, changes)
	}
	e := snapNewEngine(s, true)
	if _, ok := p["datasets"]; ok {
		l, problems, err := snapDatasetsFromParam(p)
		if err != nil {
			return nil, err
		}
		if len(problems) > 0 {
			return nil, apiFail("Cannot preview.", problems...)
		}
		e.config, e.hasConfig = l, true
	}
	e.forceNow = toBool(param(p, "now", false))
	lines := []string{}
	e.out = func(l string) { lines = append(lines, l) }
	e.run()
	plan := e.Plan
	if plan == nil {
		plan = []snapPlanItem{}
	}
	return M{
		"plan":     plan,
		"created":  e.Created,
		"pruned":   e.Pruned,
		"freed":    e.Freed,
		"errors":   e.Errors,
		"warnings": snapLastN(e.Warnings, 50),
		"lines":    lines,
		"now":      e.now.Unix(),
	}, nil
}

func apiSnapSchedule(p M) (M, error) {
	expr := strings.TrimSpace(pstr(p, "expr", ""))
	if prob := validCronProblem(expr); prob != "" {
		return M{"problem": prob, "next": []int64{}}, nil
	}
	c, err := cronParse(expr)
	if err != nil {
		return M{"problem": err.Error(), "next": []int64{}}, nil
	}
	return M{"problem": "", "next": c.upcoming(now(), 5)}, nil
}

func apiSnapLog(p M) (M, error) {
	n := count(param(p, "lines", 400), 5000)
	if n == 0 {
		n = 400
	}
	return M{"log": tail(snapLogFile(), n, 2*1048576), "size": fileMtime(snapLogFile())}, nil
}

func snapRunning() M {
	m, ok := jsonRead(snapRunningFile())
	if !ok || !pidAlive(m["pid"]) {
		return nil
	}
	return m
}

func init() {
	registerAction("snap_datasets", apiSnapDatasets)
	registerAction("snap_datasets_save", apiSnapDatasetsSave)
	registerAction("snap_list", apiSnapList)
	registerAction("snap_destroy", apiSnapDestroy)
	registerAction("snap_hold", snapHoldAction(true))
	registerAction("snap_release", snapHoldAction(false))
	registerAction("snap_rollback_check", apiSnapRollbackCheck)
	registerAction("snap_rollback", apiSnapRollback)
	registerAction("snap_create", apiSnapCreate)
	registerAction("snap_run", apiSnapRun)
	registerAction("snap_preview", apiSnapPreview)
	registerAction("snap_schedule", apiSnapSchedule)
	registerAction("snap_log", apiSnapLog)
}
