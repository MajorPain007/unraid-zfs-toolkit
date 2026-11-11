package zt

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type convStoppedSvc struct {
	svc     convService
	folders []string
}

type convRunner struct {
	cfg         convConfig
	ctx         context.Context
	log         *convLog
	status      *convStatusWriter
	only        map[string]bool
	world       *convWorld
	plan        []convFolder
	stopped     []convStoppedSvc
	unsafe      map[string]bool
	converted   []string
	failed      []string
	skipped     []string
	cur         *convJournal
	created     bool
	restored    bool
	interrupted bool
}

func (r *convRunner) dry() bool { return r.cfg.DryRun }

func (r *convRunner) notify(subject, msg, level string) {
	if r.cfg.Notify {
		notify(subject, msg, level)
	}
}

func (r *convRunner) cancelled() bool {
	if r.ctx.Err() != nil {
		r.interrupted = true
		return true
	}
	return false
}

func (r *convRunner) setItem(path string, force bool, fn func(it *convItem)) {
	r.status.update(force, func(st *convStatusFile) {
		for i := range st.Items {
			if st.Items[i].Path == path {
				fn(&st.Items[i])
			}
		}
	})
}

func (r *convRunner) setPhase(phase, folder string) {
	r.status.update(true, func(st *convStatusFile) {
		st.Phase = phase
		st.CurrentFolder = folder
	})
}

func (r *convRunner) preflight() bool {
	r.log.step("Pre-flight checks")
	for _, p := range r.cfg.Problems {
		r.log.warn("%s", p)
	}
	if len(r.cfg.Sources) == 0 {
		r.log.err("No source datasets defined. Check your configuration.")
		return false
	}
	r.world = convLoadWorld()
	if !r.world.ok {
		r.log.err("Cannot list ZFS datasets: %s", r.world.err)
		return false
	}
	good := true
	for _, s := range r.cfg.Sources {
		mp, prob := convSourceProblem(r.world, s.Dataset)
		if prob != "" {
			r.log.err("Source %s", prob)
			good = false
			continue
		}
		if !isDir(mp) {
			r.log.err("Source path does not exist: %s", mp)
			good = false
			continue
		}
		r.log.ok("Source '%s' is valid (%s).", s.Dataset, mp)
	}
	return good
}

func (r *convRunner) recoverInterrupted() bool {
	j, ok := convReadJournal()
	if !ok {
		return true
	}
	path, dataset := j.path(), j.dataset()
	r.log.step("Earlier conversion of '" + j.Entry + "' was cut off")
	w := convLoadWorld()
	if j.Temp == "" || !isDir(j.Temp) {
		r.log.info("Nothing of it is left to finish or undo.")
		convClearJournal()
		return true
	}
	if !w.exists(dataset) {
		if isDir(path) && !dirHasEntries(path) && len(w.mountsInside(path)) == 0 {
			os.Remove(path)
		}
		if !exists(path) && os.Rename(j.Temp, path) == nil {
			r.log.ok("Put '%s' back where it was. Restart whatever uses it if it is running.", j.Entry)
			convClearJournal()
			return true
		}
	} else if w.datasetAt(path) == dataset {
		finish := j.Committed
		if !finish {
			finish = r.copyHolds(j.Temp, path)
		}
		if finish {
			if r.cfg.Cleanup {
				if err := r.removeTree(context.Background(), j.Temp); err != nil {
					r.log.warn("Could not delete %s completely: %s. Delete it by hand.", j.Temp, err)
				}
			}
			r.log.ok("Finished the conversion of '%s': the copy in %s is complete.", j.Entry, dataset)
			r.converted = append(r.converted, j.Entry+" (finished)")
			convClearJournal()
			return true
		}
	}
	r.log.err("The conversion of '%s' was cut off and cannot be finished or undone by itself.", j.Entry)
	r.log.err("The original is in %s. %s holds a copy that does not match it, or is missing.", j.Temp, dataset)
	r.log.err("Keep one of them: delete %s to keep the dataset, or destroy %s and rename %s back to %s. The next run carries on after that.", j.Temp, dataset, j.Temp, path)
	r.notify("ZFS conversion needs attention", "An earlier conversion of '"+j.Entry+"' was cut off and has to be settled by hand. See the conversion log.", "alert")
	return false
}

