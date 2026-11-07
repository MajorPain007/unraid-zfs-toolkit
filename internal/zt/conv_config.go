package zt

import (
	"strings"
)

var (
	convJournalPath = envOr("ZDC_CONVERSION_JOURNAL", ConfigDir+"/conversion_in_progress")
	convHistoryPath = envOr("ZDC_CONVERSION_HISTORY", ConfigDir+"/conversion_history.json")
	convDockerBin   = envOr("ZDC_DOCKER", "docker")
	convVirshBin    = envOr("ZDC_VIRSH", "virsh")
	convRsyncBin    = envOr("ZDC_RSYNC", "rsync")
	convStatusPath  = TmpDir + "/status.json"
	convLockPath    = TmpDir + "/conversion.lock"
	convCronLog     = TmpDir + "/cron_auto.log"
)

type convSource struct {
	Dataset string `json:"dataset"`
	Kind    string `json:"kind"`
}

type convConfig struct {
	DryRun        bool
	Cleanup       bool
	ReplaceSpaces bool
	Containers    bool
	VMs           bool
	Notify        bool
	VMWait        int
	Buffer        int
	Tolerance     int
	Sources       []convSource
	Problems      []string
}

func convClamp(s Settings, key string, def, lo, hi int) int {
	v := strings.TrimSpace(s.Get(key, ""))
	if !isDigits(v) {
		return def
	}
	n := digitsInt(v)
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

func convConfigFrom(s Settings) convConfig {
	c := convConfig{
		DryRun:        s.Get("dry_run", "yes") != "no",
		Cleanup:       s.Yes("cleanup"),
		ReplaceSpaces: s.Yes("replace_spaces"),
		Containers:    s.Yes("should_process_containers"),
		VMs:           s.Yes("should_process_vms"),
		Notify:        s.Yes("send_notifications"),
		VMWait:        convClamp(s, "vm_forceshutdown_wait", 90, 10, 600),
		Buffer:        convClamp(s, "buffer_zone", 11, 0, 100),
		Tolerance:     convClamp(s, "validation_tolerance", 5, 0, 20),
	}
	seen := map[string]bool{}
	add := func(ds, kind, label string) {
		ds = strings.Trim(strings.TrimSpace(ds), "/")
		if ds == "" {
			return
		}
		if !convValidDataset(ds) {
			c.Problems = append(c.Problems, label+" '"+ds+"' is not a valid dataset name - skipped.")
			return
		}
		if seen[ds] {
			return
		}
		seen[ds] = true
		c.Sources = append(c.Sources, convSource{Dataset: ds, Kind: kind})
	}
	if c.Containers {
		add(strings.Trim(s.Get("appdata_pool", "cache"), "/")+"/"+strings.Trim(s.Get("appdata_dataset", "appdata"), "/"), "appdata", "Appdata dataset")
	}
	if c.VMs {
		add(strings.Trim(s.Get("vm_pool", "cache"), "/")+"/"+strings.Trim(s.Get("vm_dataset", "domains"), "/"), "domains", "VM dataset")
	}
	for _, e := range convSplitExtra(s.Raw("extra_datasets")) {
		add(e, "extra", "Extra dataset")
	}
	return c
}
