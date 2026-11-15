package zt

import (
	"os"
	"time"
)

func init() {
	register("stop-all", "for the array stop: end a conversion (its folder is put back, nothing is started again), restores and replication; stop-all [seconds to wait, default 90]", stopAll)
}

func stopAll(args []string) int {
	wait := 90
	if len(args) > 0 {
		wait = min(max(toInt(args[0]), 5), 600)
	}
	os.MkdirAll(TmpDir, 0o755)
	os.WriteFile(arrayStoppingFile(), nil, 0o644)
	convStop()
	for _, j := range restoreAll() {
		if restoreActive(j) {
			apiRestoreStop(M{"id": j.ID})
		}
	}
	if sendReadStatus().Running {
		apiSendStop(M{})
	}
	for deadline := time.Now().Add(time.Duration(wait) * time.Second); time.Now().Before(deadline); time.Sleep(time.Second) {
		_, converting := convRunning()
		restoring := false
		for _, j := range restoreAll() {
			restoring = restoring || restoreActive(j)
		}
		if !converting && !restoring && !sendReadStatus().Running {
			return 0
		}
	}
	return 1
}