func (r *convRunner) sizeOf(path string) (int64, int64, bool) {
	u, err := convDirUsage(r.ctx, path)
	if err != nil {
		return -1, -1, false
	}
	return u.Allocated, u.Apparent, true
}

func (r *convRunner) planConversions() {
	r.log.step("Planning")
	r.world = convLoadWorld()
	scans := convClassify(convPlanInput{
		World: r.world, Sources: r.cfg.Sources, ReplaceSpaces: r.cfg.ReplaceSpaces, Buffer: r.cfg.Buffer,
		ReadDir: convReadDir, Size: r.sizeOf,
	})
	items := []convItem{}
	for _, sc := range scans {
		if sc.Error != "" {
			r.log.err("%s", sc.Error)
			continue
		}
		for _, f := range sc.Folders {
			switch f.State {
			case "dataset":
				continue
			case "kept":
				if f.Reason == "kept_copy" {
					r.log.warn("Leaving '%s' alone: %s", f.Name, f.Why)
				} else {
					r.log.warn("Skipping '%s': %s", f.Name, f.Why)
				}
				r.skipped = append(r.skipped, f.Name)
				continue
			case "blocked":
				label := f.Name
				if f.Reason == "bad_name" {
					label = "(control character in name)"
				}
				r.log.warn("Skipping '%s': %s", label, f.Why)
				r.skipped = append(r.skipped, label)
				continue
			}
			if r.only != nil && !r.only[f.Path] {
				r.log.info("Not selected for this run: '%s'.", f.Name)
				continue
			}
			if r.cancelled() {
				return
			}
			r.plan = append(r.plan, f)
			items = append(items, convItem{Path: f.Path, Name: f.Name, Source: f.Source, Dataset: f.Dataset, As: f.As, State: "pending", Size: f.Size, Bytes: f.Bytes})
			if f.As == "" {
				r.log.info("Will convert '%s/%s'.", f.Source, f.Name)
			} else {
				r.log.info("Will convert '%s/%s' as dataset '%s', mounted at the same path.", f.Source, f.Name, f.As)
			}
		}
	}
	r.status.update(true, func(st *convStatusFile) { st.Items = items })
}

func (r *convRunner) plannedOwner(s convService) []string {
	out := []string{}
	for _, f := range r.plan {
		mp := dirOf(f.Path)
		if convUsesFolders(s, []string{mp}, f.Path) {
			out = append(out, f.Path)
		}
	}
	return out
}

func (r *convRunner) dropFromPlan(paths []string, why string) {
	drop := map[string]bool{}
	for _, p := range paths {
		drop[p] = true
	}
	keep := []convFolder{}
	for _, f := range r.plan {
		if drop[f.Path] {
			r.log.err("Not converting '%s': %s", f.Name, why)
			r.failed = append(r.failed, f.Name)
			r.setItem(f.Path, true, func(it *convItem) { it.State = "failed"; it.Message = why })
			continue
		}
		keep = append(keep, f)
	}
	r.plan = keep
}

func (r *convRunner) recordStopped(s convService, folders []string) {
	r.stopped = append(r.stopped, convStoppedSvc{svc: s, folders: folders})
	r.status.update(true, func(st *convStatusFile) {
		st.Stopped = append(st.Stopped, convUser{Kind: s.Kind, Name: s.Name, Running: false})
	})
}

func (r *convRunner) stopContainers() {
	r.log.step("Stop Docker containers (if needed)")
	r.setPhase("stopping", "")
	list, warn := convContainers(false)
	if warn != "" {
		if r.cfg.Containers {
			r.log.warn("%s", warn)
		} else {
			r.log.info("%s", warn)
		}
		return
	}
	if len(list) == 0 {
		r.log.info("No running containers.")
	}
	names := []string{}
	for _, c := range list {
		if r.cancelled() {
			return
		}
		folders := r.plannedOwner(c)
		if len(folders) == 0 {
			if len(c.Paths) == 0 {
				r.log.info("Container %s: no bind mounts, skipping.", c.Name)
			} else {
				r.log.info("Container %s: nothing of it is converted in this run, no action needed.", c.Name)
			}
			continue
		}
		r.log.info("Container %s: '%s' is about to be converted -> will stop container.", c.Name, folders[0])
		if r.dry() {
			r.log.info("Dry run: would stop container %s", c.Name)
			r.recordStopped(c, folders)
			names = append(names, c.Name)
			continue
		}
		r.log.info("Stopping container %s...", c.Name)
		res := runCmd(5*time.Minute, convDockerBin, "stop", c.ID)
		if !res.OK() {
			r.log.warn("docker stop %s: %s", c.Name, res.Message())
		}
		chk := runCmd(30*time.Second, convDockerBin, "inspect", "--format", "{{.State.Running}}", c.ID)
		if !res.OK() && !(chk.OK() && strings.TrimSpace(chk.Stdout) == "false") {
			r.dropFromPlan(folders, "container "+c.Name+" could not be stopped.")
			continue
		}
		r.recordStopped(c, folders)
		names = append(names, c.Name)
	}
	if len(names) > 0 {
		r.log.ok("Stopped containers: %s", strings.Join(names, " "))
	}
}

