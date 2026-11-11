package zt

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestConvDatasetName(t *testing.T) {
	cases := []struct{ in, no, yes string }{
		{"plex", "plex", "plex"},
		{"TV Shows", "TV Shows", "TV_Shows"},
		{"_backup", "_backup", "_backup"},
		{"Büro (alt)", "Buero _alt_", "Buero__alt_"},
		{"x+y", "x_y", "x_y"},
		{".hidden", ".hidden", ".hidden"},
		{"a:b-c.d", "a:b-c.d", "a:b-c.d"},
		{"Straße", "Strasse", "Strasse"},
		{"café", "caf__", "caf__"},
		{"50%", "50_", "50_"},
	}
	for _, c := range cases {
		if got := convDatasetName(c.in, false); got != c.no {
			t.Errorf("%q without replace: got %q want %q", c.in, got, c.no)
		}
		if got := convDatasetName(c.in, true); got != c.yes {
			t.Errorf("%q with replace: got %q want %q", c.in, got, c.yes)
		}
	}
}

func TestConvValidDataset(t *testing.T) {
	good := []string{"cache/appdata", "cache/TV Shows", "cache/_backup", "cache/a:b", "cache", "tank/media/photos"}
	bad := []string{"cache/Büro", "cache/a(b)", "TV Shows/x", "cache/appdata;rm", "/cache/appdata", "cache/app@data", "", "cache/", "_pool/x", "cache//x", "cache/.."}
	for _, g := range good {
		if !convValidDataset(g) {
			t.Errorf("%q should be valid", g)
		}
	}
	for _, b := range bad {
		if convValidDataset(b) {
			t.Errorf("%q should be rejected", b)
		}
	}
}

