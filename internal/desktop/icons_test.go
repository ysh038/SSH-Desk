package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tuidesk/internal/apps/files"
	"tuidesk/internal/config"
	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// iconCenter is a point on desktop icon i (its label line).
func (f *fmHarness) iconCenter(i int) (int, int) {
	r := f.d.iconRect(i)
	return r.X + iconW/2, r.Y + 1
}

func (f *fmHarness) withIcons(icons ...config.DesktopIcon) {
	cfg := pcConfig()
	cfg.Icons = icons
	f.d.SetConfig(cfg, "")
	f.withEditor(&fakeRunner{})
}

func TestFileManagerIconView(t *testing.T) {
	f := newFM(t)
	for _, n := range []string{"a.conf", "b.sh", "c.log", "d.txt", "e.txt", "한글.yaml"} {
		f.touch("v/"+n, n)
	}
	os.Mkdir(filepath.Join(f.root, "v", "sub"), 0o755)
	app, w := f.open(filepath.Join(f.root, "v"), wm.Rect{X: 1, Y: 2, W: 60, H: 16})
	in := w.Inner()

	// Mouse: the "Icons" switch on the path row.
	f.draw()
	x, y, ok := findText(f.draw(), " Icons ")
	if !ok {
		t.Fatal("no List|Icons switch")
	}
	f.click(x+1, y)
	f.wait()
	c := f.draw()
	gridY := in.Y + 3
	if !strings.Contains(c.Row(gridY), "[DIR]") || !strings.Contains(c.Row(gridY), "[CFG]") || !strings.Contains(c.Row(gridY+1), "a.conf") {
		t.Fatalf("icon grid:\n%s\n%s", c.Row(gridY), c.Row(gridY+1))
	}
	if !strings.Contains(c.Row(gridY+3), "한글.yaml") {
		t.Fatalf("second grid row: %q", c.Row(gridY+3))
	}
	// 4 columns fit in 58 columns: sub a.conf b.sh c.log / d.txt e.txt 한글.yaml
	if n := strings.Count(strings.SplitN(c.Row(gridY), "│", 3)[1], "["); n != 4 {
		t.Fatalf("columns: %q", c.Row(gridY))
	}

	// Keyboard: Right moves one item, Down one grid row.
	f.key("Right", 0)
	f.key("Down", 0)
	if e := app.Selection(); len(e) != 1 || e[0].Name != "e.txt" {
		t.Fatalf("cursor on %v", e)
	}
	f.key("Right", term.ModShift)
	if s := statusRow(f.draw(), w); !strings.Contains(s, "2 selected") {
		t.Fatalf("status %q", s)
	}

	// Mouse: drag the selected e.txt onto the sub/ cell → Move.
	f.drag(in.X+cellWForTest+3, gridY+3, in.X+3, gridY)
	if !f.d.MenuOpen() {
		t.Fatal("dropping on a folder icon should ask Move/Copy")
	}
	f.clickMenuItem("Move here")
	f.settle()
	if got := strings.Join(f.ls("v/sub"), ","); got != "e.txt,한글.yaml" {
		t.Fatalf("sub = %s", got)
	}

	// Double-click a folder cell opens it; the view stays icons.
	f.click(in.X+3, gridY)
	f.click(in.X+3, gridY)
	f.settle()
	if app.Dir() != filepath.Join(f.root, "v", "sub") {
		t.Fatalf("dir %s", app.Dir())
	}
	if !strings.Contains(f.draw().Row(gridY), "[TXT]") {
		t.Fatalf("icon view should stay on after navigating: %q", f.draw().Row(gridY))
	}

	// Keyboard: View menu switches back to the list.
	f.key("F10", 0)
	for f.d.menu != nil && !strings.Contains(strings.Join(menuLabels(f), "|"), "List view") {
		f.key("Right", 0)
	}
	f.clickMenuItem("List view")
	if !strings.Contains(f.draw().Row(gridY), "e.txt") || strings.Contains(f.draw().Row(gridY), "[TXT]") {
		t.Fatalf("list row %q", f.draw().Row(gridY))
	}
}

const cellWForTest = 14

func menuLabels(f *fmHarness) []string {
	var out []string
	for _, it := range f.d.menu.Items {
		out = append(out, it.Label)
	}
	return out
}

