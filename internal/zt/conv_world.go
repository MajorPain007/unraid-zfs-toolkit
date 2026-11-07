package zt

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type convDataset struct {
	Name       string
	Mounted    bool
	Mountpoint string
	Avail      int64
	Used       int64
}

type convWorld struct {
	datasets  map[string]convDataset
	mountedAt map[string]string
	mounts    []string
	ok        bool
	err       string
}

func convLoadWorld() *convWorld {
	w := &convWorld{datasets: map[string]convDataset{}, mountedAt: map[string]string{}}
	r := zfs("list", "-H", "-p", "-t", "filesystem", "-o", "name,mounted,mountpoint,avail,used")
	if !r.OK() {
		w.err = r.Message()
		return w
	}
	w.ok = true
	for _, l := range r.Lines() {
		f := strings.Split(l, "\t")
		if len(f) < 5 {
			continue
		}
		d := convDataset{Name: f[0], Mounted: f[1] == "yes", Mountpoint: f[2], Avail: toInt64(f[3]), Used: toInt64(f[4])}
		w.datasets[d.Name] = d
		if d.Mounted && strings.HasPrefix(d.Mountpoint, "/") {
			w.mountedAt[d.Mountpoint] = d.Name
		}
	}
	for _, m := range mountInfo("self") {
		w.mounts = append(w.mounts, m.Point)
	}
	return w
}

func (w *convWorld) exists(ds string) bool {
	_, ok := w.datasets[ds]
	return ok
}

func (w *convWorld) datasetAt(path string) string {
	return w.mountedAt[path]
}

func (w *convWorld) mountsInside(path string) []string {
	out := []string{}
	for _, m := range w.mounts {
		if m == path || strings.HasPrefix(m, path+"/") {
			out = append(out, m)
		}
	}
	return out
}

type convEntry struct {
	Name  string
	IsDir bool
	Link  bool
}

func convReadDir(dir string) ([]convEntry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []convEntry{}
	for _, de := range des {
		e := convEntry{Name: de.Name()}
		if de.Type()&fs.ModeSymlink != 0 {
			e.Link = true
		} else {
			e.IsDir = de.IsDir()
		}
		out = append(out, e)
	}
	return out, nil
}

type convUsage struct {
	Allocated int64
	Apparent  int64
	Entries   int64
	Done      bool
}

func convDirUsage(ctx context.Context, root string) (convUsage, error) {
	u := convUsage{}
	seen := map[[2]uint64]bool{}
	n := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if n++; n&1023 == 0 && ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if p == root {
				return err
			}
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		if p != root {
			u.Entries++
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if ok && !info.IsDir() && st.Nlink > 1 {
			key := [2]uint64{uint64(st.Dev), uint64(st.Ino)}
			if seen[key] {
				return nil
			}
			seen[key] = true
		}
		if ok {
			u.Allocated += int64(st.Blocks) * 512
		}
		if !info.IsDir() {
			u.Apparent += info.Size()
		}
		return nil
	})
	if err != nil {
		return u, err
	}
	u.Done = true
	return u, nil
}
