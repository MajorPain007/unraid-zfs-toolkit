package zt

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var reDiffEscape = regexp.MustCompile(`\\[0-7]{4}`)

func diffUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	return reDiffEscape.ReplaceAllStringFunc(s, func(m string) string {
		v, _ := strconv.ParseUint(m[1:], 8, 32)
		return string([]byte{byte(v & 0xff)})
	})
}

type diffEntry struct {
	Change  string `json:"change"`
	Kind    string `json:"kind"`
	Type    string `json:"type"`
	Path    string `json:"path"`
	NewPath string `json:"new_path,omitempty"`
}

var diffTypes = map[string]string{"F": "file", "/": "dir", "@": "symlink", "|": "fifo", "B": "block", "C": "char", "=": "socket", "P": "door"}
var diffKinds = map[string]string{"-": "removed", "+": "added", "M": "modified", "R": "renamed"}

func diffRel(p, mount string) string {
	mount = strings.TrimRight(mount, "/")
	if p == mount {
		return "/"
	}
	if mount != "" && strings.HasPrefix(p, mount+"/") {
		return p[len(mount):]
	}
	return p
}

func diffParse(lines []string, mount string) []diffEntry {
	out := []diffEntry{}
	for _, l := range lines {
		f := strings.Split(l, "\t")
		if len(f) < 3 {
			continue
		}
		e := diffEntry{Change: f[0], Kind: diffKinds[f[0]], Type: diffTypes[f[1]], Path: diffRel(diffUnescape(f[2]), mount)}
		if e.Kind == "" {
			e.Kind = f[0]
		}
		if e.Type == "" {
			e.Type = f[1]
		}
		if len(f) > 3 {
			e.NewPath = diffRel(diffUnescape(f[3]), mount)
		}
		out = append(out, e)
	}
	return out
}

func diffUnder(path, prefix string) bool {
	return prefix == "/" || prefix == "" || under(path, prefix)
}

func diffCreation(full string) int64 {
	r := zfs("list", "-Hp", "-t", "snapshot", "-o", "creation", full)
	if !r.OK() {
		return -1
	}
	return toInt64(strings.TrimSpace(r.Stdout))
}

func apiDiffSnapshots(p M) (M, error) {
	ds := strings.TrimSpace(pstr(p, "dataset", ""))
	from := strings.TrimSpace(pstr(p, "from", ""))
	to := strings.TrimSpace(pstr(p, "to", ""))
	if !brValidDataset(ds) {
		return nil, apiFail("That is not a valid dataset name.")
	}
	if !brValidSnap(from) || (to != "" && !brValidSnap(to)) {
		return nil, apiFail("That is not a valid snapshot name.")
	}
	if from == to {
		return nil, apiFail("Pick two different snapshots.")
	}
	prefix, err := brRel(pstr(p, "path", "/"))
	if err != nil {
		return nil, err
	}
	limit := toInt(param(p, "limit", 5000))
	if limit < 1 || limit > 50000 {
		limit = 5000
	}
	mount, err := brDatasetMount(ds)
	if err != nil {
		return nil, err
	}
	fromFull := ds + "@" + from
	toArg := ds
	if to != "" {
		toArg = ds + "@" + to
	}
	cFrom := diffCreation(fromFull)
	if cFrom < 0 {
		return nil, apiFailf("Snapshot %s does not exist.", fromFull)
	}
	reversed := false
	if to != "" {
		cTo := diffCreation(toArg)
		if cTo < 0 {
			return nil, apiFailf("Snapshot %s does not exist.", toArg)
		}
		if cFrom > cTo {
			fromFull, toArg = toArg, fromFull
			reversed = true
		}
	}
	r := runCmd(5*time.Minute, ZfsBin, "diff", "-FH", fromFull, toArg)
	if !r.OK() {
		msg := r.Message()
		low := strings.ToLower(msg)
		switch {
		case strings.Contains(low, "permission") || strings.Contains(low, "not allowed"):
			msg += " (zfs diff needs root and a mounted dataset)"
		case strings.Contains(low, "earlier snapshot"):
			msg += " (both snapshots must belong to this dataset; a snapshot from before a rollback cannot be compared)"
		case r.Code == -1:
			msg = "zfs diff took longer than five minutes and was stopped."
		}
		return nil, apiFail(msg)
	}
	all := diffParse(r.Lines(), mount)
	counts := M{"added": 0, "removed": 0, "modified": 0, "renamed": 0}
	entries := []diffEntry{}
	for _, e := range all {
		if e.Path == "/" && e.Kind == "modified" {
			continue
		}
		if !diffUnder(e.Path, prefix) && !(e.NewPath != "" && diffUnder(e.NewPath, prefix)) {
			continue
		}
		if n, ok := counts[e.Kind].(int); ok {
			counts[e.Kind] = n + 1
		}
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool { return natLess(entries[i].Path, entries[j].Path) })
	total := len(entries)
	truncated := total > limit
	if truncated {
		entries = entries[:limit]
	}
	return M{
		"dataset":    ds,
		"mountpoint": mount,
		"from":       fromFull,
		"to":         toArg,
		"live":       to == "",
		"reversed":   reversed,
		"path":       prefix,
		"entries":    entries,
		"counts":     counts,
		"total":      total,
		"truncated":  truncated,
	}, nil
}

func init() {
	registerAction("diff_snapshots", apiDiffSnapshots)
}
