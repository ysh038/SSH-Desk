package fsops

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// All tests work inside t.TempDir() only (CLAUDE.md rule 6).

func newFS(t *testing.T) (*FS, string) {
	t.Helper()
	root := t.TempDir()
	f := &FS{TrashDir: filepath.Join(root, "Trash"), Now: func() time.Time { return time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC) }}
	return f, root
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool { _, err := os.Lstat(path); return err == nil }

func names(t *testing.T, dir string) []string {
	t.Helper()
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, d := range des {
		out = append(out, d.Name())
	}
	sort.Strings(out)
	return out
}

func TestReadDirAndSort(t *testing.T) {
	_, root := newFS(t)
	write(t, filepath.Join(root, "b.log"), "12345")
	write(t, filepath.Join(root, "한글 파일명.txt"), "x")
	write(t, filepath.Join(root, "A.txt"), "")
	write(t, filepath.Join(root, ".hidden"), "")
	os.Mkdir(filepath.Join(root, "zdir"), 0o755)
	os.Symlink("zdir", filepath.Join(root, "link-to-dir"))

	es, err := ReadDir(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	Sort(es, SortName, false)
	var got []string
	for _, e := range es {
		got = append(got, e.Name)
	}
	want := []string{"link-to-dir", "zdir", ".hidden", "A.txt", "b.log", "한글 파일명.txt"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("order = %v", got)
	}
	if !es[0].IsLink || !es[0].IsDir {
		t.Fatalf("symlink to dir: %+v", es[0])
	}
	Sort(es, SortSize, true)
	if es[2].Name != "b.log" {
		t.Fatalf("size sort: %s", es[2].Name)
	}
}

func TestCopyTreePreservesContentModeAndLinks(t *testing.T) {
	f, root := newFS(t)
	src := filepath.Join(root, "site")
	write(t, filepath.Join(src, "index.html"), "<h1>hi</h1>")
	write(t, filepath.Join(src, "bin", "run.sh"), "#!/bin/sh")
	os.Chmod(filepath.Join(src, "bin", "run.sh"), 0o755)
	os.Symlink("index.html", filepath.Join(src, "home.html"))
	dst := filepath.Join(root, "backup")
	os.Mkdir(dst, 0o755)

	var last Progress
	rec, err := f.Copy(context.Background(), []string{src}, dst, Skip, func(p Progress) { last = p })
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dst, "site", "index.html")); got != "<h1>hi</h1>" {
		t.Fatalf("content %q", got)
	}
	info, _ := os.Stat(filepath.Join(dst, "site", "bin", "run.sh"))
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", info.Mode())
	}
	if target, _ := os.Readlink(filepath.Join(dst, "site", "home.html")); target != "index.html" {
		t.Fatalf("link target %q", target)
	}
	if last.Done != last.Total || last.Total != int64(len("<h1>hi</h1>")+len("#!/bin/sh")) {
		t.Fatalf("progress %+v", last)
	}
	if !exists(filepath.Join(src, "index.html")) {
		t.Fatal("copy must keep the source")
	}
	// Undo sends the copy to the trash.
	if err := f.Undo(rec); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(dst, "site")) || !exists(filepath.Join(f.TrashDir, "files", "site")) {
		t.Fatal("undo copy should trash the copy")
	}
}

func TestCopyIntoItselfIsRefused(t *testing.T) {
	f, root := newFS(t)
	src := filepath.Join(root, "a")
	os.MkdirAll(filepath.Join(src, "sub"), 0o755)
	if _, err := f.Copy(context.Background(), []string{src}, filepath.Join(src, "sub"), Skip, nil); err == nil {
		t.Fatal("expected error")
	}
	if _, err := f.Move(context.Background(), []string{src}, src, Skip, nil); err == nil {
		t.Fatal("expected error moving into itself")
	}
}

