package zt

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var reBadName = regexp.MustCompile(`[\x00-\x1f\x7f]`)

func natLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ca, cb := a[i], b[j]
		if isDigitByte(ca) && isDigitByte(cb) {
			si := i
			for i < len(a) && isDigitByte(a[i]) {
				i++
			}
			sj := j
			for j < len(b) && isDigitByte(b[j]) {
				j++
			}
			na := strings.TrimLeft(a[si:i], "0")
			nb := strings.TrimLeft(b[sj:j], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ca != cb {
			return ca < cb
		}
		i++
		j++
	}
	return len(a)-i < len(b)-j
}

func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }

type browseItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
}

func apiBrowse(p M) (M, error) {
	path := absPath(param(p, "path", "/mnt"))
	root := M{"path": "/", "entries": []browseItem{
		{Name: "mnt", Path: "/mnt", Dir: true},
		{Name: "boot", Path: "/boot", Dir: true},
	}}
	if path == "" || path == "/" {
		return root, nil
	}
	if !(under(path, "/mnt") || under(path, "/boot")) {
		return nil, apiFail("Only folders under /mnt and /boot can be chosen.")
	}
	for !isDir(path) && path != "/mnt" && path != "/boot" {
		path = filepath.Dir(path)
	}
	if !isDir(path) {
		return root, nil
	}
	files := toBool(param(p, "files", false))
	f, err := os.Open(path)
	if err != nil {
		return nil, apiFail("Cannot open " + path)
	}
	names, _ := f.Readdirnames(-1)
	f.Close()
	entries := []browseItem{}
	for _, e := range names {
		if len(entries) >= 5000 {
			break
		}
		if e == ".zfs" {
			continue
		}
		full := path + "/" + e
		dir := isDir(full)
		if dir || files {
			entries = append(entries, browseItem{Name: e, Path: full, Dir: dir})
		}
	}
	sort.SliceStable(entries, func(a, b int) bool {
		if entries[a].Dir != entries[b].Dir {
			return entries[a].Dir
		}
		return natLess(entries[a].Name, entries[b].Name)
	})
	return M{"path": path, "entries": entries}, nil
}

func apiMkdir(p M) (M, error) {
	parent := absPath(param(p, "path", ""))
	name := strings.Trim(pstr(p, "name", ""), " \t\n\r\x00\x0b")
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") || len(name) > 255 || reBadName.MatchString(name) {
		return nil, apiFail("A folder name cannot contain a slash, and cannot be . or ..")
	}
	if !under(parent, "/mnt") || parent == "/mnt" || !isDir(parent) {
		return nil, apiFail("New folders can be made in a share, pool or disk under /mnt.")
	}
	path := strings.TrimRight(parent, "/") + "/" + name
	if exists(path) {
		return nil, apiFail(path + " exists already.")
	}
	if !mkdirLikeParent(path) {
		return nil, apiFail("Could not make " + path + ".")
	}
	return M{"path": path}, nil
}

func init() {
	registerAction("browse", apiBrowse)
	registerAction("mkdir", apiMkdir)
}
