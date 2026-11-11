package zt

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBrRel(t *testing.T) {
	good := map[string]string{
		"":                "/",
		"/":               "/",
		"a/b":             "/a/b",
		"/a//b/./c/":      "/a/b/c",
		"TV Shows/Show A": "/TV Shows/Show A",
		"/Büro (alt)":     "/Büro (alt)",
		"...":             "/...",
	}
	for in, want := range good {
		got, err := brRel(in)
		if err != nil || got != want {
			t.Errorf("brRel(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"..", "/a/../b", "a/..", "/../etc", "a\nb", "a\x00b"} {
		if _, err := brRel(in); err == nil {
			t.Errorf("brRel(%q) accepted", in)
		}
	}
}

func TestBrNames(t *testing.T) {
	for _, s := range []string{"auto-hourly-2026-04-12_14-00", "before upgrade", "zdc-send-ab12-1"} {
		if !brValidSnap(s) {
			t.Errorf("snapshot %q rejected", s)
		}
	}
	for _, s := range []string{"", ".", "..", "a/b", "a@b", " x", "a\nb"} {
		if brValidSnap(s) {
			t.Errorf("snapshot %q accepted", s)
		}
	}
	if brKind("auto-daily-2026-01-01") != "daily" || brKind("zdc-send-x-1") != "replication" || brKind("mine") != "manual" {
		t.Error("brKind")
	}
}

func TestBrPathHelpers(t *testing.T) {
	if brJoin("/mnt/x/", "/") != "/mnt/x" || brJoin("/mnt/x", "/a") != "/mnt/x/a" {
		t.Error("brJoin")
	}
	if brParent("/a/b") != "/a" || brParent("/a") != "/" || brParent("/") != "/" {
		t.Error("brParent")
	}
	if brBase("/a/b c") != "b c" || brBase("/") != "" {
		t.Error("brBase")
	}
}

func TestBrWalkRefusesLinks(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(root+"/snap/dir/sub", 0o755)
	os.WriteFile(root+"/snap/dir/f", []byte("x"), 0o644)
	os.Symlink("/etc", root+"/snap/out")
	os.Symlink("dir", root+"/snap/in")
	base := root + "/snap"
	if p, info, err := brWalk(base, "/dir/sub", false); err != nil || !info.IsDir() || p != base+"/dir/sub" {
		t.Fatalf("walk dir: %v %v", p, err)
	}
	if _, _, err := brWalk(base, "/out", false); err == nil {
		t.Error("link accepted as a folder")
	}
	if _, _, err := brWalk(base, "/out/passwd", true); err == nil {
		t.Error("walked through an escaping link")
	}
	if _, _, err := brWalk(base, "/in/f", true); err == nil {
		t.Error("walked through an inner link")
	}
	if _, info, err := brWalk(base, "/out", true); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("final link should be allowed when asked")
	}
	if _, _, err := brWalk(base, "/dir/f/x", false); err == nil {
		t.Error("walked through a file")
	}
	if _, _, err := brWalk(base, "/nope", false); !os.IsNotExist(err) {
		t.Errorf("missing: %v", err)
	}
	os.MkdirAll(root+"/live/a", 0o755)
	os.Symlink("/etc", root+"/live/a/l")
	if brLiveLink(root+"/live", "/a") != "" {
		t.Error("brLiveLink false positive")
	}
	if brLiveLink(root+"/live", "/a/l/x") != root+"/live/a/l" {
		t.Error("brLiveLink missed a link")
	}
	if brLiveLink(root+"/live", "/a/missing/deeper") != "" {
		t.Error("brLiveLink on missing parts")
	}
}

func TestDiffUnescape(t *testing.T) {
	cases := map[string]string{
		`/mnt/c/TV\0040Shows`:     "/mnt/c/TV Shows",
		`/mnt/c/B\0303\0274ro`:    "/mnt/c/Büro",
		`/mnt/c/back\0134slash`:   `/mnt/c/back\slash`,
		`/mnt/c/tab\0011x`:        "/mnt/c/tab\tx",
		`/mnt/c/plain`:            "/mnt/c/plain",
		`/mnt/c/short\040escape`:  `/mnt/c/short\040escape`,
		`/mnt/c/\0342\0202\0254x`: "/mnt/c/€x",
	}
	for in, want := range cases {
		if got := diffUnescape(in); got != want {
			t.Errorf("diffUnescape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDiffParse(t *testing.T) {
	lines := []string{
		"M\t/\t/mnt/c/data/",
		"+\tF\t/mnt/c/data/new\\0040file.txt",
		"-\t/\t/mnt/c/data/old",
		"R\tF\t/mnt/c/data/a\t/mnt/c/data/B\\0303\\0274ro",
		"M\t@\t/mnt/c/data/link",
		"bad line",
	}
	got := diffParse(lines, "/mnt/c/data")
	if len(got) != 5 {
		t.Fatalf("got %d entries", len(got))
	}
	if got[0].Path != "/" || got[0].Type != "dir" || got[0].Kind != "modified" {
		t.Errorf("root: %+v", got[0])
	}
	if got[1].Path != "/new file.txt" || got[1].Kind != "added" || got[1].Type != "file" {
		t.Errorf("added: %+v", got[1])
	}
	if got[2].Kind != "removed" || got[2].Path != "/old" {
		t.Errorf("removed: %+v", got[2])
	}
	if got[3].Kind != "renamed" || got[3].NewPath != "/Büro" {
		t.Errorf("renamed: %+v", got[3])
	}
	if got[4].Type != "symlink" {
		t.Errorf("symlink: %+v", got[4])
	}
	if diffRel("/mnt/c/database", "/mnt/c/data") != "/mnt/c/database" {
		t.Error("diffRel matched a sibling")
	}
	if !diffUnder("/a/b", "/a") || diffUnder("/ab", "/a") || !diffUnder("/x", "/") {
		t.Error("diffUnder")
	}
}

func TestRsyncArgs(t *testing.T) {
	j := &restoreJob{Overwrite: "never", Delete: true}
	a := strings.Join(restoreRsyncArgs(j, restoreItem{Src: "/s/d", Into: "/nonexistent-zt/d", Type: "dir"}, true), " ")
	for _, want := range []string{"-aH", "-AX", "--numeric-ids", "--ignore-existing", "--delete", "--exclude=/.zfs", "-- /s/d/ /nonexistent-zt/d/"} {
		if !strings.Contains(a, want) {
			t.Errorf("dir args %q lack %q", a, want)
		}
	}
	j = &restoreJob{Overwrite: "always", Delete: true}
	a = strings.Join(restoreRsyncArgs(j, restoreItem{Src: "/s/f", Into: "/t/f", Type: "file"}, false), " ")
	if strings.Contains(a, "--delete") || strings.Contains(a, "-AX") || !strings.Contains(a, "--ignore-times") || !strings.HasSuffix(a, "-- /s/f /t/f") {
		t.Errorf("file args %q", a)
	}
	if rsyncEscape("/plain dir") != "/plain dir" || rsyncEscape("/a[1]*") != `/a\[1]\*` {
		t.Errorf("rsyncEscape: %q", rsyncEscape("/a[1]*"))
	}
}

func TestRsyncProgress(t *testing.T) {
	data := []byte("      1,048,576  12%   10.00MB/s    0:00:01 (xfr#1, to-chk=3/5)\r     8,388,608 100%   20.00MB/s    0:00:02\nrsync: error\n")
	var lines []string
	for len(data) > 0 {
		adv, tok, _ := splitCRLF(data, true)
		lines = append(lines, string(tok))
		data = data[adv:]
	}
	if len(lines) != 3 {
		t.Fatalf("lines %q", lines)
	}
	m := reRsyncPct.FindStringSubmatch(lines[0])
	if m == nil || m[2] != "12" {
		t.Errorf("progress %q", lines[0])
	}
	if reRsyncPct.MatchString(lines[2]) {
		t.Error("error line taken as progress")
	}
	if !reRsyncNoAttr.MatchString(`rsync: [receiver] set_acl: sys_acl_set_file(x, ACCESS): Operation not supported (95)`) {
		t.Error("acl failure not recognised")
	}
}

func TestRestorePreview(t *testing.T) {
	root := t.TempDir()
	src, live := root+"/snap/d", root+"/live/d"
	os.MkdirAll(src+"/sub", 0o755)
	os.MkdirAll(live+"/sub", 0o755)
	old := time.Unix(1700000000, 0)
	write := func(p, s string, mt time.Time) {
		os.WriteFile(p, []byte(s), 0o644)
		os.Chtimes(p, mt, mt)
	}
	write(src+"/same", "same", old)
	write(live+"/same", "same", old)
	write(src+"/changed", "new content", old)
	write(live+"/changed", "old", old)
	write(src+"/sub/new", "brand new", old)
	write(live+"/extra", "only live", old)
	os.MkdirAll(live+"/extradir/x", 0o755)
	item := restoreItem{Src: src, Into: live, Type: "dir"}

	st := restorePreview(&restoreJob{Overwrite: "changed", Items: []restoreItem{item}}, time.Minute, 1000)
	if st.Files != 3 || st.New != 1 || st.Replace != 1 || st.Same != 1 || st.Remove != 0 || st.Partial {
		t.Errorf("changed: %+v", st)
	}
	if len(st.Replaced) != 1 || st.Replaced[0].Path != live+"/changed" || st.Replaced[0].Size != 3 || st.Replaced[0].NewSize != 11 {
		t.Errorf("replaced list: %+v", st.Replaced)
	}
	st = restorePreview(&restoreJob{Overwrite: "never", Items: []restoreItem{item}}, time.Minute, 1000)
	if st.Kept != 1 || st.Replace != 0 || st.New != 1 {
		t.Errorf("never: %+v", st)
	}
	st = restorePreview(&restoreJob{Overwrite: "changed", Delete: true, Items: []restoreItem{item}}, time.Minute, 1000)
	if st.Remove != 2 {
		t.Errorf("delete: %+v", st)
	}
	st = restorePreview(&restoreJob{Overwrite: "changed", Items: []restoreItem{item}}, time.Minute, 2)
	if !st.Partial {
		t.Error("limit not reported")
	}
}

func TestZipStream(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(root+"/d/sub", 0o755)
	os.WriteFile(root+"/d/a b.txt", []byte(strings.Repeat("hello ", 100)), 0o644)
	os.WriteFile(root+"/d/sub/c.jpg", []byte("jpeg"), 0o600)
	os.Symlink("/etc/passwd", root+"/d/link")
	var buf bytes.Buffer
	if err := brZip(&buf, root+"/d", "top"); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*zip.File{}
	for _, f := range zr.File {
		got[f.Name] = f
	}
	for _, n := range []string{"top/", "top/a b.txt", "top/sub/", "top/sub/c.jpg", "top/link"} {
		if got[n] == nil {
			t.Errorf("zip lacks %q", n)
		}
	}
	if f := got["top/link"]; f != nil {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		if f.Mode()&os.ModeSymlink == 0 || string(b) != "/etc/passwd" {
			t.Error("link stored wrongly, or followed")
		}
	}
	if f := got["top/sub/c.jpg"]; f != nil && f.Method != zip.Store {
		t.Error("jpg should be stored")
	}
	if f := got["top/a b.txt"]; f != nil {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		if len(b) != 600 || f.Method != zip.Deflate {
			t.Error("text file content or method")
		}
	}
}

func TestRestoreDismissStaysInRestoreDir(t *testing.T) {
	d := t.TempDir()
	old := TmpDir
	TmpDir = d + "/tmp/zt"
	defer func() { TmpDir = old }()
	if err := os.MkdirAll(restoreDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	out := apiDispatch("restore_dismiss", M{"id": "../../../outside/deep/x"})
	if out["ok"] != false {
		t.Errorf("a traversing id was accepted: %v", out)
	}
	if _, ok := restoreUpdate("../../../outside2/y", func(j *restoreJob) bool { return true }); ok {
		t.Error("restoreUpdate accepted a traversing id")
	}
	for _, p := range []string{d + "/outside", d + "/outside2", d + "/tmp/outside"} {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("%s was created", p)
		}
	}
}

func restoreFixture(t *testing.T) (mount, outside string) {
	t.Helper()
	d := t.TempDir()
	mount, outside = d+"/live", d+"/outside"
	snap := mount + "/.zfs/snapshot/s1"
	for _, dir := range []string{snap, outside, d + "/bin"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\n[ \"$1\" = get ] && printf 'type\\tfilesystem\\nmounted\\tyes\\nmountpoint\\t%s\\n' '" + mount + "'\nexit 0\n"
	if err := os.WriteFile(d+"/bin/zfs", []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := ZfsBin
	ZfsBin = d + "/bin/zfs"
	t.Cleanup(func() { ZfsBin = old })
	if err := os.WriteFile(snap+"/a.txt", []byte("from the snapshot"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside+"/b.txt", []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("outside-link-target", snap+"/l"); err != nil {
		t.Fatal(err)
	}
	return mount, outside
}

func TestRestoreFileOntoSymlinkRefused(t *testing.T) {
	mount, outside := restoreFixture(t)
	req := M{"dataset": "tank/data", "snapshot": "s1", "items": []any{"/a.txt"}}
	for _, target := range []string{outside, outside + "/b.txt", outside + "/missing"} {
		if err := os.Symlink(target, mount+"/a.txt"); err != nil {
			t.Fatal(err)
		}
		out := apiDispatch("restore_preview", req)
		if msg, _ := out["error"].(string); out["ok"] != false || !strings.Contains(msg, "symbolic link") {
			t.Errorf("restoring a file onto a symlink to %s: %v", target, out)
		}
		if err := os.Remove(mount + "/a.txt"); err != nil {
			t.Fatal(err)
		}
	}
	if out := apiDispatch("restore_preview", req); out["ok"] != true {
		t.Errorf("a plain restore is refused: %v", out)
	}
}

func TestRestoreSymlinkOntoSymlinkedFolderRefused(t *testing.T) {
	mount, outside := restoreFixture(t)
	req := M{"dataset": "tank/data", "snapshot": "s1", "items": []any{"/l"}}
	if err := os.Symlink(outside, mount+"/l"); err != nil {
		t.Fatal(err)
	}
	if out := apiDispatch("restore_preview", req); out["ok"] != false {
		t.Errorf("a link restored onto a symlink to a folder: %v", out)
	}
	if err := os.Remove(mount + "/l"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside+"/b.txt", mount+"/l"); err != nil {
		t.Fatal(err)
	}
	if out := apiDispatch("restore_preview", req); out["ok"] != true {
		t.Errorf("a link restored onto a link to a file is refused: %v", out)
	}
}
