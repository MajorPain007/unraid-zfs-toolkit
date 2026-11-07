package zt

import (
	"sort"
	"strings"
)

type convUser struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Running bool   `json:"running"`
}

type convFolder struct {
	Source  string     `json:"source"`
	Name    string     `json:"name"`
	Path    string     `json:"path"`
	Dataset string     `json:"dataset"`
	As      string     `json:"as"`
	State   string     `json:"state"`
	Reason  string     `json:"reason"`
	Why     string     `json:"why"`
	Size    int64      `json:"size"`
	Bytes   int64      `json:"bytes"`
	Users   []convUser `json:"users"`
}

type convSourceScan struct {
	Dataset    string       `json:"dataset"`
	Kind       string       `json:"kind"`
	Mountpoint string       `json:"mountpoint"`
	Avail      int64        `json:"avail"`
	Error      string       `json:"error"`
	Folders    []convFolder `json:"folders"`
}

type convPlanInput struct {
	World         *convWorld
	Sources       []convSource
	ReplaceSpaces bool
	Buffer        int
	ReadDir       func(string) ([]convEntry, error)
	Size          func(path string) (alloc, apparent int64, known bool)
}

func convRequired(size int64, buffer int) int64 {
	return size + size*int64(buffer)/100
}

func convSourceProblem(w *convWorld, ds string) (string, string) {
	d, ok := w.datasets[ds]
	if !ok {
		return "", "'" + ds + "' is not a ZFS dataset. Sources must be datasets to host child datasets."
	}
	if !d.Mounted || !strings.HasPrefix(d.Mountpoint, "/") {
		return "", "'" + ds + "' is not mounted, so its folders cannot be reached."
	}
	return d.Mountpoint, ""
}

func convClassify(in convPlanInput) []convSourceScan {
	out := []convSourceScan{}
	claimed := map[string]string{}
	for _, src := range in.Sources {
		sc := convSourceScan{Dataset: src.Dataset, Kind: src.Kind, Folders: []convFolder{}}
		mp, prob := convSourceProblem(in.World, src.Dataset)
		if prob != "" {
			sc.Error = prob
			out = append(out, sc)
			continue
		}
		sc.Mountpoint = mp
		sc.Avail = in.World.datasets[src.Dataset].Avail
		entries, err := in.ReadDir(mp)
		if err != nil {
			sc.Error = "Cannot read " + mp + ": " + err.Error()
			out = append(out, sc)
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return natLess(entries[i].Name, entries[j].Name) })
		for _, e := range entries {
			if !e.IsDir || e.Link || strings.HasPrefix(e.Name, ".") {
				continue
			}
			path := strings.TrimRight(mp, "/") + "/" + e.Name
			f := convFolder{Source: src.Dataset, Name: e.Name, Path: path, Size: -1, Bytes: -1, Users: []convUser{}}
			if ds := in.World.datasetAt(path); ds != "" {
				f.State = "dataset"
				f.Dataset = ds
				f.Size = in.World.datasets[ds].Used
				sc.Folders = append(sc.Folders, f)
				continue
			}
			convJudge(in, &f, mp, sc.Avail, claimed)
			sc.Folders = append(sc.Folders, f)
		}
		out = append(out, sc)
	}
	return out
}

func convJudge(in convPlanInput, f *convFolder, mp string, avail int64, claimed map[string]string) {
	block := func(state, reason, why string) {
		f.State, f.Reason, f.Why = state, reason, why
	}
	if hasControl(f.Name) {
		block("blocked", "bad_name", "The name contains a line break or another control character. Rename the folder first.")
		return
	}
	if strings.HasSuffix(f.Name, "_temp") {
		twin := strings.TrimSuffix(f.Path, "_temp")
		if in.World.datasetAt(twin) != "" {
			block("kept", "kept_copy", "Next to the dataset at "+twin+" this is a copy kept with Cleanup off, or an original an older version left behind. It is left alone - delete it once you are sure it is not needed.")
			return
		}
	}
	name := convDatasetName(f.Name, in.ReplaceSpaces)
	f.Dataset = f.Source + "/" + name
	if name != f.Name {
		f.As = name
	}
	if in.Size != nil {
		alloc, apparent, known := in.Size(f.Path)
		if known {
			f.Size, f.Bytes = alloc, apparent
		}
	}
	if in.World.exists(f.Dataset) {
		block("kept", "unmounted_dataset", "The dataset "+f.Dataset+" exists but is not mounted at "+f.Path+".")
		return
	}
	if other, ok := claimed[f.Dataset]; ok {
		block("blocked", "collision", "'"+other+"' gets the same dataset name "+f.Dataset+". Rename one of the two folders.")
		return
	}
	if exists(f.Path + "_temp") {
		block("blocked", "temp_exists", f.Path+"_temp already exists. Move or delete it before this folder can be converted.")
		return
	}
	if m := in.World.mountsInside(f.Path); len(m) > 0 {
		why := "Something is mounted at " + m[0] + ". Copying the folder would copy that file system into the dataset."
		if m[0] == f.Path {
			why = "The folder is itself a mount point of another file system."
		}
		block("blocked", "mounts", why)
		return
	}
	if f.Size >= 0 {
		need := convRequired(f.Size, in.Buffer)
		if avail < need {
			block("blocked", "space", "Not enough space on "+f.Source+": "+humanBytes(float64(avail))+" free, "+humanBytes(float64(need))+" needed (folder + buffer).")
			return
		}
	}
	claimed[f.Dataset] = f.Name
	f.State = "convert"
}

func convOwnerFolder(path, mp string) string {
	mp = strings.TrimRight(mp, "/")
	if !strings.HasPrefix(path, mp+"/") {
		return ""
	}
	rest := strings.TrimLeft(path[len(mp)+1:], "/")
	if rest == "" {
		return ""
	}
	child, _, _ := strings.Cut(rest, "/")
	if child == "" || child == "." || child == ".." {
		return ""
	}
	return mp + "/" + child
}

func convCount(scans []convSourceScan, state string) int {
	n := 0
	for _, s := range scans {
		for _, f := range s.Folders {
			if f.State == state {
				n++
			}
		}
	}
	return n
}
