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
	jobs := sendLoadJobs()
	if len(jobs) != 1 || jobs[0].ID != "t1" || !jobs[0].AllowRollback || jobs[0].KeepDest != 2 || jobs[0].Readonly {
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
