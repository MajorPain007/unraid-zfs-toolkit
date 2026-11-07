package zt

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type brDownload struct {
	snap *brSnap
	rel  string
	full string
	info os.FileInfo
	name string
}

func brDownloadOpen(p M) (*brDownload, error) {
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
	d := &brDownload{snap: b, rel: rel, full: full, info: info}
	switch {
	case info.IsDir():
		base := brBase(rel)
		if rel == "/" {
			base = brBase("/"+b.Dataset) + "@" + b.Snap
		}
		d.name = base + ".zip"
	case info.Mode().IsRegular():
		d.name = brBase(rel)
	default:
		return nil, apiFailf("%s is not a regular file or folder.", rel)
	}
	return d, nil
}

func brDownloadInfo(p M) M {
	d, err := brDownloadOpen(p)
	if err != nil {
		return M{"ok": false, "error": err.Error()}
	}
	var size any
	if !d.info.IsDir() {
		size = d.info.Size()
	}
	return M{"ok": true, "name": d.name, "dir": d.info.IsDir(), "size": size}
}

var brStored = map[string]bool{}

func init() {
	for _, e := range strings.Fields("zip gz tgz bz2 xz zst 7z rar txz lz4 jpg jpeg png gif webp heic mp4 mkv avi mov m4v webm mp3 flac aac ogg opus m4a img iso qcow2 vmdk vhdx deb rpm") {
		brStored["."+e] = true
	}
}

func brZip(w io.Writer, root, top string) error {
	zw := zip.NewWriter(w)
	err := filepath.WalkDir(root, func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		name := top
		if rel != "." {
			name = top + "/" + filepath.ToSlash(rel)
		}
		info, err := de.Info()
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = name
		switch {
		case info.IsDir():
			h.Name += "/"
			h.Method = zip.Store
			_, err = zw.CreateHeader(h)
			return err
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			h.Method = zip.Store
			fw, err := zw.CreateHeader(h)
			if err != nil {
				return err
			}
			_, err = io.WriteString(fw, target)
			return err
		case !info.Mode().IsRegular():
			return nil
		}
		if brStored[strings.ToLower(filepath.Ext(path))] || info.Size() > 256<<20 {
			h.Method = zip.Store
		} else {
			h.Method = zip.Deflate
		}
		fw, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(fw, f)
		f.Close()
		return err
	})
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	return err
}

func brDownloadRun(p M, w io.Writer) error {
	d, err := brDownloadOpen(p)
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(w, 1<<20)
	if d.info.IsDir() {
		err = brZip(bw, d.full, strings.TrimSuffix(d.name, ".zip"))
	} else {
		var f *os.File
		f, err = os.Open(d.full)
		if err == nil {
			_, err = io.Copy(bw, f)
			f.Close()
		}
	}
	if ferr := bw.Flush(); err == nil {
		err = ferr
	}
	return err
}

func init() {
	register("download-info", "describe a snapshot file or folder to download: JSON {dataset, snapshot, path} on stdin", func(args []string) int {
		os.Stdout.Write(jsonEncode(brDownloadInfo(readParams(os.Stdin)), false))
		return 0
	})
	register("download", "stream a snapshot file, or a folder as zip, to stdout: JSON {dataset, snapshot, path} on stdin", func(args []string) int {
		if err := brDownloadRun(readParams(os.Stdin), os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	})
}
