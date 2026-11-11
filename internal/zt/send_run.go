package zt

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type sendCheck struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

type sendPlan struct {
	Job          sendJob     `json:"-"`
	Checks       []sendCheck `json:"checks"`
	Fatal        bool        `json:"fatal"`
	DestExists   bool        `json:"dest_exists"`
	Full         bool        `json:"full"`
	Base         string      `json:"base"`
	BaseCreation int64       `json:"base_creation"`
	Parents      []string    `json:"parents"`
	Rollback     []string    `json:"rollback"`
	Checkpoint   string      `json:"checkpoint"`
	Estimate     int64       `json:"estimate"`
	EstimateNote string      `json:"estimate_note"`
	Commands     []string    `json:"commands"`
	newestSrc    string
}

func (p *sendPlan) add(level, format string, a ...any) {
	p.Checks = append(p.Checks, sendCheck{Level: level, Text: fmt.Sprintf(format, a...)})
	if level == "err" {
		p.Fatal = true
	}
}

func sendPool(ds string) string {
	if i := strings.Index(ds, "/"); i >= 0 {
		return ds[:i]
	}
	return ds
}

func sendPlanJob(ctx context.Context, j sendJob) *sendPlan {
	p := &sendPlan{Job: j, Checks: []sendCheck{}, Parents: []string{}, Rollback: []string{}, Commands: []string{}}
	if why := sendJobProblem(j); why != "" {
		if why == "incomplete" {
			why = "source and destination have to be filled in"
		}
		p.add("err", "The job is not valid: %s.", why)
		return p
	}
	r := srcZfs(ctx, time.Minute, "list", "-H", "-o", "name,encryption", j.Source)
	if !r.OK() {
		p.add("err", "Source dataset %s does not exist on this server.", j.Source)
		return p
	}
	if f := strings.Split(strings.TrimSpace(r.Stdout), "\t"); len(f) > 1 && f[1] != "off" && f[1] != "-" && !j.Raw {
		p.add("warn", "%s is encrypted; without Raw it is sent decrypted and stored unencrypted on the destination unless the parent there is encrypted.", j.Source)
	} else {
		p.add("ok", "Source %s exists.", j.Source)
	}
	srcSnaps := sendParseSnaps(srcZfs(ctx, 2*time.Minute, append(append([]string{}, sendSnapCols...), j.Source)...))
	if len(srcSnaps) > 0 {
		p.newestSrc = srcSnaps[len(srcSnaps)-1].Name
	}
	where := "this server"
	if j.remote() {
		where = j.SSHHost
		if j.SSHKey != "" {
			if _, err := sendKeyForUse(j.SSHKey); err != nil {
				p.add("err", "%s.", strings.ToUpper(err.Error()[:1])+err.Error()[1:])
				return p
			}
		}
		ssh, err := j.sshArgs()
		if err != nil {
			p.add("err", "%s", err.Error())
			return p
		}
		t := sendExec(ctx, 30*time.Second, nil, sendSSHBin, append(ssh, "true")...)
		if !t.OK() {
			msg := t.Message()
			if x := sendSSHExplain(msg); x != "" {
				p.add("err", "Cannot log in to %s port %d: %s (%s)", j.SSHHost, j.SSHPort, x, sendLastLines(msg, 2))
			} else {
				p.add("err", "Cannot log in to %s port %d with key based authentication: %s", j.SSHHost, j.SSHPort, sendLastLines(msg, 2))
			}
			return p
		}
		p.add("ok", "SSH login to %s port %d works.", j.SSHHost, j.SSHPort)
	}
	pool := sendPool(j.Dest)
	r = j.destZfs(ctx, time.Minute, "list", "-H", "-o", "name", pool)
	if !r.OK() {
		msg := r.Message()
		if j.remote() && sendSSHExplain(msg) == "zfs was not found on the server." {
			p.add("err", "zfs was not found on %s.", where)
		} else {
			p.add("err", "Pool %s does not exist on %s.", pool, where)
		}
		return p
	}
	r = j.destZfs(ctx, time.Minute, "list", "-H", "-o", "name", j.Dest)
	p.DestExists = r.OK()
	if !p.DestExists {
		p.Full = true
		parts := strings.Split(j.Dest, "/")
		for i := 1; i < len(parts)-1; i++ {
			parent := strings.Join(parts[:i+1], "/")
			if !j.destZfs(ctx, time.Minute, "list", "-H", "-o", "name", parent).OK() {
				p.Parents = append(p.Parents, parent)
			}
		}
		if len(p.Parents) > 0 {
			p.add("info", "%s does not exist yet on %s; %s will be created first, then everything is sent in full.", j.Dest, where, strings.Join(p.Parents, ", "))
		} else {
			p.add("info", "%s does not exist yet on %s; the first run sends everything in full.", j.Dest, where)
		}
		return p
	}
	dstSnaps := sendParseSnaps(j.destZfs(ctx, 2*time.Minute, append(append([]string{}, sendSnapCols...), j.Dest)...))
	if len(dstSnaps) == 0 {
		p.add("err", "%s already exists on %s but has no snapshots, so it was not made by replication. zfs recv only fills a new dataset - choose another destination or remove that dataset yourself.", j.Dest, where)
		return p
	}
	base, ok := sendFindBase(srcSnaps, dstSnaps)
	if !ok {
		p.add("err", "%s exists on %s but shares no snapshot with %s, so nothing can be sent incrementally. Either the snapshots it was built from were deleted here, or it holds something else. Rename or remove it there for a fresh full copy - this plugin never deletes it for you.", j.Dest, where, j.Source)
		return p
	}
	p.Base = base.Name
	p.BaseCreation = base.Creation
	p.add("ok", "Destination %s exists; the newest snapshot both sides have is @%s (%s).", j.Dest, base.Name, time.Unix(base.Creation, 0).Format("2006-01-02 15:04"))
	if newer := sendNewerOnDest(dstSnaps, base); len(newer) > 0 {
		list := strings.Join(sendFirst(newer, 5), ", ")
		if j.AllowRollback {
			p.Rollback = newer
			p.add("warn", "Force is on: %d snapshot(s) on the destination newer than @%s will be destroyed: %s.", len(newer), base.Name, list)
		} else {
			p.add("err", "The destination has %d snapshot(s) newer than the last common one (@%s): %s. zfs recv refuses that. Remove them on the destination, or turn on Force to let them be destroyed.", len(newer), base.Name, list)
		}
	}
	if j.remote() {
		ssh, _ := j.sshArgs()
		sendExec(ctx, time.Minute, nil, sendSSHBin, append(ssh, sendRemoteCommand("zpool", "sync", pool))...)
	} else {
		sendExec(ctx, time.Minute, nil, ZpoolBin, "sync", pool)
	}
	wargs := []string{"get", "-H", "-p", "-o", "name,value", "-t", "filesystem,volume"}
	if j.Recursive {
		wargs = append(wargs, "-r")
	}
	modified := []string{}
	for _, l := range j.destZfs(ctx, time.Minute, append(wargs, "written", j.Dest)...).Lines() {
		f := strings.Split(l, "\t")
		if len(f) == 2 && toInt64(f[1]) > 0 {
			modified = append(modified, f[0])
		}
	}
	if len(modified) > 0 {
		list := strings.Join(sendFirst(modified, 3), ", ")
		if j.AllowRollback {
			p.add("info", "The destination reports changes since its last snapshot (%s); Force rolls them back.", list)
		} else {
			p.add("warn", "The destination reports changes since its last snapshot (%s). If zfs recv refuses because of that, roll the copy back once, and make it read-only so it stays untouched.", list)
		}
	}
	return p
}

