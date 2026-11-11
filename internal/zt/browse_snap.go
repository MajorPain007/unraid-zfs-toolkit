package zt

import (
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	reBrSnap = regexp.MustCompile(`^[A-Za-z0-9_.:-][A-Za-z0-9_.: -]*$`)
	reBrAuto = regexp.MustCompile(`^auto-(frequent|hourly|daily|weekly|monthly|yearly)-`)
)

const brMaxEntries = 20000

func brValidSnap(name string) bool {
	return len(name) <= 255 && name != "." && name != ".." && reBrSnap.MatchString(name)
}

func brRel(p string) (string, error) {
	if hasControl(p) {
		return "", apiFail("The path contains a control character.")
	}
	parts := []string{}
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "", ".":
			continue
		case "..":
			return "", apiFail("Paths with .. are not allowed.")
		}
		parts = append(parts, seg)
	}
	return "/" + strings.Join(parts, "/"), nil
}

func brJoin(base, rel string) string {
	if rel == "/" || rel == "" {
		return strings.TrimRight(base, "/")
	}
	return strings.TrimRight(base, "/") + rel
}

func brParent(rel string) string {
	i := strings.LastIndex(rel, "/")
	if i <= 0 {
		return "/"
	}
	return rel[:i]
}

func brBase(rel string) string {
	return rel[strings.LastIndex(rel, "/")+1:]
}

func brWalk(base, rel string, allowLastLink bool) (string, os.FileInfo, error) {
	cur := strings.TrimRight(base, "/")
	info, err := os.Lstat(brJoin(base, "/"))
	if err != nil {
		return "", nil, err
	}
	if rel == "/" {
		return cur, info, nil
	}
	segs := strings.Split(strings.TrimPrefix(rel, "/"), "/")
	for i, seg := range segs {
		cur += "/" + seg
		info, err = os.Lstat(cur)
		if err != nil {
			return "", nil, err
		}
		last := i == len(segs)-1
		if info.Mode()&os.ModeSymlink != 0 && !(last && allowLastLink) {
			return "", nil, apiFail(strings.Join(segs[:i+1], "/") + " is a symbolic link. Links are not followed, so nothing can leave the snapshot.")
		}
		if !last && !info.IsDir() {
			return "", nil, apiFail(strings.Join(segs[:i+1], "/") + " is not a folder.")
		}
	}
	return cur, info, nil
}

func brLiveLink(mount, rel string) string {
	cur := strings.TrimRight(mount, "/")
	if rel == "/" {
		return ""
	}
	for _, seg := range strings.Split(strings.TrimPrefix(rel, "/"), "/") {
		cur += "/" + seg
		info, err := os.Lstat(cur)
		if err != nil {
			return ""
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return cur
		}
		if !info.IsDir() {
			return ""
		}
	}
	return ""
}

type brSnap struct {
	Dataset string
	Snap    string
	Mount   string
	Base    string
}

func brDatasetMount(ds string) (string, error) {
	r := zfs("get", "-Hp", "-o", "property,value", "type,mounted,mountpoint", ds)
	if !r.OK() {
		return "", apiFailf("Dataset %s does not exist.", ds)
	}
	props := map[string]string{}
	for _, l := range r.Lines() {
		if k, v, ok := strings.Cut(l, "\t"); ok {
			props[k] = v
		}
	}
	if props["type"] != "filesystem" {
		return "", apiFailf("%s is not a file system - only file systems have files to browse.", ds)
	}
	mp := props["mountpoint"]
	if !strings.HasPrefix(mp, "/") {
		return "", apiFailf("%s has mountpoint=%s, so its snapshots cannot be reached through the file system.", ds, mp)
	}
	if props["mounted"] != "yes" {
		return "", apiFailf("%s is not mounted at %s. Its snapshots are reachable only while it is mounted.", ds, mp)
	}
	return mp, nil
}

func brOpen(p M) (*brSnap, error) {
	ds := strings.TrimSpace(pstr(p, "dataset", ""))
	snap := strings.TrimSpace(pstr(p, "snapshot", ""))
	if !validDataset(ds) {
		return nil, apiFail("That is not a valid dataset name.")
	}
	if !brValidSnap(snap) {
		return nil, apiFail("That is not a valid snapshot name.")
	}
	mp, err := brDatasetMount(ds)
	if err != nil {
		return nil, err
	}
	if r := zfs("list", "-H", "-o", "name", "-t", "snapshot", ds+"@"+snap); !r.OK() {
		return nil, apiFailf("Snapshot %s@%s no longer exists. It was probably destroyed since the list was loaded - reload the list.", ds, snap)
	}
	b := &brSnap{Dataset: ds, Snap: snap, Mount: mp, Base: strings.TrimRight(mp, "/") + "/.zfs/snapshot/" + snap}
	if err := b.reachable(); err != nil {
		return nil, err
	}
	return b, nil
}

