package zt

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	reDiagHost     = regexp.MustCompile(`"ssh_host"\s*:\s*"([^"]+)"`)
	reDiagJSONPort = regexp.MustCompile(`("ssh_port"\s*:\s*)"?\d+"?`)
	reDiagPort     = regexp.MustCompile(`(\bport| -p) \d+`)
)

func diagRedact(report, jobs string) string {
	for _, m := range reDiagHost.FindAllStringSubmatch(jobs, -1) {
		report = sendHideHost(report, m[1])
	}
	report = reDiagJSONPort.ReplaceAllString(report, `${1}"…"`)
	return reDiagPort.ReplaceAllString(report, "${1} …")
}

func diagRun(name string, args ...string) string {
	r := runCmd(60*time.Second, name, args...)
	out := strings.TrimRight(r.Stdout, "\n")
	if e := strings.TrimSpace(r.Stderr); e != "" {
		if out != "" {
			out += "\n"
		}
		out += e
	}
	return out
}

func diagSection(b *strings.Builder, title, body string) {
	line := strings.Repeat("=", 72)
	b.WriteString(line + "\n== " + title + "\n" + line + "\n" + strings.TrimRight(body, "\n") + "\n\n")
}

func diagFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "(not present)"
	}
	return string(b)
}

func diagTail(path string, n int) string {
	if !exists(path) {
		return "(not present)"
	}
	return tail(path, n, 4<<20)
}

func diagnosticsReport() string {
	var b strings.Builder
	b.WriteString("ZFS Toolkit - diagnostics\n")
	b.WriteString("Generated: " + time.Now().Format("2006-01-02 15:04:05 MST") + "\n")
	v := pluginVersion()
	if v == "" {
		v = "(unknown)"
	}
	b.WriteString("Plugin version: " + v + "\n\n")
	tool := func(n string) string {
		r := runCmd(5*time.Second, "sh", "-c", "command -v "+n)
		if s := strings.TrimSpace(r.Stdout); s != "" {
			return s
		}
		return "no"
	}
	diagSection(&b, "System", strings.Join([]string{
		"Unraid: " + strings.TrimSpace(diagFile("/etc/unraid-version")),
		"Kernel: " + diagRun("uname", "-r"),
		"zfs:    " + strings.ReplaceAll(diagRun(ZfsBin, "version"), "\n", " / "),
		"tools:  pv=" + tool("pv") + " mbuffer=" + tool("mbuffer") + " rsync=" + tool("rsync"),
		"binary: " + SelfBin,
	}, "\n"))
	diagSection(&b, "zpool list", diagRun(ZpoolBin, "list"))
	diagSection(&b, "zpool status", diagRun(ZpoolBin, "status"))
	diagSection(&b, "zfs list", diagRun(ZfsBin, "list", "-o", "name,used,avail,refer,usedbysnapshots,mountpoint"))
	counts := map[string]int{}
	auto := []string{}
	r := zfs("list", "-H", "-t", "snapshot", "-o", "name,used,creation", "-s", "creation")
	for _, l := range r.Lines() {
		name := strings.SplitN(l, "\t", 2)[0]
		if i := strings.Index(name, "@"); i > 0 {
			counts[name[:i]]++
			if strings.HasPrefix(name[i+1:], "auto-") {
				auto = append(auto, l)
			}
		}
	}
	type kv struct {
		k string
		n int
	}
	list := []kv{}
	for k, n := range counts {
		list = append(list, kv{k, n})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n || (list[i].n == list[j].n && list[i].k < list[j].k) })
	lines := []string{}
	for i, e := range list {
		if i >= 50 {
			break
		}
		lines = append(lines, strings.Repeat(" ", max(0, 6-len(itoa(e.n))))+itoa(e.n)+" "+e.k)
	}
	diagSection(&b, "Snapshot count per dataset", strings.Join(lines, "\n"))
	if len(auto) > 40 {
		auto = auto[len(auto)-40:]
	}
	diagSection(&b, "Auto snapshots (newest 40)", strings.Join(auto, "\n"))
	cron := []string{}
	for _, c := range cronJobs {
		st := c.state()
		live := "not live"
		if st.Live {
			live = "live via " + st.Source
		}
		entry := st.Entry
		if entry == "" {
			entry = "(none)"
		}
		cron = append(cron, c.Label+": "+live+"\n  "+entry)
	}
	diagSection(&b, "Cron", strings.Join(cron, "\n"))
	files, _ := filepath.Glob(CronDir + "/*.cron")
	cf := []string{}
	for _, f := range files {
		cf = append(cf, "--- "+f+"\n"+strings.TrimRight(diagFile(f), "\n"))
	}
	diagSection(&b, "Plugin cron files (read by update_cron)", strings.Join(cf, "\n"))
	for _, f := range []string{"settings.cfg", "snap_datasets.json", "send_jobs.json"} {
		diagSection(&b, "config/"+f, diagFile(ConfigDir+"/"+f))
	}
	diagSection(&b, "snapshots.log (last 200)", diagTail(TmpDir+"/snapshots.log", 200))
	diagSection(&b, "send.log (last 200)", diagTail(TmpDir+"/send.log", 200))
	diagSection(&b, "snapshot_status.json", diagTail(TmpDir+"/snapshot_status.json", 5))
	diagSection(&b, "send_status.json", diagTail(TmpDir+"/send_status.json", 5))
	diagSection(&b, "cron_setup.log", diagTail(TmpDir+"/cron_setup.log", 40))
	conv, _ := filepath.Glob(TmpDir + "/conversion_*.log")
	sort.Slice(conv, func(i, j int) bool { return fileMtime(conv[i]) > fileMtime(conv[j]) })
	for i, c := range conv {
		if i >= 2 {
			break
		}
		diagSection(&b, "conversion: "+filepath.Base(c), diagTail(c, 300))
	}
	return diagRedact(b.String(), diagFile(sendJobsFile()))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	d := []byte{}
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	if neg {
		d = append([]byte{'-'}, d...)
	}
	return string(d)
}

func apiDiagnostics(p M) (M, error) {
	return M{
		"name":   "zfs-toolkit-diagnostics-" + time.Now().Format("20060102-150405") + ".txt",
		"report": diagnosticsReport(),
	}, nil
}

func apiSystem(p M) (M, error) {
	tools := M{}
	for _, t := range []string{"rsync", "pv", "mbuffer", "docker", "virsh"} {
		r := runCmd(5*time.Second, "sh", "-c", "command -v "+t)
		tools[t] = strings.TrimSpace(r.Stdout) != ""
	}
	zv := strings.SplitN(diagRun(ZfsBin, "version"), "\n", 2)[0]
	return M{
		"zfs_version": zv,
		"unraid":      strings.Trim(strings.TrimPrefix(strings.TrimSpace(diagFile("/etc/unraid-version")), "version="), "\""),
		"kernel":      diagRun("uname", "-r"),
		"timezone":    time.Local.String(),
		"tools":       tools,
		"cron_log":    diagTail(TmpDir+"/cron_setup.log", 20),
	}, nil
}

func init() {
	registerAction("diagnostics", apiDiagnostics)
	registerAction("system", apiSystem)
	register("diagnostics", "print the diagnostics report", func(args []string) int {
		os.Stdout.WriteString(diagnosticsReport())
		return 0
	})
}
