package desktop

import (
	"fmt"
	"os"
	"strings"

	"tuidesk/internal/apps/terminal"
	"tuidesk/internal/config"
	"tuidesk/internal/host"
	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// SetApps installs the external app manifests (SPEC 8.3 step 1).
func (d *Desktop) SetApps(apps []config.AppManifest) { d.apps = apps }

// OpenTerminal opens a terminal window (host.Host).
func (d *Desktop) OpenTerminal(dir string, argv []string, hold bool) {
	d.openTerminal(dir, argv, hold, "")
}

func (d *Desktop) openTerminal(dir string, argv []string, hold bool, name string) *wm.Window {
	if dir == "" {
		dir = d.HomeDir
	}
	t := terminal.New(d, terminal.Spec{Argv: argv, Dir: dir}, name)
	t.Hold = hold
	w := d.OpenWindow(t)
	a := d.Area()
	d.M.ResizeTo(w, min(82, a.W-2), min(26, a.H-1))
	d.Notify("Keys go to the program · "+term.KeyLabel(terminal.MenuKey)+" opens the terminal menu", false)
	return w
}

// runApp starts an app manifest in a terminal window.
func (d *Desktop) runApp(m config.AppManifest) {
	dir := d.HomeDir
	switch cwd := strings.TrimSpace(m.Cwd); {
	case cwd == "{selected_dir}":
		dir = d.selectedDir()
	case cwd != "":
		dir = iconPath(cwd)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		d.Notify("Folder not found: "+d.display(dir), true)
		return
	}
	d.openTerminal(dir, []string{"/bin/sh", "-c", m.Command}, m.Hold, m.Name)
}

// selectedDir is the folder chosen in the focused file manager, else home.
func (d *Desktop) selectedDir() string {
	if f := d.M.Focused(); f != nil {
		if s, ok := f.App.(interface{ SelectedDir() string }); ok {
			return s.SelectedDir()
		}
	}
	return d.HomeDir
}

func (d *Desktop) findApp(name string) (config.AppManifest, bool) {
	for _, a := range d.apps {
		if a.Name == name {
			return a, true
		}
	}
	return config.AppManifest{}, false
}

// DesktopAction lets apps that take every key reach the desktop (host.Host).
func (d *Desktop) DesktopAction(action string) {
	switch action {
	case "launcher":
		d.openLauncher(0, 1)
	case "windowmenu":
		if f := d.M.Focused(); f != nil {
			d.openWindowMenu(f, f.Rect.X+1, f.Rect.Y+1)
		}
	default:
		d.run(action, nil)
	}
}

// running is what the program in window w is still running ("" if nothing
// would be lost by closing it).
func running(w *wm.Window) string {
	if r, ok := w.App.(interface{ Running() string }); ok {
		return r.Running()
	}
	return ""
}

// confirmRunning asks before closing a window whose program still runs.
func (d *Desktop) confirmRunning(w *wm.Window, proceed func()) bool {
	prog := running(w)
	if prog == "" {
		return false
	}
	d.Confirm("Close terminal", fmt.Sprintf("%s is still running in this terminal.\nClosing the window stops it.", d.display(prog)),
		[]string{"Close", "Cancel"}, func(c int) {
			if c == 0 {
				proceed()
			}
		})
	return true
}

var _ host.Host = (*Desktop)(nil)
