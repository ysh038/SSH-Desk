package desktop

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"tuidesk/internal/apps/files"
	"tuidesk/internal/fsops"
	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// fmHarness extends the desktop harness with real file manager windows on a
// temporary directory (CLAUDE.md rule 6: never touch real paths).
type fmHarness struct {
	*harness
	root string
}

func newFM(t *testing.T) *fmHarness {
	h := newHarness(t, 0)
	root := t.TempDir()
	h.d.SetFS(&fsops.FS{TrashDir: filepath.Join(root, ".trash"), Now: time.Now})
	h.d.NewFiles = func(dir string) wm.App { return files.New(h.d, dir) }
	return &fmHarness{harness: h, root: root}
}

func (f *fmHarness) touch(rel, body string) {
	p := filepath.Join(f.root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fmHarness) open(dir string, r wm.Rect) (*files.App, *wm.Window) {
	app := files.New(f.d, dir)
	w := f.d.OpenWindow(app)
	w.Rect = r
	f.settle()
	return app, w
}

// settle runs posted work until directory loads and background operations finish.
func (f *fmHarness) settle() {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f.d.Flush()
		busy := f.d.Busy() > 0
		for _, w := range f.d.M.Windows() {
			if a, ok := w.App.(*files.App); ok && a.Loading() {
				busy = true
			}
		}
		if !busy {
			f.d.Flush()
			f.draw()
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	f.t.Fatal("timed out waiting for file operations")
}

func (f *fmHarness) ls(rel string) []string {
	des, err := os.ReadDir(filepath.Join(f.root, rel))
	if err != nil {
		f.t.Fatal(err)
	}
	var out []string
	for _, d := range des {
		out = append(out, d.Name())
	}
	sort.Strings(out)
	return out
}

// row returns the screen y of list row i in window w
// (menu bar, path bar and column header come first).
func row(w *wm.Window, i int) int { return w.Inner().Y + 3 + i }

func (f *fmHarness) clickMenuItem(label string) {
	f.t.Helper()
	if f.d.menu == nil {
		f.t.Fatal("no menu open")
	}
	for i, it := range f.d.menu.Items {
		if it.Label == label {
			f.wait()
			f.click(f.d.menu.Rect.X+3, f.d.menu.Rect.Y+1+i)
			f.wait()
			return
		}
	}
	f.t.Fatalf("menu has no %q", label)
}

// SPEC scenario S2, mouse only: box-select several log files, drag them onto
// the backup/ folder in another window, choose Move, then right-click one
// file and rename it.
func TestScenarioS2MouseOnly(t *testing.T) {
	f := newFM(t)
	for _, n := range []string{"access.log", "app.log", "error.log"} {
		f.touch("logs/"+n, n)
	}
	f.touch("logs/notes.txt", "keep me")
	os.Mkdir(filepath.Join(f.root, "backup"), 0o755)

	_, src := f.open(filepath.Join(f.root, "logs"), wm.Rect{X: 1, Y: 2, W: 48, H: 14})
	_, dst := f.open(f.root, wm.Rect{X: 50, Y: 2, W: 48, H: 14})
	x := src.Inner().X + 4

	// 1. Box selection: drag from the first log row to the third.
	f.d.M.Raise(src)
	f.drag(x, row(src, 0), x, row(src, 2))
	if s := statusRow(f.draw(), src); !strings.Contains(s, "3 selected") {
		t.Fatalf("status = %q", s)
	}

	// 2. Drag the selection onto backup/ (row 0 in the other window).
	f.drag(x, row(src, 1), dst.Inner().X+4, row(dst, 0))
	if !f.d.MenuOpen() {
		t.Fatal("drop should open the Move/Copy popup")
	}
	f.clickMenuItem("Move here")
	f.settle()

	if got := strings.Join(f.ls("backup"), ","); got != "access.log,app.log,error.log" {
		t.Fatalf("backup = %s", got)
	}
	if got := strings.Join(f.ls("logs"), ","); got != "notes.txt" {
		t.Fatalf("logs = %s", got)
	}

	// 3. Open backup/ in the second window (double-click), right-click a
	//    file and choose Rename.
	f.d.M.Raise(dst)
	f.click(dst.Inner().X+4, row(dst, 0))
	f.click(dst.Inner().X+4, row(dst, 0))
	f.settle()
	f.wait()
	f.rclick(dst.Inner().X+4, row(dst, 1)) // app.log
	f.clickMenuItem("Rename")
	// The stem "app" starts selected (typing would replace it); → keeps it
	// and puts the cursor before the extension.
	f.key("Right", 0)
	for _, r := range "-2026-11-05" {
		f.rune(r, 0)
	}
	f.key("Enter", 0)
	f.settle()
	if got := strings.Join(f.ls("backup"), ","); got != "access.log,app-2026-11-05.log,error.log" {
		t.Fatalf("after rename backup = %s", got)
	}

	// Undo the rename, then the move.
	f.rune('z', term.ModCtrl)
	f.settle()
	f.rune('z', term.ModCtrl)
	f.settle()
	if got := strings.Join(f.ls("logs"), ","); got != "access.log,app.log,error.log,notes.txt" {
		t.Fatalf("after undo logs = %s", got)
	}
}

func (f *fmHarness) mode(rel string) string {
	info, err := os.Stat(filepath.Join(f.root, rel))
	if err != nil {
		f.t.Fatal(err)
	}
	return fsops.Octal(info.Mode())
}

func (f *fmHarness) typeText(s string) {
	for _, r := range s {
		f.rune(r, 0)
	}
}

func TestKeyboardFileOperations(t *testing.T) {
	f := newFM(t)
	f.touch("a/report.txt", "r")
	f.touch("a/.env", "secret")
	f.touch("a/run.sh", "#!/bin/sh")
	f.touch("b/report.txt", "old")
	_, w := f.open(filepath.Join(f.root, "a"), wm.Rect{X: 1, Y: 2, W: 70, H: 16})

	// Hidden files are off by default; Alt+. shows them.
	c := f.draw()
	if strings.Contains(c.Row(row(w, 0)), ".env") {
		t.Fatal(".env should be hidden")
	}
	f.rune('.', term.ModAlt)
	c = f.draw()
	if !strings.Contains(c.Row(row(w, 0)), ".env") {
		t.Fatalf("Alt+. should show hidden: %q", c.Row(row(w, 0)))
	}
	f.rune('.', term.ModAlt)

	// Typing filters; Esc clears.
	f.typeText("rep")
	c = f.draw()
	if !strings.Contains(c.Row(row(w, 0)), "report.txt") || strings.Contains(c.Row(row(w, 1)), "run.sh") {
		t.Fatalf("filter rows: %q / %q", c.Row(row(w, 0)), c.Row(row(w, 1)))
	}

	// Copy report.txt, paste into b/ where it exists: choose Keep both.
	f.rune('c', term.ModCtrl)
	f.key("Esc", 0)
	app2, _ := f.open(filepath.Join(f.root, "b"), wm.Rect{X: 2, Y: 3, W: 70, H: 16})
	_ = app2
	f.rune('v', term.ModCtrl)
	if !f.d.DialogOpen() {
		t.Fatal("conflict dialog expected")
	}
	f.rune('k', 0) // Keep both
	f.settle()
	if got := strings.Join(f.ls("b"), ","); got != "report (2).txt,report.txt" {
		t.Fatalf("b = %s", got)
	}

	// Paste again and Replace: the old file goes to the trash.
	f.rune('v', term.ModCtrl)
	f.rune('r', 0)
	f.settle()
	data, _ := os.ReadFile(filepath.Join(f.root, "b", "report.txt"))
	if string(data) != "r" {
		t.Fatalf("replace: %q", data)
	}
	trashed, _ := os.ReadDir(filepath.Join(f.root, ".trash", "files"))
	if len(trashed) != 1 {
		t.Fatalf("replaced file should be in the trash, have %d", len(trashed))
	}

	// Delete moves to the trash; Ctrl+Z restores.
	f.key("Home", 0)
	f.key("Delete", 0)
	f.settle()
	if len(f.ls("b")) != 1 {
		t.Fatalf("after trash b = %v", f.ls("b"))
	}
	f.rune('z', term.ModCtrl)
	f.settle()
	if len(f.ls("b")) != 2 {
		t.Fatalf("after undo b = %v", f.ls("b"))
	}

	// Shift+Delete asks twice; cancelling the second keeps the file.
	f.key("Delete", term.ModShift)
	f.key("Enter", 0) // Delete
	f.key("Esc", 0)   // second confirmation: cancel
	if len(f.ls("b")) != 2 {
		t.Fatal("cancel must keep the file")
	}
	f.key("Delete", term.ModShift)
	f.key("Enter", 0)
	f.key("Enter", 0)
	f.settle()
	if len(f.ls("b")) != 1 {
		t.Fatalf("permanent delete: %v", f.ls("b"))
	}
}

func TestNewFolderAndExecutableFromContextMenu(t *testing.T) {
	f := newFM(t)
	f.touch("deploy.sh", "#!/bin/sh")
	_, w := f.open(f.root, wm.Rect{X: 1, Y: 2, W: 70, H: 16})

	// Right-click the empty area: New folder.
	f.rclick(w.Inner().X+5, row(w, 5))
	f.clickMenuItem("New folder")
	if !f.d.DialogOpen() {
		t.Fatal("prompt expected")
	}
	f.rune('u', term.ModCtrl) // clear the suggested name
	f.typeText("releases")
	f.key("Enter", 0)
	f.settle()
	if got := strings.Join(f.ls(""), ","); !strings.Contains(got, "releases") {
		t.Fatalf("root = %s", got)
	}

	// Right-click deploy.sh (row 1, after the folder): Make executable.
	f.rclick(w.Inner().X+5, row(w, 1))
	f.clickMenuItem("Make executable (+x)")
	if got := f.mode("deploy.sh"); got != "755" {
		t.Fatalf("+x → %s", got)
	}
	f.rune('z', term.ModCtrl)
	f.settle()
	if got := f.mode("deploy.sh"); got != "644" {
		t.Fatalf("undo +x → %s", got)
	}
}

func TestPermissionsDialog(t *testing.T) {
	f := newFM(t)
	f.touch("www/index.html", "")
	f.touch("www/css/site.css", "")
	os.Chmod(filepath.Join(f.root, "www", "css"), 0o700)
	os.Chmod(filepath.Join(f.root, "www", "index.html"), 0o600)
	_, w := f.open(f.root, wm.Rect{X: 1, Y: 2, W: 70, H: 16})

	// Keyboard: select www, Alt+Enter, Tab to presets, pick 750 (5th), apply.
	f.key("Home", 0)
	f.key("Enter", term.ModAlt)
	if !f.d.DialogOpen() {
		t.Fatal("permissions dialog expected")
	}
	c := f.draw()
	found := false
	for y := 0; y < c.H; y++ {
		if strings.Contains(c.Row(y), "Permissions: www") {
			found = true
		}
	}
	if !found {
		t.Fatal("dialog title not drawn")
	}
	f.key("Tab", 0)          // octal
	f.key("Tab", 0)          // presets
	for i := 0; i < 5; i++ { // 644 755 600 700 640 [750]
		f.key("Right", 0)
	}
	f.key("Space", 0)
	// Tab to "Apply to contents", tick it, keep 755/644 and apply.
	f.key("Tab", 0)
	f.key("Space", 0)
	f.key("Enter", 0)
	if !f.d.DialogOpen() {
		t.Fatal("recursive apply must ask for confirmation")
	}
	f.key("Enter", 0)
	f.settle()
	if f.mode("www") != "750" || f.mode("www/css") != "755" || f.mode("www/index.html") != "644" {
		t.Fatalf("modes www=%s css=%s index=%s", f.mode("www"), f.mode("www/css"), f.mode("www/index.html"))
	}
	f.rune('z', term.ModCtrl)
	f.settle()
	if f.mode("www/css") != "700" || f.mode("www/index.html") != "600" {
		t.Fatal("undo recursive chmod")
	}

	// Mouse: open from the context menu and toggle Others/Write (o+w → warning).
	f.rclick(w.Inner().X+5, row(w, 0))
	f.clickMenuItem("Permissions...")
	x0, y0, ok := findText(f.draw(), "┌─ Permissions")
	if !ok {
		t.Fatal("dialog not found")
	}
	dlg := wm.Rect{X: x0, Y: y0}
	// Others row is gridY+2, Write column is gridX+8.
	f.click(dlg.X+21+8+1, dlg.Y+2+2)
	f.wait()
	f.key("Enter", 0) // apply (focus is on the grid)
	c = f.draw()
	warned := false
	for y := 0; y < c.H; y++ {
		if strings.Contains(c.Row(y), "everyone can write") {
			warned = true
		}
	}
	if !warned {
		t.Fatal("o+w should warn before applying")
	}
	f.key("Esc", 0)
	if f.mode("www") != "755" {
		t.Fatalf("cancelled change applied: %s", f.mode("www"))
	}
}

func TestConsoleShowsEscapedHangulNames(t *testing.T) {
	f := newFM(t)
	f.touch("한글 로그.log", "")
	f.d.SetConsole(true)
	_, w := f.open(f.root, wm.Rect{X: 1, Y: 2, W: 70, H: 10})
	c := f.draw()
	want := string(rune(92)) + "uD55C" + string(rune(92)) + "uAE00"
	if !strings.Contains(c.Row(row(w, 0)), want) {
		t.Fatalf("row = %q", c.Row(row(w, 0)))
	}
}

func TestLargeFolderLoadsAsync(t *testing.T) {
	if testing.Short() {
		t.Skip("creates 20k files")
	}
	f := newFM(t)
	dir := filepath.Join(f.root, "big")
	os.Mkdir(dir, 0o755)
	for i := 0; i < 20000; i++ {
		fh, err := os.Create(filepath.Join(dir, "f"+strings.Repeat("0", 5-len(itoa(i)))+itoa(i)+".log"))
		if err != nil {
			t.Fatal(err)
		}
		fh.Close()
	}
	start := time.Now()
	app := files.New(f.d, dir)
	if !app.Loading() {
		t.Fatal("load must be asynchronous")
	}
	w := f.d.OpenWindow(app)
	w.Rect = wm.Rect{X: 1, Y: 2, W: 70, H: 20}
	f.settle()
	loaded := time.Since(start)
	f.key("End", 0)
	drawStart := time.Now()
	c := f.draw()
	drawTime := time.Since(drawStart)
	if !strings.Contains(statusRow(c, w), "20000 items") {
		t.Fatalf("status %q", statusRow(c, w))
	}
	t.Logf("20k entries: load %v, draw after End %v", loaded, drawTime)
	if drawTime > 50*time.Millisecond {
		t.Fatalf("drawing must not depend on folder size: %v", drawTime)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

// findText returns the screen column and row where text starts.
func findText(c *wm.Canvas, text string) (int, int, bool) {
	want := []rune(text)
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			cx, i := x, 0
			for ; i < len(want) && cx < c.W; i++ {
				cell := c.At(cx, y)
				if cell.Rune != want[i] {
					break
				}
				cx += int(max(cell.Width, 1))
			}
			if i == len(want) {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

func (f *fmHarness) modClick(x, y int, mod term.Mod) {
	f.mouse(x, y, term.ButtonLeft, mod)
	f.mouse(x, y, term.ButtonNone, mod)
	f.wait()
}

// statusRow is above the hint bar.
func statusRow(c *wm.Canvas, w *wm.Window) string { return c.Row(w.Inner().Y + w.Inner().H - 2) }

func TestMultiSelectWindowsAndMacStyles(t *testing.T) {
	f := newFM(t)
	for _, n := range []string{"a.log", "b.log", "c.log", "d.log", "e.log"} {
		f.touch("x/"+n, n)
	}
	_, w := f.open(filepath.Join(f.root, "x"), wm.Rect{X: 1, Y: 2, W: 70, H: 14})
	x := w.Inner().X + 4

	// Ctrl+click (Windows) and Option+click (Mac, arrives as Alt) toggle rows.
	f.click(x, row(w, 0))
	f.wait()
	f.modClick(x, row(w, 2), term.ModCtrl)
	f.modClick(x, row(w, 4), term.ModAlt)
	if s := statusRow(f.draw(), w); !strings.Contains(s, "3 selected") {
		t.Fatalf("status %q", s)
	}
	f.modClick(x, row(w, 2), term.ModCtrl) // toggle off
	if s := statusRow(f.draw(), w); !strings.Contains(s, "2 selected") {
		t.Fatalf("status %q", s)
	}

	// Shift+Down extends from the anchor (keyboard, Windows and Mac).
	f.key("Home", 0)
	f.key("Down", term.ModShift)
	f.key("Down", term.ModShift)
	if s := statusRow(f.draw(), w); !strings.Contains(s, "3 selected") {
		t.Fatalf("shift range status %q", s)
	}
}

func TestTerminalPasteInFileListPastesFiles(t *testing.T) {
	f := newFM(t)
	f.touch("src/report.txt", "r")
	os.Mkdir(filepath.Join(f.root, "dst"), 0o755)
	f.open(filepath.Join(f.root, "src"), wm.Rect{X: 1, Y: 2, W: 70, H: 14})
	f.rune('c', term.ModCtrl)
	f.open(filepath.Join(f.root, "dst"), wm.Rect{X: 2, Y: 3, W: 70, H: 14})
	// Windows Terminal sends Ctrl+V (and a Mac sends Cmd+V) as pasted text.
	f.d.Handle(term.PasteEvent{Text: "whatever was on the system clipboard"})
	f.settle()
	if got := strings.Join(f.ls("dst"), ","); got != "report.txt" {
		t.Fatalf("dst = %s", got)
	}
}

func TestNewWindowOpensFileManagerAtRoot(t *testing.T) {
	f := newFM(t)
	f.touch("etc/nginx.conf", "")
	f.d.RootDir = f.root
	f.rune('n', term.ModCtrl)
	f.settle()
	a, ok := f.d.M.Focused().App.(*files.App)
	if !ok || a.Dir() != f.root {
		t.Fatalf("new window = %T", f.d.M.Focused().App)
	}
	if !strings.Contains(f.draw().Row(row(f.d.M.Focused(), 0)), "etc/") {
		t.Fatal("root listing not shown")
	}
}

func TestMenuBarPaletteAndHints(t *testing.T) {
	f := newFM(t)
	f.touch("d/.secret", "")
	f.touch("d/a.txt", "")
	_, w := f.open(filepath.Join(f.root, "d"), wm.Rect{X: 1, Y: 2, W: 80, H: 16})
	c := f.draw()
	if bar := c.Row(w.Inner().Y); !strings.Contains(bar, "File") || !strings.Contains(bar, "View") {
		t.Fatalf("menu bar = %q", bar)
	}
	if hints := c.Row(w.Inner().Y + w.Inner().H - 1); !strings.Contains(hints, "F2 Rename") || !strings.Contains(hints, "Ctrl+K Commands") {
		t.Fatalf("hint bar = %q", hints)
	}

	// F10 opens File; Right moves to Edit.
	f.key("F10", 0)
	if !f.d.MenuOpen() || f.d.menu.Items[0].Label != "Open" {
		t.Fatal("F10 should open the File menu")
	}
	f.key("Right", 0)
	if !f.d.MenuOpen() || f.d.menu.Items[0].Label != "Undo" {
		t.Fatalf("Right should open Edit, got %q", f.d.menu.Items[0].Label)
	}
	f.key("Esc", 0)

	// Clicking "View" on the bar opens it.
	vx, vy, ok := findText(f.draw(), " View ")
	if !ok {
		t.Fatal("View not on the bar")
	}
	f.click(vx+2, vy)
	if !f.d.MenuOpen() || f.d.menu.Items[0].Label != "Refresh" {
		t.Fatal("click should open View")
	}
	f.key("Esc", 0)

	// Ctrl+K command search: "hidden" + Enter shows dotfiles.
	f.rune('k', term.ModCtrl)
	if !f.d.DialogOpen() {
		t.Fatal("palette expected")
	}
	f.typeText("hidden")
	f.key("Enter", 0)
	if f.d.DialogOpen() || !strings.Contains(f.draw().Row(row(w, 0)), ".secret") {
		t.Fatal("palette should run Show / hide hidden files")
	}

	// Mac labels in the menu.
	defer term.SetLabelStyle(term.LabelPC)
	term.SetLabelStyle(term.LabelMac)
	f.key("F10", 0)
	f.key("Right", 0) // Edit
	found := false
	for y := 0; y < f.d.H; y++ {
		if strings.Contains(f.draw().Row(y), "⌃Z") {
			found = true
		}
	}
	if !found {
		t.Fatal("Edit menu should show ⌃Z on a Mac keyboard")
	}
}
