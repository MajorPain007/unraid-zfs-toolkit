package zt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func diagFixture(t *testing.T, jobs string) string {
	t.Helper()
	d := t.TempDir()
	oldCfg, oldTmp, oldCron, oldSpool, oldZfs, oldZpool := ConfigDir, TmpDir, CronDir, CronSpool, ZfsBin, ZpoolBin
	ConfigDir, TmpDir, CronDir, CronSpool = filepath.Join(d, "cfg"), filepath.Join(d, "tmp"), filepath.Join(d, "cfg"), filepath.Join(d, "spool")
	ZfsBin, ZpoolBin = filepath.Join(d, "zfs"), filepath.Join(d, "zpool")
	t.Cleanup(func() {
		ConfigDir, TmpDir, CronDir, CronSpool, ZfsBin, ZpoolBin = oldCfg, oldTmp, oldCron, oldSpool, oldZfs, oldZpool
	})
	for _, dir := range []string{ConfigDir, TmpDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(ConfigDir+"/send_jobs.json", []byte(jobs), 0o600); err != nil {
		t.Fatal(err)
	}
	log := "[2026-11-11 03:00:01] SSH login to backup@offsite.example.net port 23 works.\n" +
		"[2026-11-11 03:00:02] Cannot log in to 10.9.8.7 port 2222 with key based authentication: denied\n" +
		"[2026-11-11 03:00:03] ssh -p 2222 10.9.8.7 'zfs list -H -o name'\n"
	if err := os.WriteFile(TmpDir+"/send.log", []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	return diagnosticsReport()
}

func diagHidesHosts(t *testing.T, report string) {
	t.Helper()
	for _, secret := range []string{"offsite.example.net", "10.9.8.7", "2222", "port 23", "\"ssh_port\": 23"} {
		if strings.Contains(report, secret) {
			t.Errorf("the report contains %q", secret)
		}
	}
	if !strings.Contains(report, "backup@") {
		t.Errorf("the user part of the host should stay readable")
	}
	if !strings.Contains(report, "\"name\": \"Offsite\"") {
		t.Errorf("the job list itself should stay in the report")
	}
}

func TestDiagnosticsHideReplicationHosts(t *testing.T) {
	jobs := `{"jobs": [
  {"id": "a", "name": "Offsite", "source": "cache/appdata", "dest": "tank/replica", "transport": "ssh", "ssh_host": "backup@offsite.example.net", "ssh_port": 23},
  {"id": "b", "name": "Second", "source": "cache/domains", "dest": "pool/replica", "transport": "ssh", "ssh_host": "10.9.8.7", "ssh_port": 2222},
  {"id": "c", "name": "Local", "source": "cache/isos", "dest": "disk1/replica", "transport": "local", "ssh_host": "", "ssh_port": 22}
]}
`
	diagHidesHosts(t, diagFixture(t, jobs))
}

func TestDiagnosticsHideHostsOfUnreadableJobList(t *testing.T) {
	jobs := `{"jobs": [
  {"id": "a", "name": "Offsite", "source": "cache/appdata", "dest": "tank/replica", "transport": "ssh", "ssh_host": "backup@offsite.example.net", "ssh_port": 23},
  {"id": "b", "name": "Second", "source": "cache/domains", "dest": "pool/replica", "transport": "ssh", "ssh_host": "10.9.8.7", "ssh_port": 2222},
  {"id": "c", "na`
	diagHidesHosts(t, diagFixture(t, jobs))
}

func TestDiagnosticsHideHostsOfDeletedJobs(t *testing.T) {
	fakeNotify(t)
	d := t.TempDir()
	for name, body := range map[string]string{
		"zpool": "echo tank",
		"zfs":   "case \"$*\" in *encryption*) printf 'tank/a\\toff\\n' ;; esac",
		"ssh":   "echo 'ssh: connect to host offsite.example.org port 2200: Connection refused' >&2\necho 'Backup@OFFSITE.Example.Org: Permission denied (publickey).' >&2\nexit 255",
	} {
		if err := os.WriteFile(filepath.Join(d, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	oldCfg, oldTmp, oldZfs, oldZpool, oldSSH := ConfigDir, TmpDir, ZfsBin, ZpoolBin, sendSSHBin
	ConfigDir, TmpDir, ZfsBin, ZpoolBin, sendSSHBin = filepath.Join(d, "cfg"), filepath.Join(d, "tmp"), filepath.Join(d, "zfs"), filepath.Join(d, "zpool"), filepath.Join(d, "ssh")
	defer func() { ConfigDir, TmpDir, ZfsBin, ZpoolBin, sendSSHBin = oldCfg, oldTmp, oldZfs, oldZpool, oldSSH }()
	if err := os.MkdirAll(ConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	job := `{"jobs":[{"id":"a","name":"Offsite","source":"tank/a","dest":"backup/replica/a","transport":"ssh","ssh_host":"backup@Offsite.Example.Org","ssh_port":2200,"enabled":true}]}`
	if err := os.WriteFile(sendJobsFile(), []byte(job), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := sendRun(nil); code == 0 {
		t.Fatal("the run should fail at the SSH login")
	}
	if code := sendRun([]string{"--test", "a"}); code == 0 {
		t.Fatal("the test run should fail at the SSH login")
	}
	if b, _ := os.ReadFile(sendLogFile()); !strings.Contains(string(b), "Cannot log in") {
		t.Fatalf("the run did not reach the SSH login:\n%s", b)
	}
	if b, _ := os.ReadFile(sendStatusFile()); strings.Contains(strings.ToLower(string(b)), "offsite.example.org") {
		t.Errorf("send_status.json names the host:\n%s", b)
	}
	if err := os.WriteFile(sendJobsFile(), []byte(`{"jobs":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	report := diagnosticsReport()
	if strings.Contains(strings.ToLower(report), "offsite.example.org") {
		t.Errorf("the report names the host of a deleted job:\n%s", report)
	}
	if !strings.Contains(report, "Cannot log in") {
		t.Errorf("the failed login should stay in the report:\n%s", report)
	}
}

func TestSendHideHostOnlyWholeNames(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"tank/nasdata -> root@NAS:backup/nas.copy", "tank/nasdata -> root@…:backup/….copy"},
		{"ssh: Could not resolve hostname nas: Name does not resolve", "ssh: Could not resolve hostname …: Name does not resolve"},
		{"nas nas", "… …"},
		{"anas", "anas"},
		{"Job 'xnas-nas' on [nas]", "Job 'xnas-nas' on […]"},
	} {
		if got := sendHideHost(c.in, "root@nas"); got != c.want {
			t.Errorf("%q: got %q, want %q", c.in, got, c.want)
		}
	}
}
