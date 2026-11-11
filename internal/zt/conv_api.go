package zt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type convOptions struct {
	Source  string
	DryRun  bool
	Only    []string
	LogFile string
}

func convNewLogName(source string) string {
	prefix := "conversion_"
	if source == "auto" {
		prefix = "auto_"
	}
	base := TmpDir + "/" + prefix + time.Now().Format("20060102_150405")
	p := base + ".log"
	for i := 2; exists(p); i++ {
		p = base + "_" + strconv.Itoa(i) + ".log"
	}
	return p
}

func convExecute(opts convOptions, tee io.Writer) int {
	os.MkdirAll(TmpDir, 0o755)
	lock := lockFile(convLockPath, false)
	if lock == nil {
		if tee != nil {
			fmt.Fprintln(tee, "A conversion is already running.")
		}
		return 3
	}
	defer lock.Unlock()
	if st, running := convRunning(); running && st.PID != os.Getpid() {
		if tee != nil {
			fmt.Fprintf(tee, "A conversion is already running (PID %d).\n", st.PID)
		}
		return 3
	}
	if opts.LogFile == "" || !convLogAllowed(opts.LogFile) {
		opts.LogFile = convNewLogName(opts.Source)
	}
	lg, err := convOpenLog(opts.LogFile, tee)
	if err != nil {
		if tee != nil {
			fmt.Fprintln(tee, "Cannot open the log:", err)
		}
		return 1
	}
	defer lg.close()
	convPruneLogs(10)

	cfg := convConfigFrom(loadSettings())
	if opts.DryRun {
		cfg.DryRun = true
	}
	started := time.Now()
	sw := &convStatusWriter{st: convStatusFile{
		PID: os.Getpid(), LogFile: opts.LogFile, Started: dateTime(started), StartedTS: started.Unix(),
		Status: "running", Source: opts.Source, Engine: "go", DryRun: cfg.DryRun, Phase: "starting",
		Items: []convItem{}, Stopped: []convUser{},
	}}
	sw.update(true, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	go func() {
		first := true
		for range sigs {
			if first {
				lg.raw("")
				lg.warn("Stop requested - finishing the current step safely.")
				first = false
			}
			cancel()
		}
	}()

	r := &convRunner{cfg: cfg, ctx: ctx, log: lg, status: sw, unsafe: map[string]bool{}}
	if len(opts.Only) > 0 {
		r.only = map[string]bool{}
		for _, p := range opts.Only {
			r.only[p] = true
		}
	}
	code := r.run()

	state := "completed"
	switch {
	case r.interrupted || code == 130:
		state = "stopped"
	case code != 0 || len(r.failed) > 0 || lg.errors > 0:
		state = "error"
	}
	res := &convResult{
		Converted: convNonNil(r.converted), Failed: convNonNil(r.failed), Skipped: convNonNil(r.skipped),
		Interrupted: state == "stopped", Errors: lg.errors, Warnings: lg.warnings,
	}
	res.Message = convResultMessage(res, cfg.DryRun, len(r.plan))
	fin := time.Now().Unix()
	sw.update(true, func(st *convStatusFile) {
		st.Status = state
		st.Phase = ""
		st.CurrentFolder = ""
		st.Finished = fin
		st.Result = res
	})
	if !cfg.DryRun && (len(r.converted) > 0 || len(r.failed) > 0 || r.interrupted) {
		convAppendHistory(convHistoryEntry{
			Started: started.Unix(), Finished: fin, Source: opts.Source, Status: state, DryRun: cfg.DryRun,
			Converted: res.Converted, Failed: res.Failed, Interrupted: res.Interrupted, LogFile: opts.LogFile,
		})
	}
	if opts.Source == "auto" {
		logAppendMax(convCronLog, "Auto-conversion finished: "+res.Message, 256*1024)
	}
	return code
}

func convNonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func convResultMessage(r *convResult, dry bool, planned int) string {
	if r.Interrupted {
		return fmt.Sprintf("Stopped. %d converted, %d failed.", len(r.Converted), len(r.Failed))
	}
	if dry {
		if planned == 0 {
			return "Dry run: nothing to convert."
		}
		return fmt.Sprintf("Dry run: %d folder(s) would be converted.", planned)
	}
	if len(r.Converted) == 0 && len(r.Failed) == 0 {
		if r.Errors > 0 {
			return "The run could not start, see the log."
		}
		return "Nothing to convert."
	}
	msg := fmt.Sprintf("%d converted", len(r.Converted))
	if len(r.Failed) > 0 {
		msg += fmt.Sprintf(", %d failed", len(r.Failed))
	}
	return msg + "."
}

func convSpawn(args ...string) (int, error) {
	cmd := exec.Command(SelfBin, args...)
	cmd.Dir = "/"
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st, ok := convReadStatus(); ok && st.PID == pid {
			return pid, nil
		}
		select {
		case <-exited:
			if st, ok := convReadStatus(); ok && st.PID == pid {
				return pid, nil
			}
			return 0, fmt.Errorf("the conversion ended right away - another one is probably running")
		case <-time.After(100 * time.Millisecond):
		}
	}
	return pid, nil
}

