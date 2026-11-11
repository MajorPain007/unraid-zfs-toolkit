package zt

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSendJobFromMapDefaults(t *testing.T) {
	j := sendJobFromMap(M{"source": "/cache/appdata/", "dest": "backup/replica/appdata", "ssh_port": "0", "keep_dest": -3, "transport": "nope"}, 2)
	if j.ID != "job3" || !j.Enabled || !j.Compressed || j.Raw || j.AllowRollback || j.Recursive {
		t.Fatalf("defaults wrong: %+v", j)
	}
	if j.Source != "cache/appdata" || j.SSHPort != 22 || j.KeepDest != 0 || j.Transport != "local" || j.Name != "job3" {
		t.Fatalf("normalisation wrong: %+v", j)
	}
	k := sendJobFromMap(M{"id": "a b", "enabled": "0", "compressed": false, "raw": "yes", "recursive": 1.0, "transport": "ssh", "ssh_port": 2222.0}, 0)
	if k.ID != "job1" || k.Enabled || k.Compressed || !k.Raw || !k.Recursive || k.Transport != "ssh" || k.SSHPort != 2222 {
		t.Fatalf("parsing wrong: %+v", k)
	}
}

func TestSendJobProblem(t *testing.T) {
	cases := []struct {
		j    sendJob
		want string
	}{
		{sendJob{Source: "cache/appdata", Dest: "backup/replica/appdata", Transport: "local"}, ""},
		{sendJob{Source: "cache/TV Shows", Dest: "backup/replica/TV Shows", Transport: "local"}, ""},
		{sendJob{Source: "", Dest: "backup/x", Transport: "local"}, "incomplete"},
		{sendJob{Source: "cache/a", Dest: "backup", Transport: "local"}, "whole pool"},
		{sendJob{Source: "cache/a", Dest: "cache/a", Transport: "local"}, "identical"},
		{sendJob{Source: "cache/a", Dest: "cache/a/copy", Transport: "local"}, "inside the source"},
		{sendJob{Source: "cache/a/b", Dest: "cache/a", Transport: "local", Recursive: true}, "inside the destination"},
		{sendJob{Source: "cache/Büro", Dest: "backup/x", Transport: "local"}, "not a valid"},
		{sendJob{Source: "cache/a", Dest: "tank/a", Transport: "ssh", SSHHost: "-oProxyCommand"}, "SSH host"},
		{sendJob{Source: "cache/a", Dest: "tank/a", Transport: "ssh", SSHHost: "root@10.0.0.5", SSHKey: "/boot/../etc/shadow"}, "key path"},
		{sendJob{Source: "cache/a", Dest: "cache/a", Transport: "ssh", SSHHost: "root@10.0.0.5"}, ""},
	}
	for _, c := range cases {
		got := sendJobProblem(c.j)
		if (c.want == "" && got != "") || (c.want != "" && !strings.Contains(got, c.want)) {
			t.Errorf("%+v: got %q want %q", c.j, got, c.want)
		}
	}
}

func TestSendNormalizeListKeepsIncompleteDisabled(t *testing.T) {
	raw := []any{
		M{"id": "job1", "name": "one", "source": "cache/appdata", "dest": "backup/replica/appdata", "enabled": true},
		M{"name": "draft", "source": "cache/x", "dest": "", "enabled": true},
		M{"id": "job1", "name": "dup", "source": "cache/a", "dest": "cache/a", "enabled": true},
	}
	jobs, warnings := sendNormalizeList(raw)
	if len(jobs) != 3 {
		t.Fatalf("lost a job: %+v", jobs)
	}
	if !jobs[0].Enabled || jobs[1].Enabled || jobs[2].Enabled {
		t.Fatalf("enabled flags wrong: %+v", jobs)
	}
	if jobs[1].ID != "job2" || jobs[2].ID != "job1_" {
		t.Fatalf("ids wrong: %s %s", jobs[1].ID, jobs[2].ID)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "dup") {
		t.Fatalf("warnings wrong: %v", warnings)
	}
	empty, _ := sendNormalizeList([]any{})
	if empty == nil || len(empty) != 0 {
		t.Fatal("an empty list has to stay a list")
	}
}

func TestSendShareWarning(t *testing.T) {
	yes := func(string) bool { return true }
	no := func(string) bool { return false }
	j := sendJob{Source: "cache/appdata", Dest: "backup/appdata", Transport: "local"}
	if w := sendShareWarning(j, yes); !strings.Contains(w, "part of the share \"appdata\"") || !strings.Contains(w, "backup/replica/appdata") {
		t.Fatalf("got %q", w)
	}
	if w := sendShareWarning(j, no); !strings.Contains(w, "share \"appdata\"") {
		t.Fatalf("got %q", w)
	}
	j.Dest = "backup/replica/appdata"
	if w := sendShareWarning(j, yes); w != "" {
		t.Fatalf("deeper target warned: %q", w)
	}
	j.Dest = "tank/appdata"
	j.Transport = "ssh"
	if w := sendShareWarning(j, yes); w != "" {
		t.Fatalf("remote target warned: %q", w)
	}
}