func sendFirst(l []string, n int) []string {
	if len(l) <= n {
		return l
	}
	out := append([]string{}, l[:n]...)
	return append(out, fmt.Sprintf("and %d more", len(l)-n))
}

func sendEstimate(ctx context.Context, args []string) int64 {
	a := append([]string{"send", "-n", "-v", "-P"}, args[1:]...)
	r := srcZfs(ctx, 5*time.Minute, a...)
	if !r.OK() {
		return -1
	}
	for _, l := range strings.Split(r.Stdout+"\n"+r.Stderr, "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && f[0] == "size" {
			return toInt64(f[1])
		}
	}
	return -1
}

func (p *sendPlan) describe(ctx context.Context, checkpoint string, estimate bool) {
	j := p.Job
	p.Checkpoint = checkpoint
	for _, parent := range p.Parents {
		p.Commands = append(p.Commands, sendShow(j, true, "zfs", "create", "-u", parent))
	}
	snap := []string{"zfs", "snapshot"}
	if j.Recursive {
		snap = append(snap, "-r")
	}
	p.Commands = append(p.Commands, sendShow(j, false, append(snap, j.Source+"@"+checkpoint)...))
	sa := j.sendArgs(checkpoint, p.Base)
	ra := j.recvArgs()
	pipe := sendShow(j, false, append([]string{"zfs"}, sa...)...) + " | " + sendShow(j, true, append([]string{"zfs"}, ra...)...)
	p.Commands = append(p.Commands, pipe)
	if !estimate {
		return
	}
	if p.newestSrc == "" {
		p.Estimate = -1
		p.EstimateNote = "no snapshot yet to measure"
		return
	}
	if p.newestSrc == p.Base {
		p.Estimate = 0
		p.EstimateNote = "plus whatever changed since @" + p.Base
		return
	}
	p.Estimate = sendEstimate(ctx, j.sendArgs(p.newestSrc, p.Base))
	p.EstimateNote = "up to @" + p.newestSrc + ", plus whatever changed since"
}

