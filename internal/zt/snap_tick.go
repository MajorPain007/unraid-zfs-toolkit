package zt

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type snapZFS struct{}

func (snapZFS) ready() bool { return zfsReady() }

func (snapZFS) datasets() ([]string, error) {
	r := zfs("list", "-H", "-o", "name", "-t", "filesystem,volume")
	if !r.OK() {
		return nil, errors.New(r.Message())
	}
	return r.Lines(), nil
}

func snapParseRows(lines []string) []snapRow {
	out := []snapRow{}
	for _, l := range lines {
		f := strings.Split(l, "\t")
		if len(f) < 4 || !strings.Contains(f[0], "@") {
			continue
		}
		refs := 0
		if f[3] != "-" {
			refs = toInt(f[3])
		}
		out = append(out, snapRow{Name: f[0], Creation: toInt64(f[1]), Used: toInt64(f[2]), Refs: refs})
	}
	return out
}

func (snapZFS) snapshots(pools []string) ([]snapRow, error) {
	args := append([]string{"list", "-Hp", "-t", "snapshot", "-o", "name,creation,used,userrefs", "-s", "creation", "-r"}, pools...)
	r := zfs(args...)
	if !r.OK() && strings.TrimSpace(r.Stdout) == "" {
		return nil, errors.New(r.Message())
	}
	return snapParseRows(r.Lines()), nil
}

func (snapZFS) create(name string, recursive bool) error {
	args := []string{"snapshot"}
	if recursive {
		args = append(args, "-r")
	}
	r := zfs(append(args, name)...)
	if !r.OK() {
		msg := strings.TrimSpace(r.Stderr + " " + r.Stdout)
		if msg == "" {
			msg = "no error output from zfs"
		}
		return errors.New(msg)
	}
	return nil
}

func (snapZFS) destroy(name string, recursive bool) error {
	args := []string{"destroy"}
	if recursive {
		args = append(args, "-r")
	}
	r := zfs(append(args, name)...)
	if !r.OK() {
		return errors.New(r.Message())
	}
	return nil
}

func (snapZFS) poolSpace(pool string) (int64, int64, int, bool) {
	r := zpool("list", "-Hp", "-o", "size,free,capacity", pool)
	if !r.OK() || len(r.Lines()) == 0 {
		return 0, 0, 0, false
	}
	f := strings.Split(r.Lines()[0], "\t")
	if len(f) < 3 || !isDigits(f[1]) {
		return 0, 0, 0, false
	}
	return toInt64(f[0]), toInt64(f[1]), phpInt(strings.TrimSuffix(f[2], "%")), true
}

func (snapZFS) sync(pool string) {
	zpool("sync", pool)
}

func snapLogFile() string     { return TmpDir + "/snapshots.log" }
func snapStatusFile() string  { return TmpDir + "/snapshot_status.json" }
func snapLastFile() string    { return TmpDir + "/snapshot_last.txt" }
func snapRunningFile() string { return TmpDir + "/snapshot_running.json" }
func snapNotifyFile() string  { return TmpDir + "/snapshot_notify.json" }

type snapStatus struct {
	LastRun   string   `json:"last_run"`
	Duration  int64    `json:"duration"`
	Datasets  int      `json:"datasets"`
	Created   int      `json:"created"`
	Pruned    int      `json:"pruned"`
	Freed     int64    `json:"freed"`
	Errors    int      `json:"errors"`
	LastError string   `json:"last_error"`
	Forced    int      `json:"forced"`
	DryRun    int      `json:"dry_run"`
	Ended     int64    `json:"ended"`
	Warnings  []string `json:"warnings"`
}

func snapNotifyLimited(key, subject, text, level string, every time.Duration) {
	l := lockFile(TmpDir+"/snapshot_notify.lock", true)
	defer l.Unlock()
	seen, _ := jsonRead(snapNotifyFile())
	if seen == nil {
		seen = M{}
	}
	if last := toInt64(seen[key]); last > 0 && time.Since(time.Unix(last, 0)) < every {
		return
	}
	if notify(subject, text, level) {
		seen[key] = now()
		jsonWrite(snapNotifyFile(), seen, 0o644)
	}
}

func snapTryLock(name string) (*fileLock, bool) {
	path := TmpDir + "/" + name + ".lock"
	os.MkdirAll(TmpDir, 0o755)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, false
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return nil, true
	}
	return &fileLock{f: f}, false
}