func TestSendCheckpointOwnership(t *testing.T) {
	if !sendIsCheckpointOf("zdc-send-job1-20261107-120000", "job1") {
		t.Fatal("own checkpoint not recognised")
	}
	if !sendIsCheckpointOf("zdc-send-job1-20261107-120000-2", "job1") {
		t.Fatal("suffixed checkpoint not recognised")
	}
	for _, s := range []string{"zdc-send-job1-x-20261107-120000", "zdc-send-job10-20261107-120000", "auto-daily-2026-11-07", "zdc-send-job1-latest"} {
		if sendIsCheckpointOf(s, "job1") {
			t.Fatalf("%s taken as a checkpoint of job1", s)
		}
	}
}

func TestSendPruneSelection(t *testing.T) {
	rows := []string{
		"cache/appdata@auto-hourly-2026-11-07_10-00",
		"cache/appdata@zdc-send-j-20261105-040000",
		"cache/appdata@zdc-send-j-20261106-040000",
		"cache/appdata@zdc-send-j-20261107-040000",
		"cache/appdata@zdc-send-j-20261107-040000-2",
		"cache/appdata@zdc-send-j-x-20261101-040000",
		"cache/appdata@zdc-send-other-20261101-040000",
		"cache/appdata/plex@zdc-send-j-20261105-040000",
		"cache/appdata/plex@zdc-send-j-20261106-040000",
		"cache/appdata/plex@zdc-send-j-20261107-040000",
		"cache/appdata/Home Assistant@zdc-send-j-20261105-040000",
	}
	got := sendPruneSelection(rows, "j", 2)
	want := []string{
		"cache/appdata@zdc-send-j-20261105-040000",
		"cache/appdata@zdc-send-j-20261106-040000",
		"cache/appdata/plex@zdc-send-j-20261105-040000",
		"cache/appdata/plex@zdc-send-j-20261106-040000",
		"cache/appdata/Home Assistant@zdc-send-j-20261105-040000",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if sendPruneSelection(rows, "j", 0) == nil {
		t.Fatal("keep 0 must still keep the newest and prune the rest")
	}
	if len(sendPruneSelection(rows[:3], "j", 5)) != 0 {
		t.Fatal("pruned below the keep count")
	}
	grouped := sendGroupDestroy(got)
	if !reflect.DeepEqual(grouped, []string{
		"cache/appdata@zdc-send-j-20261105-040000,zdc-send-j-20261106-040000",
		"cache/appdata/plex@zdc-send-j-20261105-040000,zdc-send-j-20261106-040000",
		"cache/appdata/Home Assistant@zdc-send-j-20261105-040000",
	}) {
		t.Fatalf("grouped %v", grouped)
	}
}

func TestSendStampOrder(t *testing.T) {
	if !sendStampLess("20261107-040000", "20261107-040000-2") || !sendStampLess("20261107-040000-2", "20261107-040000-10") || !sendStampLess("20261107-040000-10", "20261107-040001") {
		t.Fatal("stamp order wrong")
	}
}

func TestSendFindBase(t *testing.T) {
	src := []sendSnap{{"a", "1", 1}, {"b", "2", 2}, {"c", "3", 3}, {"d", "4", 4}}
	dst := []sendSnap{{"a", "1", 1}, {"c", "3", 3}, {"x", "9", 5}}
	b, ok := sendFindBase(src, dst)
	if !ok || b.Name != "c" {
		t.Fatalf("got %+v %v", b, ok)
	}
	if newer := sendNewerOnDest(dst, b); !reflect.DeepEqual(newer, []string{"x"}) {
		t.Fatalf("newer %v", newer)
	}
	if _, ok := sendFindBase(src, []sendSnap{{"c", "999", 3}}); ok {
		t.Fatal("a snapshot with the same name but another guid is not a common base")
	}
	if _, ok := sendFindBase(src, nil); ok {
		t.Fatal("base without destination snapshots")
	}
}

func TestSendRemoteCommandQuoting(t *testing.T) {
	got := sendRemoteCommand("zfs", "recv", "-u", "-x", "mountpoint", "tank/TV Shows", "it's")
	want := `zfs recv -u -x mountpoint 'tank/TV Shows' 'it'\''s'`
	if got != want {
		t.Fatalf("got %s", got)
	}
	if sendRemoteCommand("a;rm -rf /") != `'a;rm -rf /'` || sendRemoteCommand("$(x)") != `'$(x)'` || sendRemoteCommand("") != `''` {
		t.Fatal("metacharacters not quoted")
	}
}

func TestSendArgs(t *testing.T) {
	j := sendJob{Source: "cache/a", Dest: "backup/replica/a", Recursive: true, Compressed: true, AllowRollback: true, Readonly: true}
	if got := strings.Join(j.sendArgs("cp", "base"), " "); got != "send -R -L -e -c -I @base cache/a@cp" {
		t.Fatalf("send %s", got)
	}
	if got := strings.Join(j.recvArgs(), " "); got != "recv -u -x mountpoint -F -o readonly=on backup/replica/a" {
		t.Fatalf("recv %s", got)
	}
	j = sendJob{Source: "cache/a", Dest: "backup/replica/a", Raw: true}
	if got := strings.Join(j.sendArgs("cp", ""), " "); got != "send -w cache/a@cp" {
		t.Fatalf("send %s", got)
	}
	if got := strings.Join(j.recvArgs(), " "); got != "recv -u -x mountpoint backup/replica/a" {
		t.Fatalf("recv without force %s", got)
	}
}

func TestSendLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	old := ConfigDir
	ConfigDir = dir
	defer func() { ConfigDir = old }()
	legacy := `{"jobs":[{"id":"t1","enabled":true,"name":"t","source":"cache/appdata","dest":"backup/replica/appdata","recursive":true,"transport":"local","ssh_host":"","ssh_port":22,"ssh_key":"","raw":false,"compressed":true,"allow_rollback":true,"keep_dest":2}]}`
	os.WriteFile(filepath.Join(dir, "send_jobs.json"), []byte(legacy), 0o644)
	jobs, err := sendLoadJobs()
	if err != nil || len(jobs) != 1 || jobs[0].ID != "t1" || !jobs[0].AllowRollback || jobs[0].KeepDest != 2 || jobs[0].Readonly {
		t.Fatalf("legacy file read wrong: %+v", jobs)
	}
	if err := sendWriteJobs(nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "send_jobs.json"))
	if !strings.Contains(string(b), `"jobs": []`) {
		t.Fatalf("empty list written as %s", b)
	}
}