func convStartDetached(source string, dry bool, only []string) (int, string, error) {
	logFile := convNewLogName(source)
	args := []string{"convert", "--child", "--source", source, "--log", logFile}
	if dry {
		args = append(args, "--dry-run")
	}
	if len(only) > 0 {
		b, _ := json.Marshal(only)
		args = append(args, "--only", string(b))
	}
	pid, err := convSpawn(args...)
	return pid, logFile, err
}

func convStop() (bool, string) {
	st, running := convRunning()
	if !running {
		return false, "No conversion is running."
	}
	syscall.Kill(st.PID, syscall.SIGTERM)
	if st.Engine != "go" {
		runCmd(10*time.Second, "pkill", "-TERM", "-P", strconv.Itoa(st.PID))
		if m, ok := jsonRead(convStatusPath); ok {
			m["status"] = "stopped"
			jsonWrite(convStatusPath, m, 0o644)
		}
	}
	return true, fmt.Sprintf("Stop sent to PID %d.", st.PID)
}

func convCLI(args []string) int {
	opts := convOptions{Source: "cli"}
	auto, child := false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--auto":
			auto = true
		case "--child":
			child = true
		case "--dry-run":
			opts.DryRun = true
		case "--source", "--log", "--only":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, args[i]+" needs a value")
				return 2
			}
			v := args[i+1]
			i++
			switch args[i-1] {
			case "--source":
				if inList(v, "manual", "auto", "cli") {
					opts.Source = v
				}
			case "--log":
				opts.LogFile = v
			case "--only":
				if json.Unmarshal([]byte(v), &opts.Only) != nil {
					fmt.Fprintln(os.Stderr, "--only takes a JSON list of folder paths")
					return 2
				}
			}
		default:
			fmt.Fprintln(os.Stderr, "unknown option "+args[i])
			return 2
		}
	}
	if child {
		return convExecute(opts, nil)
	}
	if auto {
		return convAuto()
	}
	return convExecute(opts, os.Stdout)
}

func convAuto() int {
	os.MkdirAll(TmpDir, 0o755)
	logTrim(convCronLog, 256*1024)
	say := func(s string) { fmt.Println("[" + dateTime(time.Now()) + "] " + s) }
	if !zfsReady() {
		say("Skipped: ZFS not available or no pool imported (array not started?).")
		return 0
	}
	if st, running := convRunning(); running {
		say(fmt.Sprintf("Auto-conversion skipped: already running (PID %d)", st.PID))
		return 0
	}
	if !isFile(SettingsCfg) {
		say("Config not found: " + SettingsCfg)
		return 1
	}
	pid, logFile, err := convStartDetached("auto", false, nil)
	if err != nil {
		say("Auto-conversion not started: " + err.Error())
		return 1
	}
	say(fmt.Sprintf("Auto-conversion started (PID %d, log: %s)", pid, logFile))
	return 0
}

