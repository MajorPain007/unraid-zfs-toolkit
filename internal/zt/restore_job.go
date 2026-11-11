package zt

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	reRestoreID   = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{4}$`)
	reRsyncPct    = regexp.MustCompile(`^\s*([0-9,]+)\s+([0-9]+)%`)
	reRsyncNoAttr = regexp.MustCompile(`(?i)(acl|xattr)[^\n]*(not supported|operation not supported)`)
	rsyncBin      = envOr("ZDC_RSYNC", "rsync")
)

const restoreKeep = 20

type restoreItem struct {
	Rel   string `json:"rel"`
	Src   string `json:"src"`
	Into  string `json:"into"`
	Type  string `json:"type"`
	Bytes int64  `json:"bytes"`
}

type restoreJob struct {
	ID        string        `json:"id"`
	Dataset   string        `json:"dataset"`
	Snapshot  string        `json:"snapshot"`
	Mount     string        `json:"mountpoint"`
	Target    string        `json:"target"`
	TargetDir string        `json:"target_dir"`
	Overwrite string        `json:"overwrite"`
	Delete    bool          `json:"delete"`
	Items     []restoreItem `json:"items"`
	Created   int64         `json:"created"`
	Nano      int64         `json:"nano"`
	State     string        `json:"state"`
	PID       int           `json:"pid"`
	Started   int64         `json:"started"`
	Finished  int64         `json:"finished"`
	Done      int           `json:"done"`
	Failed    int           `json:"failed"`
	Current   string        `json:"current"`
	Bytes     int64         `json:"bytes"`
	BytesDone int64         `json:"bytes_done"`
	Message   string        `json:"message"`
	Dismissed bool          `json:"dismissed"`
}

func restoreDir() string { return TmpDir + "/restores" }

func restorePath(id, ext string) string { return restoreDir() + "/" + id + ext }

func restoreLoad(id string) (*restoreJob, bool) {
	if !reRestoreID.MatchString(id) {
		return nil, false
	}
	var j restoreJob
	if !jsonReadInto(restorePath(id, ".json"), &j) || j.ID != id {
		return nil, false
	}
	return &j, true
}

func restoreUpdate(id string, fn func(j *restoreJob) bool) (*restoreJob, bool) {
	if !reRestoreID.MatchString(id) {
		return nil, false
	}
	l := lockFile(restorePath(id, ".lock"), true)
	defer l.Unlock()
	j, ok := restoreLoad(id)
	if !ok {
		return nil, false
	}
	if fn(j) {
		jsonWrite(restorePath(id, ".json"), j, 0o644)
	}
	return j, true
}

func restoreWorkerAlive(j *restoreJob) bool {
	if j.PID <= 1 || !pidAlive(j.PID) {
		return false
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(j.PID) + "/cmdline")
	if err != nil {
		return false
	}
	return bytes.Contains(b, []byte("restore-run\x00"+j.ID))
}

func restoreActive(j *restoreJob) bool {
	return j.State == "queued" || j.State == "running"
}

func restoreCheckAlive(j *restoreJob) *restoreJob {
	if !restoreActive(j) {
		return j
	}
	if j.State == "queued" && now()-j.Created < 30 {
		return j
	}
	if j.State == "running" && restoreWorkerAlive(j) {
		return j
	}
	nj, ok := restoreUpdate(j.ID, func(x *restoreJob) bool {
		if !restoreActive(x) || (x.State == "running" && restoreWorkerAlive(x)) {
			return false
		}
		x.State = "error"
		x.Finished = now()
		x.Message = "The restore stopped unexpectedly (its process is gone). Files finished before that are in place."
		return true
	})
	if ok {
		return nj
	}
	return j
}

func restoreAll() []*restoreJob {
	m, _ := filepath.Glob(restoreDir() + "/*.json")
	out := []*restoreJob{}
	for _, f := range m {
		id := strings.TrimSuffix(filepath.Base(f), ".json")
		if j, ok := restoreLoad(id); ok {
			out = append(out, restoreCheckAlive(j))
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Nano != out[b].Nano {
			return out[a].Nano > out[b].Nano
		}
		return out[a].ID > out[b].ID
	})
	return out
}

func restorePrune() {
	all := restoreAll()
	kept := 0
	for _, j := range all {
		if restoreActive(j) {
			continue
		}
		kept++
		if kept > restoreKeep {
			for _, ext := range []string{".json", ".log", ".lock"} {
				os.Remove(restorePath(j.ID, ext))
			}
		}
	}
}

func restoreMounts(into string) []string {
	out := []string{}
	for _, m := range mountInfo("self") {
		if m.Point != into && under(m.Point, into) {
			out = append(out, m.Point[len(strings.TrimRight(into, "/")):])
		}
	}
	sort.Strings(out)
	return out
}

func restorePlan(p M) (*restoreJob, error) {
	b, err := brOpen(p)
	if err != nil {
		return nil, err
	}
	raw, err := pjsonList(p, "items")
	if err != nil {
		return nil, apiFail("Nothing selected.")
	}
	if len(raw) == 0 {
		return nil, apiFail("Nothing selected.")
	}
	if len(raw) > 1000 {
		return nil, apiFail("Too many items - select at most 1000, or restore their folder.")
	}
	target := pstr(p, "target", "original")
	if !inList(target, "original", "path") {
		return nil, apiFail("Unknown restore target.")
	}
	overwrite := pstr(p, "overwrite", "changed")
	if !inList(overwrite, "changed", "never", "always") {
		return nil, apiFail("Unknown overwrite rule.")
	}
	job := &restoreJob{
		Dataset:   b.Dataset,
		Snapshot:  b.Snap,
		Mount:     b.Mount,
		Target:    target,
		Overwrite: overwrite,
		Delete:    target == "original" && toBool(param(p, "delete", false)),
	}
	if target == "path" {
		dir, err := restoreTargetDir(b, strings.TrimSpace(pstr(p, "target_path", "")))
		if err != nil {
			return nil, err
		}
		job.TargetDir = dir
	}
	rels := []string{}
	seen := map[string]bool{}
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, apiFail("Malformed item list.")
		}
		rel, err := brRel(s)
		if err != nil {
			return nil, err
		}
		if !seen[rel] {
			seen[rel] = true
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	kept := []string{}
	for _, r := range rels {
		covered := false
		for _, k := range kept {
			if k == "/" || under(r, k) {
				covered = true
				break
			}
		}
		if !covered {
			kept = append(kept, r)
		}
	}
	intos := map[string]string{}
	for _, rel := range kept {
		src, info, err := brWalk(b.Base, rel, true)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, apiFailf("%s does not exist in this snapshot.", rel)
			}
			return nil, err
		}
		typ := brType(info.Mode())
		if typ == "other" {
			return nil, apiFailf("%s is a device, socket or pipe and cannot be restored.", rel)
		}
		var into string
		if target == "original" {
			into = brJoin(b.Mount, rel)
			if l := brLiveLink(b.Mount, brParent(rel)); l != "" {
				return nil, apiFailf("%s is a symbolic link in the live data. Restoring through it could write outside the dataset - restore into another folder instead.", l)
			}
		} else {
			name := brBase(rel)
			if rel == "/" {
				name = brBase("/" + b.Dataset)
			}
			into = strings.TrimRight(job.TargetDir, "/") + "/" + name
		}
		if prev, dup := intos[into]; dup {
			return nil, apiFailf("%s and %s would both land at %s - restore them separately.", prev, rel, into)
		}
		intos[into] = rel
		if strings.Contains(into+"/", "/.zfs/") {
			return nil, apiFail("The destination is inside a snapshot, which is read-only.")
		}
		if li, err := os.Lstat(into); err == nil {
			lt := brType(li.Mode())
			switch {
			case lt == "symlink" && (typ != "symlink" || isDir(into)):
				return nil, apiFailf("%s is a symbolic link now. Restoring through it could write elsewhere - restore into another folder instead.", into)
			case lt == "dir" && typ != "dir":
				return nil, apiFailf("%s is a folder now, but a file in the snapshot. Restore it into another folder instead.", into)
			case lt != "dir" && lt != "symlink" && typ == "dir":
				return nil, apiFailf("%s is a file now, but a folder in the snapshot. Restore it into another folder instead.", into)
			}
		}
		job.Items = append(job.Items, restoreItem{Rel: rel, Src: src, Into: into, Type: typ})
	}
	return job, nil
}

func restoreTargetDir(b *brSnap, tp string) (string, error) {
	if tp == "" {
		return "", apiFail("Choose a folder to restore into.")
	}
	var dir string
	if strings.HasPrefix(tp, "/") {
		dir = absPath(tp)
		if dir == "" {
			return "", apiFail("The destination may not contain .. or control characters.")
		}
		if !under(dir, "/mnt") || dir == "/mnt" {
			return "", apiFail("The destination must be inside /mnt - a share, pool or disk.")
		}
	} else {
		rel, err := brRel(tp)
		if err != nil {
			return "", err
		}
		if l := brLiveLink(b.Mount, rel); l != "" {
			return "", apiFailf("%s is a symbolic link. Pick the real folder instead.", l)
		}
		dir = brJoin(b.Mount, rel)
	}
	if strings.Contains(dir+"/", "/.zfs/") || strings.HasSuffix(dir, "/.zfs") {
		return "", apiFail("The destination is inside a snapshot, which is read-only.")
	}
	if st, err := os.Stat(dir); err == nil && !st.IsDir() {
		return "", apiFailf("%s is a file, not a folder.", dir)
	}
	return dir, nil
}

type previewFile struct {
	Path     string `json:"path"`
	Type     string `json:"type"`
	Size     int64  `json:"size"`
	Mtime    int64  `json:"mtime"`
	NewSize  int64  `json:"new_size"`
	NewMtime int64  `json:"new_mtime"`
}

type restoreStats struct {
	Files    int           `json:"files"`
	Dirs     int           `json:"dirs"`
	Bytes    int64         `json:"bytes"`
	New      int           `json:"new"`
	Replace  int           `json:"replace"`
	Same     int           `json:"same"`
	Kept     int           `json:"kept"`
	Remove   int           `json:"remove"`
	Copy     int64         `json:"copy_bytes"`
	Replaced []previewFile `json:"replaced"`
	Removed  []string      `json:"removed"`
	Partial  bool          `json:"partial"`
}

var errPreviewStop = errors.New("stop")

func restoreSame(a, b os.FileInfo, srcPath, dstPath string) bool {
	at, bt := brType(a.Mode()), brType(b.Mode())
	if at != bt {
		return false
	}
	switch at {
	case "file":
		return a.Size() == b.Size() && a.ModTime().Unix() == b.ModTime().Unix()
	case "symlink":
		x, _ := os.Readlink(srcPath)
		y, _ := os.Readlink(dstPath)
		return x == y
	}
	return true
}

func restorePreview(job *restoreJob, budget time.Duration, maxEntries int) restoreStats {
	st := restoreStats{Replaced: []previewFile{}, Removed: []string{}}
	deadline := time.Now().Add(budget)
	visited := 0
	stop := func() bool {
		visited++
		if visited > maxEntries || (visited%256 == 0 && time.Now().After(deadline)) {
			st.Partial = true
			return true
		}
		return false
	}
	for _, it := range job.Items {
		filepath.WalkDir(it.Src, func(path string, de fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if stop() {
				return errPreviewStop
			}
			suffix := strings.TrimPrefix(path, it.Src)
			if it.Type == "dir" && suffix == "/.zfs" {
				return filepath.SkipDir
			}
			info, err := de.Info()
			if err != nil {
				return nil
			}
			dst := it.Into + suffix
			if info.IsDir() {
				st.Dirs++
				return nil
			}
			st.Files++
			if info.Mode().IsRegular() {
				st.Bytes += info.Size()
			}
			li, err := os.Lstat(dst)
			if err != nil {
				st.New++
				st.Copy += info.Size()
				return nil
			}
			if restoreSame(info, li, path, dst) && job.Overwrite != "always" {
				st.Same++
				return nil
			}
			if job.Overwrite == "never" {
				st.Kept++
				return nil
			}
			if restoreSame(info, li, path, dst) {
				st.Same++
			} else {
				st.Replace++
				if len(st.Replaced) < 200 {
					st.Replaced = append(st.Replaced, previewFile{Path: dst, Type: brType(li.Mode()), Size: li.Size(), Mtime: li.ModTime().Unix(), NewSize: info.Size(), NewMtime: info.ModTime().Unix()})
				}
			}
			st.Copy += info.Size()
			return nil
		})
		if st.Partial {
			break
		}
		if job.Delete && it.Type == "dir" && isDir(it.Into) {
			skip := map[string]bool{"/.zfs": true}
			for _, m := range restoreMounts(it.Into) {
				skip[m] = true
			}
			filepath.WalkDir(it.Into, func(path string, de fs.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if stop() {
					return errPreviewStop
				}
				suffix := strings.TrimPrefix(path, it.Into)
				if suffix == "" {
					return nil
				}
				if skip[suffix] {
					return filepath.SkipDir
				}
				if _, err := os.Lstat(it.Src + suffix); err != nil {
					st.Remove++
					if len(st.Removed) < 200 {
						st.Removed = append(st.Removed, path)
					}
					if de.IsDir() {
						return filepath.SkipDir
					}
				}
				return nil
			})
		}
		if st.Partial {
			break
		}
	}
	return st
}

func restoreView(j *restoreJob) M {
	items := make([]string, 0, len(j.Items))
	for i, it := range j.Items {
		if i >= 50 {
			break
		}
		items = append(items, it.Rel)
	}
	pct := 0.0
	if j.State == "done" {
		pct = 100
	} else if j.Bytes > 0 {
		pct = float64(j.BytesDone) * 100 / float64(j.Bytes)
		if pct > 100 {
			pct = 100
		}
	}
	return M{
		"id":         j.ID,
		"dataset":    j.Dataset,
		"snapshot":   j.Snapshot,
		"target":     j.Target,
		"target_dir": j.TargetDir,
		"overwrite":  j.Overwrite,
		"delete":     j.Delete,
		"items":      items,
		"total":      len(j.Items),
		"state":      j.State,
		"created":    j.Created,
		"started":    j.Started,
		"finished":   j.Finished,
		"done":       j.Done,
		"failed":     j.Failed,
		"current":    j.Current,
		"bytes":      j.Bytes,
		"bytes_done": j.BytesDone,
		"percent":    pct,
		"message":    j.Message,
		"dismissed":  j.Dismissed,
	}
}

func apiRestorePreview(p M) (M, error) {
	job, err := restorePlan(p)
	if err != nil {
		return nil, err
	}
	into := make([]M, 0, len(job.Items))
	for _, it := range job.Items {
		_, e := os.Lstat(it.Into)
		into = append(into, M{"rel": it.Rel, "into": it.Into, "type": it.Type, "exists": e == nil})
	}
	return M{"items": into, "target_dir": job.TargetDir, "stats": restorePreview(job, 6*time.Second, 200000)}, nil
}

func apiRestoreStart(p M) (M, error) {
	job, err := restorePlan(p)
	if err != nil {
		return nil, err
	}
	if _, err := exec.LookPath(rsyncBin); err != nil {
		return nil, apiFail("rsync is not installed, so nothing can be restored.")
	}
	os.MkdirAll(restoreDir(), 0o755)
	job.ID = time.Now().Format("20060102-150405") + "-" + randHex(2)
	job.Created = now()
	job.Nano = time.Now().UnixNano()
	job.State = "queued"
	if !jsonWrite(restorePath(job.ID, ".json"), job, 0o644) {
		return nil, apiFail("Cannot write the restore job to " + restoreDir() + ".")
	}
	restoreSpawn("restore-run", job.ID)
	return M{"id": job.ID, "restore": restoreView(job)}, nil
}

func restoreSpawn(args ...string) {
	cmd := exec.Command(SelfBin, args...)
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Env = os.Environ()
	if cmd.Start() == nil {
		cmd.Process.Release()
	}
}

func apiRestoreStatus(p M) (M, error) {
	id := pstr(p, "id", "")
	j, ok := restoreLoad(id)
	if !ok {
		return nil, apiFail("That restore is not known (any more).")
	}
	j = restoreCheckAlive(j)
	lines := toInt(param(p, "lines", 40))
	if lines < 1 || lines > 2000 {
		lines = 40
	}
	return M{"restore": restoreView(j), "log": tail(restorePath(id, ".log"), lines, 262144)}, nil
}

func apiRestoreList(p M) (M, error) {
	out := []M{}
	for _, j := range restoreAll() {
		out = append(out, restoreView(j))
	}
	return M{"restores": out}, nil
}

func apiRestoreStop(p M) (M, error) {
	id := pstr(p, "id", "")
	j, ok := restoreLoad(id)
	if !ok {
		return nil, apiFail("That restore is not known (any more).")
	}
	j = restoreCheckAlive(j)
	if !restoreActive(j) {
		return M{"restore": restoreView(j)}, nil
	}
	if j.State == "running" && restoreWorkerAlive(j) {
		syscall.Kill(-j.PID, syscall.SIGTERM)
		return M{"restore": restoreView(j), "stopping": true}, nil
	}
	nj, _ := restoreUpdate(id, func(x *restoreJob) bool {
		if x.State != "queued" {
			return false
		}
		x.State = "cancelled"
		x.Finished = now()
		x.Message = "Stopped before it started."
		return true
	})
	if nj == nil {
		nj = j
	}
	return M{"restore": restoreView(nj)}, nil
}

func apiRestoreDismiss(p M) (M, error) {
	id := pstr(p, "id", "")
	j, ok := restoreUpdate(id, func(x *restoreJob) bool {
		if restoreActive(x) || x.Dismissed {
			return false
		}
		x.Dismissed = true
		return true
	})
	if !ok {
		return nil, apiFail("That restore is not known (any more).")
	}
	return M{"restore": restoreView(j)}, nil
}

func restoreOverview(s Settings) any {
	all := restoreAll()
	active := []M{}
	var last *restoreJob
	for _, j := range all {
		if restoreActive(j) {
			active = append(active, restoreView(j))
		} else if last == nil {
			last = j
		}
	}
	out := M{"running": len(active) > 0, "active": active, "last": nil}
	if last != nil {
		out["last"] = restoreView(last)
	}
	switch {
	case len(active) > 0:
		out["count"] = len(active)
		j := active[0]
		out["status"] = M{"level": "run", "text": "Restoring from " + str(j["snapshot"]), "sub": fmt.Sprintf("%.0f%%", j["percent"])}
	case last != nil && !last.Dismissed && (last.State == "error" || last.State == "cancelled"):
		lvl := "err"
		if last.State == "cancelled" {
			lvl = "warn"
		}
		out["status"] = M{"level": lvl, "text": "Last restore " + map[bool]string{true: "failed", false: "was stopped"}[last.State == "error"], "sub": last.Message}
	case last != nil:
		out["status"] = M{"level": "ok", "text": "Last restore " + last.State, "sub": dateTime(time.Unix(last.Finished, 0))}
	default:
		out["status"] = M{"level": "idle", "text": "No restore yet", "sub": ""}
	}
	return out
}

type restoreRunner struct {
	job       *restoreJob
	log       *os.File
	mu        sync.Mutex
	cur       *exec.Cmd
	cancelled atomic.Bool
	lastSave  time.Time
}

func (r *restoreRunner) logf(format string, a ...any) {
	fmt.Fprintf(r.log, "%s %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, a...))
}

func (r *restoreRunner) save(force bool) {
	if !force && time.Since(r.lastSave) < time.Second {
		return
	}
	r.lastSave = time.Now()
	snap := *r.job
	restoreUpdate(r.job.ID, func(x *restoreJob) bool {
		dismissed := x.Dismissed
		*x = snap
		x.Dismissed = dismissed
		return true
	})
}

func restoreRsyncArgs(j *restoreJob, it restoreItem, attrs bool) []string {
	args := []string{"-aH", "--numeric-ids", "--info=progress2", "--no-inc-recursive"}
	if attrs {
		args = append(args, "-AX")
	}
	switch j.Overwrite {
	case "never":
		args = append(args, "--ignore-existing")
	case "always":
		args = append(args, "--ignore-times")
	}
	if it.Type == "dir" {
		args = append(args, "--exclude=/.zfs")
		for _, m := range restoreMounts(it.Into) {
			args = append(args, "--exclude="+rsyncEscape(m))
		}
		if j.Delete {
			args = append(args, "--delete")
		}
		return append(args, "--", it.Src+"/", it.Into+"/")
	}
	return append(args, "--", it.Src, it.Into)
}

func rsyncEscape(p string) string {
	if !strings.ContainsAny(p, "*?[") {
		return p
	}
	var b strings.Builder
	for _, c := range p {
		if strings.ContainsRune(`*?[\`, c) {
			b.WriteByte('\\')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func splitCRLF(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (r *restoreRunner) rsync(it restoreItem, base int64, attrs bool) (int, string) {
	cmd := exec.Command(rsyncBin, restoreRsyncArgs(r.job, it, attrs)...)
	cmd.Dir = "/"
	out, err := cmd.StdoutPipe()
	if err != nil {
		return -1, err.Error()
	}
	var errb bytes.Buffer
	cmd.Stderr = io.MultiWriter(&errb, r.log)
	r.mu.Lock()
	if r.cancelled.Load() {
		r.mu.Unlock()
		return 20, "cancelled"
	}
	if err := cmd.Start(); err != nil {
		r.mu.Unlock()
		return -1, err.Error()
	}
	r.cur = cmd
	r.mu.Unlock()
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	sc.Split(splitCRLF)
	for sc.Scan() {
		line := sc.Text()
		if m := reRsyncPct.FindStringSubmatch(line); m != nil {
			pct, _ := strconv.Atoi(m[2])
			r.job.BytesDone = base + it.Bytes*int64(pct)/100
			r.save(false)
			continue
		}
		if strings.TrimSpace(line) != "" {
			r.logf("  %s", strings.TrimSpace(line))
		}
	}
	err = cmd.Wait()
	r.mu.Lock()
	r.cur = nil
	r.mu.Unlock()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), errb.String()
		}
		return -1, err.Error()
	}
	return 0, errb.String()
}

func restoreItemBytes(it restoreItem, deadline time.Time) int64 {
	if it.Type != "dir" {
		if st, err := os.Lstat(it.Src); err == nil && st.Mode().IsRegular() {
			return st.Size()
		}
		return 0
	}
	var n int64
	i := 0
	filepath.WalkDir(it.Src, func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		i++
		if i%512 == 0 && time.Now().After(deadline) {
			return errPreviewStop
		}
		if de.Type().IsRegular() {
			if info, err := de.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

func restoreRun(id string) int {
	j, ok := restoreUpdate(id, func(x *restoreJob) bool {
		if x.State != "queued" {
			return false
		}
		x.State = "running"
		x.PID = os.Getpid()
		x.Started = now()
		return true
	})
	if !ok || j.State != "running" || j.PID != os.Getpid() {
		return 1
	}
	logf, err := os.OpenFile(restorePath(id, ".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		logf, _ = os.Open(os.DevNull)
	}
	defer logf.Close()
	r := &restoreRunner{job: j, log: logf}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		<-sigs
		r.cancelled.Store(true)
		r.mu.Lock()
		if r.cur != nil && r.cur.Process != nil {
			r.cur.Process.Signal(syscall.SIGTERM)
		}
		r.mu.Unlock()
	}()
	r.logf("Restore from %s@%s, %d item(s), existing files: %s%s", j.Dataset, j.Snapshot, len(j.Items), j.Overwrite, map[bool]string{true: ", deleting files not in the snapshot", false: ""}[j.Delete])
	deadline := time.Now().Add(20 * time.Second)
	j.Bytes = 0
	for i := range j.Items {
		j.Items[i].Bytes = restoreItemBytes(j.Items[i], deadline)
		j.Bytes += j.Items[i].Bytes
	}
	r.save(true)
	var base int64
	for _, it := range j.Items {
		if r.cancelled.Load() {
			break
		}
		j.Current = it.Rel
		r.save(true)
		r.logf("%s %s -> %s", it.Type, it.Rel, it.Into)
		code, msg := 0, ""
		if !mkdirLikeParent(filepath.Dir(it.Into)) {
			code, msg = -1, "cannot create "+filepath.Dir(it.Into)
			r.logf("  ERROR: %s", msg)
		} else {
			code, msg = r.rsync(it, base, true)
			if code == 23 && reRsyncNoAttr.MatchString(msg) && !r.cancelled.Load() {
				r.logf("  The destination does not support ACLs or extended attributes - copying again without them.")
				code, msg = r.rsync(it, base, false)
			}
		}
		base += it.Bytes
		j.BytesDone = base
		switch {
		case r.cancelled.Load():
			r.logf("  stopped")
		case code == 0:
			j.Done++
		default:
			j.Failed++
			r.logf("  ERROR: rsync exited with code %d", code)
		}
		r.save(true)
	}
	j.Current = ""
	j.Finished = now()
	switch {
	case r.cancelled.Load():
		j.State = "cancelled"
		j.Message = fmt.Sprintf("Stopped after %d of %d item(s). The file being copied was discarded; finished files stay.", j.Done, len(j.Items))
	case j.Failed > 0:
		j.State = "error"
		j.Message = fmt.Sprintf("%d restored, %d failed - see the log.", j.Done, j.Failed)
	default:
		j.State = "done"
		j.Message = fmt.Sprintf("%s restored.", map[bool]string{true: "1 item", false: strconv.Itoa(j.Done) + " items"}[j.Done == 1])
	}
	r.logf("Finished: %s - %s", j.State, j.Message)
	r.save(true)
	if notificationsOn() {
		switch {
		case j.State == "error":
			notify("Restore failed", j.Dataset+"@"+j.Snapshot+": "+j.Message, "warning")
		case j.State == "done" && j.Finished-j.Started > 300:
			notify("Restore finished", j.Dataset+"@"+j.Snapshot+": "+j.Message, "normal")
		}
	}
	restorePrune()
	if j.State == "done" {
		return 0
	}
	return 1
}

func init() {
	registerAction("restore_preview", apiRestorePreview)
	registerAction("restore_start", apiRestoreStart)
	registerAction("restore_status", apiRestoreStatus)
	registerAction("restore_list", apiRestoreList)
	registerAction("restore_stop", apiRestoreStop)
	registerAction("restore_dismiss", apiRestoreDismiss)
	registerOverview("browse", restoreOverview)
	register("restore-run", "run a queued restore in the background: restore-run <id>", func(args []string) int {
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "usage: restore-run <id>")
			return 2
		}
		return restoreRun(args[0])
	})
}
