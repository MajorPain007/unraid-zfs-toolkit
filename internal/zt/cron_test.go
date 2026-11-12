package zt

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func cronTestZone(t *testing.T) {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = old })
}

func cronAt(t *testing.T, s string) int64 {
	t.Helper()
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if tm, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return tm.Unix()
		}
	}
	t.Fatalf("bad time %s", s)
	return 0
}

func cronFmt(ts int64, ok bool) string {
	if !ok {
		return "null"
	}
	return time.Unix(ts, 0).In(time.Local).Format("2006-01-02 15:04 Mon")
}

func cronMust(t *testing.T, expr string) *cronExpr {
	t.Helper()
	c, err := cronParse(expr)
	if err != nil {
		t.Fatalf("%s: %v", expr, err)
	}
	return c
}

func cronEq(t *testing.T, want, got any, what ...string) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("%s: expected %v, got %v", strings.Join(what, ""), want, got)
	}
}

func TestValidCronMistakesAreNamed(t *testing.T) {
	cases := map[string]string{
		"":                    "no cron expression",
		"   ":                 "no cron expression",
		"* * * *":             "5 fields",
		"* * * * * *":         "5 fields",
		"60 * * * *":          "minute must be 0-59",
		"* 24 * * *":          "hour must be 0-23",
		"* * 0 * *":           "day of month must be 1-31",
		"* * 32 * *":          "day of month must be 1-31",
		"* * * 13 *":          "month must be 1-12",
		"* * * * 8":           "weekday must be 0-7",
		"5-1 * * * *":         "range must run upwards",
		"5-60 * * * *":        "range must run upwards",
		"1-2-3 * * * *":       "range must be two numbers",
		"*/0 * * * *":         "step must be 1-59",
		"*/60 * * * *":        "step must be 1-59",
		"1,,2 * * * *":        "empty value",
		"x * * * *":           "only digits",
		"0 9 * * mon-fri":     "only digits",
		"0 4 * jan *":         "only digits",
		"@daily":              "only digits",
		"@reboot":             "only digits",
		"0 2 * * *; rm -rf /": "shell command",
		"0 2 * * * && reboot": "shell command",
		"0 2 * * * | sh":      "shell command",
		"$(id) * * * *":       "shell command",
		"`id` * * * *":        "shell command",
		"0 2 * * * > /boot/x": "shell command",
		"0 2 * * * < /x":      "shell command",
		"0 2 * * *\nreboot":   "shell command",
		"0 2 * * *\r":         "shell command",
		"0 (2) * * *":         "shell command",
		"0\f2 * * *":          "only digits",
		"0 2\v* * *":          "only digits",
		"0 2 * * *\v":         "only digits",
		"\f0 2 * * *":         "only digits",
		"0\x002 * * *":        "only digits",
		"0\u00a02 * * *":      "only digits",
	}
	for expr, needle := range cases {
		p := validCronProblem(expr)
		if p == "" {
			t.Errorf("%q was accepted", expr)
		} else if !strings.Contains(strings.ToLower(p), needle) {
			t.Errorf("%q: %q lacks %q", expr, p, needle)
		}
		if validCron(expr) {
			t.Errorf("validCron accepted %q", expr)
		}
	}
}

func TestValidCronUsualFormsAccepted(t *testing.T) {
	for _, expr := range []string{"*/15 * * * *", "0 3 * * *", "30 2 * * 1-5", "0 0 1 */3 *", "5/15 * * * *",
		"  0  3  *  *  *  ", "0 0 * * 7", "0 4 * 1,7 0", "1-5/2 * * * *", "0\t3 * * *", "59 23 31 12 7", "*/59 */23 * * *",
		"0-5 * * * *", "1-59 * * * *", "5-5 * * * *", "*/1 * * * *", "0 0-23 1-31 1-12 0-7"} {
		cronEq(t, "", validCronProblem(expr), expr)
		cronEq(t, true, validCron(expr), expr)
	}
}

