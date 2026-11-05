package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"tuidesk/internal/apps/files"
	"tuidesk/internal/config"
	"tuidesk/internal/host"
	"tuidesk/internal/term"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

// Desktop icons (SPEC 7.1): shortcuts to folders, files and apps, defined in
// the settings file and drawn in columns from the right edge of the desktop,
// under the windows. Keyboard: when no window is focused (F9 › Show
// desktop, or everything minimized) the arrow keys select an icon, Enter
// opens it and Shift+F10 opens its menu.

const (
	iconW = 12 // cell width (label up to iconW-1 columns)
	iconH = 3  // icon line, label line, gap
)

// iconRect is the screen cell of icon i.
func (d *Desktop) iconRect(i int) wm.Rect {
	a := d.Area()
	per := max((a.H-1)/iconH, 1)
	col, row := i/per, i%per
	return wm.Rect{X: a.X + a.W - (col+1)*iconW - 1, Y: a.Y + 1 + row*iconH, W: iconW, H: iconH - 1}
}

// iconAt returns the icon under screen (x, y), or -1.
func (d *Desktop) iconAt(x, y int) int {
	for i := range d.cfg.Icons {
		if d.iconRect(i).Contains(x, y) {
			return i
		}
	}
	return -1
}

// iconPath expands "~" in an icon path.
func iconPath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// iconTarget describes what an icon opens.
type iconTarget struct {
	path  string // expanded; "" for apps
	isDir bool
	ok    bool // the path exists (apps: always)
}

func (d *Desktop) target(ic config.DesktopIcon) iconTarget {
	if ic.Path == "" {
		return iconTarget{ok: ic.App != ""}
	}
	p := iconPath(ic.Path)
	info, err := os.Stat(p)
	if err != nil {
		return iconTarget{path: p}
	}
	return iconTarget{path: p, isDir: info.IsDir(), ok: true}
}

func (d *Desktop) iconText(ic config.DesktopIcon) string {
	if ic.Path == "" {
		return "[APP]"
	}
	p := iconPath(ic.Path)
	info, err := os.Lstat(p)
	if err != nil {
		return "[ ? ]"
	}
	isDir := info.IsDir()
	link := info.Mode()&os.ModeSymlink != 0
	if link {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			isDir, link = true, false // a link to a folder works like the folder
		}
	}
	return files.IconText(filepath.Base(p), isDir, link, uint32(info.Mode().Perm()))
}

func (d *Desktop) display(s string) string {
	if d.svc.console {
		return term.EscapeNonASCII(s)
	}
	return s
}

func (d *Desktop) drawIcons(c *wm.Canvas) {
	area := d.Area()
	focused := d.M.Focused() == nil
	for i, ic := range d.cfg.Icons {
		r := d.iconRect(i)
		if r.X < area.X || r.Y+r.H > area.Y+area.H {
			continue // does not fit
		}
		st := wm.StyleDesktop
		switch {
		case i == d.iconHover:
			st = wm.StyleDropTarget
		case i == d.iconSel && focused:
			st = wm.StyleSelected
		case i == d.iconSel:
			st = wm.StyleMarked
		}
		s := wm.NewSurface(c, r)
		icon := d.iconText(ic)
		name := clip(d.display(ic.Name), iconW-1)
		if st != wm.StyleDesktop {
			s.Sub(wm.Rect{X: (iconW - term.Width(icon)) / 2, Y: 0, W: term.Width(icon), H: 1}).Clear(st)
			s.Sub(wm.Rect{X: (iconW - term.Width(name)) / 2, Y: 1, W: term.Width(name), H: 1}).Clear(st)
		}
		s.Text((iconW-term.Width(icon))/2, 0, icon, st)
		s.Text((iconW-term.Width(name))/2, 1, name, st)
	}
}

