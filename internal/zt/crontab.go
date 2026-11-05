package zt

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var cronFieldNames = []string{"Minute", "Hour", "Day of month", "Month", "Weekday"}
var cronLo = []int{0, 0, 1, 1, 0}
var cronHi = []int{59, 23, 31, 12, 7}

var reCronChars = regexp.MustCompile(`^[0-9*/,\s-]+$`)
var reCronDanger = regexp.MustCompile("[;&|$`()<>\r\n]")

func validCronProblem(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "No cron expression entered."
	}
	if reCronDanger.MatchString(expr) {
		return "Contains characters that could be read as a shell command."
	}
	if !reCronChars.MatchString(expr) {
		return "Only digits and * / , - are allowed. Use numbers for weekdays and months."
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Sprintf("A cron expression has 5 fields (minute hour day month weekday), this one has %d.", len(fields))
	}
	for i, field := range fields {
		for _, part := range strings.Split(field, ",") {
			if part == "" {
				return cronFieldNames[i] + ": empty value in the list."
			}
			if j := strings.Index(part, "/"); j >= 0 {
				step := part[j+1:]
				part = part[:j]
				if !isDigits(step) || digitsInt(step) < 1 || digitsInt(step) > cronHi[i] {
					return fmt.Sprintf("%s: step must be 1-%d.", cronFieldNames[i], cronHi[i])
				}
			}
			if part == "*" {
				continue
			}
			if j := strings.Index(part, "-"); j >= 0 {
				a, b := part[:j], part[j+1:]
				if !isDigits(a) || !isDigits(b) {
					return cronFieldNames[i] + ": range must be two numbers."
				}
				if digitsInt(a) < cronLo[i] || digitsInt(b) > cronHi[i] || digitsInt(a) > digitsInt(b) {
					return fmt.Sprintf("%s: range must run upwards inside %d-%d.", cronFieldNames[i], cronLo[i], cronHi[i])
				}
				continue
			}
			if !isDigits(part) || digitsInt(part) < cronLo[i] || digitsInt(part) > cronHi[i] {
				return fmt.Sprintf("%s must be %d-%d, got \"%s\".", cronFieldNames[i], cronLo[i], cronHi[i], part)
			}
		}
	}
	return ""
}

func validCron(expr string) bool {
	return validCronProblem(expr) == ""
}

type cronJob struct {
	Base    string
	Label   string
	Command string
	Log     string
	Enabled func(s Settings) bool
	Expr    func(s Settings) string
}

func clampInt(s Settings, key string, lo, hi, def int) int {
	v := strings.TrimSpace(s.Get(key, strconv.Itoa(def)))
	if !isDigits(v) {
		return def
	}
	n := digitsInt(v)
	if n < lo || n > hi {
		return def
	}
	return n
}

var cronJobs = []cronJob{
	{
		Base:    Name,
		Label:   "Conversion",
		Command: "convert --auto",
		Log:     "cron_auto.log",
		Enabled: func(s Settings) bool { return s.Yes("cron_enabled") },
		Expr: func(s Settings) string {
			h := clampInt(s, "cron_hour", 0, 23, 2)
			m := clampInt(s, "cron_minute", 0, 59, 0)
			w := clampInt(s, "cron_weekday", 0, 7, 0)
			switch s.Get("cron_preset", "daily") {
			case "hourly":
				return "0 * * * *"
			case "6hourly":
				return fmt.Sprintf("%d */6 * * *", m)
			case "daily":
				return fmt.Sprintf("%d %d * * *", m, h)
			case "weekly":
				return fmt.Sprintf("%d %d * * %d", m, h, w)
			case "custom":
				return s.Get("cron_custom", "0 2 * * *")
			}
			return "0 2 * * *"
		},
	},
	{
		Base:    Name + "-snapshots",
		Label:   "Snapshot",
		Command: "snapshot-tick",
		Log:     "snapshots.log",
		Enabled: func(s Settings) bool { return s.Yes("snapshots_enabled") },
		Expr: func(s Settings) string {
			switch s.Get("snap_schedule_preset", "15min") {
			case "5min":
				return "*/5 * * * *"
			case "15min":
				return "*/15 * * * *"
			case "30min":
				return "*/30 * * * *"
			case "hourly":
				return "0 * * * *"
			case "custom":
				return s.Get("snap_schedule_custom", "*/15 * * * *")
			}
			return "*/15 * * * *"
		},
	},
	{
		Base:    Name + "-send",
		Label:   "Replication",
		Command: "send-run",
		Log:     "send.log",
		Enabled: func(s Settings) bool { return s.Yes("send_enabled") },
		Expr: func(s Settings) string {
			h := clampInt(s, "send_schedule_hour", 0, 23, 4)
			switch s.Get("send_schedule_preset", "daily") {
			case "hourly":
				return "15 * * * *"
			case "6hourly":
				return "15 */6 * * *"
			case "daily":
				return fmt.Sprintf("0 %d * * *", h)
			case "weekly":
				return fmt.Sprintf("0 %d * * 0", h)
			case "custom":
				return s.Get("send_schedule_custom", "0 4 * * *")
			}
			return "0 4 * * *"
		},
	},
}

var legacyCronPatterns = []string{"run_auto.sh", "snapshot_manager.sh", "zfs_send.sh"}

func cronJobByBase(base string) *cronJob {
	for i := range cronJobs {
		if cronJobs[i].Base == base {
			return &cronJobs[i]
		}
	}
	return nil
}