const convScanBudget = 40 * time.Second

func convScan(withDetails bool) M {
	cfg := convConfigFrom(loadSettings())
	w := convLoadWorld()
	in := convPlanInput{World: w, Sources: cfg.Sources, ReplaceSpaces: cfg.ReplaceSpaces, Buffer: cfg.Buffer, ReadDir: convReadDir}
	out := M{"problems": convNonNil(cfg.Problems), "dry_run": cfg.DryRun}
	if withDetails {
		ctx, cancel := context.WithTimeout(context.Background(), convScanBudget)
		defer cancel()
		sizes := convMeasureAll(ctx, w, cfg.Sources)
		in.Size = func(p string) (int64, int64, bool) {
			u, ok := sizes[p]
			if !ok || !u.Done {
				return -1, -1, false
			}
			return u.Allocated, u.Apparent, true
		}
	}
	scans := []convSourceScan{}
	if w.ok {
		scans = convClassify(in)
	} else {
		out["error"] = "Cannot list ZFS datasets: " + w.err
	}
	if withDetails {
		services := []convService{}
		notes := []string{}
		if cs, warn := convContainers(true); warn == "" {
			services = append(services, cs...)
		} else {
			notes = append(notes, warn)
		}
		if vs, warn := convVMs(true); warn == "" {
			services = append(services, vs...)
		} else {
			notes = append(notes, warn)
		}
		convAttachUsers(scans, services)
		out["service_notes"] = notes
	}
	out["sources"] = scans
	out["counts"] = M{
		"convert": convCount(scans, "convert"), "dataset": convCount(scans, "dataset"),
		"kept": convCount(scans, "kept"), "blocked": convCount(scans, "blocked"),
	}
	return out
}

func convMeasureAll(ctx context.Context, w *convWorld, sources []convSource) map[string]convUsage {
	paths := []string{}
	for _, s := range sources {
		mp, prob := convSourceProblem(w, s.Dataset)
		if prob != "" {
			continue
		}
		entries, err := convReadDir(mp)
		if err != nil {
			continue
		}
		for _, e := range entries {
			p := strings.TrimRight(mp, "/") + "/" + e.Name
			if e.IsDir && !e.Link && w.datasetAt(p) == "" {
				paths = append(paths, p)
			}
		}
	}
	out := map[string]convUsage{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	jobs := make(chan string)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				u, _ := convDirUsage(ctx, p)
				mu.Lock()
				out[p] = u
				mu.Unlock()
			}
		}()
	}
	for _, p := range paths {
		jobs <- p
	}
	close(jobs)
	wg.Wait()
	return out
}

func convStatusView() M {
	st, ok := convCurrentStatus()
	out := M{"status": "idle", "running": false}
	if ok {
		running := st.Status == "running"
		out = M{
			"status": st.Status, "running": running, "pid": st.PID, "log_file": st.LogFile,
			"started": st.StartedTS, "source": st.Source, "engine": st.Engine, "dry_run": st.DryRun,
			"phase": st.Phase, "current_folder": st.CurrentFolder, "items": st.Items, "stopped": st.Stopped,
			"finished": st.Finished, "result": st.Result,
		}
		if st.Items == nil {
			out["items"] = []convItem{}
		}
		if st.Stopped == nil {
			out["stopped"] = []convUser{}
		}
		if st.StartedTS == 0 && st.Started != "" {
			if t, err := time.ParseInLocation("2006-01-02 15:04:05", st.Started, time.Local); err == nil {
				out["started"] = t.Unix()
			}
		}
		if st.Engine != "go" && st.LogFile != "" && running {
			out["current_folder"] = convLegacyCurrent(st.LogFile)
		}
	}
	if j, pending := convJournalPending(); pending {
		out["journal"] = M{"entry": j.Entry, "source": j.Source, "dataset": j.dataset(), "temp": j.Temp, "path": j.path(), "committed": j.Committed}
	} else {
		out["journal"] = nil
	}
	return out
}