func TestMoveAndUndo(t *testing.T) {
	f, root := newFS(t)
	logs := []string{}
	for _, n := range []string{"app.log", "error.log", "한글.log"} {
		p := filepath.Join(root, n)
		write(t, p, n)
		logs = append(logs, p)
	}
	backup := filepath.Join(root, "backup")
	os.Mkdir(backup, 0o755)
	rec, err := f.Move(context.Background(), logs, backup, Skip, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(t, backup); strings.Join(got, ",") != "app.log,error.log,한글.log" {
		t.Fatalf("backup = %v", got)
	}
	if exists(logs[0]) {
		t.Fatal("source should be gone")
	}
	if err := f.Undo(rec); err != nil {
		t.Fatal(err)
	}
	if len(names(t, backup)) != 0 || read(t, logs[2]) != "한글.log" {
		t.Fatal("undo move failed")
	}
}

func TestConflictPolicies(t *testing.T) {
	f, root := newFS(t)
	src := filepath.Join(root, "app.log")
	backup := filepath.Join(root, "backup")
	write(t, src, "new")
	write(t, filepath.Join(backup, "app.log"), "old")

	if c := Conflicts([]string{src}, backup); len(c) != 1 || c[0] != "app.log" {
		t.Fatalf("conflicts %v", c)
	}

	// Skip leaves both untouched.
	f.Move(context.Background(), []string{src}, backup, Skip, nil)
	if read(t, src) != "new" || read(t, filepath.Join(backup, "app.log")) != "old" {
		t.Fatal("skip")
	}

	// Keep both: unique name.
	rec, _ := f.Copy(context.Background(), []string{src}, backup, KeepBoth, nil)
	if read(t, filepath.Join(backup, "app (2).log")) != "new" {
		t.Fatalf("keep both: %v", names(t, backup))
	}
	f.Undo(rec)

	// Replace: the old file goes to the trash, undo brings it back.
	rec, err := f.Move(context.Background(), []string{src}, backup, Replace, nil)
	if err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(backup, "app.log")) != "new" {
		t.Fatal("replace")
	}
	if trashed := names(t, filepath.Join(f.TrashDir, "files")); len(trashed) == 0 {
		t.Fatal("replaced file must be in the trash")
	}
	if err := f.Undo(rec); err != nil {
		t.Fatal(err)
	}
	if read(t, src) != "new" || read(t, filepath.Join(backup, "app.log")) != "old" {
		t.Fatal("undo replace should restore both files")
	}
}

func TestTrashFollowsFreedesktopSpec(t *testing.T) {
	f, root := newFS(t)
	p1 := filepath.Join(root, "x", "한글 로그.log")
	p2 := filepath.Join(root, "y", "한글 로그.log")
	write(t, p1, "1")
	write(t, p2, "2")
	rec, err := f.Trash([]string{p1, p2})
	if err != nil {
		t.Fatal(err)
	}
	files := names(t, filepath.Join(f.TrashDir, "files"))
	if strings.Join(files, ",") != "한글 로그.2.log,한글 로그.log" {
		t.Fatalf("trash files = %v", files)
	}
	info := read(t, filepath.Join(f.TrashDir, "info", "한글 로그.log.trashinfo"))
	if !strings.HasPrefix(info, "[Trash Info]\nPath=") || !strings.Contains(info, "%ED%95%9C%EA%B8%80%20%EB%A1%9C%EA%B7%B8.log") ||
		!strings.Contains(info, "DeletionDate=2026-11-05T10:00:00") {
		t.Fatalf("trashinfo = %q", info)
	}
	if err := f.Undo(rec); err != nil {
		t.Fatal(err)
	}
	if read(t, p1) != "1" || read(t, p2) != "2" || len(names(t, filepath.Join(f.TrashDir, "info"))) != 0 {
		t.Fatal("restore from trash failed")
	}
}