func TestSendSSHExplain(t *testing.T) {
	if !strings.Contains(sendSSHExplain("root@x: Permission denied (publickey)."), "did not accept the key") {
		t.Fatal("permission denied")
	}
	if !strings.Contains(sendSSHExplain("@@@ WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED! @@@"), "host key") {
		t.Fatal("host key")
	}
	if sendSSHExplain("all fine") != "" {
		t.Fatal("explained nothing")
	}
}

func TestSendNextCheckpoint(t *testing.T) {
	if got := sendNextCheckpoint("j", "20261107-120000", nil); got != "zdc-send-j-20261107-120000" {
		t.Fatal(got)
	}
	if got := sendNextCheckpoint("j", "20261107-120000", []string{"zdc-send-j-20261107-120000"}); got != "zdc-send-j-20261107-120000-2" {
		t.Fatal(got)
	}
	if got := sendNextCheckpoint("j", "20261107-120000", []string{"zdc-send-j-20261107-120000-3", "auto-x"}); got != "zdc-send-j-20261107-120000-4" {
		t.Fatalf("a pruned name must never be reused: %s", got)
	}
	if got := sendNextCheckpoint("j", "20261107-120000", []string{"zdc-send-j-20261107-110000"}); got != "zdc-send-j-20261107-120000" {
		t.Fatal(got)
	}
}

