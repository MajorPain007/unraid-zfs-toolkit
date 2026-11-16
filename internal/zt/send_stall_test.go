package zt

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAStalledReplicationIsEnded(t *testing.T) {
	d := t.TempDir()
	zfs := "#!/bin/sh\nif [ \"$1\" = send ]; then printf data; exec sleep 300; fi\nexec cat >/dev/null\n"
	if err := os.WriteFile(d+"/zfs", []byte(zfs), 0o755); err != nil {
		t.Fatal(err)
	}
	oldZfs, oldStall := ZfsBin, sendStallAfter
	t.Cleanup(func() { ZfsBin, sendStallAfter = oldZfs, oldStall })
	ZfsBin, sendStallAfter = d+"/zfs", 1500*time.Millisecond

	stats := &sendPipeStats{}
	start := time.Now()
	err := sendPipeline(context.Background(), sendJob{Transport: "local"}, []string{"send", "a@b"}, []string{"recv", "c"}, stats, func() {})
	if err == nil || !strings.Contains(err.Error(), "no data moved") {
		t.Fatalf("a transfer without progress must end with a clear error, got %v", err)
	}
	if took := time.Since(start); took < sendStallAfter || took > 10*time.Second {
		t.Fatalf("ended after %v, want just after %v", took, sendStallAfter)
	}
}

func TestASlowReplicationIsNotEnded(t *testing.T) {
	d := t.TempDir()
	zfs := "#!/bin/sh\nif [ \"$1\" = send ]; then for i in 1 2 3 4 5 6; do printf x; sleep 0.5; done; exit 0; fi\nexec cat >/dev/null\n"
	if err := os.WriteFile(d+"/zfs", []byte(zfs), 0o755); err != nil {
		t.Fatal(err)
	}
	oldZfs, oldStall := ZfsBin, sendStallAfter
	t.Cleanup(func() { ZfsBin, sendStallAfter = oldZfs, oldStall })
	ZfsBin, sendStallAfter = d+"/zfs", 1500*time.Millisecond

	if err := sendPipeline(context.Background(), sendJob{Transport: "local"}, []string{"send", "a@b"}, []string{"recv", "c"}, &sendPipeStats{}, func() {}); err != nil {
		t.Fatalf("a slow transfer that keeps moving must finish, got %v", err)
	}
}