func TestRenameCreateDuplicate(t *testing.T) {
	f, root := newFS(t)
	p := filepath.Join(root, "nginx.conf")
	write(t, p, "conf")
	rec, err := f.Rename(p, "nginx.conf.bak")
	if err != nil || !exists(filepath.Join(root, "nginx.conf.bak")) {
		t.Fatalf("rename: %v", err)
	}
	f.Undo(rec)
	if !exists(p) {
		t.Fatal("undo rename")
	}
	for _, bad := range []string{"", ".", "..", "a/b"} {
		if _, err := f.Rename(p, bad); err == nil {
			t.Errorf("rename to %q should fail", bad)
		}
	}
	write(t, filepath.Join(root, "other"), "")
	if _, err := f.Rename(p, "other"); err == nil {
		t.Error("rename onto existing should fail")
	}

	dir, _, err := f.Mkdir(root, "새 폴더")
	if err != nil || !exists(dir) {
		t.Fatalf("mkdir: %v", err)
	}
	fp, rec, err := f.CreateFile(dir, "notes.txt")
	if err != nil || !exists(fp) {
		t.Fatalf("create: %v", err)
	}
	f.Undo(rec)
	if exists(fp) {
		t.Fatal("undo create")
	}

	if _, err := f.Duplicate(context.Background(), []string{p}); err != nil {
		t.Fatal(err)
	}
	f.Duplicate(context.Background(), []string{p})
	if !exists(filepath.Join(root, "nginx copy.conf")) || !exists(filepath.Join(root, "nginx copy 2.conf")) {
		t.Fatalf("duplicates: %v", names(t, root))
	}
}

func TestCancelRemovesPartialCopy(t *testing.T) {
	f, root := newFS(t)
	src := filepath.Join(root, "big.bin")
	write(t, src, strings.Repeat("x", 3<<20))
	dst := filepath.Join(root, "out")
	os.Mkdir(dst, 0o755)
	ctx, cancel := context.WithCancel(context.Background())
	_, err := f.Copy(ctx, []string{src}, dst, Skip, func(p Progress) { cancel() })
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if exists(filepath.Join(dst, "big.bin")) {
		t.Fatal("partial file left behind")
	}
}

func TestIsText(t *testing.T) {
	_, root := newFS(t)
	write(t, filepath.Join(root, "a.conf"), "server {\n  # 한글 주석\n}\n")
	os.WriteFile(filepath.Join(root, "b.bin"), []byte{0x7f, 'E', 'L', 'F', 0, 1}, 0o644)
	if !IsText(filepath.Join(root, "a.conf")) || IsText(filepath.Join(root, "b.bin")) {
		t.Fatal("IsText")
	}
}

// ---- permissions ----

