package zt

import (
	"os"
	"strings"
	"testing"
)

func TestOnlyOurOldCronLinesAreRemoved(t *testing.T) {
	d := t.TempDir()
	tab := d + "/tab"
	os.WriteFile(tab, []byte("0 2 * * * /usr/local/emhttp/plugins/zfs.dataset.converter/scripts/run_auto.sh\n0 3 * * * /root/backup/run_auto.sh\n*/5 * * * * /usr/local/bin/zfs_send.sh tank\n"), 0o644)
	os.WriteFile(d+"/crontab", []byte("#!/bin/sh\nif [ \"$1\" = -l ]; then cat "+tab+"; else cat > "+tab+"; fi\n"), 0o755)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	crontabDrop(legacyCronPatterns...)
	b, _ := os.ReadFile(tab)
	if strings.Contains(string(b), "zfs.dataset.converter") {
		t.Fatalf("the old plugin's line must go: %s", b)
	}
	if !strings.Contains(string(b), "/root/backup/run_auto.sh") || !strings.Contains(string(b), "/usr/local/bin/zfs_send.sh") {
		t.Fatalf("someone else's lines with the same script names must stay: %s", b)
	}
}

func TestSnapshotRunningNeedsOurProcess(t *testing.T) {
	old := TmpDir
	TmpDir = t.TempDir()
	t.Cleanup(func() { TmpDir = old })
	jsonWrite(snapRunningFile(), M{"pid": os.Getpid()})
	if _, err := os.Stat("/proc/self/cmdline"); err == nil && snapRunning() != nil {
		t.Fatal("a live process that is not snapshot-tick is not a running snapshot job")
	}
}