// openIcon opens icon i: a folder in a new file manager, a file in the
// editor, or an app.
func (d *Desktop) openIcon(i int) {
	if i < 0 || i >= len(d.cfg.Icons) {
		return
	}
	ic := d.cfg.Icons[i]
	t := d.target(ic)
	switch {
	case ic.Path == "":
		switch ic.App {
		case "editor":
			d.run("editor", nil)
		case "files":
			d.openFolder(d.HomeDir)
		case "help":
			d.run("help", nil)
		default:
			d.Notify("Unknown app: "+ic.App, true)
		}
	case !t.ok:
		d.Notify("Not found: "+d.display(t.path), true)
	case t.isDir:
		d.openFolder(t.path)
	case d.OpenText != nil:
		d.OpenText(t.path)
	}
}

// openFolder opens a file manager window on dir.
func (d *Desktop) openFolder(dir string) {
	if d.NewFiles == nil {
		return
	}
	w := d.OpenWindow(d.NewFiles(dir))
	d.M.ResizeTo(w, min(84, d.Area().W-4), min(22, d.Area().H-2))
}

func (d *Desktop) iconMenu(i, x, y int) {
	d.iconSel = i
	ic := d.cfg.Icons[i]
	d.openMenu([]ui.MenuItem{
		{Label: "Open", Hotkey: 'o', Shortcut: "Enter", Action: "open"},
		{Label: "Rename...", Hotkey: 'r', Action: "rename"},
		{Label: "Copy path", Hotkey: 'c', Action: "copypath", Disabled: ic.Path == ""},
		{Separator: true},
		{Label: "Remove from desktop", Hotkey: 'm', Shortcut: "Delete", Action: "remove"},
	}, nil, x, y)
	d.menuFn = func(a string) {
		switch a {
		case "open":
			d.openIcon(i)
		case "rename":
			d.renameIcon(i)
		case "copypath":
			d.SetClipboardText(iconPath(ic.Path))
			d.Notify("Copied path", false)
		case "remove":
			d.removeIcon(i)
		}
	}
}

func (d *Desktop) renameIcon(i int) {
	p := ui.NewPrompt("Rename icon", "Name:", d.cfg.Icons[i].Name, func(v string) {
		if i < len(d.cfg.Icons) {
			d.cfg.Icons[i].Name = strings.TrimSpace(v)
			d.iconsChanged()
		}
	})
	p.Validate = func(v string) error {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("enter a name")
		}
		return nil
	}
	d.Prompt(p)
}

// removeIcon takes a shortcut off the desktop. The file or folder itself is
// not touched.
func (d *Desktop) removeIcon(i int) {
	if i < 0 || i >= len(d.cfg.Icons) {
		return
	}
	name := d.cfg.Icons[i].Name
	d.cfg.Icons = slices.Delete(slices.Clone(d.cfg.Icons), i, i+1)
	d.iconSel = min(d.iconSel, len(d.cfg.Icons)-1)
	d.iconsChanged()
	d.Notify("Removed "+d.display(name)+" from the desktop (the files are not deleted)", false)
}

func (d *Desktop) iconsChanged() {
	d.cfg.IconsEdited = true
	d.saveConfig()
}

// AddToDesktop pins paths as desktop icons (host.Host).
func (d *Desktop) AddToDesktop(paths []string) {
	added := 0
	for _, p := range paths {
		p = filepath.Clean(p)
		dup := false
		for _, ic := range d.cfg.Icons {
			if ic.Path != "" && filepath.Clean(iconPath(ic.Path)) == p {
				dup = true
			}
		}
		if dup {
			continue
		}
		name := filepath.Base(p)
		if home, err := os.UserHomeDir(); err == nil && p == home {
			name = "Home"
		}
		d.cfg.Icons = append(slices.Clone(d.cfg.Icons), config.DesktopIcon{Name: name, Path: p})
		d.iconSel = len(d.cfg.Icons) - 1
		added++
	}
	if added == 0 {
		d.Notify("Already on the desktop", false)
		return
	}
	d.iconsChanged()
	d.Notify(fmt.Sprintf("Added %d item(s) to the desktop", added), false)
}

