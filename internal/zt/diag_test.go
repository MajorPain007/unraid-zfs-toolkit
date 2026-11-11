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
