package zt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func sendSpawn(args ...string) error {
	cmd := exec.Command(SelfBin, args...)
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func sendJobsView(jobs []sendJob) []sendJob {
	if jobs == nil {
		return []sendJob{}
	}
	return jobs
}

func sendStateView(st sendStatus) M {
	return M{
		"running":    st.Running,
		"pid":        st.PID,
		"progress":   st.Progress,
		"current_id": st.CurrentID,
		"queue":      st.Queue,
		"dry_run":    st.DryRun == 1,
		"started":    st.Started,
		"ended":      st.Ended,
		"jobs":       st.Jobs,
		"last_run":   st.LastRun,
		"errors":     st.Errors,
		"last_error": st.LastError,
	}
}

func apiSendJobs(p M) (M, error) {
	jobs, err := sendLoadJobs()
	if err != nil {
		return nil, err
	}
	st := sendReadStatus()
	return M{
		"jobs":   sendJobsView(jobs),
		"state":  sendStateView(st),
		"key":    sendKeyInfo(),
		"has_pv": false,
	}, nil
}

func apiSendSave(p M) (M, error) {
	raw, err := pjsonList(p, "jobs")
	if err != nil {
		if m, merr := pjsonMap(p, "jobs"); merr == nil {
			l, _ := m["jobs"].([]any)
			raw = l
		} else {
			return nil, err
		}
	}
	if _, err := sendLoadJobs(); err != nil {
		return nil, err
	}
	jobs, warnings := sendNormalizeList(raw)
	if err := sendWriteJobs(jobs); err != nil {
		return nil, err
	}
	return M{"jobs": jobs, "saved": len(jobs), "warnings": warnings, "warning": strings.Join(warnings, "; ")}, nil
}

func apiSendSaveJob(p M) (M, error) {
	in, err := pjsonMap(p, "job")
	if err != nil {
		return nil, err
	}
	jobs, err := sendLoadJobs()
	if err != nil {
		return nil, err
	}
	raw := []any{}
	id := sendClean(in["id"])
	replaced := false
	for _, j := range jobs {
		m := sendJobMap(j)
		if id != "" && j.ID == id {
			m = in
			replaced = true
		}
		raw = append(raw, m)
	}
	if !replaced {
		if id != "" {
			for _, j := range jobs {
				if j.ID == id {
					return nil, apiFailf("A job with id %s exists already", id)
				}
			}
		}
		raw = append(raw, in)
	}
	idx := len(raw) - 1
	if replaced {
		for i, j := range jobs {
			if j.ID == id {
				idx = i
			}
		}
	}
	normalized, _ := sendNormalizeList(raw)
	saved := normalized[idx]
	problems := []string{}
	warnings := []string{}
	if why := sendJobProblem(saved); why != "" {
		if why == "incomplete" {
			why = "source and destination have to be filled in"
		}
		problems = append(problems, why)
	} else {
		if w := sendShareWarning(saved, sendUserShareExists); w != "" {
			warnings = append(warnings, w)
		}
		if w := sendMirrorWarning(saved); w != "" {
			warnings = append(warnings, w)
		}
	}
	if err := sendWriteJobs(normalized); err != nil {
		return nil, err
	}
	return M{"job": saved, "jobs": normalized, "problems": problems, "warnings": warnings}, nil
}

func sendJobMap(j sendJob) M {
	var m M
	json.Unmarshal(jsonEncode(j, false), &m)
	return m
}

func apiSendDelete(p M) (M, error) {
	id := pstr(p, "id", "")
	jobs, err := sendLoadJobs()
	if err != nil {
		return nil, err
	}
	keep := []sendJob{}
	var gone *sendJob
	for i := range jobs {
		if jobs[i].ID == id {
			gone = &jobs[i]
			continue
		}
		keep = append(keep, jobs[i])
	}
	if gone == nil {
		return nil, apiFailf("No job with id %s", id)
	}
	if sendReadStatus().Running && inList(id, sendReadStatus().Queue...) {
		return nil, apiFail("This job is part of the replication running right now. Stop it first.")
	}
	removed := []string{}
	if toBool(param(p, "checkpoints", false)) && validDataset(gone.Source) {
		args := []string{"list", "-H", "-t", "snapshot", "-o", "name"}
		if gone.Recursive {
			args = append(args, "-r")
		} else {
			args = append(args, "-d", "1")
		}
		rows := []string{}
		for _, l := range zfs(append(args, gone.Source)...).Lines() {
			if _, snap, ok := strings.Cut(l, "@"); ok && sendIsCheckpointOf(snap, gone.ID) {
				rows = append(rows, l)
			}
		}
		for _, t := range sendGroupDestroy(rows) {
			if zfs("destroy", t).OK() {
				removed = append(removed, t)
			}
		}
	}
	if err := sendWriteJobs(keep); err != nil {
		return nil, err
	}
	return M{"jobs": keep, "removed": removed}, nil
}

func apiSendToggle(p M) (M, error) {
	id := pstr(p, "id", "")
	on := toBool(param(p, "enabled", false))
	jobs, err := sendLoadJobs()
	if err != nil {
		return nil, err
	}
	for i := range jobs {
		if jobs[i].ID != id {
			continue
		}
		if on {
			if why := sendJobProblem(jobs[i]); why != "" {
				return nil, apiFail("The job cannot be enabled: " + why)
			}
		}
		jobs[i].Enabled = on
		if err := sendWriteJobs(jobs); err != nil {
			return nil, err
		}
		return M{"job": jobs[i], "jobs": jobs}, nil
	}
	return nil, apiFailf("No job with id %s", id)
}

func sendIDsParam(p M) ([]string, error) {
	ids := []string{}
	if v, ok := p["ids"]; ok && v != nil && str(v) != "" {
		l, err := pjsonList(p, "ids")
		if err != nil {
			return nil, err
		}
		for _, x := range l {
			ids = append(ids, str(x))
		}
	}
	if id := pstr(p, "job", pstr(p, "id", "")); id != "" {
		ids = append(ids, id)
	}
	for _, id := range ids {
		if !reSendID.MatchString(id) {
			return nil, apiFail("Invalid job id")
		}
	}
	return ids, nil
}

func apiSendRun(p M) (M, error) {
	if sendReadStatus().Running {
		return nil, apiFail("A replication run is already in progress")
	}
	ids, err := sendIDsParam(p)
	if err != nil {
		return nil, err
	}
	args := []string{"send-run"}
	if toBool(param(p, "dry_run", false)) {
		args = append(args, "--dry-run")
	}
	if toBool(param(p, "force", false)) {
		if len(ids) == 0 {
			return nil, apiFail("Force for one run needs a job.")
		}
		args = append(args, "--force")
	}
	args = append(args, ids...)
	if len(ids) == 0 {
		n := 0
		jobs, _ := sendLoadJobs()
		for _, j := range jobs {
			if j.Enabled {
				n++
			}
		}
		if n == 0 {
			return nil, apiFail("No job is enabled.")
		}
	}
	if err := sendSpawn(args...); err != nil {
		return nil, apiFail("Failed to start the replication worker: " + err.Error())
	}
	for i := 0; i < 30; i++ {
		if sendReadStatus().Running {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return M{"state": sendStateView(sendReadStatus())}, nil
}

func apiSendStop(p M) (M, error) {
	st := sendReadStatus()
	if !st.Running {
		return nil, apiFail("No replication is running.")
	}
	if err := syscall.Kill(st.PID, syscall.SIGTERM); err != nil {
		return nil, apiFail("Could not stop the run: " + err.Error())
	}
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if !sendReadStatus().Running {
			break
		}
	}
	return M{"state": sendStateView(sendReadStatus())}, nil
}

func sendJobParam(p M) (sendJob, error) {
	if v, ok := p["job"]; ok && v != nil {
		if m, err := pjsonMap(p, "job"); err == nil {
			return sendJobFromMap(m, 0), nil
		}
	}
	id := pstr(p, "id", "")
	j, ok := sendFindJob(id)
	if !ok {
		return sendJob{}, apiFailf("No job with id %s", id)
	}
	return j, nil
}

func apiSendTest(p M) (M, error) {
	j, err := sendJobParam(p)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	plan := sendPlanJob(ctx, j)
	return M{"checks": plan.Checks, "ok": !plan.Fatal, "plan": plan}, nil
}

func apiSendPreview(p M) (M, error) {
	j, err := sendJobParam(p)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	plan := sendPlanJob(ctx, j)
	if !plan.Fatal {
		plan.describe(ctx, sendCheckpointPrefix(j.ID)+time.Now().Format("20060102-150405"), true)
	}
	return M{"plan": plan, "ok": !plan.Fatal}, nil
}

func apiSendStatus(p M) (M, error) {
	st := sendReadStatus()
	cron := cronJobs[2].state()
	run := M{
		"last_run": st.LastRun, "duration": st.Duration, "jobs_run": st.JobsRun, "jobs_ok": st.JobsOK,
		"errors": st.Errors, "current": st.Current, "last_error": st.LastError, "dry_run": st.DryRun,
	}
	return M{
		"run":          run,
		"running":      st.Running,
		"state":        sendStateView(st),
		"entry":        cron.Entry,
		"cron_healthy": cron.Healthy,
		"log":          tail(sendLogFile(), 40, 256*1024),
	}, nil
}

func apiSendLog(p M) (M, error) {
	run := pstr(p, "run", "")
	path := sendLogFile()
	if run != "" {
		if !reSendID.MatchString(strings.ReplaceAll(run, "-", "_")) || strings.Contains(run, "..") {
			return nil, apiFail("Invalid run")
		}
		path = filepath.Join(sendRunLogDir(), run+".log")
		if !isFile(path) {
			return nil, apiFail("This log is no longer available - run logs live in RAM and are lost on reboot.")
		}
	}
	st, err := os.Stat(path)
	if err != nil {
		return M{"text": "", "offset": 0, "size": 0, "running": sendReadStatus().Running}, nil
	}
	size := st.Size()
	offset := toInt64(param(p, "offset", -1))
	limit := int64(512 * 1024)
	if offset < 0 || offset > size {
		offset = size - limit
		if offset < 0 {
			offset = 0
		}
	}
	end := size
	if end-offset > limit {
		end = offset + limit
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, apiFail("Cannot read the log")
	}
	defer f.Close()
	buf := make([]byte, end-offset)
	n, _ := f.ReadAt(buf, offset)
	text := string(buf[:n])
	if offset > 0 && toInt64(param(p, "offset", -1)) < 0 {
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			offset += int64(i + 1)
			text = text[i+1:]
		}
	}
	st2 := sendReadStatus()
	running := st2.Running
	if run != "" {
		running = st2.Running && st2.Progress != nil && strings.HasPrefix(run, st2.CurrentID+"-")
	}
	return M{"text": text, "offset": offset + int64(len(text)), "start": offset, "size": size, "running": running}, nil
}

func sendKeyInfo() M {
	path := sendKeyFile()
	out := M{"path": path, "exists": false, "public": "", "fingerprint": ""}
	pub, err := os.ReadFile(path + ".pub")
	if err != nil || !isFile(path) {
		return out
	}
	out["exists"] = true
	out["public"] = strings.TrimSpace(string(pub))
	if r := runCmd(10*time.Second, "ssh-keygen", "-l", "-f", path+".pub"); r.OK() {
		out["fingerprint"] = strings.TrimSpace(r.Stdout)
	}
	return out
}

func apiSendKey(p M) (M, error) {
	op := pstr(p, "op", "show")
	path := sendKeyFile()
	switch op {
	case "show":
	case "create", "renew":
		if isFile(path) && op == "create" {
			break
		}
		if op == "renew" {
			os.Remove(path)
			os.Remove(path + ".pub")
		}
		os.MkdirAll(ConfigDir, 0o755)
		r := runCmd(30*time.Second, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "zfs-toolkit@"+serverName(), "-f", path)
		if !r.OK() {
			return nil, apiFail("ssh-keygen failed: " + r.Message())
		}
		os.Chmod(path, 0o600)
	default:
		return nil, apiFail("Unknown key operation")
	}
	return M{"key": sendKeyInfo()}, nil
}

func apiSendForgetHost(p M) (M, error) {
	host := pstr(p, "host", "")
	port := toInt(param(p, "port", 22))
	if _, h, ok := strings.Cut(host, "@"); ok {
		host = h
	}
	if host == "" || !reSendHost.MatchString(host) || strings.HasPrefix(host, "-") {
		return nil, apiFail("Invalid host")
	}
	name := host
	if port != 22 && port > 0 {
		name = fmt.Sprintf("[%s]:%d", host, port)
	}
	if !isFile(sendKnownHosts()) {
		return M{"removed": false}, nil
	}
	r := runCmd(10*time.Second, "ssh-keygen", "-R", name, "-f", sendKnownHosts())
	os.Remove(sendKnownHosts() + ".old")
	return M{"removed": r.OK()}, nil
}

type sendDataset struct {
	Name       string `json:"name"`
	Used       int64  `json:"used"`
	Avail      int64  `json:"avail"`
	Mountpoint string `json:"mountpoint"`
	Encryption string `json:"encryption"`
	Children   int    `json:"children"`
	Snapshots  int    `json:"snapshots"`
	Readonly   bool   `json:"readonly"`
	Replica    bool   `json:"replica"`
}

func sendParseDatasets(lines []string) []sendDataset {
	out := []sendDataset{}
	idx := map[string]int{}
	for _, l := range lines {
		f := strings.Split(l, "\t")
		if len(f) < 6 {
			continue
		}
		d := sendDataset{Name: f[0], Used: toInt64(f[1]), Avail: toInt64(f[2]), Mountpoint: f[3], Encryption: f[4], Readonly: f[5] == "on"}
		idx[d.Name] = len(out)
		out = append(out, d)
	}
	for i := range out {
		if j := strings.LastIndex(out[i].Name, "/"); j > 0 {
			if pi, ok := idx[out[i].Name[:j]]; ok {
				out[pi].Children++
			}
		}
	}
	return out
}

var sendDatasetCols = []string{"list", "-H", "-p", "-t", "filesystem,volume", "-o", "name,used,avail,mountpoint,encryption,readonly"}

func apiSendDatasets(p M) (M, error) {
	r := zfs(sendDatasetCols...)
	if !r.OK() {
		return nil, apiFail("zfs list failed: " + r.Message())
	}
	ds := sendParseDatasets(r.Lines())
	counts := map[string]int{}
	for _, l := range zfs("list", "-H", "-t", "snapshot", "-o", "name").Lines() {
		if d, _, ok := strings.Cut(l, "@"); ok {
			counts[d]++
		}
	}
	dests := map[string]bool{}
	jobs, _ := sendLoadJobs()
	for _, j := range jobs {
		if !j.remote() {
			dests[j.Dest] = true
		}
	}
	for i := range ds {
		ds[i].Snapshots = counts[ds[i].Name]
		for d := range dests {
			if under(ds[i].Name, d) {
				ds[i].Replica = true
			}
		}
	}
	return M{"datasets": ds, "server": serverName()}, nil
}

func apiSendTargets(p M) (M, error) {
	j, err := sendJobParam(p)
	if err != nil {
		return nil, err
	}
	if j.remote() && (j.SSHHost == "" || !reSendHost.MatchString(j.SSHHost) || strings.HasPrefix(j.SSHHost, "-")) {
		return nil, apiFail("Enter the SSH host first (user@host).")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r := j.destZfs(ctx, time.Minute, sendDatasetCols...)
	if !r.OK() {
		msg := r.Message()
		if x := sendSSHExplain(msg); x != "" && j.remote() {
			return nil, apiFail("Cannot list the datasets on " + j.SSHHost + ": " + x)
		}
		return nil, apiFail("Cannot list the datasets: " + sendLastLines(msg, 3))
	}
	return M{"datasets": sendParseDatasets(r.Lines())}, nil
}

func apiSendInspect(p M) (M, error) {
	j, err := sendJobParam(p)
	if err != nil {
		return nil, err
	}
	if why := sendJobProblem(j); why != "" {
		return nil, apiFail("The job cannot be inspected: " + why)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	src := sendParseSnaps(srcZfs(ctx, time.Minute, append(append([]string{}, sendSnapCols...), j.Source)...))
	out := M{"source": M{"name": j.Source, "snapshots": len(src)}, "dest": M{"name": j.Dest, "exists": false}}
	if r := srcZfs(ctx, time.Minute, "list", "-H", "-p", "-o", "used,avail", j.Source); r.OK() {
		f := strings.Split(strings.TrimSpace(r.Stdout), "\t")
		if len(f) == 2 {
			out["source"].(M)["used"] = toInt64(f[0])
			out["source"].(M)["avail"] = toInt64(f[1])
		}
	}
	if len(src) > 0 {
		out["source"].(M)["newest"] = src[len(src)-1]
	}
	out["source"].(M)["checkpoints"] = sendCheckpointList(src, j.ID)
	dr := j.destZfs(ctx, time.Minute, "list", "-H", "-p", "-o", "used,avail,readonly", j.Dest)
	if !dr.OK() {
		msg := dr.Message()
		if j.remote() && sendSSHExplain(msg) != "" && !strings.Contains(msg, "does not exist") {
			out["dest"].(M)["error"] = sendSSHExplain(msg)
		}
		out["pending"] = len(src)
		return out, nil
	}
	d := out["dest"].(M)
	d["exists"] = true
	if f := strings.Split(strings.TrimSpace(dr.Stdout), "\t"); len(f) == 3 {
		d["used"] = toInt64(f[0])
		d["avail"] = toInt64(f[1])
		d["readonly"] = f[2] == "on"
	}
	dst := sendParseSnaps(j.destZfs(ctx, time.Minute, append(append([]string{}, sendSnapCols...), j.Dest)...))
	d["snapshots"] = len(dst)
	if len(dst) > 0 {
		d["newest"] = dst[len(dst)-1]
	}
	d["checkpoints"] = sendCheckpointList(dst, j.ID)
	if base, ok := sendFindBase(src, dst); ok {
		out["base"] = base
		pending := []string{}
		after := false
		for _, s := range src {
			if after {
				pending = append(pending, s.Name)
			}
			if s.GUID == base.GUID {
				after = true
			}
		}
		out["pending"] = len(pending)
		out["pending_names"] = sendFirst(pending, 12)
		out["dest_newer"] = sendNewerOnDest(dst, base)
	} else {
		out["pending"] = len(src)
	}
	return out, nil
}

func sendCheckpointList(snaps []sendSnap, id string) []sendSnap {
	out := []sendSnap{}
	for _, s := range snaps {
		if sendIsCheckpointOf(s.Name, id) {
			out = append(out, s)
		}
	}
	return out
}

func sendOverview(s Settings) any {
	jobs, jobsErr := sendLoadJobs()
	st := sendReadStatus()
	enabled := 0
	failed := []string{}
	var lastOK int64
	list := []M{}
	for _, j := range jobs {
		if j.Enabled {
			enabled++
		}
		var last *sendResult
		if js := st.Jobs[j.ID]; js != nil {
			last = js.Last
		}
		if last != nil && j.Enabled && last.Status == "error" {
			failed = append(failed, j.Name)
		}
		if last != nil && last.Status == "ok" && last.Ended > lastOK {
			lastOK = last.Ended
		}
		list = append(list, M{"id": j.ID, "name": j.Name, "enabled": j.Enabled, "source": j.Source, "target": j.target(), "transport": j.Transport, "last": last})
	}
	status := M{"level": "idle", "text": "Replication is not set up", "sub": "Copy snapshots to a second pool or another server."}
	switch {
	case st.Running:
		text := "Replicating"
		if st.DryRun == 1 {
			text = "Replication dry run"
		}
		sub := st.Current
		if st.Progress != nil && st.Progress.Bytes > 0 {
			sub += " - " + humanBytes(float64(st.Progress.Bytes))
		}
		status = M{"level": "run", "text": text, "sub": sub}
	case jobsErr != nil:
		status = M{"level": "err", "text": "The replication job list cannot be read", "sub": jobsErr.Error()}
	case len(failed) > 0:
		status = M{"level": "err", "text": "Replication failed", "sub": strings.Join(failed, ", ")}
	case enabled > 0 && !s.Yes("send_enabled"):
		status = M{"level": "ok", "text": fmt.Sprintf("%d replication job(s), started by hand", enabled), "sub": "No schedule is set."}
	case enabled > 0:
		sub := "Not run yet"
		if lastOK > 0 {
			sub = "Last copy " + dateTime(time.Unix(lastOK, 0))
		}
		status = M{"level": "ok", "text": fmt.Sprintf("%d replication job(s) scheduled", enabled), "sub": sub}
	case len(jobs) > 0:
		status = M{"level": "idle", "text": "All replication jobs are disabled", "sub": ""}
	}
	return M{
		"status":   status,
		"count":    enabled,
		"total":    len(jobs),
		"running":  st.Running,
		"dry_run":  st.Running && st.DryRun == 1,
		"progress": st.Progress,
		"current":  st.CurrentID,
		"queue":    st.Queue,
		"jobs":     list,
		"last_ok":  lastOK,
	}
}

func init() {
	registerAction("send_jobs", apiSendJobs)
	registerAction("send_save", apiSendSave)
	registerAction("send_save_job", apiSendSaveJob)
	registerAction("send_delete", apiSendDelete)
	registerAction("send_toggle", apiSendToggle)
	registerAction("send_run", apiSendRun)
	registerAction("send_stop", apiSendStop)
	registerAction("send_test", apiSendTest)
	registerAction("send_preview", apiSendPreview)
	registerAction("send_status", apiSendStatus)
	registerAction("send_log", apiSendLog)
	registerAction("send_key", apiSendKey)
	registerAction("send_forget_host", apiSendForgetHost)
	registerAction("send_datasets", apiSendDatasets)
	registerAction("send_targets", apiSendTargets)
	registerAction("send_inspect", apiSendInspect)
	registerOverview("replication", sendOverview)
}