func (c cronJob) line(expr string) string {
	return expr + " " + SelfBin + " " + c.Command + " >> " + TmpDir + "/" + c.Log + " 2>&1"
}

func (c cronJob) pattern() string {
	return SelfBin + " " + c.Command
}

func (c cronJob) file() string {
	return CronDir + "/" + c.Base + ".cron"
}

func crontabLines(spool bool) []string {
	args := []string{"-l"}
	if spool {
		args = []string{"-c", CronSpool, "-l"}
	}
	r := runCmd(10*time.Second, "crontab", args...)
	if !r.OK() {
		return nil
	}
	return r.Lines()
}

func crontabWrite(lines []string) {
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	cmd.Run()
}

func crontabDrop(patterns ...string) {
	lines := crontabLines(false)
	if lines == nil {
		return
	}
	keep := []string{}
	changed := false
	for _, l := range lines {
		drop := strings.HasPrefix(l, "# zfs-toolkit")
		for _, p := range patterns {
			if strings.Contains(l, p) {
				drop = true
			}
		}
		if drop {
			changed = true
			continue
		}
		keep = append(keep, l)
	}
	if changed {
		crontabWrite(keep)
	}
}

func refreshCron() {
	if r := runCmd(30*time.Second, "update_cron"); r.OK() {
		return
	}
	if fileExecutable("/etc/rc.d/rc.crond") {
		runCmd(30*time.Second, "/etc/rc.d/rc.crond", "restart")
	}
}

type cronState struct {
	Entry   string `json:"entry"`
	Live    bool   `json:"live"`
	Source  string `json:"source"`
	Healthy bool   `json:"healthy"`
}

func (c cronJob) state() cronState {
	st := cronState{}
	if b, err := os.ReadFile(c.file()); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			if l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			if strings.Contains(l, c.pattern()) {
				st.Entry = l
				break
			}
		}
	}
	for _, probe := range []struct {
		spool bool
		label string
	}{{true, "cron.d"}, {false, "crontab"}} {
		for _, l := range crontabLines(probe.spool) {
			if strings.Contains(l, c.pattern()) {
				st.Live = true
				st.Source = probe.label
				if st.Entry == "" {
					st.Entry = strings.TrimSpace(l)
				}
				break
			}
		}
		if st.Live {
			break
		}
	}
	if st.Source == "" && st.Entry != "" {
		st.Source = "file"
	}
	st.Healthy = st.Live
	return st
}

func (c cronJob) live() bool {
	for _, spool := range []bool{true, false} {
		for _, l := range crontabLines(spool) {
			if strings.Contains(l, c.pattern()) {
				return true
			}
		}
	}
	return false
}

func (c cronJob) remove() {
	os.Remove(c.file())
	os.Remove(CronSpool + "/" + c.Base)
	crontabDrop(c.pattern())
	refreshCron()
}

func (c cronJob) install(expr string) (string, error) {
	if !validCron(expr) {
		return "", fmt.Errorf("refusing to install invalid cron expression: '%s'", expr)
	}
	os.Remove(CronSpool + "/" + c.Base)
	crontabDrop(c.pattern())
	os.MkdirAll(CronDir, 0o755)
	content := "# Managed by " + Name + " - edit via Settings > ZFS Toolkit\n" + c.line(expr) + "\n"
	if old, err := os.ReadFile(c.file()); err != nil || string(old) != content {
		if !fileWrite(c.file(), []byte(content), 0o644) {
			return "", fmt.Errorf("cannot write %s", c.file())
		}
	}
	refreshCron()
	if c.live() {
		return "Cron installed via " + c.file() + ": " + expr, nil
	}
	warn := ""
	if !exists("/var/log/plugins/" + Name + ".plg") {
		warn = "/var/log/plugins/" + Name + ".plg is missing, so update_cron does not read " + CronDir + ". "
	}
	lines := crontabLines(false)
	lines = append(lines, "# zfs-toolkit", c.line(expr))
	crontabWrite(lines)
	if c.live() {
		return warn + "Cron installed via crontab fallback: " + expr + " - that entry is transient, the next update_cron by any plugin will drop it.", nil
	}
	return "", fmt.Errorf("%scron entry could not be installed (%s)", warn, expr)
}

func (c cronJob) sync(s Settings) (string, error) {
	if !c.Enabled(s) {
		c.remove()
		return c.Label + " cron job removed.", nil
	}
	expr := c.Expr(s)
	if p := validCronProblem(expr); p != "" {
		return "", fmt.Errorf("invalid cron expression '%s': %s - the previous schedule was left untouched", expr, p)
	}
	return c.install(expr)
}

func cronSyncAll() []string {
	s := loadSettings()
	crontabDrop(legacyCronPatterns...)
	warnings := []string{}
	for _, c := range cronJobs {
		msg, err := c.sync(s)
		line := msg
		if err != nil {
			warnings = append(warnings, c.Label+" cron: "+err.Error())
			line = "ERROR: " + err.Error()
		}
		logAppendMax(TmpDir+"/cron_setup.log", line, 256*1024)
	}
	return warnings
}

func init() {
	register("cron-sync", "write the cron files for conversion, snapshots and replication from settings.cfg", func(args []string) int {
		os.MkdirAll(TmpDir, 0o755)
		w := cronSyncAll()
		for _, l := range w {
			fmt.Fprintln(os.Stderr, l)
		}
		if len(w) > 0 {
			return 1
		}
		return 0
	})
}