func brReadable(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return nil
	}
	return err
}

func (b *brSnap) reachable() error {
	err := brReadable(b.Base)
	if err == nil {
		return nil
	}
	runCmd(30*time.Second, "ls", "-A", "--", b.Base)
	if err = brReadable(b.Base); err == nil {
		return nil
	}
	who := "uid " + strconv.Itoa(os.Geteuid())
	if os.Geteuid() == 0 {
		who += " (root)"
	} else {
		who += " (not root)"
	}
	return apiFailf("Cannot read %s - %s. Running as %s. Snapshots are read-only, so this is usually the snapshot failing to mount rather than a permission problem.", b.Base, brErrText(err), who)
}

func brErrText(err error) string {
	if pe, ok := err.(*os.PathError); ok {
		return pe.Err.Error()
	}
	return err.Error()
}

func brKind(snap string) string {
	if m := reBrAuto.FindStringSubmatch(snap); m != nil {
		return m[1]
	}
	if strings.HasPrefix(snap, "zdc-send-") {
		return "replication"
	}
	return "manual"
}

type brEntry struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Type   string `json:"type"`
	Size   int64  `json:"size"`
	Mtime  int64  `json:"mtime"`
	Mode   string `json:"mode"`
	Target string `json:"target,omitempty"`
	Live   string `json:"live,omitempty"`
	Child  string `json:"child,omitempty"`
	Same   bool   `json:"child_snap,omitempty"`
}

func brType(m os.FileMode) string {
	switch {
	case m.IsDir():
		return "dir"
	case m&os.ModeSymlink != 0:
		return "symlink"
	case m.IsRegular():
		return "file"
	}
	return "other"
}

func apiBrowseList(p M) (M, error) {
	b, err := brOpen(p)
	if err != nil {
		return nil, err
	}
	rel, err := brRel(pstr(p, "path", "/"))
	if err != nil {
		return nil, err
	}
	full, info, err := brWalk(b.Base, rel, false)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apiFailf("%s does not exist in this snapshot.", rel)
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, apiFailf("%s is not a folder.", rel)
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, apiFailf("Cannot read %s - %s.", full, brErrText(err))
	}
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		return nil, apiFailf("Cannot read %s - %s.", full, brErrText(err))
	}
	sort.Slice(names, func(i, j int) bool { return natLess(names[i], names[j]) })
	truncated := len(names) > brMaxEntries
	if truncated {
		names = names[:brMaxEntries]
	}
	liveDir := brJoin(b.Mount, rel)
	liveOK := brLiveLink(b.Mount, rel) == "" && isDir(liveDir)
	if li, err := os.Lstat(liveDir); err != nil || li.Mode()&os.ModeSymlink != 0 {
		liveOK = false
	}
	children := brChildren(b, rel)
	entries := make([]brEntry, 0, len(names))
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
		path := full + "/" + n
		fi, err := os.Lstat(path)
		if err != nil {
			continue
		}
		e := brEntry{
			Name:  n,
			Path:  brJoin(rel, "/"+n),
			Type:  brType(fi.Mode()),
			Mtime: fi.ModTime().Unix(),
			Mode:  fi.Mode().String(),
		}
		if rel == "/" {
			e.Path = "/" + n
		}
		if e.Type == "file" {
			e.Size = fi.Size()
		}
		if e.Type == "symlink" {
			e.Target, _ = os.Readlink(path)
		}
		if c, ok := children[e.Path]; ok && e.Type == "dir" {
			e.Child, e.Same = c.name, c.snap
		} else if liveOK {
			e.Live = brCompareLive(fi, e.Target, liveDir+"/"+n)
		}
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Type == "dir" && entries[j].Type != "dir"
	})
	added := 0
	if liveOK {
		if lf, err := os.Open(liveDir); err == nil {
			ln, _ := lf.Readdirnames(-1)
			lf.Close()
			for _, n := range ln {
				if !seen[n] && !(rel == "/" && n == ".zfs") {
					added++
				}
			}
		}
	}
	return M{
		"dataset":    b.Dataset,
		"snapshot":   b.Snap,
		"mountpoint": b.Mount,
		"path":       rel,
		"entries":    entries,
		"truncated":  truncated,
		"live":       liveOK,
		"live_added": added,
	}, nil
}

type brChild struct {
	name string
	snap bool
}

func brChildren(b *brSnap, rel string) map[string]brChild {
	out := map[string]brChild{}
	r := zfs("list", "-H", "-o", "name,mountpoint", "-t", "filesystem", "-r", b.Dataset)
	if !r.OK() {
		return out
	}
	mount := strings.TrimRight(b.Mount, "/")
	for _, l := range r.Lines() {
		name, mp, ok := strings.Cut(l, "\t")
		if !ok || name == b.Dataset || !strings.HasPrefix(mp, mount+"/") {
			continue
		}
		p := mp[len(mount):]
		if brParent(p) == rel {
			out[p] = brChild{name: name}
		}
	}
	if len(out) == 0 {
		return out
	}
	if s := zfs("list", "-H", "-o", "name", "-t", "snapshot", "-r", b.Dataset); s.OK() {
		has := map[string]bool{}
		for _, l := range s.Lines() {
			if ds, sn, ok := strings.Cut(l, "@"); ok && sn == b.Snap {
				has[ds] = true
			}
		}
		for p, c := range out {
			c.snap = has[c.name]
			out[p] = c
		}
	}
	return out
}