func TestCronInstallRefusesControlCharacters(t *testing.T) {
	d := t.TempDir()
	bin := filepath.Join(d, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(d, "calls")
	for _, name := range []string{"crontab", "update_cron"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\necho "+name+" >> "+calls+"\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	oldDir, oldSpool, oldTmp := CronDir, CronSpool, TmpDir
	CronDir, CronSpool, TmpDir = filepath.Join(d, "cfg"), filepath.Join(d, "spool"), filepath.Join(d, "tmp")
	defer func() { CronDir, CronSpool, TmpDir = oldDir, oldSpool, oldTmp }()
	job := cronJobs[1]
	for _, expr := range []string{"0\f2 * * *", "0 2 * * *\v", "0 2\x00* * *"} {
		if _, err := job.install(expr); err == nil {
			t.Errorf("%q was installed", expr)
		}
		if b, err := os.ReadFile(job.file()); err == nil {
			t.Fatalf("%q was written as %q", expr, b)
		}
		if b, err := os.ReadFile(calls); err == nil {
			t.Fatalf("%q reached crontab: %s", expr, b)
		}
	}
	job.install("0 2 * * *")
	b, err := os.ReadFile(job.file())
	if err != nil || !strings.Contains(string(b), "\n0 2 * * * ") {
		t.Fatalf("a valid expression was not written: %q %v", b, err)
	}
}

func TestCronStepRunsOnMultiples(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "*/15 * * * *")
	cronEq(t, "2026-10-01 10:15 Thu", cronFmt(c.prev(cronAt(t, "2026-10-01 10:29:59"))))
	cronEq(t, "2026-10-01 10:30 Thu", cronFmt(c.next(cronAt(t, "2026-10-01 10:15:00"))))
}

func TestCronStarCoversTheLastValue(t *testing.T) {
	cronTestZone(t)
	cronEq(t, "2026-12-31 23:59 Thu", cronFmt(cronMust(t, "* * * * *").next(cronAt(t, "2026-12-31 23:58"))))
}

func TestCronStepAfterASingleValueRunsToTheEnd(t *testing.T) {
	cronTestZone(t)
	cronEq(t, "2026-10-01 10:59 Thu", cronFmt(cronMust(t, "1/2 * * * *").prev(cronAt(t, "2026-10-01 10:59:30"))))
}

func TestCronStepStartsAtA(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "5/15 * * * *")
	mins := []string{}
	for _, ts := range c.upcoming(cronAt(t, "2026-10-01 09:59"), 4) {
		mins = append(mins, time.Unix(ts, 0).In(time.Local).Format("04"))
	}
	cronEq(t, []string{"05", "20", "35", "50"}, mins)
}

func TestCronLatestRunTodayOrYesterday(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "0 3 * * *")
	cronEq(t, "2026-10-01 03:00 Thu", cronFmt(c.prev(cronAt(t, "2026-10-01 10:00"))))
	cronEq(t, "2026-09-30 03:00 Wed", cronFmt(c.prev(cronAt(t, "2026-10-01 02:59"))))
	cronEq(t, "2026-10-01 03:00 Thu", cronFmt(c.prev(cronAt(t, "2026-10-01 03:00:30"))), "within the minute")
	cronEq(t, "2026-10-02 03:00 Fri", cronFmt(c.next(cronAt(t, "2026-10-01 03:00:00"))), "next is strictly later")
}

func TestCronTurnOfMonthAndYear(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "30 23 31 12 *")
	cronEq(t, "2025-12-31 23:30 Wed", cronFmt(c.prev(cronAt(t, "2026-10-01 12:00"))))
	cronEq(t, "2026-12-31 23:30 Thu", cronFmt(c.next(cronAt(t, "2026-10-01 12:00"))))
}

func TestCronDomOrDowWhenBothGiven(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "0 3 1 * 1")
	days := []string{}
	for _, ts := range c.upcoming(cronAt(t, "2026-10-01 04:00"), 4) {
		days = append(days, time.Unix(ts, 0).In(time.Local).Format("Mon 2"))
	}
	cronEq(t, []string{"Mon 5", "Mon 12", "Mon 19", "Mon 26"}, days)
	ts, _ := c.next(cronAt(t, "2026-10-31 12:00"))
	cronEq(t, "Sun 1", time.Unix(ts, 0).In(time.Local).Format("Mon 2"), "November 1st is a Sunday")
}