func TestSendInspectRejectsOptionHost(t *testing.T) {
	d := t.TempDir()
	rec := filepath.Join(d, "ssh.args")
	for name, body := range map[string]string{"ssh": "printf '%s\\n' \"$@\" > " + rec + "\nexit 1", "zfs": "exit 1"} {
		if err := os.WriteFile(filepath.Join(d, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	oldSSH, oldZfs, oldTmp, oldCfg := sendSSHBin, ZfsBin, TmpDir, ConfigDir
	sendSSHBin, ZfsBin, TmpDir, ConfigDir = filepath.Join(d, "ssh"), filepath.Join(d, "zfs"), filepath.Join(d, "tmp"), filepath.Join(d, "cfg")
	defer func() { sendSSHBin, ZfsBin, TmpDir, ConfigDir = oldSSH, oldZfs, oldTmp, oldCfg }()
	job := `{"id":"j1","source":"tank/a","dest":"tank/b/c","transport":"ssh","ssh_host":"-oProxyCommand=touch /tmp/pwn","ssh_port":22}`
	out := apiDispatch("send_inspect", M{"job": job})
	if out["ok"] != false || out["error"] == nil {
		t.Fatalf("send_inspect accepted the job: %v", out)
	}
	if b, err := os.ReadFile(rec); err == nil {
		t.Fatalf("ssh was started with:\n%s", b)
	}
}

func TestSendSSHArgsHostNeverAnOption(t *testing.T) {
	a, err := sendJob{SSHHost: "-oProxyCommand=x", SSHPort: 22}.sshArgs()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) < 2 || a[len(a)-1] != "-oProxyCommand=x" || a[len(a)-2] != "--" {
		t.Fatalf("host can be read as an option: %q", a)
	}
}

func TestSendJobsCorruptIsNeverOverwritten(t *testing.T) {
	d := t.TempDir()
	oldCfg, oldTmp := ConfigDir, TmpDir
	ConfigDir, TmpDir = filepath.Join(d, "cfg"), filepath.Join(d, "tmp")
	defer func() { ConfigDir, TmpDir = oldCfg, oldTmp }()
	if out := apiDispatch("send_save_job", M{"job": `{"id":"a","source":"tank/a","dest":"tank/r/a"}`}); out["ok"] != true {
		t.Fatalf("saving into a missing file failed: %v", out)
	}
	good := `{"jobs":[{"id":"a","source":"tank/a","dest":"tank/r/a","enabled":true},{"id":"b","source":"tank/b","dest":"tank/r/b","enabled":true}]}`
	cut := []byte(good[:60])
	if err := os.WriteFile(sendJobsFile(), cut, 0o600); err != nil {
		t.Fatal(err)
	}
	if out := apiDispatch("send_jobs", M{}); out["ok"] != false || !strings.Contains(str(out["error"]), "Nothing is saved over it") {
		t.Errorf("send_jobs hides the broken file: %v", out)
	}
	for _, c := range []struct {
		action string
		p      M
	}{
		{"send_save_job", M{"job": `{"id":"c","source":"tank/c","dest":"tank/r/c"}`}},
		{"send_save", M{"jobs": `[]`}},
		{"send_toggle", M{"id": "a", "enabled": 0}},
		{"send_delete", M{"id": "a"}},
	} {
		if out := apiDispatch(c.action, c.p); out["ok"] != false || !strings.Contains(str(out["error"]), "Nothing is saved over it") {
			t.Errorf("%s did not refuse: %v", c.action, out)
		}
	}
	if b, _ := os.ReadFile(sendJobsFile()); string(b) != string(cut) {
		t.Fatalf("the broken file was overwritten with %s", b)
	}
}

func TestSendRunReportsUnreadableJobs(t *testing.T) {
	notified := fakeNotify(t)
	d := t.TempDir()
	for name, body := range map[string]string{"zpool": "echo tank", "zfs": "exit 1"} {
		if err := os.WriteFile(filepath.Join(d, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	oldCfg, oldTmp, oldZfs, oldZpool := ConfigDir, TmpDir, ZfsBin, ZpoolBin
	ConfigDir, TmpDir, ZfsBin, ZpoolBin = filepath.Join(d, "cfg"), filepath.Join(d, "tmp"), filepath.Join(d, "zfs"), filepath.Join(d, "zpool")
	defer func() { ConfigDir, TmpDir, ZfsBin, ZpoolBin = oldCfg, oldTmp, oldZfs, oldZpool }()
	if err := os.MkdirAll(ConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	good := `{"jobs":[{"id":"a","source":"tank/a","dest":"tank/r/a","enabled":true}]}`
	if err := os.WriteFile(sendJobsFile(), []byte(good[:30]), 0o600); err != nil {
		t.Fatal(err)
	}
	for run := 1; run <= 2; run++ {
		if code := sendRun(nil); code == 0 {
			t.Errorf("run %d: send-run exits 0 with an unreadable job list", run)
		}
	}
	logb, _ := os.ReadFile(sendLogFile())
	if !strings.Contains(string(logb), "ERROR: ") || !strings.Contains(string(logb), "send_jobs.json") {
		t.Errorf("the log does not name the broken file:\n%s", logb)
	}
	if strings.Contains(string(logb), "No enabled replication jobs") {
		t.Errorf("the log claims there are no jobs:\n%s", logb)
	}
	if n := notified(); n != 1 {
		t.Errorf("%d notification(s) for two runs, want exactly one", n)
	}
	st := sendOverview(loadSettings()).(M)["status"].(M)
	if st["level"] != "err" || !strings.Contains(str(st["sub"]), "send_jobs.json") {
		t.Errorf("overview status %v, want err naming the file", st)
	}
}