func TestOctalAndSymbolic(t *testing.T) {
	cases := []struct {
		in   string
		sym  string
		back string
	}{
		{"755", "rwxr-xr-x", "755"},
		{"0644", "rw-r--r--", "644"},
		{"600", "rw-------", "600"},
		{"4755", "rwsr-xr-x", "4755"},
		{"1777", "rwxrwxrwt", "1777"},
		{"2640", "rw-r-S---", "2640"},
	}
	for _, c := range cases {
		m, err := ParseOctal(c.in)
		if err != nil {
			t.Fatal(err)
		}
		if Symbolic(m) != c.sym || Octal(m) != c.back {
			t.Errorf("%s → %s %s", c.in, Symbolic(m), Octal(m))
		}
	}
	for _, bad := range []string{"", "75", "888", "77777", "rwx"} {
		if _, err := ParseOctal(bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}

func TestMixedSelectionOnlyChangesEditedCells(t *testing.T) {
	modes := []fs.FileMode{0o644, 0o600, 0o755}
	before := Aggregate(modes)
	// owner r/w are On everywhere; owner x is Mixed; group r is Mixed.
	if before[0] != On || before[2] != Mixed || before[3] != Mixed || before[4] != Off {
		t.Fatalf("aggregate %v", before)
	}
	after := before
	after[2] = On // user ticks owner x only
	c := ChangeFrom(before, after)
	got := []string{}
	for _, m := range modes {
		got = append(got, Octal(c.Apply(m)))
	}
	if strings.Join(got, ",") != "744,700,755" {
		t.Fatalf("applied %v", got)
	}
}

func TestPlusMinusXAndWarnings(t *testing.T) {
	if Octal(PlusX(0o644)) != "755" || Octal(PlusX(0o600)) != "700" || Octal(PlusX(0o640)) != "750" {
		t.Fatal("PlusX")
	}
	if Octal(MinusX(0o755)) != "644" {
		t.Fatal("MinusX")
	}
	if w := Warnings(0o644, 0o666); len(w) != 1 || !strings.Contains(w[0], "o+w") {
		t.Fatalf("warnings %v", w)
	}
	if w := Warnings(0o644, 0o244); len(w) != 1 || !strings.Contains(w[0], "read") {
		t.Fatalf("warnings %v", w)
	}
	if w := Warnings(0o644, 0o755); len(w) != 0 {
		t.Fatalf("warnings %v", w)
	}
}

func TestRecursiveChmodPlanAndUndo(t *testing.T) {
	f, root := newFS(t)
	site := filepath.Join(root, "www")
	write(t, filepath.Join(site, "index.html"), "")
	write(t, filepath.Join(site, "css", "a.css"), "")
	os.Chmod(filepath.Join(site, "index.html"), 0o600)
	os.Chmod(filepath.Join(site, "css"), 0o700)
	os.Symlink("/etc/passwd", filepath.Join(site, "passwd-link"))

	dm, fm := fs.FileMode(0o755), fs.FileMode(0o644)
	plan, err := PlanChmod([]string{site}, ExactChange(0o755), true, &dm, &fm)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 4 { // www, css, css/a.css, index.html (symlink skipped)
		t.Fatalf("plan = %+v", plan)
	}
	rec, err := f.Chmod(plan)
	if err != nil {
		t.Fatal(err)
	}
	mode := func(p string) string { i, _ := os.Stat(p); return Octal(i.Mode()) }
	if mode(filepath.Join(site, "css")) != "755" || mode(filepath.Join(site, "index.html")) != "644" {
		t.Fatal("chmod not applied")
	}
	if err := f.Undo(rec); err != nil {
		t.Fatal(err)
	}
	if mode(filepath.Join(site, "css")) != "700" || mode(filepath.Join(site, "index.html")) != "600" {
		t.Fatal("undo chmod")
	}
}

func TestUniqueAndDuplicateNames(t *testing.T) {
	_, root := newFS(t)
	write(t, filepath.Join(root, "a.tar.gz"), "")
	write(t, filepath.Join(root, ".bashrc"), "")
	if got := UniqueName(root, "a.tar.gz"); got != "a.tar (2).gz" {
		t.Fatalf("unique = %q", got)
	}
	if got := UniqueName(root, ".bashrc"); got != ".bashrc (2)" {
		t.Fatalf("unique = %q", got)
	}
	if got := DuplicateName(root, ".bashrc"); got != ".bashrc copy" {
		t.Fatalf("dup = %q", got)
	}
}

// Regression: removing x from a folder must not block changing its contents.
func TestRecursiveChmodRemovingFolderExecute(t *testing.T) {
	f, root := newFS(t)
	site := filepath.Join(root, "www")
	write(t, filepath.Join(site, "css", "a.css"), "")
	dm, fm := fs.FileMode(0o755), fs.FileMode(0o644)
	plan, err := PlanChmod([]string{site}, ExactChange(0o640), true, &dm, &fm)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := f.Chmod(plan)
	if err != nil {
		t.Fatalf("chmod: %v", err)
	}
	info, _ := os.Stat(site)
	if Octal(info.Mode()) != "640" {
		t.Fatalf("www = %s", Octal(info.Mode()))
	}
	if err := f.Undo(rec); err != nil {
		t.Fatalf("undo: %v", err)
	}
	info, _ = os.Stat(filepath.Join(site, "css", "a.css"))
	if Octal(info.Mode()) != "644" {
		t.Fatalf("a.css = %s", Octal(info.Mode()))
	}
}
