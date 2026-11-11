package zt

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type sendJob struct {
	ID            string `json:"id"`
	Enabled       bool   `json:"enabled"`
	Name          string `json:"name"`
	Source        string `json:"source"`
	Dest          string `json:"dest"`
	Recursive     bool   `json:"recursive"`
	Transport     string `json:"transport"`
	SSHHost       string `json:"ssh_host"`
	SSHPort       int    `json:"ssh_port"`
	SSHKey        string `json:"ssh_key"`
	Raw           bool   `json:"raw"`
	Compressed    bool   `json:"compressed"`
	AllowRollback bool   `json:"allow_rollback"`
	KeepDest      int    `json:"keep_dest"`
	Readonly      bool   `json:"readonly"`
	BwLimit       int    `json:"bwlimit"`
}

var (
	reSendID       = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	reSendDataset  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]*(/[A-Za-z0-9_.: -]+)*$`)
	reSendHost     = regexp.MustCompile(`^[A-Za-z0-9._@:\[\]-]+$`)
	reSendKey      = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)
	reSendPlainArg = regexp.MustCompile(`^[A-Za-z0-9_./=:@%+,-]+$`)
)

func sendJobsFile() string   { return ConfigDir + "/send_jobs.json" }
func sendLogFile() string    { return TmpDir + "/send.log" }
func sendStatusFile() string { return TmpDir + "/send_status.json" }
func sendLockFile() string   { return TmpDir + "/send.lock" }
func sendRunLogDir() string  { return TmpDir + "/send-logs" }
func sendKeyFile() string    { return ConfigDir + "/id_send" }
func sendKnownHosts() string { return ConfigDir + "/send_known_hosts" }

func sendDatasetOK(name string) bool {
	return name != "" && len(name) < 256 && reSendDataset.MatchString(name) && !strings.Contains(name, "//")
}

func sendBool(v any, def bool) bool {
	if v == nil {
		return def
	}
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case int:
		return x != 0
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		if isNumeric(s) {
			return toInt(s) != 0
		}
		return inList(s, "1", "true", "yes", "on")
	}
	return def
}

func sendClean(v any) string {
	s, _ := scalarString(v)
	return strings.TrimSpace(strings.NewReplacer("\x1f", " ", "\t", " ", "\n", " ", "\r", " ").Replace(s))
}

func sendJobFromMap(m M, index int) sendJob {
	j := sendJob{
		ID:            sendClean(m["id"]),
		Enabled:       sendBool(m["enabled"], true),
		Name:          sendClean(m["name"]),
		Source:        strings.Trim(sendClean(m["source"]), "/"),
		Dest:          strings.Trim(sendClean(m["dest"]), "/"),
		Recursive:     sendBool(m["recursive"], false),
		Transport:     "local",
		SSHHost:       sendClean(m["ssh_host"]),
		SSHPort:       toInt(m["ssh_port"]),
		SSHKey:        sendClean(m["ssh_key"]),
		Raw:           sendBool(m["raw"], false),
		Compressed:    sendBool(m["compressed"], true),
		AllowRollback: sendBool(m["allow_rollback"], false),
		KeepDest:      toInt(m["keep_dest"]),
		Readonly:      sendBool(m["readonly"], false),
		BwLimit:       toInt(m["bwlimit"]),
	}
	if sendClean(m["transport"]) == "ssh" {
		j.Transport = "ssh"
	}
	if j.ID == "" || !reSendID.MatchString(j.ID) || len(j.ID) > 40 {
		j.ID = "job" + strconv.Itoa(index+1)
	}
	if j.Name == "" {
		j.Name = j.ID
	}
	if len([]rune(j.Name)) > 80 {
		j.Name = string([]rune(j.Name)[:80])
	}
	if j.SSHPort < 1 || j.SSHPort > 65535 {
		j.SSHPort = 22
	}
	if j.KeepDest < 0 {
		j.KeepDest = 0
	}
	if j.KeepDest > 10000 {
		j.KeepDest = 10000
	}
	if j.BwLimit < 0 {
		j.BwLimit = 0
	}
	return j
}

func (j sendJob) remote() bool { return j.Transport == "ssh" }

func (j sendJob) target() string {
	if j.remote() {
		return j.SSHHost + ":" + j.Dest
	}
	return j.Dest
}

func sendLoadJobs() ([]sendJob, error) {
	out := []sendJob{}
	b, err := os.ReadFile(sendJobsFile())
	if os.IsNotExist(err) {
		return out, nil
	}
	var m M
	if err == nil {
		err = json.Unmarshal(b, &m)
	}
	if err != nil {
		return nil, apiFailf("Could not read %s: %v. Nothing is saved over it until it can be read; fix or remove the file.", sendJobsFile(), err)
	}
	raw, _ := m["jobs"].([]any)
	seen := map[string]bool{}
	for i, x := range raw {
		jm, ok := x.(M)
		if !ok {
			continue
		}
		j := sendJobFromMap(jm, i)
		for seen[j.ID] {
			j.ID += "_"
		}
		seen[j.ID] = true
		out = append(out, j)
	}
	return out, nil
}

func sendFindJob(id string) (sendJob, bool) {
	jobs, _ := sendLoadJobs()
	for _, j := range jobs {
		if j.ID == id {
			return j, true
		}
	}
	return sendJob{}, false
}

func sendWriteJobs(jobs []sendJob) error {
	if jobs == nil {
		jobs = []sendJob{}
	}
	os.MkdirAll(ConfigDir, 0o755)
	if !jsonWrite(sendJobsFile(), M{"jobs": jobs}, 0o644) {
		return apiFailf("Cannot write %s", sendJobsFile())
	}
	return nil
}

func sendJobProblem(j sendJob) string {
	if j.Source == "" || j.Dest == "" {
		return "incomplete"
	}
	if !sendDatasetOK(j.Source) {
		return fmt.Sprintf("source \"%s\" is not a valid dataset name", j.Source)
	}
	if !sendDatasetOK(j.Dest) {
		return fmt.Sprintf("destination \"%s\" is not a valid dataset name", j.Dest)
	}
	if !strings.Contains(j.Dest, "/") {
		return fmt.Sprintf("destination \"%s\" is a whole pool - the copy needs a dataset of its own, e.g. %s/replica/%s", j.Dest, j.Dest, sendLastComponent(j.Source))
	}
	if j.remote() {
		if j.SSHHost == "" || !reSendHost.MatchString(j.SSHHost) || strings.HasPrefix(j.SSHHost, "-") {
			return "SSH host missing or invalid (expected user@host)"
		}
		if j.SSHKey != "" && (!reSendKey.MatchString(j.SSHKey) || strings.Contains(j.SSHKey, "..")) {
			return "SSH key path is invalid"
		}
		return ""
	}
	if j.Source == j.Dest {
		return "source and destination are identical"
	}
	if strings.HasPrefix(j.Dest+"/", j.Source+"/") {
		return "destination lies inside the source dataset"
	}
	if j.Recursive && strings.HasPrefix(j.Source+"/", j.Dest+"/") {
		return "source lies inside the destination dataset"
	}
	return ""
}

func sendLastComponent(ds string) string {
	if i := strings.LastIndex(ds, "/"); i >= 0 {
		return ds[i+1:]
	}
	return ds
}

func sendShareWarning(j sendJob, shareExists func(string) bool) string {
	if j.remote() || j.Source == "" || j.Dest == "" {
		return ""
	}
	parts := strings.Split(j.Dest, "/")
	if len(parts) != 2 {
		return ""
	}
	suggest := parts[0] + "/replica/" + parts[1]
	if shareExists(parts[1]) {
		return fmt.Sprintf("%s will be mounted at /mnt/%s, which makes it part of the share \"%s\". Put the copy one level down, e.g. %s.", j.Dest, j.Dest, parts[1], suggest)
	}
	return fmt.Sprintf("%s sits directly below the pool, so from the next boot on Unraid shows it as a share \"%s\". Put the copy one level down, e.g. %s.", j.Dest, parts[1], suggest)
}

func sendMirrorWarning(j sendJob) string {
	if j.AllowRollback && j.Recursive && j.KeepDest > 2 {
		return "Force with children makes the destination mirror the source: zfs recv -F removes snapshots there that are gone here, so keeping more than two checkpoints on the destination has no effect."
	}
	return ""
}

func sendUserShareExists(name string) bool {
	return isDir("/mnt/user/" + name)
}

func sendNormalizeList(raw []any) ([]sendJob, []string) {
	jobs := []sendJob{}
	warnings := []string{}
	seen := map[string]bool{}
	used := map[string]bool{}
	for _, x := range raw {
		if m, ok := x.(M); ok {
			if id := sendClean(m["id"]); reSendID.MatchString(id) {
				used[id] = true
			}
		}
	}
	for i, x := range raw {
		m, ok := x.(M)
		if !ok {
			continue
		}
		if id := sendClean(m["id"]); id == "" || !reSendID.MatchString(id) {
			m["id"] = sendFreeID(used)
			used[str(m["id"])] = true
		}
		j := sendJobFromMap(m, i)
		for seen[j.ID] {
			j.ID += "_"
		}
		seen[j.ID] = true
		label := j.Name
		if label == "" {
			label = "#" + strconv.Itoa(i+1)
		}
		if why := sendJobProblem(j); why != "" {
			j.Enabled = false
			if why != "incomplete" {
				warnings = append(warnings, "Job "+label+": "+why+" - disabled")
			}
		} else {
			if w := sendShareWarning(j, sendUserShareExists); w != "" {
				warnings = append(warnings, "Job "+label+": "+w)
			}
			if w := sendMirrorWarning(j); w != "" {
				warnings = append(warnings, "Job "+label+": "+w)
			}
		}
		jobs = append(jobs, j)
	}
	return jobs, warnings
}

func sendFreeID(used map[string]bool) string {
	for n := 1; ; n++ {
		id := "job" + strconv.Itoa(n)
		if !used[id] {
			return id
		}
	}
}

var reCheckpointStamp = regexp.MustCompile(`^\d{8}-\d{6}(-\d+)?$`)

func sendCheckpointPrefix(id string) string { return "zdc-send-" + id + "-" }

func sendIsCheckpointOf(snap, id string) bool {
	p := sendCheckpointPrefix(id)
	return strings.HasPrefix(snap, p) && reCheckpointStamp.MatchString(snap[len(p):])
}

func sendStampLess(a, b string) bool {
	as, an := sendStampSplit(a)
	bs, bn := sendStampSplit(b)
	if as != bs {
		return as < bs
	}
	return an < bn
}

func sendStampSplit(stamp string) (string, int) {
	if len(stamp) > 16 {
		return stamp[:15], toInt(stamp[16:])
	}
	return stamp, 0
}

func sendPruneSelection(rows []string, id string, keep int) []string {
	if keep < 1 {
		keep = 1
	}
	names := map[string]bool{}
	for _, r := range rows {
		_, snap, ok := strings.Cut(r, "@")
		if ok && sendIsCheckpointOf(snap, id) {
			names[snap] = true
		}
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	p := len(sendCheckpointPrefix(id))
	sort.Slice(sorted, func(a, b int) bool { return sendStampLess(sorted[a][p:], sorted[b][p:]) })
	if len(sorted) <= keep {
		return nil
	}
	old := map[string]bool{}
	for _, n := range sorted[:len(sorted)-keep] {
		old[n] = true
	}
	out := []string{}
	for _, r := range rows {
		_, snap, ok := strings.Cut(r, "@")
		if ok && old[snap] {
			out = append(out, r)
		}
	}
	return out
}

func sendGroupDestroy(rows []string) []string {
	order := []string{}
	by := map[string][]string{}
	for _, r := range rows {
		ds, snap, ok := strings.Cut(r, "@")
		if !ok || snap == "" {
			continue
		}
		if _, seen := by[ds]; !seen {
			order = append(order, ds)
		}
		by[ds] = append(by[ds], snap)
	}
	out := []string{}
	for _, ds := range order {
		out = append(out, ds+"@"+strings.Join(by[ds], ","))
	}
	return out
}

type sendSnap struct {
	Name     string `json:"name"`
	GUID     string `json:"guid"`
	Creation int64  `json:"creation"`
}

func sendFindBase(src, dst []sendSnap) (sendSnap, bool) {
	have := map[string]bool{}
	for _, s := range dst {
		if s.GUID != "" {
			have[s.GUID] = true
		}
	}
	for i := len(src) - 1; i >= 0; i-- {
		if have[src[i].GUID] {
			return src[i], true
		}
	}
	return sendSnap{}, false
}

func sendNewerOnDest(dst []sendSnap, base sendSnap) []string {
	out := []string{}
	found := false
	for _, s := range dst {
		if found {
			out = append(out, s.Name)
		} else if s.GUID == base.GUID {
			found = true
		}
	}
	return out
}

func sendRemoteCommand(args ...string) string {
	q := make([]string, len(args))
	for i, a := range args {
		if a != "" && reSendPlainArg.MatchString(a) {
			q[i] = a
		} else {
			q[i] = shellQuote(a)
		}
	}
	return strings.Join(q, " ")
}

func (j sendJob) sendArgs(checkpoint, base string) []string {
	a := []string{"send"}
	if j.Recursive {
		a = append(a, "-R")
	}
	if j.Raw {
		a = append(a, "-w")
	}
	if j.Compressed {
		a = append(a, "-L", "-e", "-c")
	}
	if base != "" {
		a = append(a, "-I", "@"+base)
	}
	return append(a, j.Source+"@"+checkpoint)
}

func (j sendJob) recvArgs() []string {
	a := []string{"recv", "-u", "-x", "mountpoint"}
	if j.AllowRollback {
		a = append(a, "-F")
	}
	if j.Readonly {
		a = append(a, "-o", "readonly=on")
	}
	return append(a, j.Dest)
}

func sendNextCheckpoint(id, stamp string, existing []string) string {
	prefix := sendCheckpointPrefix(id)
	newest := ""
	for _, n := range existing {
		if sendIsCheckpointOf(n, id) && (newest == "" || sendStampLess(newest, n[len(prefix):])) {
			newest = n[len(prefix):]
		}
	}
	cand := stamp
	for n := 2; newest != "" && !sendStampLess(newest, cand); n++ {
		cand = fmt.Sprintf("%s-%d", stamp, n)
	}
	return prefix + cand
}
