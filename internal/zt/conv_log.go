package zt

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type convLog struct {
	mu       sync.Mutex
	f        *os.File
	tee      io.Writer
	errors   int
	warnings int
}

func convOpenLog(path string, tee io.Writer) (*convLog, error) {
	os.MkdirAll(dirOf(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &convLog{f: f, tee: tee}, nil
}

func (l *convLog) raw(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		l.f.WriteString(s + "\n")
	}
	if l.tee != nil {
		io.WriteString(l.tee, s+"\n")
	}
}

func (l *convLog) stamp(prefix, format string, a ...any) {
	msg := format
	if len(a) > 0 {
		msg = fmt.Sprintf(format, a...)
	}
	for _, line := range strings.Split(strings.TrimRight(msg, "\n"), "\n") {
		l.raw("[" + time.Now().Format("15:04:05") + "] " + prefix + line)
	}
}

func (l *convLog) info(format string, a ...any) { l.stamp("", format, a...) }
func (l *convLog) ok(format string, a ...any)   { l.stamp("OK: ", format, a...) }

func (l *convLog) warn(format string, a ...any) {
	l.mu.Lock()
	l.warnings++
	l.mu.Unlock()
	l.stamp("WARNING: ", format, a...)
}

func (l *convLog) err(format string, a ...any) {
	l.mu.Lock()
	l.errors++
	l.mu.Unlock()
	l.stamp("ERROR: ", format, a...)
}

func (l *convLog) step(s string) {
	l.raw("")
	l.raw("=== Step " + s + " ===")
}

func (l *convLog) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		l.f.Close()
		l.f = nil
	}
}

type convLogFile struct {
	File   string `json:"file"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Mtime  int64  `json:"mtime"`
	Size   int64  `json:"size"`
}

func convLogFiles() []convLogFile {
	out := []convLogFile{}
	for _, pat := range []string{"conversion_*.log", "auto_*.log"} {
		m, _ := filepath.Glob(TmpDir + "/" + pat)
		for _, p := range m {
			st, err := os.Stat(p)
			if err != nil || !st.Mode().IsRegular() {
				continue
			}
			src := "manual"
			if strings.HasPrefix(convBase(p), "auto_") {
				src = "auto"
			}
			out = append(out, convLogFile{File: p, Name: convBase(p), Source: src, Mtime: st.ModTime().Unix(), Size: st.Size()})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mtime != out[j].Mtime {
			return out[i].Mtime > out[j].Mtime
		}
		return out[i].Name > out[j].Name
	})
	return out
}

func convPruneLogs(keep int) {
	for _, pat := range []string{"conversion_*.log", "auto_*.log"} {
		m, _ := filepath.Glob(TmpDir + "/" + pat)
		type fe struct {
			p string
			t int64
		}
		files := []fe{}
		for _, p := range m {
			files = append(files, fe{p, fileMtime(p)})
		}
		sort.Slice(files, func(i, j int) bool {
			if files[i].t != files[j].t {
				return files[i].t > files[j].t
			}
			return files[i].p > files[j].p
		})
		for i := keep; i < len(files); i++ {
			os.Remove(files[i].p)
		}
	}
}

func convLogAllowed(p string) bool {
	if p == "" || strings.Contains(p, "..") || dirOf(p) != TmpDir {
		return false
	}
	b := convBase(p)
	return (strings.HasPrefix(b, "conversion_") || strings.HasPrefix(b, "auto_")) && strings.HasSuffix(b, ".log")
}

func convBase(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func convClassifyLog(path string) string {
	t := tail(path, 80, 64*1024)
	if t == "" && !isFile(path) {
		return "idle"
	}
	if strings.Contains(t, "VALIDATION FAILED") || strings.Contains(t, "] ERROR:") {
		return "error"
	}
	if strings.Contains(t, "Script execution completed successfully") || strings.Contains(t, "Nothing to convert") {
		return "completed"
	}
	return "stopped"
}
