package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tuidesk/internal/apps/terminal"
	"tuidesk/internal/config"
	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

func (f *fmHarness) terminal() (*terminal.App, *wm.Window) {
	for _, w := range f.d.M.Windows() {
		if t, ok := w.App.(*terminal.App); ok {
			return t, w
		}
	}
	return nil, nil
}

// waitScreen pumps posted output until the terminal shows want.
func (f *fmHarness) waitScreen(app *terminal.App, want string) string {
	f.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		f.d.Flush()
		f.draw()
		if s := app.ScreenText(); strings.Contains(s, want) {
			return s
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.t.Fatalf("terminal never showed %q; screen:\n%s", want, app.ScreenText())
	return ""
}

func (f *fmHarness) waitFor(cond func() bool, what string) {
	f.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		f.d.Flush()
		f.draw()
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.t.Fatalf("timed out waiting for %s", what)
}

func (f *fmHarness) shell() (*terminal.App, *wm.Window) {
	f.d.OpenTerminal(f.root, []string{"/bin/sh"}, false)
	app, w := f.terminal()
	if app == nil {
		f.t.Fatal("no terminal window")
	}
	f.t.Cleanup(func() { app.Close() })
	return app, w
}

func TestTerminalRunsShell(t *testing.T) {
	f := newFM(t)
	f.d.SetConfig(pcConfig(), "")
	app, w := f.shell()
	f.typeText("printf '%s\\n' \"$TERM\" 한글-ok")
	f.key("Enter", 0)
	scr := f.waitScreen(app, "한글-ok")
	if !strings.Contains(scr, "xterm-256color") {
		t.Fatalf("TERM:\n%s", scr)
	}
	// The window's inner size is the pty size (SIGWINCH on resize).
	f.d.M.ResizeTo(w, 60, 16)
	f.draw()
	f.typeText("stty size")
	f.key("Enter", 0)
	f.waitScreen(app, "14 58")
	// Every key goes to the program: Ctrl+W does not close the window.
	f.rune('w', term.ModCtrl)
	if _, w2 := f.terminal(); w2 == nil {
		t.Fatal("Ctrl+W must reach the shell, not close the terminal")
	}
	// Ctrl+] opens the terminal menu; Paste sends the clipboard.
	f.d.SetClipboardText("echo pasted-$((40+2))\n")
	f.rune(']', term.ModCtrl)
	if !f.d.MenuOpen() || !strings.Contains(strings.Join(menuLabels(f), "|"), "Next window") {
		t.Fatalf("menu: %v", f.d.menu)
	}
	f.clickMenuItem("Paste")
	f.waitScreen(app, "pasted-42")
	// Mouse selection copies text.
	f.draw()
	in := w.Inner()
	scr = app.ScreenText()
	row := -1
	for i, l := range strings.Split(scr, "\n") {
		if l == "pasted-42" {
			row = i
		}
	}
	if row < 0 {
		t.Fatalf("no output line:\n%s", scr)
	}
	f.drag(in.X, in.Y+row, in.X+8, in.Y+row)
	if got := f.d.ClipboardText(); got != "pasted-42" {
		t.Fatalf("selection copied %q", got)
	}
	// exit closes the window.
	f.typeText("exit")
	f.key("Enter", 0)
	f.waitFor(func() bool { _, w := f.terminal(); return w == nil }, "the window to close after exit")
}

func TestTerminalAsksBeforeClosingRunningProgram(t *testing.T) {
	f := newFM(t)
	f.d.SetConfig(pcConfig(), "")
	app, _ := f.shell()
	f.typeText("sleep 30")
	f.key("Enter", 0)
	f.waitFor(func() bool { return app.Running() != "" }, "sleep to be in the foreground")
	f.rune(']', term.ModCtrl)
	f.clickMenuItem("Close terminal")
	if !f.screenHas("still running") {
		t.Fatal("closing a busy terminal should ask")
	}
	f.key("Enter", 0) // Close
	if _, w := f.terminal(); w != nil {
		t.Fatal("window should be closed")
	}
}

func TestRunExecutableHoldsOutput(t *testing.T) {
	f := newFM(t)
	f.d.SetConfig(pcConfig(), "")
	f.touch("bin/job.sh", "#!/bin/sh\necho job-done\nexit 3\n")
	os.Chmod(filepath.Join(f.root, "bin/job.sh"), 0o755)
	_, fm := f.open(filepath.Join(f.root, "bin"), wm.Rect{X: 1, Y: 2, W: 60, H: 14})
	f.click(fm.Inner().X+4, row(fm, 0))
	f.click(fm.Inner().X+4, row(fm, 0))
	if !f.screenHas("is executable") {
		t.Fatal("double-click on an executable should ask")
	}
	f.key("Enter", 0) // Run in terminal
	app, _ := f.terminal()
	if app == nil {
		t.Fatal("no terminal")
	}
	t.Cleanup(func() { app.Close() })
	f.waitScreen(app, "job-done")
	f.waitFor(func() bool { ok, code := app.Exited(); return ok && code == 3 }, "exit code 3")
	if !f.screenHas("Process exited (code 3)") {
		t.Fatal("exit banner")
	}
	f.key("Enter", 0)
	if _, w := f.terminal(); w != nil {
		t.Fatal("Enter should close a finished terminal")
	}
}

func TestOpenTerminalHereAndAppManifest(t *testing.T) {
	f := newFM(t)
	f.d.SetConfig(pcConfig(), "")
	os.Mkdir(filepath.Join(f.root, "proj"), 0o755)
	_, fm := f.open(f.root, wm.Rect{X: 1, Y: 2, W: 60, H: 14})
	_ = fm
	f.rune('t', term.ModCtrl) // Open terminal here
	app, _ := f.terminal()
	if app == nil {
		t.Fatal("Ctrl+T in the file manager should open a terminal")
	}
	app.Close()
	f.d.CloseApp(app)

	// App manifest with {selected_dir}: the selected folder.
	f.d.SetApps([]config.AppManifest{{Name: "pwd-app", Command: "pwd; sleep 5", Cwd: "{selected_dir}"}})
	f.d.M.Raise(fm)
	f.click(fm.Inner().X+4, row(fm, 0)) // proj/
	f.key("F9", 0)
	if !strings.Contains(strings.Join(menuLabels(f), "|"), "pwd-app") {
		t.Fatalf("launcher %v", menuLabels(f))
	}
	f.clickMenuItem("pwd-app")
	app, w := f.terminal()
	t.Cleanup(func() { app.Close() })
	scr := f.waitScreen(app, "proj")
	if !strings.Contains(w.Name(), "pwd-app") || !strings.Contains(scr, filepath.Base(f.root)) {
		t.Fatalf("title %q screen:\n%s", w.Name(), scr)
	}
}