func sendShow(j sendJob, dest bool, args ...string) string {
	if dest && j.remote() {
		return "ssh -p " + fmt.Sprint(j.SSHPort) + " " + j.SSHHost + " " + shellQuote(sendRemoteCommand(args...))
	}
	return sendRemoteCommand(args...)
}

type sendResult struct {
	Run        string `json:"run"`
	Status     string `json:"status"`
	Dry        bool   `json:"dry"`
	Started    int64  `json:"started"`
	Ended      int64  `json:"ended"`
	Bytes      int64  `json:"bytes"`
	Estimate   int64  `json:"estimate"`
	Full       bool   `json:"full"`
	Base       string `json:"base"`
	Checkpoint string `json:"checkpoint"`
	Error      string `json:"error"`
}

type sendJobState struct {
	Last   *sendResult  `json:"last"`
	Recent []sendResult `json:"recent"`
}

type sendProgress struct {
	Job     string  `json:"job"`
	Name    string  `json:"name"`
	Phase   string  `json:"phase"`
	Bytes   int64   `json:"bytes"`
	Total   int64   `json:"total"`
	Rate    float64 `json:"rate"`
	Started int64   `json:"started"`
}

type sendStatus struct {
	LastRun   string                   `json:"last_run"`
	Duration  int64                    `json:"duration"`
	JobsRun   int                      `json:"jobs_run"`
	JobsOK    int                      `json:"jobs_ok"`
	Errors    int                      `json:"errors"`
	Current   string                   `json:"current"`
	LastError string                   `json:"last_error"`
	DryRun    int                      `json:"dry_run"`
	Running   bool                     `json:"running"`
	PID       int                      `json:"pid"`
	Started   int64                    `json:"started"`
	Ended     int64                    `json:"ended"`
	CurrentID string                   `json:"current_id"`
	Queue     []string                 `json:"queue"`
	Progress  *sendProgress            `json:"progress"`
	Jobs      map[string]*sendJobState `json:"jobs"`
}

func sendReadStatus() sendStatus {
	st := sendStatus{}
	jsonReadInto(sendStatusFile(), &st)
	if st.Jobs == nil {
		st.Jobs = map[string]*sendJobState{}
	}
	if st.Running && !sendPIDIsRunner(st.PID) {
		st.Running = false
		st.Progress = nil
		if st.CurrentID != "" {
			if js := st.Jobs[st.CurrentID]; js != nil && js.Last != nil && js.Last.Status == "running" {
				js.Last.Status = "error"
				js.Last.Error = "the run was interrupted"
			}
		}
	}
	return st
}