func (r *convRunner) stopVMs() {
	r.log.step("Stop Virtual Machines (if needed)")
	r.setPhase("stopping", "")
	list, warn := convVMs(false)
	if warn != "" {
		if r.cfg.VMs {
			r.log.warn("%s", warn)
		} else {
			r.log.info("%s", warn)
		}
		return
	}
	if len(list) == 0 {
		r.log.info("No running VMs.")
	}
	names := []string{}
	for _, vm := range list {
		if r.cancelled() {
			return
		}
		folders := r.plannedOwner(vm)
		if len(folders) == 0 {
			if len(vm.Paths) == 0 {
				r.log.info("No disk found for VM %s. Skipping.", vm.Name)
			} else {
				r.log.info("VM %s: nothing of it is converted in this run, no action needed.", vm.Name)
			}
			continue
		}
		r.log.info("VM %s: vdisk folder '%s' is about to be converted -> will stop VM.", vm.Name, folders[0])
		if r.dry() {
			r.log.info("Dry run: would stop VM %s", vm.Name)
			r.recordStopped(vm, folders)
			names = append(names, vm.Name)
			continue
		}
		runCmd(time.Minute, convVirshBin, "shutdown", vm.Name)
		start := time.Now()
		for convVMRunning(vm.Name) {
			if time.Since(start) >= time.Duration(r.cfg.VMWait)*time.Second {
				r.log.warn("VM %s did not shut down after %ds. Forcing shutdown.", vm.Name, r.cfg.VMWait)
				runCmd(time.Minute, convVirshBin, "destroy", vm.Name)
				break
			}
			if !convSleep(r.ctx, 2*time.Second) {
				break
			}
		}
		if convVMRunning(vm.Name) {
			r.recordStopped(vm, folders)
			r.dropFromPlan(folders, "VM "+vm.Name+" is still running.")
			continue
		}
		r.recordStopped(vm, folders)
		names = append(names, vm.Name)
	}
	if len(names) > 0 {
		r.log.ok("Stopped VMs: %s", strings.Join(names, " "))
	}
}

func (r *convRunner) heldBack(s convStoppedSvc) string {
	for _, f := range s.folders {
		if r.unsafe[f] {
			return f
		}
	}
	return ""
}

func (r *convRunner) restoreServices() {
	if r.restored {
		return
	}
	r.restored = true
	if len(r.stopped) == 0 {
		return
	}
	r.setPhase("restarting", "")
	for _, kind := range []string{"container", "vm"} {
		header := false
		for _, s := range r.stopped {
			if s.svc.Kind != kind {
				continue
			}
			if !header {
				if kind == "container" {
					r.log.step("Restart Docker containers")
				} else {
					r.log.step("Restart Virtual Machines")
				}
				header = true
			}
			label := "Container"
			if kind == "vm" {
				label = "VM"
			}
			if f := r.heldBack(s); f != "" {
				r.log.err("%s %s stays stopped: its folder %s could not be put back.", label, s.svc.Name, f)
				continue
			}
			if r.dry() {
				r.log.info("Dry run: would restart %s %s", strings.ToLower(label), s.svc.Name)
				continue
			}
			r.log.info("Restarting %s %s...", strings.ToLower(label), s.svc.Name)
			var res result
			if kind == "container" {
				res = runCmd(5*time.Minute, convDockerBin, "start", s.svc.Name)
			} else {
				if convVMRunning(s.svc.Name) {
					r.log.warn("VM %s is still running, it was not shut down.", s.svc.Name)
					continue
				}
				res = runCmd(2*time.Minute, convVirshBin, "start", s.svc.Name)
			}
			if !res.OK() {
				r.log.err("Could not restart %s %s: %s", strings.ToLower(label), s.svc.Name, res.Message())
				r.notify("ZFS conversion: restart failed", label+" "+s.svc.Name+" could not be restarted after the conversion. Start it by hand.", "warning")
			}
		}
	}
}