func TestDesktopIconsMouseAndKeyboard(t *testing.T) {
	f := newFM(t)
	f.touch("srv/app.conf", "x")
	os.Mkdir(filepath.Join(f.root, "logs"), 0o755)
	f.withIcons(
		config.DesktopIcon{Name: "Server", Path: filepath.Join(f.root, "srv")},
		config.DesktopIcon{Name: "로그", Path: filepath.Join(f.root, "logs")},
		config.DesktopIcon{Name: "Gone", Path: filepath.Join(f.root, "missing")},
		config.DesktopIcon{Name: "Text editor", App: "editor"},
	)
	c := f.draw()
	r := f.d.iconRect(0)
	if !strings.Contains(c.Row(r.Y), "[DIR]") || !strings.Contains(c.Row(r.Y+1), "Server") ||
		!strings.Contains(c.Row(f.d.iconRect(1).Y+1), "로그") || !strings.Contains(c.Row(f.d.iconRect(2).Y), "[ ? ]") {
		t.Fatalf("icons:\n%s\n%s", c.Row(r.Y), c.Row(r.Y+1))
	}

	// Double-click opens a file manager on the folder.
	x, y := f.iconCenter(0)
	f.click(x, y)
	f.click(x, y)
	f.settle()
	fm, ok := f.d.M.Focused().App.(*files.App)
	if !ok || fm.Dir() != filepath.Join(f.root, "srv") {
		t.Fatalf("focused %v", f.d.M.Focused().Name())
	}
	// A missing path only shows a toast.
	f.wait()
	x, y = f.iconCenter(2)
	f.click(x, y)
	f.click(x, y)
	if !f.screenHas("Not found") {
		t.Fatal("missing target should be reported")
	}

	// Keyboard: F9 › Show desktop, arrows, Enter.
	f.key("F9", 0)
	f.rune('d', 0)
	if f.d.M.Focused() != nil {
		t.Fatal("show desktop should leave no window focused")
	}
	f.key("Down", 0) // the click above left icon 2 selected → 3 (editor)
	if f.d.iconSel != 3 {
		t.Fatalf("selected %d", f.d.iconSel)
	}
	f.key("Enter", 0)
	if _, ok := f.d.M.Focused().App.(interface{ Unsaved() int }); !ok {
		t.Fatalf("Enter on the editor icon should open the editor, focused %q", f.d.M.Focused().Name())
	}

	// Keyboard: Shift+F10 on an icon → Remove from desktop (files stay).
	f.key("F9", 0)
	f.rune('d', 0)
	f.key("Up", 0)
	f.key("Up", 0)
	f.key("F10", term.ModShift)
	f.clickMenuItem("Remove from desktop")
	if len(f.d.cfg.Icons) != 3 || f.d.cfg.Icons[1].Name != "Gone" || !f.d.cfg.IconsEdited {
		t.Fatalf("icons %+v", f.d.cfg.Icons)
	}
	if _, err := os.Stat(filepath.Join(f.root, "logs")); err != nil {
		t.Fatal("removing an icon must not touch the folder")
	}
	// Tab brings a window back.
	f.key("Tab", 0)
	if f.d.M.Focused() == nil {
		t.Fatal("Tab should focus a window again")
	}
}

func TestDropOntoDesktop(t *testing.T) {
	f := newFM(t)
	f.touch("work/a.txt", "A")
	f.touch("work/b.txt", "B")
	os.Mkdir(filepath.Join(f.root, "backup"), 0o755)
	f.withIcons(
		config.DesktopIcon{Name: "Backup", Path: filepath.Join(f.root, "backup")},
		config.DesktopIcon{Name: "Text editor", App: "editor"},
	)
	_, w := f.open(filepath.Join(f.root, "work"), wm.Rect{X: 1, Y: 2, W: 60, H: 14})
	x := w.Inner().X + 4

	// a.txt onto the Backup folder icon → Copy.
	f.click(x, row(w, 0))
	f.wait()
	ix, iy := f.iconCenter(0)
	f.drag(x, row(w, 0), ix, iy)
	if !f.d.MenuOpen() || !strings.Contains(strings.Join(menuLabels(f), "|"), "Copy here") {
		t.Fatal("drop on a folder icon should ask Move/Copy")
	}
	f.clickMenuItem("Copy here")
	f.settle()
	if got := strings.Join(f.ls("backup"), ","); got != "a.txt" {
		t.Fatalf("backup = %s", got)
	}
	if got := strings.Join(f.ls("work"), ","); got != "a.txt,b.txt" {
		t.Fatalf("copy must keep the source: %s", got)
	}

	// b.txt onto empty desktop → Add to desktop.
	f.d.M.Raise(w)
	f.click(x, row(w, 1))
	f.wait()
	f.drag(x, row(w, 1), 75, 20)
	f.clickMenuItem("Add to desktop")
	if n := len(f.d.cfg.Icons); n != 3 || f.d.cfg.Icons[2].Path != filepath.Join(f.root, "work", "b.txt") {
		t.Fatalf("icons %+v", f.d.cfg.Icons)
	}
	if !strings.Contains(f.draw().Row(f.d.iconRect(2).Y), "[TXT]") {
		t.Fatal("new shortcut should be drawn")
	}

	// a.txt onto the editor icon opens it.
	f.d.M.Raise(w)
	f.click(x, row(w, 0))
	f.wait()
	ix, iy = f.iconCenter(1)
	f.drag(x, row(w, 0), ix, iy)
	ed := f.editor()
	if d := ed.Doc(); d == nil || filepath.Base(d.Path) != "a.txt" {
		t.Fatalf("editor doc %+v", d)
	}

	// Keyboard alternative: File › Add to desktop from the file manager.
	f.d.M.Raise(w)
	f.key("F10", 0)
	f.clickMenuItem("Add to desktop")
	if len(f.d.cfg.Icons) != 4 || f.d.cfg.Icons[3].Path != filepath.Join(f.root, "work", "a.txt") {
		t.Fatal("Add to desktop from the menu")
	}
}