func sendPIDIsRunner(pid int) bool {
	if !pidAlive(pid) {
		return false
	}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return true
	}
	return strings.Contains(string(b), "send-run")
}

type sendRunner struct {
	ctx    context.Context
	dry    bool
	st     sendStatus
	start  time.Time
	runLog *os.File
	toOut  bool
	stats  *sendPipeStats
	force  bool
}

func (r *sendRunner) line(level, msg string) {
	prefix := ""
	if r.dry {
		prefix = "[dry-run] "
	}
	switch level {
	case "ok":
		prefix += "OK: "
	case "warn":
		prefix += "WARNING: "
	case "err":
		prefix += "ERROR: "
	}
	text := prefix + msg
	logAppendMax(sendLogFile(), text, 2*1024*1024)
	stamped := dateTime(time.Now()) + " " + text + "\n"
	if r.runLog != nil {
		r.runLog.WriteString(stamped)
	}
	if r.toOut {
		os.Stdout.WriteString(stamped)
	}
}

func (r *sendRunner) logf(level, format string, a ...any) {
	r.line(level, fmt.Sprintf(format, a...))
}

func (r *sendRunner) save() {
	r.st.Duration = int64(time.Since(r.start).Seconds())
	r.st.LastRun = dateTime(time.Now())
	if r.st.Progress != nil && r.stats != nil {
		r.st.Progress.Bytes = r.stats.get()
		if el := time.Since(r.stats.started).Seconds(); el > 0.5 && !r.stats.started.IsZero() {
			r.st.Progress.Rate = float64(r.st.Progress.Bytes) / el
		}
	}
	os.MkdirAll(TmpDir, 0o755)
	jsonWrite(sendStatusFile(), r.st, 0o644)
}

func sendStdoutIsLog() bool {
	a, err1 := os.Stdout.Stat()
	b, err2 := os.Stat(sendLogFile())
	if err1 != nil || err2 != nil {
		return false
	}
	return os.SameFile(a, b)
}

func (r *sendRunner) record(j sendJob, res *sendResult) {
	js := r.st.Jobs[j.ID]
	if js == nil {
		js = &sendJobState{}
		r.st.Jobs[j.ID] = js
	}
	if !res.Dry {
		cp := *res
		js.Last = &cp
	}
	found := false
	for i := range js.Recent {
		if js.Recent[i].Run == res.Run {
			js.Recent[i] = *res
			found = true
		}
	}
	if !found {
		js.Recent = append(js.Recent, *res)
	}
	if len(js.Recent) > 30 {
		js.Recent = js.Recent[len(js.Recent)-30:]
	}
}

func sendRunLogName(id string, t time.Time) string {
	return id + "-" + t.Format("20060102-150405") + ".log"
}

func sendPruneRunLogs(id string, keep int) {
	files, _ := filepath.Glob(filepath.Join(sendRunLogDir(), id+"-*.log"))
	mine := []string{}
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), ".log")
		if reCheckpointStamp.MatchString(strings.TrimPrefix(base, id+"-")) {
			mine = append(mine, f)
		}
	}
	sort.Strings(mine)
	for len(mine) > keep {
		os.Remove(mine[0])
		mine = mine[1:]
	}
}

func (r *sendRunner) checkpointName(j sendJob) string {
	names := []string{}
	for _, l := range srcZfs(r.ctx, time.Minute, "list", "-H", "-t", "snapshot", "-o", "name", "-d", "1", j.Source).Lines() {
		if _, snap, ok := strings.Cut(l, "@"); ok {
			names = append(names, snap)
		}
	}
	return sendNextCheckpoint(j.ID, time.Now().Format("20060102-150405"), names)
}

