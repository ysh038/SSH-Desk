package fsops

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type member struct {
	name, body, link string
	typ              byte
	mode             int64
}

// makeTar writes a tar (gzipped when gz) built from ms to path.
func makeTar(t *testing.T, path string, gz bool, ms ...member) {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, m := range ms {
		typ := m.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		mode := m.mode
		if mode == 0 {
			mode = 0o644
			if typ == tar.TypeDir {
				mode = 0o755
			}
		}
		h := &tar.Header{Name: m.name, Typeflag: typ, Mode: mode, Size: int64(len(m.body)), Linkname: m.link}
		if typ != tar.TypeReg {
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			if _, err := tw.Write([]byte(m.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	if gz {
		var zb bytes.Buffer
		zw := gzip.NewWriter(&zb)
		zw.Write(data)
		zw.Close()
		data = zb.Bytes()
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTarNames(t *testing.T) {
	for name, want := range map[string]bool{
		"a.tar": true, "a.tar.gz": true, "A.TGZ": true, "한글 백업.tar.gz": true,
		"a.gz": false, "a.zip": false, ".tar": false, "tar": false,
	} {
		if got := IsTarName(name); got != want {
			t.Errorf("IsTarName(%q) = %v", name, got)
		}
	}
	for name, want := range map[string]string{
		"app-1.0.tar.gz": "app-1.0", "x.tgz": "x", "로그.tar": "로그", "plain.txt": "plain.txt",
	} {
		if got := ArchiveStem(name); got != want {
			t.Errorf("ArchiveStem(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestExtractSingleTopFolder(t *testing.T) {
	f, root := newFS(t)
	arc := filepath.Join(root, "app-1.0.tar.gz")
	makeTar(t, arc, true,
		member{name: "app-1.0/", typ: tar.TypeDir},
		member{name: "app-1.0/README", body: "hi"},
		member{name: "app-1.0/bin/run.sh", body: "#!/bin/sh\n", mode: 0o755},
		member{name: "app-1.0/한글 설정.conf", body: "값=1"},
		member{name: "app-1.0/latest", typ: tar.TypeSymlink, link: "README"},
		member{name: "app-1.0/hard", typ: tar.TypeLink, link: "app-1.0/README"},
	)
	rec, err := f.Extract(context.Background(), []string{arc}, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "app-1.0")
	if got := rec.Paths(); !reflect.DeepEqual(got, []string{out}) {
		t.Fatalf("paths %v", got)
	}
	if read(t, filepath.Join(out, "README")) != "hi" || read(t, filepath.Join(out, "한글 설정.conf")) != "값=1" {
		t.Fatal("content")
	}
	if info, _ := os.Stat(filepath.Join(out, "bin", "run.sh")); info.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", info.Mode())
	}
	if l, _ := os.Readlink(filepath.Join(out, "latest")); l != "README" {
		t.Fatalf("symlink %q", l)
	}
	if read(t, filepath.Join(out, "hard")) != "hi" {
		t.Fatal("hard link")
	}
	// No staging folder is left behind.
	if got := names(t, root); !reflect.DeepEqual(got, []string{"app-1.0", "app-1.0.tar.gz"}) {
		t.Fatalf("root %v", got)
	}
	// Undo moves the extracted folder to the trash.
	if err := f.Undo(rec); err != nil {
		t.Fatal(err)
	}
	if exists(out) {
		t.Fatal("undo left the folder")
	}
}

func TestExtractManyTopItemsMakesFolderAndNeverOverwrites(t *testing.T) {
	f, root := newFS(t)
	arc := filepath.Join(root, "로그 백업.tar") // plain tar, no gzip
	makeTar(t, arc, false,
		member{name: "./a.log", body: "a"},
		member{name: "./b.log", body: "b"},
	)
	write(t, filepath.Join(root, "로그 백업", "keep"), "old")
	rec, err := f.Extract(context.Background(), []string{arc}, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "로그 백업 (2)")
	if got := rec.Paths(); !reflect.DeepEqual(got, []string{out}) {
		t.Fatalf("paths %v", got)
	}
	if got := names(t, out); !reflect.DeepEqual(got, []string{"a.log", "b.log"}) {
		t.Fatalf("out %v", got)
	}
	if info, _ := os.Stat(out); info.Mode().Perm() != 0o755 {
		t.Fatalf("folder mode %v", info.Mode())
	}
	if read(t, filepath.Join(root, "로그 백업", "keep")) != "old" {
		t.Fatal("existing folder touched")
	}
}

func TestExtractDetectsGzipByContent(t *testing.T) {
	f, root := newFS(t)
	arc := filepath.Join(root, "x.tar") // gzipped, misnamed
	makeTar(t, arc, true, member{name: "x.txt", body: "x"})
	if _, err := f.Extract(context.Background(), []string{arc}, root, nil); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(root, "x.txt")) != "x" {
		t.Fatal("content")
	}
}

func TestExtractRefusesUnsafeMembers(t *testing.T) {
	cases := map[string][]member{
		"dotdot": {{name: "ok.txt", body: "1"}, {name: "../evil.txt", body: "x"}},
		"through symlink": {
			{name: "d/", typ: tar.TypeDir},
			{name: "d/out", typ: tar.TypeSymlink, link: "../../"},
			{name: "d/out/evil.txt", body: "x"},
		},
		"hard link outside": {{name: "h", typ: tar.TypeLink, link: "../../etc/passwd"}},
	}
	for name, ms := range cases {
		t.Run(name, func(t *testing.T) {
			f, root := newFS(t)
			dst := filepath.Join(root, "dst")
			os.Mkdir(dst, 0o755)
			arc := filepath.Join(root, "bad.tar.gz")
			makeTar(t, arc, true, ms...)
			_, err := f.Extract(context.Background(), []string{arc}, dst, nil)
			if !errors.Is(err, ErrUnsafeMember) {
				t.Fatalf("err = %v", err)
			}
			if got := names(t, dst); len(got) != 0 {
				t.Fatalf("left %v", got)
			}
			if exists(filepath.Join(root, "evil.txt")) {
				t.Fatal("wrote outside")
			}
		})
	}
}

func TestExtractStripsLeadingSlash(t *testing.T) {
	f, root := newFS(t)
	arc := filepath.Join(root, "abs.tar")
	makeTar(t, arc, false, member{name: "/etc/app.conf", body: "c"})
	rec, err := f.Extract(context.Background(), []string{arc}, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(root, "etc", "app.conf")) != "c" || rec.Paths()[0] != filepath.Join(root, "etc") {
		t.Fatal("not extracted under the folder")
	}
}

func TestExtractBrokenArchive(t *testing.T) {
	f, root := newFS(t)
	arc := filepath.Join(root, "broken.tar.gz")
	write(t, arc, "\x1f\x8bnot really gzip")
	if _, err := f.Extract(context.Background(), []string{arc}, root, nil); err == nil {
		t.Fatal("no error")
	}
	if got := names(t, root); !reflect.DeepEqual(got, []string{"broken.tar.gz"}) {
		t.Fatalf("left %v", got)
	}
}

func TestCompressRoundTrip(t *testing.T) {
	f, root := newFS(t)
	src := filepath.Join(root, "프로젝트")
	write(t, filepath.Join(src, "main.go"), "package main")
	write(t, filepath.Join(src, "conf", "한글.toml"), "a = 1")
	os.Chmod(filepath.Join(src, "main.go"), 0o755)
	os.Symlink("main.go", filepath.Join(src, "link"))
	write(t, filepath.Join(root, "notes.txt"), "n")

	srcs := []string{src, filepath.Join(root, "notes.txt")}
	name := CompressName(root, srcs)
	if name != filepath.Base(root)+".tar.gz" {
		t.Fatalf("name %q", name)
	}
	if got := CompressName(root, srcs[:1]); got != "프로젝트.tar.gz" {
		t.Fatalf("one-item name %q", got)
	}
	dst := filepath.Join(root, "out.tar.gz")
	var last Progress
	rec, err := f.Compress(context.Background(), srcs, dst, func(p Progress) { last = p })
	if err != nil {
		t.Fatal(err)
	}
	if last.Done != last.Total || last.Total == 0 {
		t.Fatalf("progress %+v", last)
	}
	if got := rec.Paths(); !reflect.DeepEqual(got, []string{dst}) {
		t.Fatalf("paths %v", got)
	}
	if info, _ := os.Stat(dst); info.Mode().Perm() != 0o644 {
		t.Fatalf("archive mode %v", info.Mode())
	}

	// Unpack it again somewhere else: two top items -> folder "out".
	back := filepath.Join(root, "back")
	os.Mkdir(back, 0o755)
	if _, err := f.Extract(context.Background(), []string{dst}, back, nil); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(back, "out")
	if read(t, filepath.Join(out, "프로젝트", "conf", "한글.toml")) != "a = 1" || read(t, filepath.Join(out, "notes.txt")) != "n" {
		t.Fatal("content")
	}
	if info, _ := os.Stat(filepath.Join(out, "프로젝트", "main.go")); info.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", info.Mode())
	}
	if l, _ := os.Readlink(filepath.Join(out, "프로젝트", "link")); l != "main.go" {
		t.Fatalf("symlink %q", l)
	}
}

func TestCompressRefusesExistingAndCleansUpOnCancel(t *testing.T) {
	f, root := newFS(t)
	write(t, filepath.Join(root, "a.txt"), "a")
	write(t, filepath.Join(root, "a.tar.gz"), "old")
	if _, err := f.Compress(context.Background(), []string{filepath.Join(root, "a.txt")}, filepath.Join(root, "a.tar.gz"), nil); err == nil {
		t.Fatal("overwrote")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := f.Compress(ctx, []string{filepath.Join(root, "a.txt")}, filepath.Join(root, "b.tar.gz"), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if got := names(t, root); !reflect.DeepEqual(got, []string{"a.tar.gz", "a.txt"}) {
		t.Fatalf("left %v", got)
	}
	if read(t, filepath.Join(root, "a.tar.gz")) != "old" {
		t.Fatal("existing archive changed")
	}
}