func TestCronStarFieldMakesItBoth(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "0 0 */2 * 1")
	for _, ts := range c.upcoming(cronAt(t, "2026-10-01"), 6) {
		tm := time.Unix(ts, 0).In(time.Local)
		cronEq(t, "Mon", tm.Format("Mon"), tm.Format("2006-01-02"))
		cronEq(t, 1, tm.Day()%2, tm.Format("2006-01-02"), " is an odd day")
	}
}

func TestCronSundayAndWeekdayRange(t *testing.T) {
	cronTestZone(t)
	cronEq(t, cronFmt(cronMust(t, "0 0 * * 0").next(cronAt(t, "2026-10-01"))),
		cronFmt(cronMust(t, "0 0 * * 7").next(cronAt(t, "2026-10-01"))))
	cronEq(t, "2026-10-04 00:00 Sun", cronFmt(cronMust(t, "0 0 * * 7").next(cronAt(t, "2026-10-01"))))
	cronEq(t, "2026-10-02 09:00 Fri", cronFmt(cronMust(t, "0 9 * * 1-5").next(cronAt(t, "2026-10-01 09:00"))))
	cronEq(t, "2026-10-05 09:00 Mon", cronFmt(cronMust(t, "0 9 * * 1-5").next(cronAt(t, "2026-10-02 09:00"))))
}

func TestCronFebruary29(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "0 0 29 2 *")
	cronEq(t, "2024-02-29 00:00 Thu", cronFmt(c.prev(cronAt(t, "2026-10-01"))))
	cronEq(t, "2028-02-29 00:00 Tue", cronFmt(c.next(cronAt(t, "2026-10-01"))))
}

func TestCronNeverFires(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "0 0 31 2 *")
	cronEq(t, "null", cronFmt(c.prev(cronAt(t, "2026-10-01"))))
	cronEq(t, "null", cronFmt(c.next(cronAt(t, "2026-10-01"))))
}

func TestCronDSTSkippedHour(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "30 2 * * *")
	cronEq(t, "2026-03-29 03:30 Sun", cronFmt(c.next(cronAt(t, "2026-03-28 12:00"))))
	cronEq(t, "2026-03-28 02:30 Sat", cronFmt(c.prev(cronAt(t, "2026-03-29 03:10"))), "not yet at 03:10")
	cronEq(t, "2026-03-29 03:30 Sun", cronFmt(c.prev(cronAt(t, "2026-03-29 03:40"))))
	cronEq(t, "2026-03-30 02:30 Mon", cronFmt(c.next(cronAt(t, "2026-03-29 03:30"))))
}

func TestCronRareScheduleIsQuick(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "0 0 29 2 1")
	t0 := time.Now()
	for i := 0; i < 50; i++ {
		cronMust(t, "0 0 29 2 *").prev(cronAt(t, "2026-10-01"))
	}
	if d := time.Since(t0); d > 2*time.Second {
		t.Errorf("fifty look-ups took %v", d)
	}
	cronEq(t, "2027-02-01 00:00 Mon", cronFmt(c.next(cronAt(t, "2026-10-01"))), "the first Monday in February")
}

func TestCronParseAcceptsOnlyWhatCronInstalls(t *testing.T) {
	for _, expr := range []string{"*/15 * * * *", "0 3 * * 1-5", "  0  3  *  *  *  ", "0\t3 * * *", "59 23 31 12 7", "5/15 * * * *",
		"@daily", "@hourly", "0 9 * * mon-fri", "0 4 * jan *", "0 0 * * SUN", "0\f2 * * *", "0 2 * * *\v", "\v0 2 * * *", "0 2 * * *\x00",
		"*/60 * * * *", "0 2 * * *; rm -rf /", "", "* * * *"} {
		_, err := cronParse(expr)
		if (err == nil) != validCron(expr) {
			t.Errorf("%q: cronParse error %v, validCron %v", expr, err, validCron(expr))
		}
	}
}