func TestConvOwnerFolder(t *testing.T) {
	mp := "/mnt/cache/appdata"
	cases := map[string]string{
		"/mnt/cache/appdata/plex":             "/mnt/cache/appdata/plex",
		"/mnt/cache/appdata/plex/config/x":    "/mnt/cache/appdata/plex",
		"/mnt/cache/appdata":                  "",
		"/mnt/cache/appdata/":                 "",
		"/mnt/cache/appdatax/plex":            "",
		"/mnt/cache/other/plex":               "",
		"/mnt/cache/appdata/Home Assistant/a": "/mnt/cache/appdata/Home Assistant",
	}
	for in, want := range cases {
		if got := convOwnerFolder(in, mp); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestConvJournalParse(t *testing.T) {
	j, ok := convParseJournal("cache/appdata\nBüro (alt)\nBuero _alt_\n/mnt/cache/appdata/Büro (alt)_temp\n")
	if !ok || j.Source != "cache/appdata" || j.Entry != "Büro (alt)" || j.Name != "Buero _alt_" || j.Committed {
		t.Fatalf("old journal: %+v", j)
	}
	if j.path() != "/mnt/cache/appdata/Büro (alt)" || j.dataset() != "cache/appdata/Buero _alt_" {
		t.Errorf("path/dataset: %q %q", j.path(), j.dataset())
	}
	j.Committed = true
	back, _ := convParseJournal(j.render())
	if back != j {
		t.Errorf("round trip: %+v != %+v", back, j)
	}
	if _, ok := convParseJournal(""); ok {
		t.Error("an empty journal parsed as valid")
	}
	odd, _ := convParseJournal("pool/ds\nfoo\nfoo\n/elsewhere/bar_temp\n")
	if odd.path() != "/mnt/pool/ds/foo" {
		t.Errorf("mismatching temp falls back to /mnt: %q", odd.path())
	}
}

func testWorld() *convWorld {
	w := &convWorld{datasets: map[string]convDataset{}, mountedAt: map[string]string{}, ok: true}
	add := func(name, mp string, avail int64) {
		w.datasets[name] = convDataset{Name: name, Mounted: mp != "", Mountpoint: mp, Avail: avail}
		if mp != "" {
			w.mountedAt[mp] = name
		}
	}
	add("cache/appdata", "/mnt/cache/appdata", 1000)
	add("cache/appdata/tdarr", "/mnt/cache/appdata/tdarr", 1000)
	add("cache/appdata/ghost", "", 1000)
	add("cache/appdata/odd name", "/mnt/cache/appdata/Odd Name", 1000)
	w.mounts = []string{"/mnt/cache/appdata", "/mnt/cache/appdata/tdarr", "/mnt/cache/appdata/nested/inner"}
	return w
}

func TestConvClassify(t *testing.T) {
	w := testWorld()
	dirs := []string{".Recycle.Bin", "plex", "tdarr", "tdarr_temp", "ghost", "Büro (alt)", "big", "a+b", "a,b", "nested", "line\nbreak", "Odd Name", "TV Shows"}
	sizes := map[string]int64{"/mnt/cache/appdata/big": 950}
	in := convPlanInput{
		World:   w,
		Sources: []convSource{{Dataset: "cache/appdata", Kind: "appdata"}, {Dataset: "cache/missing", Kind: "extra"}},
		Buffer:  10,
		ReadDir: func(string) ([]convEntry, error) {
			out := []convEntry{{Name: "link", Link: true}, {Name: "file.txt"}}
			for _, d := range dirs {
				out = append(out, convEntry{Name: d, IsDir: true})
			}
			return out, nil
		},
		Size: func(p string) (int64, int64, bool) {
			if s, ok := sizes[p]; ok {
				return s, s, true
			}
			return 10, 10, true
		},
	}
	scans := convClassify(in)
	if len(scans) != 2 || scans[1].Error == "" {
		t.Fatalf("missing source not reported: %+v", scans)
	}
	got := map[string]convFolder{}
	for _, f := range scans[0].Folders {
		got[f.Name] = f
	}
	if _, ok := got["link"]; ok {
		t.Error("a symlink was listed")
	}
	if _, ok := got[".Recycle.Bin"]; ok {
		t.Error("a hidden folder was listed")
	}
	want := map[string][2]string{
		"plex":        {"convert", ""},
		"tdarr":       {"dataset", ""},
		"tdarr_temp":  {"kept", "kept_copy"},
		"ghost":       {"kept", "unmounted_dataset"},
		"Büro (alt)":  {"convert", ""},
		"big":         {"blocked", "space"},
		"a+b":         {"convert", ""},
		"a,b":         {"blocked", "collision"},
		"nested":      {"blocked", "mounts"},
		"line\nbreak": {"blocked", "bad_name"},
		"Odd Name":    {"dataset", ""},
		"TV Shows":    {"convert", ""},
	}
	for name, st := range want {
		f, ok := got[name]
		if !ok {
			t.Errorf("%q missing", name)
			continue
		}
		if f.State != st[0] || f.Reason != st[1] {
			t.Errorf("%q: got %s/%s want %s/%s (%s)", name, f.State, f.Reason, st[0], st[1], f.Why)
		}
	}
	if b := got["Büro (alt)"]; b.As != "Buero _alt_" || b.Dataset != "cache/appdata/Buero _alt_" {
		t.Errorf("Büro (alt) target: %+v", b)
	}
	if got["plex"].As != "" || got["plex"].Dataset != "cache/appdata/plex" {
		t.Errorf("plex target: %+v", got["plex"])
	}
	if got["Odd Name"].Dataset != "cache/appdata/odd name" {
		t.Errorf("already converted is detected by mountpoint: %+v", got["Odd Name"])
	}
	in.ReplaceSpaces = true
	for _, f := range convClassify(in)[0].Folders {
		if f.Name == "TV Shows" && (f.State != "convert" || f.As != "TV_Shows") {
			t.Errorf("TV Shows with replace_spaces: %+v", f)
		}
	}
	if n := convCount(scans, "convert"); n != 4 {
		t.Errorf("convert count %d", n)
	}
}

func TestConvConfig(t *testing.T) {
	s := Settings{"should_process_containers": "yes", "appdata_pool": "cache", "appdata_dataset": "appdata",
		"should_process_vms": "yes", "vm_pool": "cache", "vm_dataset": "domains",
		"extra_datasets": " tank/media , cache/appdata, bad;name ", "buffer_zone": "500", "dry_run": "maybe"}
	c := convConfigFrom(s)
	if len(c.Sources) != 3 || c.Sources[0].Kind != "appdata" || c.Sources[1].Kind != "domains" || c.Sources[2].Dataset != "tank/media" {
		t.Errorf("sources: %+v", c.Sources)
	}
	if len(c.Problems) != 1 {
		t.Errorf("problems: %v", c.Problems)
	}
	if c.Buffer != 100 || c.Tolerance != 5 || c.VMWait != 90 {
		t.Errorf("clamps: %+v", c)
	}
	if !c.DryRun {
		t.Error("anything but dry_run=no must be a dry run")
	}
	if convConfigFrom(Settings{"dry_run": "no"}).DryRun {
		t.Error("dry_run=no")
	}
}

func TestConvRequired(t *testing.T) {
	if convRequired(100, 11) != 111 || convRequired(0, 50) != 0 {
		t.Error("required space")
	}
}

func recoverFixture(t *testing.T, copy map[string]int) (*convRunner, string) {
	d := t.TempDir()
	temp, path := d+"/app_temp", d+"/app"
	os.MkdirAll(temp, 0o755)
	os.MkdirAll(path, 0o755)
	os.WriteFile(temp+"/a.conf", []byte("x"), 0o644)
	os.WriteFile(temp+"/vdisk1.img", make([]byte, 1000000), 0o644)
	for name, size := range copy {
		os.WriteFile(path+"/"+name, make([]byte, size), 0o644)
	}
	os.WriteFile(d+"/zfs", []byte("#!/bin/sh\nprintf 'cache/appdata/app\\tyes\\t%s\\t1000\\t10\\n' '"+path+"'\n"), 0o755)
	oldZfs, oldJournal := ZfsBin, convJournalPath
	ZfsBin, convJournalPath = d+"/zfs", d+"/journal"
	t.Cleanup(func() { ZfsBin, convJournalPath = oldZfs, oldJournal })
	convWriteJournal(convJournal{Source: "cache/appdata", Entry: "app", Name: "app", Temp: temp})
	return &convRunner{cfg: convConfig{Tolerance: 5, Cleanup: true}, ctx: context.Background(), log: &convLog{}}, temp
}

func TestConvRecoverKeepsOriginalOfCutCopy(t *testing.T) {
	r, temp := recoverFixture(t, map[string]int{"a.conf": 1, ".vdisk1.img.Xk3j2a": 970000})
	if r.recoverInterrupted() {
		t.Error("a copy cut off inside vdisk1.img was taken as complete")
	}
	if st, err := os.Stat(temp + "/vdisk1.img"); err != nil || st.Size() != 1000000 {
		t.Fatalf("the original is gone: %v", err)
	}
	if _, err := os.Stat(convJournalPath); err != nil {
		t.Error("the journal was cleared")
	}
	if len(r.converted) != 0 {
		t.Errorf("reported as converted: %v", r.converted)
	}
}

func TestConvRecoverFinishesCompleteCopy(t *testing.T) {
	r, temp := recoverFixture(t, map[string]int{"a.conf": 1, "vdisk1.img": 1000000})
	if !r.recoverInterrupted() {
		t.Fatal("a complete copy was not finished")
	}
	if _, err := os.Stat(temp); !os.IsNotExist(err) {
		t.Error("the original was not cleaned up")
	}
	if _, err := os.Stat(convJournalPath); !os.IsNotExist(err) {
		t.Error("the journal is still there")
	}
}

func TestConvRecoverKeepsOriginalItCannotRead(t *testing.T) {
	r, temp := recoverFixture(t, map[string]int{"a.conf": 1, "vdisk1.img": 1000000})
	os.MkdirAll(temp+"/secret", 0o755)
	os.WriteFile(temp+"/secret/x", []byte("x"), 0o644)
	os.MkdirAll(dirOf(temp)+"/app/secret", 0o755)
	os.Chmod(temp+"/secret", 0)
	t.Cleanup(func() { os.Chmod(temp+"/secret", 0o755) })
	if r.recoverInterrupted() {
		t.Error("an original that cannot be read completely was deleted")
	}
	if _, err := os.Stat(temp + "/vdisk1.img"); err != nil {
		t.Fatalf("the original is gone: %v", err)
	}
}

func TestConvMissingFrom(t *testing.T) {
	tree := func(root string, files map[string]string) {
		os.MkdirAll(root, 0o755)
		for name, body := range files {
			os.MkdirAll(dirOf(root+"/"+name), 0o755)
			if strings.HasPrefix(body, "->") {
				os.Symlink(body[2:], root+"/"+name)
			} else {
				os.WriteFile(root+"/"+name, []byte(body), 0o644)
			}
		}
	}
	orig := map[string]string{"a.conf": "x", "db/main.db": "data", "link": "->a.conf"}
	cases := []struct {
		name string
		copy map[string]string
		want string
	}{
		{"same", map[string]string{"a.conf": "x", "db/main.db": "data", "link": "->a.conf"}, ""},
		{"extra files are fine", map[string]string{"a.conf": "x", "db/main.db": "data", "link": "->a.conf", "new.log": "y"}, ""},
		{"nested file missing", map[string]string{"a.conf": "x", "db/.main.db.Xk3j2a": "data", "link": "->a.conf"}, "/db/main.db"},
		{"other size", map[string]string{"a.conf": "x", "db/main.db": "dat", "link": "->a.conf"}, "/db/main.db"},
		{"file instead of symlink", map[string]string{"a.conf": "x", "db/main.db": "data", "link": "x"}, "/link"},
		{"file instead of folder", map[string]string{"a.conf": "x", "db": "data", "link": "->a.conf"}, "/db"},
	}
	for _, c := range cases {
		d := t.TempDir()
		tree(d+"/src", orig)
		tree(d+"/dst", c.copy)
		got, err := convMissingFrom(d+"/src", d+"/dst")
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v want %q", c.name, got, err, c.want)
		}
	}
	if _, err := convMissingFrom(t.TempDir()+"/gone", t.TempDir()); err == nil {
		t.Error("an unreadable original was not reported")
	}
}