func (r *sendRunner) runJob(j sendJob) {
	started := time.Now()
	os.MkdirAll(sendRunLogDir(), 0o755)
	runID := strings.TrimSuffix(sendRunLogName(j.ID, started), ".log")
	for n := 2; n < 100 && exists(filepath.Join(sendRunLogDir(), runID+".log")); n++ {
		runID = fmt.Sprintf("%s-%d", strings.TrimSuffix(sendRunLogName(j.ID, started), ".log"), n)
	}
	res := &sendResult{Run: runID, Status: "running", Dry: r.dry, Started: started.Unix(), Estimate: -1}
	f, err := os.OpenFile(filepath.Join(sendRunLogDir(), res.Run+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		r.runLog = f
	}
	defer func() {
		if r.runLog != nil {
			r.runLog.Close()
			r.runLog = nil
		}
		sendPruneRunLogs(j.ID, 15)
	}()
	r.st.Current = j.Name
	r.st.CurrentID = j.ID
	r.st.Progress = &sendProgress{Job: j.ID, Name: j.Name, Phase: "checking", Total: -1, Started: started.Unix()}
	r.stats = nil
	r.record(j, res)
	r.save()
	r.logf("", "──────── Job '%s' (%s): %s -> %s ────────", j.Name, j.ID, j.Source, j.target())
	if r.force && !j.AllowRollback {
		j.AllowRollback = true
		r.line("warn", "Force for this run: the destination may be rolled back to the last common snapshot.")
	}

	fail := func(format string, a ...any) {
		msg := fmt.Sprintf(format, a...)
		r.line("err", "Job '"+j.Name+"': "+msg)
		res.Status = "error"
		res.Error = msg
		r.st.Errors++
		r.st.LastError = "Job '" + j.Name + "': " + msg
	}
	finish := func() {
		res.Ended = time.Now().Unix()
		if r.stats != nil {
			res.Bytes = r.stats.get()
		}
		if res.Status == "ok" {
			r.st.JobsOK++
		}
		r.record(j, res)
		r.st.Progress = nil
		r.save()
	}
	defer finish()

	p := sendPlanJob(r.ctx, j)
	for _, c := range p.Checks {
		lv := c.Level
		if lv == "info" {
			lv = ""
		}
		if lv == "err" {
			continue
		}
		r.line(lv, c.Text)
	}
	if r.ctx.Err() != nil {
		res.Status = "cancelled"
		res.Error = "stopped"
		r.line("warn", "Stopped.")
		return
	}
	if p.Fatal {
		for _, c := range p.Checks {
			if c.Level == "err" {
				fail("%s", c.Text)
				return
			}
		}
	}
	res.Full = p.Full
	res.Base = p.Base
	checkpoint := r.checkpointName(j)
	res.Checkpoint = checkpoint

	if r.dry {
		p.describe(r.ctx, checkpoint, true)
		for _, c := range p.Commands {
			r.logf("", "Would run: %s", c)
		}
		if p.Estimate >= 0 {
			r.logf("", "Estimated stream size: %s (%s)", humanBytes(float64(p.Estimate)), p.EstimateNote)
		}
		res.Estimate = p.Estimate
		res.Status = "ok"
		r.logf("ok", "Job '%s': dry run complete, nothing was changed.", j.Name)
		return
	}

	for _, parent := range p.Parents {
		cr := j.destZfs(r.ctx, time.Minute, "create", "-u", parent)
		if !cr.OK() {
			fail("cannot create %s: %s", parent, cr.Message())
			return
		}
		r.logf("ok", "Created %s", parent)
	}
	snapArgs := []string{"snapshot"}
	if j.Recursive {
		snapArgs = append(snapArgs, "-r")
	}
	sr := srcZfs(r.ctx, 5*time.Minute, append(snapArgs, j.Source+"@"+checkpoint)...)
	if !sr.OK() {
		fail("cannot create checkpoint %s@%s: %s", j.Source, checkpoint, sr.Message())
		return
	}
	r.logf("ok", "Created checkpoint %s@%s", j.Source, checkpoint)

	sa := j.sendArgs(checkpoint, p.Base)
	ra := j.recvArgs()
	r.st.Progress.Phase = "measuring"
	r.save()
	est := sendEstimate(r.ctx, sa)
	res.Estimate = est
	r.st.Progress.Total = est
	if est >= 0 {
		r.logf("", "Estimated stream size: %s", humanBytes(float64(est)))
	}
	kind := "incremental from @" + p.Base
	if p.Base == "" {
		kind = "full"
	}
	r.logf("", "Sending (%s): %s", kind, p.describeLine(sa, ra))
	r.st.Progress.Phase = "sending"
	r.stats = &sendPipeStats{}
	r.save()
	err = sendPipeline(r.ctx, j, sa, ra, r.stats, r.save)
	if err != nil {
		if err == errSendCancel {
			res.Status = "cancelled"
			res.Error = "stopped"
			r.line("warn", "Stopped during the transfer.")
		} else {
			fail("transfer failed: %s", err.Error())
		}
		r.dropCheckpoint(j, checkpoint)
		return
	}
	res.Status = "ok"
	r.logf("ok", "Job '%s': replicated %s -> %s (%s in %s)", j.Name, j.Source, j.target(), humanBytes(float64(r.stats.get())), humanDuration(time.Since(r.stats.started).Seconds()))
	r.prune(j, false, 2)
	if j.KeepDest > 0 {
		r.prune(j, true, j.KeepDest)
	}
}

func (p *sendPlan) describeLine(sa, ra []string) string {
	return sendShow(p.Job, false, append([]string{"zfs"}, sa...)...) + " | " + sendShow(p.Job, true, append([]string{"zfs"}, ra...)...)
}

func (r *sendRunner) dropCheckpoint(j sendJob, checkpoint string) {
	ctx := context.Background()
	for _, dest := range []bool{false, true} {
		args := []string{"list", "-H", "-t", "snapshot", "-o", "name"}
		if j.Recursive {
			args = append(args, "-r")
		} else {
			args = append(args, "-d", "1")
		}
		ds := j.Source
		var lr result
		if dest {
			ds = j.Dest
			lr = j.destZfs(ctx, time.Minute, append(args, ds)...)
		} else {
			lr = srcZfs(ctx, time.Minute, append(args, ds)...)
		}
		rows := []string{}
		for _, l := range lr.Lines() {
			if strings.HasSuffix(l, "@"+checkpoint) {
				rows = append(rows, l)
			}
		}
		for _, target := range sendGroupDestroy(rows) {
			var d result
			if dest {
				d = j.destZfs(ctx, time.Minute, "destroy", target)
			} else {
				d = srcZfs(ctx, time.Minute, "destroy", target)
			}
			if d.OK() {
				r.logf("", "Removed the unfinished checkpoint %s", target)
			}
		}
	}
}

func (r *sendRunner) prune(j sendJob, dest bool, keep int) {
	ds := j.Source
	args := []string{"list", "-H", "-t", "snapshot", "-o", "name"}
	if j.Recursive {
		args = append(args, "-r")
	} else {
		args = append(args, "-d", "1")
	}
	var lr result
	if dest {
		ds = j.Dest
		lr = j.destZfs(r.ctx, 2*time.Minute, append(args, ds)...)
	} else {
		lr = srcZfs(r.ctx, 2*time.Minute, append(args, ds)...)
	}
	if !lr.OK() {
		r.logf("warn", "Could not list checkpoints on %s: %s", ds, lr.Message())
		return
	}
	for _, target := range sendGroupDestroy(sendPruneSelection(lr.Lines(), j.ID, keep)) {
		var d result
		if dest {
			d = j.destZfs(r.ctx, 5*time.Minute, "destroy", target)
		} else {
			d = srcZfs(r.ctx, 5*time.Minute, "destroy", target)
		}
		where := "checkpoint"
		if dest {
			where = "destination checkpoint"
		}
		if d.OK() {
			r.logf("", "Pruned old %s %s", where, target)
		} else {
			r.logf("warn", "Could not prune %s %s: %s", where, target, d.Message())
		}
	}
}

func sendRun(args []string) int {
	dry, test, force := false, false, false
	ids := []string{}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--dry-run", "-n":
			dry = true
		case "--test":
			test = true
		case "--force":
			force = true
		case "--job":
			if i+1 < len(args) {
				ids = append(ids, args[i+1])
				i++
			}
		default:
			if !strings.HasPrefix(a, "-") {
				ids = append(ids, a)
			}
		}
	}
	os.MkdirAll(TmpDir, 0o755)
	if test {
		return sendRunTest(ids)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	r := &sendRunner{ctx: ctx, dry: dry, force: force && len(ids) > 0, start: time.Now(), toOut: !sendStdoutIsLog()}
	lock := lockFile(sendLockFile(), false)
	if lock == nil {
		r.line("warn", "Another replication run is still active - skipping.")
		return 0
	}
	defer lock.Unlock()
	prev := sendReadStatus()
	r.st = sendStatus{Jobs: prev.Jobs, Running: true, PID: os.Getpid(), Started: r.start.Unix(), Queue: []string{}}
	if dry {
		r.st.DryRun = 1
	}
	r.line("", "ZFS replication starting...")
	all, err := sendLoadJobs()
	if err != nil {
		r.logf("err", "%v No replication job runs until then.", err)
		r.st.Running, r.st.Errors, r.st.LastError, r.st.Ended = false, 1, err.Error(), time.Now().Unix()
		r.save()
		if !dry && notificationsOn() {
			snapNotifyLimited("send-jobs-unreadable", "ZFS replication stopped", "The replication job list cannot be read. "+err.Error(), "alert", 6*time.Hour)
		}
		return 1
	}
	jobs := []sendJob{}
	known := map[string]bool{}
	for _, j := range all {
		known[j.ID] = true
	}
	for _, j := range all {
		if len(ids) > 0 {
			if inList(j.ID, ids...) {
				jobs = append(jobs, j)
			}
		} else if j.Enabled {
			jobs = append(jobs, j)
		} else {
			r.logf("", "Skipping disabled job '%s' (%s).", j.Name, j.ID)
		}
	}
	for _, id := range ids {
		if !known[id] {
			r.logf("err", "No job with id '%s' found.", id)
			r.st.Errors++
			r.st.LastError = "No job with id '" + id + "' found."
		}
	}
	for _, j := range jobs {
		r.st.Queue = append(r.st.Queue, j.ID)
	}
	r.save()
	if !zfsReady() {
		r.line("warn", "ZFS is not available or no pool is imported (array not started?). Nothing to do.")
		jobs = nil
	} else if len(jobs) == 0 && len(ids) == 0 {
		r.line("warn", "No enabled replication jobs.")
	}
	for _, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		r.st.JobsRun++
		r.runJob(j)
		if len(r.st.Queue) > 0 {
			r.st.Queue = r.st.Queue[1:]
		}
	}
	r.st.Running = false
	r.st.Current = ""
	r.st.CurrentID = ""
	r.st.Queue = []string{}
	r.st.Progress = nil
	r.st.Ended = time.Now().Unix()
	r.save()
	if r.st.Errors > 0 && !dry && notificationsOn() {
		notify(fmt.Sprintf("ZFS replication - %d error(s)", r.st.Errors), "Replication finished with "+fmt.Sprint(r.st.Errors)+" error(s). Last: "+r.st.LastError, "warning")
	}
	r.logf("", "Replication finished (jobs=%d, ok=%d, errors=%d).", r.st.JobsRun, r.st.JobsOK, r.st.Errors)
	if r.st.Errors > 0 {
		return 1
	}
	return 0
}

func sendRunTest(ids []string) int {
	r := &sendRunner{ctx: context.Background(), toOut: !sendStdoutIsLog()}
	if len(ids) == 0 {
		r.line("err", "--test needs a job id.")
		return 2
	}
	code := 0
	for _, id := range ids {
		j, ok := sendFindJob(id)
		if !ok {
			r.logf("err", "No job with id '%s' found.", id)
			code = 1
			continue
		}
		r.logf("", "Testing job '%s' (%s)", j.Name, j.ID)
		p := sendPlanJob(r.ctx, j)
		for _, c := range p.Checks {
			lv := c.Level
			if lv == "info" {
				lv = ""
			}
			r.line(lv, c.Text)
		}
		if p.Fatal {
			code = 1
		} else {
			r.logf("ok", "Job '%s': preconditions OK.", j.Name)
		}
	}
	return code
}

func init() {
	register("send-run", "replicate: send-run [--dry-run] [--test] [--force] [job id...] (no id = every enabled job; --force rolls the destination back once, only with job ids)", sendRun)
}