func convLegacyCurrent(logFile string) string {
	lines := strings.Split(tail(logFile, 20, 16*1024), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if a := strings.Index(l, "Processing '"); a >= 0 {
			rest := l[a+12:]
			if b := strings.LastIndex(rest, "' ("); b >= 0 {
				return rest[:b]
			}
		}
	}
	return ""
}

func apiConvertStatus(p M) (M, error) {
	return convStatusView(), nil
}

func apiConvertStart(p M) (M, error) {
	if st, running := convRunning(); running {
		return nil, apiFailf("A conversion is already running (PID %d).", st.PID)
	}
	if !zfsReady() {
		return nil, apiFail("ZFS is not available - no pool is imported.")
	}
	dry := toBool(pstr(p, "dry_run", ""))
	only := []string{}
	if v, ok := p["only"]; ok && v != nil && str(v) != "" {
		l, err := pjsonList(p, "only")
		if err != nil {
			return nil, err
		}
		for _, x := range l {
			if s := absPath(x); s != "" {
				only = append(only, s)
			}
		}
		if len(only) == 0 {
			return nil, apiFail("No folders selected.")
		}
	}
	cfg := convConfigFrom(loadSettings())
	if len(cfg.Sources) == 0 {
		return nil, apiFail("No source datasets are set up. Turn on appdata or VM domains, or add an extra dataset.")
	}
	pid, logFile, err := convStartDetached("manual", dry, only)
	if err != nil {
		return nil, apiFail("The conversion did not start: " + err.Error())
	}
	return M{"pid": pid, "log_file": logFile, "dry_run": dry || cfg.DryRun}, nil
}

func apiConvertStop(p M) (M, error) {
	ok, msg := convStop()
	if !ok {
		return nil, apiFail(msg)
	}
	return M{"message": msg}, nil
}

func apiConvertLog(p M) (M, error) {
	file := pstr(p, "file", "")
	st, _ := convCurrentStatus()
	if file == "" {
		file = st.LogFile
		if file == "" {
			if l := convLogFiles(); len(l) > 0 {
				file = l[0].File
			}
		}
	}
	if file == "" {
		return M{"file": "", "text": "", "offset": 0, "size": 0, "status": "idle"}, nil
	}
	if !convLogAllowed(file) {
		return nil, apiFail("That is not a conversion log.")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, apiFail("The log " + convBase(file) + " is gone - logs live in RAM and are cleared at reboot.")
	}
	defer f.Close()
	info, _ := f.Stat()
	size := info.Size()
	offset := toInt64(pstr(p, "offset", "-1"))
	const chunk = 256 * 1024
	if offset < 0 {
		offset = size - chunk
		if offset < 0 {
			offset = 0
		}
	}
	if offset > size {
		offset = 0
	}
	n := size - offset
	if n > chunk {
		n = chunk
	}
	buf := make([]byte, n)
	got, _ := f.ReadAt(buf, offset)
	status := convClassifyLog(file)
	if file == st.LogFile {
		status = st.Status
	}
	return M{"file": file, "name": convBase(file), "text": string(buf[:got]), "offset": offset + int64(got), "start": offset, "size": size, "status": status}, nil
}

func apiConvertHistory(p M) (M, error) {
	logs := convLogFiles()
	st, _ := convCurrentStatus()
	type logView struct {
		convLogFile
		Status string `json:"status"`
	}
	lv := []logView{}
	for _, l := range logs {
		s := convClassifyLog(l.File)
		if l.File == st.LogFile {
			s = st.Status
		}
		lv = append(lv, logView{l, s})
	}
	return M{"runs": convReadHistory(), "logs": lv}, nil
}