func TestCronInstallReachesTheLiveCrontab(t *testing.T) {
	d := t.TempDir()
	bin := filepath.Join(d, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	live, user := filepath.Join(d, "live"), filepath.Join(d, "user")
	crontab := "#!/bin/sh\nf=" + user + "\nif [ \"$1\" = -c ]; then f=" + live + "; shift 2; fi\ncase \"$1\" in\n-l) [ -f \"$f\" ] && cat \"$f\" ;;\n-d) rm -f \"$f\" ;;\n-) cat > \"$f\" ;;\nesac\n"
	update := "#/bin/bash\ncat " + filepath.Join(d, "cfg") + "/*.cron 2>/dev/null | crontab -c /etc/cron.d -\n"
	for name, body := range map[string]string{"crontab": crontab, "update_cron": update} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	oldDir, oldSpool, oldTmp := CronDir, CronSpool, TmpDir
	CronDir, CronSpool, TmpDir = filepath.Join(d, "cfg"), filepath.Join(d, "spool"), filepath.Join(d, "tmp")
	defer func() { CronDir, CronSpool, TmpDir = oldDir, oldSpool, oldTmp }()
	job := cronJobs[1]
	read := func(f string) string { b, _ := os.ReadFile(f); return string(b) }
	for _, expr := range []string{"0 * * * *", "*/5 * * * *"} {
		msg, err := job.install(expr)
		if err != nil || strings.Contains(msg, "fallback") {
			t.Fatalf("%q: %q %v", expr, msg, err)
		}
		if !strings.Contains(read(live), job.line(expr)) {
			t.Fatalf("%q is not in the live crontab: %q", expr, read(live))
		}
		if read(user) != "" {
			t.Fatalf("%q left a transient entry: %q", expr, read(user))
		}
	}
	if strings.Contains(read(live), job.line("0 * * * *")) {
		t.Fatalf("the old schedule is still live: %q", read(live))
	}
	job.remove()
	if strings.Contains(read(live), job.pattern()) {
		t.Fatalf("a removed job is still live: %q", read(live))
	}
}

func TestCronInstallDoesNotTakeAnOldScheduleForTheNewOne(t *testing.T) {
	d := t.TempDir()
	bin := filepath.Join(d, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	live, user := filepath.Join(d, "live"), filepath.Join(d, "user")
	crontab := "#!/bin/sh\nf=" + user + "\nif [ \"$1\" = -c ]; then f=" + live + "; shift 2; fi\ncase \"$1\" in\n-l) [ -f \"$f\" ] && cat \"$f\" ;;\n-) cat > \"$f\" ;;\nesac\n"
	for name, body := range map[string]string{"crontab": crontab, "update_cron": "#!/bin/sh\nexit 0\n"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	oldDir, oldSpool, oldTmp := CronDir, CronSpool, TmpDir
	CronDir, CronSpool, TmpDir = filepath.Join(d, "cfg"), filepath.Join(d, "spool"), filepath.Join(d, "tmp")
	defer func() { CronDir, CronSpool, TmpDir = oldDir, oldSpool, oldTmp }()
	job := cronJobs[1]
	if err := os.WriteFile(live, []byte(job.line("0 * * * *")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	msg, _ := job.install("*/5 * * * *")
	if !strings.Contains(msg, "fallback") {
		t.Fatalf("the old hourly line was taken for the new schedule: %q", msg)
	}
	readOnly := "#!/bin/sh\nf=" + user + "\nif [ \"$1\" = -c ]; then f=" + live + "; shift 2; fi\n[ \"$1\" = -l ] && [ -f \"$f\" ] && cat \"$f\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "crontab"), []byte(readOnly), 0o755); err != nil {
		t.Fatal(err)
	}
	os.Remove(user)
	if msg, err := job.install("*/10 * * * *"); err == nil {
		t.Fatalf("an install that reached no crontab was reported as done: %q", msg)
	}
}