func brCompareLive(fi os.FileInfo, target, live string) string {
	li, err := os.Lstat(live)
	if err != nil {
		return "gone"
	}
	st, lt := brType(fi.Mode()), brType(li.Mode())
	if st != lt {
		return "changed"
	}
	switch st {
	case "file":
		if fi.Size() != li.Size() || fi.ModTime().Unix() != li.ModTime().Unix() {
			return "changed"
		}
	case "symlink":
		if t, _ := os.Readlink(live); t != target {
			return "changed"
		}
	}
	return ""
}

type brDataset struct {
	Name       string `json:"name"`
	Mountpoint string `json:"mountpoint"`
	Mounted    bool   `json:"mounted"`
	Used       int64  `json:"used"`
	Snapshots  int    `json:"snapshots"`
	Latest     int64  `json:"latest"`
}

func apiBrowseDatasets(p M) (M, error) {
	r := zfs("list", "-Hp", "-t", "filesystem", "-o", "name,mountpoint,mounted,used")
	if !r.OK() {
		return nil, apiFail("zfs list failed: " + r.Message())
	}
	counts := map[string]int{}
	latest := map[string]int64{}
	if s := zfs("list", "-Hp", "-t", "snapshot", "-o", "name,creation"); s.OK() {
		for _, l := range s.Lines() {
			name, c, _ := strings.Cut(l, "\t")
			ds, _, ok := strings.Cut(name, "@")
			if !ok {
				continue
			}
			counts[ds]++
			if t := toInt64(c); t > latest[ds] {
				latest[ds] = t
			}
		}
	}
	out := []brDataset{}
	for _, l := range r.Lines() {
		f := strings.Split(l, "\t")
		if len(f) < 4 {
			continue
		}
		out = append(out, brDataset{
			Name:       f[0],
			Mountpoint: f[1],
			Mounted:    f[2] == "yes",
			Used:       toInt64(f[3]),
			Snapshots:  counts[f[0]],
			Latest:     latest[f[0]],
		})
	}
	sort.Slice(out, func(i, j int) bool { return natLess(out[i].Name, out[j].Name) })
	return M{"datasets": out}, nil
}

type brSnapInfo struct {
	Name     string `json:"name"`
	Creation int64  `json:"creation"`
	Used     int64  `json:"used"`
	Refer    int64  `json:"refer"`
	Holds    int    `json:"holds"`
	Defer    bool   `json:"defer"`
	Kind     string `json:"kind"`
}

func brListSnaps(ds string) ([]brSnapInfo, error) {
	r := zfs("list", "-Hp", "-t", "snapshot", "-d", "1", "-o", "name,creation,used,referenced,userrefs,defer_destroy", "-s", "creation", ds)
	if !r.OK() {
		return nil, apiFailf("Cannot list the snapshots of %s: %s", ds, r.Message())
	}
	out := []brSnapInfo{}
	for _, l := range r.Lines() {
		f := strings.Split(l, "\t")
		if len(f) < 6 {
			continue
		}
		_, snap, ok := strings.Cut(f[0], "@")
		if !ok {
			continue
		}
		out = append(out, brSnapInfo{
			Name:     snap,
			Creation: toInt64(f[1]),
			Used:     toInt64(f[2]),
			Refer:    toInt64(f[3]),
			Holds:    toInt(f[4]),
			Defer:    f[5] == "on",
			Kind:     brKind(snap),
		})
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func apiBrowseSnapshots(p M) (M, error) {
	ds := strings.TrimSpace(pstr(p, "dataset", ""))
	if !validDataset(ds) {
		return nil, apiFail("That is not a valid dataset name.")
	}
	snaps, err := brListSnaps(ds)
	if err != nil {
		if r := zfs("list", "-H", "-o", "name", ds); !r.OK() {
			return nil, apiFailf("Dataset %s does not exist.", ds)
		}
		return nil, err
	}
	res := M{"dataset": ds, "snapshots": snaps, "mountpoint": "", "browsable": false, "problem": ""}
	if mp, err := brDatasetMount(ds); err == nil {
		res["mountpoint"] = mp
		res["browsable"] = true
	} else {
		res["problem"] = err.Error()
	}
	return res, nil
}

func init() {
	registerAction("browse_datasets", apiBrowseDatasets)
	registerAction("browse_snapshots", apiBrowseSnapshots)
	registerAction("browse_list", apiBrowseList)
}
