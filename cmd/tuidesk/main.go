// Command tuidesk is the TUI desktop environment (SPEC). Lite build: no
// network code; AI/MCP features are added only with -tags full.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"

	editorapp "tuidesk/internal/apps/editor"
	"tuidesk/internal/apps/files"
	"tuidesk/internal/apps/viewer"
	"tuidesk/internal/config"
	"tuidesk/internal/desktop"
	"tuidesk/internal/tui"
	"tuidesk/internal/wm"
)

func main() {
	if run, code := handleArgs(os.Args[1:], os.Stdout, os.Stderr); !run {
		os.Exit(code)
	}
	s, err := tcell.NewScreen()
	if err == nil {
		err = s.Init()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tuidesk: cannot initialize the terminal:", err)
		os.Exit(1)
	}
	s.EnableMouse(tcell.MouseDragEvents)
	s.EnablePaste()
	s.HideCursor()
	s.SetTitle("tuidesk")

	w, h := s.Size()
	host, _ := os.Hostname()
	d := desktop.New(w, h, host, time.Now())
	d.SetConsole(os.Getenv("TERM") == "linux")
	cfgPath := config.Path()
	cfg, _, cfgErr := config.Load(cfgPath)
	d.HomeDir, _ = os.Getwd()
	if d.HomeDir == "" {
		d.HomeDir, _ = os.UserHomeDir()
	}
	d.NewFiles = func(dir string) wm.App { return files.New(d, dir) }
	d.OpenText = func(path string) { openText(d, path) }
	fw := d.OpenWindow(d.NewFiles(d.HomeDir))
	d.M.ResizeTo(fw, min(90, w-4), min(24, h-4))
	d.SetConfig(cfg, cfgPath)
	apps, appErrs := config.LoadApps(config.AppsDir(cfgPath))
	d.SetApps(apps)
	for _, err := range appErrs {
		d.Notify("App manifest skipped: "+err.Error(), true)
	}
	if cfgErr != nil {
		d.ShowError("Settings", "Could not read "+cfgPath+"; using defaults.", cfgErr.Error())
	}
	d.Notify("F9 Menu · F1 Help · Ctrl+Q Quit", false)

	tui.NewLoop(s, d).Run()
	s.Fini()
}

// openText opens path in the editor: a new tab in an existing editor window,
// or a new window. Empty path = new untitled document. Files too large to
// edit open in the read-only viewer.
func openText(d *desktop.Desktop, path string) {
	var ed *editorapp.App
	var win *wm.Window
	for _, w := range d.M.Windows() {
		if e, ok := w.App.(*editorapp.App); ok {
			ed, win = e, w
		}
	}
	if path != "" {
		if info, err := os.Stat(path); err == nil && info.Size() > editorapp.MaxEditBytes {
			if v, err := viewer.Open(path); err == nil {
				d.OpenWindow(v)
				d.Notify("File is too large to edit; opened read-only", true)
			}
			return
		}
	}
	if ed == nil {
		ed = editorapp.New(d)
	}
	if path == "" {
		ed.NewDoc()
	} else if err := ed.Open(path); err != nil {
		d.ShowError("Cannot open file", err.Error(), "")
		if win == nil {
			return
		}
	}
	if win == nil {
		a := d.Area()
		win = d.OpenWindow(ed)
		win.Rect = wm.Rect{X: a.X + 2, Y: a.Y + 1, W: a.W - 4, H: a.H - 2}
		d.M.Clamp(win)
	}
	d.M.Raise(win)
}