func apiConvertDatasets(p M) (M, error) {
	r := zfs("list", "-H", "-t", "filesystem", "-o", "name,mountpoint")
	out := []M{}
	for _, l := range r.Lines() {
		f := strings.SplitN(l, "\t", 2)
		if len(f) == 2 {
			out = append(out, M{"name": f[0], "mountpoint": f[1]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return natLess(str(out[i]["name"]), str(out[j]["name"])) })
	return M{"datasets": out}, nil
}

func apiScanFolders(p M) (M, error) {
	out := convScan(true)
	out["status"] = convStatusView()
	return out, nil
}

func convOverview(s Settings) any {
	cfg := convConfigFrom(s)
	sv := convStatusView()
	running := sv["running"] == true
	waiting := 0
	if len(cfg.Sources) > 0 && !running {
		w := convLoadWorld()
		if w.ok {
			waiting = convCount(convClassify(convPlanInput{World: w, Sources: cfg.Sources, ReplaceSpaces: cfg.ReplaceSpaces, Buffer: cfg.Buffer, ReadDir: convReadDir}), "convert")
		}
	}
	cronOn := s.Yes("cron_enabled")
	var status M
	switch {
	case running:
		text := "Converting folders to datasets"
		if cfg.DryRun || sv["dry_run"] == true {
			text = "Conversion dry run in progress"
		}
		sub := ""
		if f := str(sv["current_folder"]); f != "" {
			sub = "Working on " + f
		}
		status = M{"level": "run", "text": text, "sub": sub}
	case sv["journal"] != nil:
		status = M{"level": "err", "text": "An interrupted conversion needs attention", "sub": "The next real run tries to settle it - see the Conversion tab."}
	case sv["status"] == "error" && sv["dry_run"] != true:
		status = M{"level": "err", "text": "The last conversion had problems", "sub": "See its log on the Conversion tab."}
	case waiting > 0 && cfg.DryRun:
		status = M{"level": "warn", "text": fmt.Sprintf("%d folder(s) waiting - dry run is on", waiting), "sub": "Turn off dry run to convert them."}
	case cronOn && cfg.DryRun:
		status = M{"level": "warn", "text": "Scheduled conversion runs as a dry run", "sub": "It changes nothing while dry run is on."}
	case waiting > 0:
		sub := "Start a conversion on the Conversion tab."
		if cronOn {
			sub = "The next scheduled run converts them."
		}
		status = M{"level": "warn", "text": fmt.Sprintf("%d folder(s) not yet a dataset", waiting), "sub": sub}
	case len(cfg.Sources) == 0:
		status = M{"level": "idle", "text": "Conversion is not set up", "sub": ""}
	default:
		status = M{"level": "idle", "text": "All folders are datasets", "sub": ""}
	}
	return M{"status": status, "count": waiting, "running": running}
}

func init() {
	register("convert", "convert folders to datasets: convert [--dry-run] [--only JSON], --auto for the scheduled run", convCLI)
	register("convert-stop", "stop a running conversion; it puts the current folder back and restarts what it stopped", func(args []string) int {
		ok, msg := convStop()
		fmt.Println(msg)
		if !ok {
			return 1
		}
		for i := 0; i < 600; i++ {
			if _, running := convRunning(); !running {
				return 0
			}
			time.Sleep(100 * time.Millisecond)
		}
		return 1
	})
	register("convert-scan", "print the scan of the conversion sources as JSON", func(args []string) int {
		os.Stdout.Write(jsonEncode(convScan(true), true))
		return 0
	})
	registerAction("scan_folders", apiScanFolders)
	registerAction("convert_start", apiConvertStart)
	registerAction("convert_stop", apiConvertStop)
	registerAction("convert_status", apiConvertStatus)
	registerAction("convert_log", apiConvertLog)
	registerAction("convert_history", apiConvertHistory)
	registerAction("convert_datasets", apiConvertDatasets)
	registerOverview("conversion", convOverview)
}
