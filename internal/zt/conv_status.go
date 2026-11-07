package zt

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type convItem struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Source  string `json:"source"`
	Dataset string `json:"dataset"`
	As      string `json:"as"`
	State   string `json:"state"`
	Message string `json:"message"`
	Size    int64  `json:"size"`
	Bytes   int64  `json:"bytes"`
	Done    int64  `json:"done"`
	Phase   string `json:"phase"`
	Rate    int64  `json:"rate"`
}

type convResult struct {
	Converted   []string `json:"converted"`
	Failed      []string `json:"failed"`
	Skipped     []string `json:"skipped"`
	Interrupted bool     `json:"interrupted"`
	Message     string   `json:"message"`
	Errors      int      `json:"errors"`
	Warnings    int      `json:"warnings"`
}

type convStatusFile struct {
	PID           int         `json:"pid"`
	LogFile       string      `json:"log_file"`
	Started       string      `json:"started"`
	StartedTS     int64       `json:"started_ts"`
	Status        string      `json:"status"`
	Source        string      `json:"source"`
	Engine        string      `json:"engine"`
	DryRun        bool        `json:"dry_run"`
	Phase         string      `json:"phase"`
	CurrentFolder string      `json:"current_folder"`
	Items         []convItem  `json:"items"`
	Stopped       []convUser  `json:"stopped"`
	Finished      int64       `json:"finished"`
	Result        *convResult `json:"result"`
}

type convStatusWriter struct {
	mu   sync.Mutex
	st   convStatusFile
	last time.Time
}

func (w *convStatusWriter) update(force bool, fn func(st *convStatusFile)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if fn != nil {
		fn(&w.st)
	}
	if !force && time.Since(w.last) < time.Second {
		return
	}
	w.last = time.Now()
	jsonWrite(convStatusPath, w.st, 0o644)
}

func convReadStatus() (convStatusFile, bool) {
	var st convStatusFile
	if !jsonReadInto(convStatusPath, &st) {
		m, ok := jsonRead(convStatusPath)
		if !ok {
			return st, false
		}
		st = convStatusFile{PID: toInt(m["pid"]), LogFile: str(m["log_file"]), Started: str(m["started"]), Status: str(m["status"]), Source: str(m["source"])}
	}
	return st, true
}

func convProcAlive(st convStatusFile) bool {
	if st.PID <= 0 || !pidAlive(st.PID) {
		return false
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(st.PID) + "/cmdline")
	if err != nil {
		return true
	}
	cmd := strings.ReplaceAll(string(b), "\x00", " ")
	if st.Engine == "go" {
		return strings.Contains(cmd, "convert")
	}
	return strings.Contains(cmd, "zfs_converter") || strings.Contains(cmd, "convert")
}

func convCurrentStatus() (convStatusFile, bool) {
	st, ok := convReadStatus()
	if !ok {
		return st, false
	}
	if st.Status == "running" && !convProcAlive(st) {
		if st.LogFile != "" && isFile(st.LogFile) {
			st.Status = convClassifyLog(st.LogFile)
		} else {
			st.Status = "stopped"
		}
		if st.Result == nil {
			st.Result = &convResult{Interrupted: st.Status == "stopped", Message: "The conversion process ended without finishing its report."}
		}
	}
	return st, true
}

func convRunning() (convStatusFile, bool) {
	st, ok := convReadStatus()
	if ok && st.Status == "running" && convProcAlive(st) {
		return st, true
	}
	return st, false
}

type convHistoryEntry struct {
	Started     int64    `json:"started"`
	Finished    int64    `json:"finished"`
	Source      string   `json:"source"`
	Status      string   `json:"status"`
	DryRun      bool     `json:"dry_run"`
	Converted   []string `json:"converted"`
	Failed      []string `json:"failed"`
	Interrupted bool     `json:"interrupted"`
	LogFile     string   `json:"log_file"`
}

func convReadHistory() []convHistoryEntry {
	var h struct {
		Runs []convHistoryEntry `json:"runs"`
	}
	if !jsonReadInto(convHistoryPath, &h) || h.Runs == nil {
		return []convHistoryEntry{}
	}
	return h.Runs
}

func convAppendHistory(e convHistoryEntry) {
	runs := append([]convHistoryEntry{e}, convReadHistory()...)
	if len(runs) > 40 {
		runs = runs[:40]
	}
	jsonWrite(convHistoryPath, M{"runs": runs}, 0o644)
}

func convJournalPending() (convJournal, bool) {
	j, ok := convReadJournal()
	return j, ok
}