func (r *convRunner) checkSpace(source string, size int64) bool {
	res := zfs("list", "-H", "-p", "-o", "avail", source)
	if !res.OK() {
		r.log.err("Cannot query available space on %s", source)
		return false
	}
	avail := toInt64(strings.TrimSpace(res.Stdout))
	need := convRequired(size, r.cfg.Buffer)
	if avail < need {
		r.log.err("Insufficient space on %s: available %s, required %s (folder + %d%% buffer).", source, humanBytes(float64(avail)), humanBytes(float64(need)), r.cfg.Buffer)
		return false
	}
	return true
}

func (r *convRunner) validate(ctx context.Context, src, dst string) bool {
	r.log.info("Validating copy...")
	a, err1 := convDirUsage(ctx, src)
	b, err2 := convDirUsage(ctx, dst)
	if err1 != nil || err2 != nil {
		r.log.err("VALIDATION FAILED: cannot read both trees (%v %v)", err1, err2)
		return false
	}
	r.log.info("  Source : %d entries, %d bytes", a.Entries, a.Apparent)
	r.log.info("  Dest   : %d entries, %d bytes", b.Entries, b.Apparent)
	if a.Entries != b.Entries {
		r.log.err("VALIDATION FAILED: entry count mismatch (src=%d, dst=%d)", a.Entries, b.Entries)
		return false
	}
	if a.Apparent > 0 {
		diff := a.Apparent - b.Apparent
		if diff < 0 {
			diff = -diff
		}
		pct := diff * 100 / a.Apparent
		if pct > int64(r.cfg.Tolerance) {
			r.log.err("VALIDATION FAILED: size difference %d%% exceeds tolerance %d%% (src=%d, dst=%d)", pct, r.cfg.Tolerance, a.Apparent, b.Apparent)
			return false
		}
	}
	r.log.ok("Validation passed.")
	return true
}

func (r *convRunner) copyHolds(src, dst string) bool {
	r.log.info("Checking that the copy holds every file of the original...")
	miss, err := convMissingFrom(src, dst)
	if err != nil {
		r.log.err("Cannot compare %s with %s: %s", src, dst, err)
		return false
	}
	if miss != "" {
		r.log.err("The copy is incomplete: %s is missing or differs from the original.", dst+miss)
		return false
	}
	r.log.ok("The copy holds every file of the original.")
	return true
}

func (r *convRunner) removeTree(ctx context.Context, p string) error {
	if p == "" || p == "/" || !strings.HasSuffix(p, "_temp") {
		return fmt.Errorf("refusing to delete %q", p)
	}
	return convExec(ctx, nil, nil, "rm", "-rf", "--", p)
}