// showDesktop minimizes every window and gives the keyboard to the icons.
func (d *Desktop) showDesktop() {
	for _, w := range d.M.Windows() {
		d.M.Minimize(w)
	}
	d.M.Unfocus()
	if d.iconSel < 0 && len(d.cfg.Icons) > 0 {
		d.iconSel = 0
	}
}

// handleIconKey handles keys while no window is focused.
func (d *Desktop) handleIconKey(k term.Key) bool {
	n := len(d.cfg.Icons)
	if n == 0 {
		return false
	}
	per := max((d.Area().H-1)/iconH, 1)
	move := func(delta int) {
		if d.iconSel < 0 {
			d.iconSel = 0
			return
		}
		d.iconSel = min(max(d.iconSel+delta, 0), n-1)
	}
	switch k.String() {
	case "Up":
		move(-1)
	case "Down":
		move(1)
	case "Left": // columns grow to the left
		move(per)
	case "Right":
		move(-per)
	case "Home":
		d.iconSel = 0
	case "End":
		d.iconSel = n - 1
	case "Enter":
		d.openIcon(d.iconSel)
	case "Delete":
		d.removeIcon(d.iconSel)
	case "F2":
		if d.iconSel >= 0 {
			d.renameIcon(d.iconSel)
		}
	case "Shift+F10":
		if d.iconSel >= 0 {
			r := d.iconRect(d.iconSel)
			d.iconMenu(d.iconSel, r.X, r.Y+1)
		} else {
			return false
		}
	default:
		return false
	}
	return true
}

// ---- drop onto the desktop ----

// iconDropKind tells what dropping on icon i does: "dir" (move/copy into
// the folder), "editor" (open the files) or "" (pin as shortcuts).
func (d *Desktop) iconDropKind(i int) string {
	if i < 0 || i >= len(d.cfg.Icons) {
		return ""
	}
	ic := d.cfg.Icons[i]
	if ic.Path == "" {
		if ic.App == "editor" {
			return "editor"
		}
		return ""
	}
	if t := d.target(ic); t.ok && t.isDir {
		return "dir"
	}
	return ""
}

// dropOnDesktop: onto a folder icon → Move/Copy popup; onto the editor icon
// → open the files; elsewhere → offer to add them as desktop shortcuts.
func (d *Desktop) dropOnDesktop(dr host.Drag, x, y int) {
	i := d.iconAt(x, y)
	switch d.iconDropKind(i) {
	case "dir":
		dir := d.target(d.cfg.Icons[i]).path
		for _, p := range dr.Paths {
			if p == dir {
				d.Notify("Cannot drop a folder onto itself", true)
				return
			}
		}
		if filepath.Clean(dr.SrcDir) == filepath.Clean(dir) {
			return
		}
		d.openMenu([]ui.MenuItem{
			{Label: "Move here", Hotkey: 'm', Action: "move"},
			{Label: "Copy here", Hotkey: 'c', Action: "copy"},
			{Separator: true},
			{Label: "Cancel", Action: "cancel"},
		}, nil, x, y)
		d.menuFn = func(a string) {
			if a != "cancel" {
				files.Transfer(d, a == "move", dr.Paths, dir)
			}
		}
	case "editor":
		opened := 0
		for _, p := range dr.Paths {
			if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() && d.OpenText != nil {
				d.OpenText(p)
				opened++
			}
		}
		if opened == 0 {
			d.Notify("Only files can be opened in the editor", true)
		}
	default:
		d.openMenu([]ui.MenuItem{
			{Label: "Add to desktop", Hotkey: 'a', Action: "pin"},
			{Separator: true},
			{Label: "Cancel", Action: "cancel"},
		}, nil, x, y)
		d.menuFn = func(a string) {
			if a == "pin" {
				d.AddToDesktop(dr.Paths)
			}
		}
	}
}