func snapNewEngine(s Settings, dry bool) *snapEngine {
	cfg, has, err := snapLoadDatasets()
	e := &snapEngine{s: s, be: snapZFS{}, now: time.Now(), dry: dry, config: cfg, hasConfig: has, notify: snapNotifyLimited}
	if err != nil {
		e.hasConfig = true
		e.config = nil
		e.configErr = err.Error()
	}
	return e
}

func snapTick(args []string) int {
	dry, forceNow, forceAll, background := false, false, false, false
	for _, a := range args {
		switch a {
		case "--now":
			forceNow = true
		case "--all":
			forceNow, forceAll = true, true
		case "--dry-run":
			dry = true
		case "--background":
			background = true
		default:
			fmt.Fprintf(os.Stderr, "unknown option %s\n", a)
			return 2
		}
	}
	os.MkdirAll(TmpDir, 0o755)
	if background {
		rest := []string{}
		for _, a := range args {
			if a != "--background" {
				rest = append(rest, a)
			}
		}
		if err := snapSpawn(rest...); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	lockName := "snapshot_manager"
	if dry {
		lockName = "snapshot_manager_dryrun"
	}
	start := time.Now()
	lock, busy := snapTryLock(lockName)
	if busy {
		tag := ""
		if dry {
			tag = "[dry-run] "
		}
		msg := "Another snapshot run is still active - skipping this cycle."
		if dry {
			msg = "Another dry run is active."
		}
		fmt.Printf("[%s] %sWARNING: %s\n", start.Format("15:04:05"), tag, msg)
		return 0
	}
	defer lock.Unlock()
	if !dry {
		jsonWrite(snapRunningFile(), M{"pid": os.Getpid(), "started": start.Unix(), "forced": forceNow}, 0o644)
		defer os.Remove(snapRunningFile())
	}
	s := loadSettings()
	e := snapNewEngine(s, dry)
	e.forceNow, e.forceAll = forceNow, forceAll
	e.out = func(line string) { fmt.Println(line) }
	manual := ""
	if forceNow {
		manual = " (manual run)"
	}
	e.logf("ZFS Snapshot Manager starting%s...", manual)
	if dry {
		e.warnf("DRY RUN - no snapshots will be created or destroyed.")
	}
	e.run()
	if !dry {
		st := snapStatus{
			LastRun:   dateTime(time.Now()),
			Duration:  int64(time.Since(start).Seconds()),
			Datasets:  e.DS,
			Created:   e.Created,
			Pruned:    e.Pruned,
			Freed:     e.Freed,
			Errors:    e.Errors,
			LastError: e.LastError,
			Forced:    map[bool]int{true: 1, false: 0}[forceNow],
			Ended:     time.Now().Unix(),
			Warnings:  snapLastN(e.Warnings, 20),
		}
		jsonWrite(snapStatusFile(), st, 0o644)
		fileWrite(snapLastFile(), []byte(st.LastRun+"\n"), 0o644)
		snapInvalidate()
		if e.Errors > 0 {
			e.sendNotify("errors:"+e.LastError, fmt.Sprintf("ZFS Snapshots - %d error(s)", e.Errors),
				fmt.Sprintf("Snapshot run finished with %d error(s). Last: %s", e.Errors, e.LastError), "warning", 6*time.Hour)
		}
	}
	e.now = time.Now()
	e.logf("Snapshot Manager finished (created=%d, pruned=%d, freed=%s, errors=%d).", e.Created, e.Pruned, humanBytes(float64(e.Freed)), e.Errors)
	if st, err := os.Stat(snapLogFile()); err == nil && st.Size() > 1048576 {
		logTrim(snapLogFile(), 524288)
	}
	return 0
}

func snapLastN(l []string, n int) []string {
	if len(l) > n {
		return l[len(l)-n:]
	}
	if l == nil {
		return []string{}
	}
	return l
}

func snapSpawn(args ...string) error {
	os.MkdirAll(TmpDir, 0o755)
	logf, err := os.OpenFile(snapLogFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer logf.Close()
	cmd := exec.Command(SelfBin, append([]string{"snapshot-tick"}, args...)...)
	cmd.Stdin = nil
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.Dir = "/"
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	cmd.Process.Release()
	return nil
}

func init() {
	register("snapshot-tick", "create due snapshots and prune by retention (cron); --now ignores the daily hour, --all also takes a frequent snapshot, --dry-run changes nothing, --background detaches", snapTick)
}