func convExec(ctx context.Context, stdout func(string), stderr func(string), name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var outPipe, errPipe io.ReadCloser
	var err error
	if outPipe, err = cmd.StdoutPipe(); err != nil {
		return err
	}
	if errPipe, err = cmd.StderrPipe(); err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		case <-done:
		}
	}()
	readers := make(chan struct{}, 2)
	pump := func(rd io.Reader, fn func(string)) {
		sc := bufio.NewScanner(rd)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		sc.Split(convSplitCR)
		for sc.Scan() {
			if fn != nil {
				fn(sc.Text())
			}
		}
		io.Copy(io.Discard, rd)
		readers <- struct{}{}
	}
	go pump(outPipe, stdout)
	go pump(errPipe, stderr)
	<-readers
	<-readers
	err = cmd.Wait()
	close(done)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func convSplitCR(data []byte, atEOF bool) (int, []byte, error) {
	for i, c := range data {
		if c == '\n' || c == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

var convProgressRe = regexp.MustCompile(`^\s*([\d,.]+)\s+(\d+)%\s+(\S+)`)

func (r *convRunner) copyTree(item string, src, dst string) error {
	var lastBytes int64
	lastAt := time.Now()
	var rate int64
	onOut := func(line string) {
		m := convProgressRe.FindStringSubmatch(line)
		if m == nil {
			if t := strings.TrimSpace(line); t != "" {
				r.log.info("rsync: %s", t)
			}
			return
		}
		n, _ := strconv.ParseInt(strings.NewReplacer(",", "", ".", "").Replace(m[1]), 10, 64)
		if dt := time.Since(lastAt).Seconds(); dt >= 1 {
			inst := float64(n-lastBytes) / dt
			if rate == 0 {
				rate = int64(inst)
			} else {
				rate = int64(float64(rate)*0.7 + inst*0.3)
			}
			lastBytes, lastAt = n, time.Now()
		}
		r.setItem(item, false, func(it *convItem) { it.Done = n; it.Rate = rate })
	}
	onErr := func(line string) {
		if t := strings.TrimSpace(line); t != "" {
			r.log.warn("rsync: %s", t)
		}
	}
	return convExec(r.ctx, onOut, onErr, convRsyncBin, "-a", "-H", "-A", "-X", "--numeric-ids", "--sparse", "--info=progress2", src+"/", dst+"/")
}

func (r *convRunner) destroyDataset(ds string) error {
	var last result
	for i := 0; i < 5; i++ {
		last = zfs("destroy", ds)
		if last.OK() {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("%s", last.Message())
}

func (r *convRunner) rollbackCurrent() bool {
	if r.cur == nil {
		return true
	}
	j := r.cur
	path, ds := j.path(), j.dataset()
	if !r.created && !exists(j.Temp) && exists(path) {
		convClearJournal()
		r.cur = nil
		return true
	}
	if r.created {
		if err := r.destroyDataset(ds); err != nil {
			r.log.err("Cannot remove the incomplete dataset '%s': %s", ds, err)
			r.log.err("The original is untouched in %s.", j.Temp)
			return false
		}
		r.created = false
		os.Remove(path)
	} else if isDir(path) && !dirHasEntries(path) {
		os.Remove(path)
	}
	if exists(path) {
		r.log.err("Cannot move %s back: %s exists. The original is untouched in %s.", j.Temp, path, j.Temp)
		return false
	}
	if err := os.Rename(j.Temp, path); err != nil {
		r.log.err("Cannot move %s back to %s: %s. The original is untouched in %s.", j.Temp, path, err, j.Temp)
		return false
	}
	convClearJournal()
	r.cur = nil
	return true
}

func (r *convRunner) abandon(f convFolder, why string) {
	if r.rollbackCurrent() {
		r.log.warn("Put '%s' back the way it was.", f.Name)
		r.setItem(f.Path, true, func(it *convItem) {
			it.State = "failed"
			it.Message = why + " - put back the way it was."
			it.Phase = ""
		})
	} else {
		r.unsafe[f.Path] = true
		r.cur = nil
		r.created = false
		r.notify("ZFS conversion needs attention", "The conversion of '"+f.Name+"' failed and could not be undone. See the conversion log.", "alert")
		r.setItem(f.Path, true, func(it *convItem) {
			it.State = "failed"
			it.Message = why + " - could NOT be undone, see the log."
			it.Phase = ""
		})
	}
	r.failed = append(r.failed, f.Name)
}

func (r *convRunner) convertOne(f convFolder) {
	path, temp := f.Path, f.Path+"_temp"
	w := convLoadWorld()
	if !isDir(path) || w.datasetAt(path) != "" {
		r.log.info("Skipping '%s': no longer a plain folder.", f.Name)
		r.setItem(path, true, func(it *convItem) { it.State = "skipped"; it.Message = "No longer a plain folder." })
		return
	}
	r.setPhase("measuring", f.Name)
	r.setItem(path, true, func(it *convItem) { it.State = "running"; it.Phase = "measuring" })
	u, err := convDirUsage(r.ctx, path)
	if err != nil {
		if r.cancelled() {
			r.setItem(path, true, func(it *convItem) { it.State = "pending"; it.Phase = "" })
			return
		}
		r.log.err("Cannot read '%s': %s", f.Name, err)
		r.failed = append(r.failed, f.Name)
		r.setItem(path, true, func(it *convItem) { it.State = "failed"; it.Message = err.Error() })
		return
	}
	r.setItem(path, true, func(it *convItem) { it.Size = u.Allocated; it.Bytes = u.Apparent })
	r.log.info("Processing '%s' (%s)...", f.Name, humanBytes(float64(u.Allocated)))
	if !r.checkSpace(f.Source, u.Allocated) {
		r.log.warn("Skipping '%s' due to insufficient space.", f.Name)
		r.failed = append(r.failed, f.Name)
		r.setItem(path, true, func(it *convItem) { it.State = "failed"; it.Message = "Not enough space." })
		return
	}
	if r.dry() {
		r.log.info("Dry run: would create dataset '%s' from folder '%s'", f.Dataset, f.Name)
		r.setItem(path, true, func(it *convItem) { it.State = "done"; it.Message = "Dry run - nothing changed."; it.Phase = "" })
		return
	}
	if exists(temp) {
		r.log.err("Temp path already exists: %s. Resolve it manually before retrying '%s'.", temp, f.Name)
		r.failed = append(r.failed, f.Name)
		r.setItem(path, true, func(it *convItem) { it.State = "failed"; it.Message = temp + " already exists." })
		return
	}
	if r.cancelled() {
		r.setItem(path, true, func(it *convItem) { it.State = "pending"; it.Phase = "" })
		return
	}
	j := convJournal{Source: f.Source, Entry: f.Name, Name: convBase(f.Dataset), Temp: temp}
	if !convWriteJournal(j) {
		r.log.err("Cannot write the journal %s - not touching '%s' without it.", convJournalPath, f.Name)
		r.failed = append(r.failed, f.Name)
		r.setItem(path, true, func(it *convItem) { it.State = "failed"; it.Message = "Cannot write the journal." })
		return
	}
	r.cur, r.created = &j, false
	if err := os.Rename(path, temp); err != nil {
		r.log.err("Failed to rename '%s' to temp: %s. Skipping.", f.Name, err)
		convClearJournal()
		r.cur = nil
		r.failed = append(r.failed, f.Name)
		r.setItem(path, true, func(it *convItem) { it.State = "failed"; it.Message = "Cannot rename to " + temp + "." })
		return
	}
	args := []string{"create"}
	if f.As != "" || dirOf(path) != strings.TrimRight(r.world.datasets[f.Source].Mountpoint, "/") {
		args = append(args, "-o", "mountpoint="+path)
	}
	r.setItem(path, true, func(it *convItem) { it.Phase = "creating" })
	res := zfs(append(args, f.Dataset)...)
	if !res.OK() {
		r.log.err("Failed to create dataset '%s': %s", f.Dataset, res.Message())
		r.abandon(f, "Creating the dataset failed")
		return
	}
	r.created = true
	if convLoadWorld().datasetAt(path) != f.Dataset {
		r.log.err("Dataset '%s' was created but is not mounted at %s.", f.Dataset, path)
		r.abandon(f, "The new dataset was not mounted")
		return
	}
	if r.cancelled() {
		r.abandon(f, "Stopped")
		return
	}
	r.log.info("Copying data with rsync...")
	r.setPhase("copying", f.Name)
	r.setItem(path, true, func(it *convItem) { it.Phase = "copying" })
	if err := r.copyTree(path, temp, path); err != nil {
		if r.cancelled() {
			r.log.warn("Stopped while converting '%s' - putting it back the way it was.", f.Name)
			r.abandon(f, "Stopped")
			return
		}
		r.log.err("rsync failed for '%s': %s", f.Name, err)
		r.abandon(f, "Copying failed")
		return
	}
	r.setPhase("validating", f.Name)
	r.setItem(path, true, func(it *convItem) { it.Phase = "validating"; it.Done = it.Bytes })
	if !r.validate(r.ctx, temp, path) {
		if r.cancelled() {
			r.log.warn("Stopped while converting '%s' - putting it back the way it was.", f.Name)
			r.abandon(f, "Stopped")
			return
		}
		r.abandon(f, "Validation failed")
		return
	}
	j.Committed = true
	if !convWriteJournal(j) {
		r.log.warn("Cannot mark the journal as committed.")
	}
	r.created = false
	r.cur = nil
	if r.cfg.Cleanup {
		r.setItem(path, true, func(it *convItem) { it.Phase = "cleanup" })
		if err := r.removeTree(r.ctx, temp); err != nil {
			if r.cancelled() {
				r.log.warn("Stopped while deleting %s. The dataset is complete; the next run deletes the rest.", temp)
			} else {
				r.log.warn("Could not delete %s completely: %s. Delete it by hand.", temp, err)
				convClearJournal()
			}
		} else {
			r.log.ok("Cleaned up temp dir.")
			convClearJournal()
		}
	} else {
		r.log.info("Cleanup disabled. The original is kept as %s.", temp)
		convClearJournal()
	}
	r.converted = append(r.converted, f.Name)
	r.setItem(path, true, func(it *convItem) { it.State = "done"; it.Phase = ""; it.Message = "" })
	r.log.ok("Successfully converted '%s' -> dataset '%s'", f.Name, f.Dataset)
}

func (r *convRunner) summary() {
	r.log.step("Summary")
	if len(r.converted) > 0 {
		r.log.ok("Successfully converted %d folder(s):", len(r.converted))
		for _, f := range r.converted {
			r.log.raw("  + " + f)
		}
	}
	if len(r.skipped) > 0 {
		r.log.warn("%d folder(s) left as they are:", len(r.skipped))
		for _, f := range r.skipped {
			r.log.raw("  = " + f)
		}
	}
	if len(r.failed) > 0 {
		r.log.err("%d folder(s) failed:", len(r.failed))
		for _, f := range r.failed {
			r.log.raw("  - " + f)
		}
	}
	if r.dry() {
		return
	}
	if len(r.converted) > 0 && len(r.failed) == 0 {
		r.notify("ZFS Conversion Complete", fmt.Sprintf("Successfully converted %d folder(s) to ZFS datasets.", len(r.converted)), "normal")
	} else if len(r.failed) > 0 {
		r.notify("ZFS Conversion - Issues Detected", fmt.Sprintf("Converted: %d, Failed: %d. Check the logs.", len(r.converted), len(r.failed)), "warning")
	}
}

func (r *convRunner) emergency() {
	if r.cur != nil {
		r.log.warn("Stopped while converting '%s' - putting it back the way it was.", r.cur.Entry)
		if !r.rollbackCurrent() {
			r.unsafe[r.cur.path()] = true
			r.notify("ZFS conversion needs attention", "A stopped conversion of '"+r.cur.Entry+"' could not be undone. See the conversion log.", "alert")
		}
	}
	r.restoreServices()
}

func (r *convRunner) run() (code int) {
	defer func() {
		if p := recover(); p != nil {
			r.log.err("Internal error: %v", p)
			r.emergency()
			code = 1
		}
	}()
	r.log.info("ZFS Toolkit starting...")
	if r.dry() {
		r.log.warn("DRY RUN MODE - no changes will be made.")
	}
	if !r.preflight() {
		return 1
	}
	if !r.dry() {
		if !r.recoverInterrupted() {
			return 1
		}
	} else if j, ok := convReadJournal(); ok {
		r.log.warn("An earlier conversion of '%s' was cut off. A real run settles that first.", j.Entry)
	}
	r.setPhase("planning", "")
	r.planConversions()
	if r.cancelled() {
		r.log.warn("Interrupted.")
		return 130
	}
	if len(r.plan) == 0 {
		if len(r.skipped) > 0 {
			r.log.ok("Nothing to convert - %d folder(s) left as they are, see above.", len(r.skipped))
		} else {
			r.log.ok("Nothing to convert. All folders are already datasets.")
		}
		r.log.info("Script execution completed successfully.")
		return 0
	}
	r.stopContainers()
	r.stopVMs()
	r.log.step("Convert")
	for _, f := range r.plan {
		if r.cancelled() {
			break
		}
		r.convertOne(f)
	}
	if r.cur != nil {
		r.emergency()
	}
	r.restoreServices()
	if r.cancelled() {
		r.log.warn("Interrupted.")
		for _, f := range r.plan {
			r.setItem(f.Path, true, func(it *convItem) {
				if it.State == "pending" || it.State == "running" {
					it.State = "cancelled"
					it.Phase = ""
				}
			})
		}
	}
	r.summary()
	if r.interrupted {
		return 130
	}
	r.log.info("Script execution completed successfully.")
	return 0
}
