package desktop

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// SPEC 7.2 tar: compress a folder from the context menu, open the archive
// with Enter and extract it next to the original, then undo.
func TestCompressAndExtractTar(t *testing.T) {
	f := newFM(t)
	f.touch("data/한글 메모.txt", "안녕")
	f.touch("data/sub/a.conf", "a=1")
	_, w := f.open(f.root, wm.Rect{X: 1, Y: 2, W: 70, H: 16})

	// Right-click the folder -> Compress -> accept the suggested name.
	f.wait()
	f.rclick(w.Inner().X+4, row(w, 0))
	f.clickMenuItem("Compress to .tar.gz...")
	if !f.d.DialogOpen() {
		t.Fatal("name prompt expected")
	}
	f.key("Enter", 0)
	f.settle()
	if got := f.ls(""); !reflect.DeepEqual(got, []string{"data", "data.tar.gz"}) {
		t.Fatalf("after compress: %v", got)
	}

	// The cursor is on the new archive; Enter asks to extract it.
	f.key("Enter", 0)
	if !f.d.DialogOpen() {
		t.Fatal("extract question expected")
	}
	f.key("Enter", 0) // Extract here
	f.settle()
	out := filepath.Join(f.root, "data (2)")
	b, err := os.ReadFile(filepath.Join(out, "한글 메모.txt"))
	if err != nil || string(b) != "안녕" {
		t.Fatalf("extracted content %q, %v", b, err)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "sub", "a.conf")); string(b) != "a=1" {
		t.Fatalf("sub content %q", b)
	}

	// Ctrl+Z moves the extracted folder to the trash.
	f.rune('z', term.ModCtrl)
	f.settle()
	if _, err := os.Lstat(out); err == nil {
		t.Fatal("undo left the extracted folder")
	}
}
