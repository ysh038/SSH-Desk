// Package mypc is the My PC window (SPEC 7.8, ADR 0005): the places, drives
// and removable devices of this server, with Mount, Unmount and Eject so
// nobody has to type lsblk, mount and umount.
package mypc

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"

	"tuidesk/internal/devices"
	"tuidesk/internal/fsops"
	"tuidesk/internal/host"
	"tuidesk/internal/sudo"
	"tuidesk/internal/term"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

// Deps is everything the window needs from the machine; tests replace it.
type Deps struct {
	Source devices.Source
	Run    sudo.Runner // runs sudo
	Usage  func(dir string) (total, free int64, err error)
	// Exists reports that a mount folder name is taken.
	Exists func(path string) bool
	// MediaBase is where volumes are mounted ("" = /media/<user>).
	MediaBase string
	User      string
	UID, GID  int
	Home      string
}

// SystemDeps reads the real machine as the current user.
func SystemDeps() Deps {
	d := Deps{Source: devices.System(), Run: sudo.Exec, Usage: devices.Usage, Exists: dirTaken, User: "user"}
	if u, err := user.Current(); err == nil {
		d.User, d.Home = u.Username, u.HomeDir
		d.UID, _ = strconv.Atoi(u.Uid)
		d.GID, _ = strconv.Atoi(u.Gid)
	}
	if d.Home == "" {
		d.Home, _ = os.UserHomeDir()
	}
	return d
}

func (a *App) mediaBase() string {
	if a.dep.MediaBase != "" {
		return a.dep.MediaBase
	}
	return devices.MediaBase(a.dep.User)
}

// dirTaken: anything but a missing path or an empty folder (a leftover
// mount folder is reused).
func dirTaken(p string) bool {
	fi, err := os.Lstat(p)
	if err != nil {
		return false
	}
	if !fi.IsDir() {
		return true
	}
	es, err := os.ReadDir(p)
	return err != nil || len(es) > 0
}

type usage struct{ total, free int64 }

// row is one line of the window: a section header, a note, a place or a
// volume.
type row struct {
	header string
	note   string
	place  string // folder of a Places row
	label  string // name of a Places row
	vol    int    // index into App.vols, or -1
}

func (r row) selectable() bool { return r.place != "" || r.vol >= 0 }

// App is the My PC window.
type App struct {
	h   host.Host
	dep Deps

	vols    []devices.Volume
	usage   map[string]usage // by mount point
	rows    []row
	sel     int
	offset  int
	bodyH   int
	vw      int
	showAll bool // also list loop images and swap
	loaded  bool
	loading bool
	err     error
	gen     int

	cmds   ui.Commands
	chrome *host.Chrome
}

// New opens the window and starts reading the devices.
func New(h host.Host, dep Deps) *App {
	a := &App{h: h, dep: dep}
	a.buildCommands()
	a.chrome = host.NewChrome(h, a, &a.cmds)
	a.rebuild()
	a.reload()
	return a
}

func (a *App) Title() string       { return "My PC" }
func (a *App) MinSize() (int, int) { return 40, 8 }
func (a *App) Close() error        { a.gen++; return nil }

// Loading reports a device scan in progress (tests).
func (a *App) Loading() bool { return a.loading }

// Volumes returns the last device list (tests).
func (a *App) Volumes() []devices.Volume { return a.vols }

func (a *App) display(s string) string {
	if a.h.Console() {
		return term.EscapeNonASCII(s)
	}
	return s
}

// Refresh reads the devices again (host.Refresher: free space changes
// after file operations).
func (a *App) Refresh() { a.reload() }

func (a *App) reload() {
	a.gen++
	gen := a.gen
	a.loading = true
	var vols []devices.Volume
	use := map[string]usage{}
	a.h.Go("Reading devices", func(ctx context.Context, _ func(fsops.Progress)) error {
		var err error
		vols, err = a.dep.Source.List(ctx)
		for _, v := range vols {
			// Not for network mounts: statfs on a dead server never returns.
			if !v.Mounted() || (v.Kind != devices.Fixed && v.Kind != devices.Removable) {
				continue
			}
			if t, f, e := a.dep.Usage(v.MountPoint()); e == nil && t > 0 {
				use[v.MountPoint()] = usage{t, f}
			}
		}
		return err
	}, func(err error) {
		if gen != a.gen {
			return
		}
		keep := a.selKey() // before the volume indexes change
		a.loading, a.loaded, a.err = false, true, err
		if err == nil {
			a.vols, a.usage = vols, use
		}
		a.rebuildKeeping(keep)
	})
}

// key identifies a row across reloads: the device survives mounting and
// unmounting.
func (a *App) key(r row) string {
	switch {
	case r.place != "":
		return "place:" + r.place
	case r.vol >= 0 && r.vol < len(a.vols):
		return "vol:" + a.vols[r.vol].Dev
	}
	return ""
}

func (a *App) selKey() string {
	if a.sel >= 0 && a.sel < len(a.rows) {
		return a.key(a.rows[a.sel])
	}
	return ""
}

// rebuild lays out the rows and keeps the selection on the same thing.
func (a *App) rebuild() { a.rebuildKeeping(a.selKey()) }

func (a *App) rebuildKeeping(keep string) {
	rows := []row{{header: "Places", vol: -1}}
	place := func(label, path string) {
		if path != "" {
			rows = append(rows, row{place: path, label: label, vol: -1})
		}
	}
	place("Home", a.dep.Home)
	place("Root", "/")
	if fs := a.h.FS(); fs != nil && fs.TrashDir != "" {
		if _, err := os.Stat(fs.TrashDir + "/files"); err == nil {
			place("Trash", fs.TrashDir+"/files")
		}
	}
	section := func(title string, kind devices.Kind, empty string) {
		start := len(rows)
		for i, v := range a.vols {
			if v.Kind == kind {
				rows = append(rows, row{vol: i})
			}
		}
		if len(rows) == start && empty == "" {
			return
		}
		head := []row{{vol: -1}, {header: title, vol: -1}}
		if len(rows) == start {
			head = append(head, row{note: empty, vol: -1})
		}
		rows = append(rows[:start], append(head, rows[start:]...)...)
	}
	switch {
	case a.err != nil:
		rows = append(rows, row{vol: -1}, row{note: "Cannot read the devices: " + a.err.Error(), vol: -1})
	case !a.loaded:
		rows = append(rows, row{vol: -1}, row{note: "Reading devices...", vol: -1})
	default:
		section("Drives", devices.Fixed, "")
		section("Removable devices", devices.Removable, "None. Plug in a USB drive, then press "+term.KeyLabel("F5")+".")
		section("Network", devices.Network, "")
		if a.showAll {
			section("System volumes", devices.Virtual, "")
		}
	}
	a.rows = rows
	a.sel = -1
	for i, r := range rows {
		if r.selectable() && keep != "" && a.key(r) == keep {
			a.sel = i
			break
		}
	}
	if a.sel < 0 {
		a.sel = 0
		a.move(1) // first selectable row
	}
}

// move steps the selection by delta selectable rows.
func (a *App) move(delta int) {
	dir := 1
	if delta < 0 {
		dir, delta = -1, -delta
	}
	i := a.sel
	for delta > 0 {
		j := i + dir
		for j >= 0 && j < len(a.rows) && !a.rows[j].selectable() {
			j += dir
		}
		if j < 0 || j >= len(a.rows) {
			break
		}
		i = j
		delta--
	}
	if i >= 0 && i < len(a.rows) && a.rows[i].selectable() {
		a.sel = i
	}
	a.ensureVisible()
}

func (a *App) ensureVisible() {
	h := max(a.bodyH, 1)
	if a.sel < a.offset+1 { // keep the section header in view too
		a.offset = max(a.sel-1, 0)
	}
	if a.sel >= a.offset+h {
		a.offset = a.sel - h + 1
	}
	a.offset = min(max(a.offset, 0), max(len(a.rows)-h, 0))
}

// current returns the selected row.
func (a *App) current() (row, bool) {
	if a.sel < 0 || a.sel >= len(a.rows) || !a.rows[a.sel].selectable() {
		return row{vol: -1}, false
	}
	return a.rows[a.sel], true
}

// volume returns the selected volume.
func (a *App) volume() (devices.Volume, bool) {
	r, ok := a.current()
	if !ok || r.vol < 0 || r.vol >= len(a.vols) {
		return devices.Volume{}, false
	}
	return a.vols[r.vol], true
}

// path is the folder behind the selected row ("" for an unmounted volume).
func (a *App) path() string {
	r, ok := a.current()
	if !ok {
		return ""
	}
	if r.place != "" {
		return r.place
	}
	v, _ := a.volume()
	return v.MountPoint()
}

// ---- events ----

func (a *App) HandleEvent(ev term.Event) bool {
	switch ev := ev.(type) {
	case term.KeyEvent:
		return a.handleKey(ev.Key)
	case term.MouseEvent:
		return a.handleMouse(ev.Gesture)
	}
	return false
}

func (a *App) handleKey(k term.Key) bool {
	page := max(a.bodyH-2, 1)
	switch k.String() {
	case "Up":
		a.move(-1)
	case "Down":
		a.move(1)
	case "PgUp":
		a.move(-page)
	case "PgDn":
		a.move(page)
	case "Home", "Ctrl+Home", "Alt+Up":
		a.move(-len(a.rows))
	case "End", "Ctrl+End", "Alt+Down":
		a.move(len(a.rows))
	case "Shift+F10":
		a.contextMenu(4, 2+a.sel-a.offset)
	default:
		return a.chrome.HandleKey(k)
	}
	return true
}

// rowAt returns the row index at local y (-1 outside the list).
func (a *App) rowAt(y int) int {
	if y < 1 || y >= 1+a.bodyH {
		return -1
	}
	if i := a.offset + y - 1; i < len(a.rows) {
		return i
	}
	return -1
}

func (a *App) handleMouse(g term.Gesture) bool {
	switch g.Kind {
	case term.GestureWheel:
		switch g.Wheel {
		case term.WheelUp:
			a.offset -= 3
		case term.WheelDown:
			a.offset += 3
		}
		a.offset = min(max(a.offset, 0), max(len(a.rows)-max(a.bodyH, 1), 0))
		return true
	case term.GesturePress:
		if i := a.rowAt(g.Y); i >= 0 && a.rows[i].selectable() {
			a.sel = i
		}
		return true
	case term.GestureClick:
		if g.Y == 0 {
			return g.Button == term.ButtonLeft && a.chrome.ClickBar(g.X)
		}
		i := a.rowAt(g.Y)
		if i < 0 || !a.rows[i].selectable() {
			return true
		}
		a.sel = i
		switch {
		case g.Button == term.ButtonRight:
			a.contextMenu(g.X, g.Y)
		case g.Button == term.ButtonLeft && g.Count == 2:
			a.open()
		}
		return true
	}
	return false
}

func (a *App) contextMenu(x, y int) {
	a.h.OpenMenu(a, x, y, a.cmds.MenuItems("Drive"), func(id string) { a.cmds.Run(id) })
}

func (a *App) buildCommands() {
	vol := func(pred func(devices.Volume) bool) func() bool {
		return func() bool {
			v, ok := a.volume()
			return ok && pred(v)
		}
	}
	hasRow := func() bool { _, ok := a.current(); return ok }
	hasPath := func() bool { return a.path() != "" }
	c := func(id, menu, title, hint string, keys []string, en func() bool, run func()) *ui.Command {
		return &ui.Command{ID: id, Menu: menu, Title: title, Hint: hint, Keys: keys, Enabled: en, Run: run}
	}
	sep := func(menu string) *ui.Command { return &ui.Command{Menu: menu, Title: "-"} }
	withVol := func(fn func(devices.Volume)) func() {
		return func() {
			if v, ok := a.volume(); ok {
				fn(v)
			}
		}
	}
	a.cmds.Add(
		c("open", "Drive", "Open", "Open", []string{"Enter"}, hasRow, a.open),
		sep("Drive"),
		c("mount", "Drive", "Mount", "", nil, vol(devices.Volume.Mountable), withVol(func(v devices.Volume) { a.mount(v, false, nil) })),
		c("mountro", "Drive", "Mount read-only", "", nil, vol(devices.Volume.Mountable), withVol(func(v devices.Volume) { a.mount(v, true, nil) })),
		c("unmount", "Drive", "Unmount", "Unmount", []string{"Ctrl+U"}, vol(devices.Volume.CanUnmount), withVol(a.unmount)),
		c("eject", "Drive", "Eject (safe to remove)", "Eject", []string{"Ctrl+E"}, vol(func(v devices.Volume) bool { return v.Ejectable }), withVol(a.eject)),
		sep("Drive"),
		c("props", "Drive", "Properties...", "", []string{"Ctrl+P", "Alt+Enter"}, vol(func(devices.Volume) bool { return true }), withVol(a.properties)),
		c("copypath", "Drive", "Copy path", "", nil, hasPath, func() {
			a.h.SetClipboardText(a.path())
			a.h.Notify("Copied "+a.display(a.path()), false)
		}),
		c("terminal", "Drive", "Open terminal here", "", []string{"Ctrl+T"}, hasPath, func() { a.h.OpenTerminal(a.path(), nil, false) }),

		c("refresh", "View", "Refresh", "Refresh", []string{"F5"}, nil, a.reload),
		c("system", "View", "Show / hide system volumes", "", nil, nil, func() {
			a.showAll = !a.showAll
			a.rebuild()
		}),
		sep("View"),
		c("palette", "View", "Command search...", "Commands", []string{"Ctrl+K"}, nil, func() { a.h.ShowPalette(&a.cmds) }),
	)
	a.cmds.ByID("palette").HintPinned = true
}

// ---- actions ----

// open shows the selected place or volume in a file window, mounting the
// volume first when needed.
func (a *App) open() {
	r, ok := a.current()
	if !ok {
		return
	}
	if r.place != "" {
		a.h.OpenFolder(r.place)
		return
	}
	v, _ := a.volume()
	name := a.display(v.Title())
	switch {
	case v.Mounted():
		a.h.OpenFolder(v.MountPoint())
	case v.Mountable():
		a.mount(v, false, a.h.OpenFolder)
	case v.FSType == "":
		a.h.Notify(name+" has no filesystem: it must be formatted before it can hold files", true)
	default:
		a.h.Notify(name+" ("+v.FSType+") cannot be opened as files", true)
	}
}

// mount mounts v under /media/<user>/ with sudo and calls then with the
// folder.
func (a *App) mount(v devices.Volume, readOnly bool, then func(dir string)) {
	if !v.Mountable() {
		return
	}
	dir := devices.UniqueDir(a.mediaBase(), devices.MountDirName(v), a.dep.Exists)
	opts := devices.MountOptions(v.FSType, a.dep.UID, a.dep.GID, readOnly)
	name := a.display(v.Title())
	host.RunSudo(a.h, a.dep.Run, a.dep.User, "Mounting "+name, [][]string{devices.MountCommand(v.Dev, dir, opts)}, func(out []byte, err error) {
		if err != nil {
			a.fail("Cannot mount "+name, name, out, err)
			return
		}
		msg := "Mounted " + name + " at " + a.display(dir)
		switch {
		case readOnly:
			msg += " (read-only)"
		case devices.HasOwners(v.FSType):
			msg += " (" + v.FSType + ": files keep their own owners)"
		}
		a.h.Notify(msg, false)
		a.h.FilesChanged()
		if then != nil {
			then(dir)
		}
	})
}

// release makes our own windows let go of dirs. It returns false (after
// telling the user) when a window cannot simply move out.
func (a *App) release(name string, dirs []string) bool {
	leavers, blocked := host.DirUsers(a.h.Apps(), dirs)
	if len(blocked) > 0 {
		a.h.ShowError("Cannot unmount "+name, name+" is still in use here:\n- "+strings.Join(blocked, "\n- ")+"\nClose them, then try again.", "")
		return false
	}
	for _, l := range leavers {
		for _, d := range dirs {
			l.LeaveDir(d)
		}
	}
	return true
}

func (a *App) unmountCommands(vs []devices.Volume) (cmds [][]string, dirs []string) {
	base := a.mediaBase()
	for _, v := range vs {
		for _, m := range v.Mounts {
			dirs = append(dirs, m)
			cmds = append(cmds, devices.UnmountCommand(m, base))
		}
	}
	return cmds, dirs
}

// unmount unmounts v. Protected volumes are never offered.
func (a *App) unmount(v devices.Volume) {
	if !v.CanUnmount() {
		return
	}
	name := a.display(v.Title())
	cmds, dirs := a.unmountCommands([]devices.Volume{v})
	if !a.release(name, dirs) {
		return
	}
	host.RunSudo(a.h, a.dep.Run, a.dep.User, "Unmounting "+name+" - do not remove the drive", cmds, func(out []byte, err error) {
		a.h.FilesChanged()
		if err != nil {
			a.fail("Cannot unmount "+name, name, out, err)
			return
		}
		a.h.Notify("Unmounted "+name, false)
	})
}

// eject unmounts every volume on v's disk and powers the disk off.
func (a *App) eject(v devices.Volume) {
	if !v.Ejectable {
		return
	}
	name := a.display(v.Title())
	if v.Model != "" {
		name = a.display(v.Model)
	}
	cmds, dirs := a.unmountCommands(devices.Siblings(a.vols, v))
	if !a.release(name, dirs) {
		return
	}
	if off, ok := devices.PowerOffCommand(v.DiskName); ok {
		cmds = append(cmds, off)
	}
	host.RunSudo(a.h, a.dep.Run, a.dep.User, "Ejecting "+name+" - do not remove the drive yet", cmds, func(out []byte, err error) {
		a.h.FilesChanged()
		if err != nil {
			a.fail("Cannot eject "+name, name, out, err)
			return
		}
		a.h.Notify("Safe to remove "+name, false)
	})
}

// fail explains a failed sudo command.
func (a *App) fail(title, name string, out []byte, err error) {
	if err == context.Canceled {
		a.h.Notify("Cancelled", true)
		return
	}
	text := strings.TrimSpace(string(out))
	msg := sudo.Explain(out, a.dep.User)
	switch {
	case msg != "":
	case strings.Contains(text, "target is busy"):
		msg = name + " is in use. Close the programs and files that use it, then try again."
	case strings.Contains(text, "unknown filesystem type"):
		msg = "This server cannot read that filesystem (the driver is not installed)."
	case text != "":
		msg = text[strings.LastIndexByte(text, '\n')+1:]
	default:
		msg = err.Error()
	}
	a.h.ShowError(title, msg, strings.TrimSpace(text+"\n"+err.Error()))
}

func (a *App) properties(v devices.Volume) {
	var b strings.Builder
	line := func(k, val string) {
		if val != "" {
			fmt.Fprintf(&b, "%-11s %s\n", k+":", a.display(val))
		}
	}
	line("Device", v.Dev)
	line("Disk", v.Model)
	line("Connection", v.Tran)
	line("Filesystem", v.FSType)
	line("Label", v.Label)
	line("UUID", v.UUID)
	if v.Size > 0 {
		line("Size", ui.HumanSize(v.Size))
	}
	line("Mounted at", strings.Join(v.Mounts, ", "))
	line("Options", v.Options)
	if u, ok := a.usage[v.MountPoint()]; ok {
		line("Used", ui.HumanSize(u.total-u.free)+" of "+ui.HumanSize(u.total))
		line("Free", ui.HumanSize(u.free))
	}
	if v.Protected {
		line("Note", "used by the system: cannot be unmounted here")
	}
	a.h.Confirm("Properties - "+a.display(v.Title()), strings.TrimRight(b.String(), "\n"), []string{"Close"}, func(int) {})
}

// ---- drawing ----

const (
	devW = 14
	fsW  = 8
	barW = 12
)

// status describes a volume's state for the last column.
func (a *App) status(v devices.Volume) string {
	ro := ""
	if v.ReadOnly && v.Mounted() {
		ro = "  read-only"
	}
	switch {
	case v.Swap:
		return "Swap"
	case v.Mounted():
		u, ok := a.usage[v.MountPoint()]
		if !ok {
			return "Mounted at " + a.display(v.MountPoint()) + ro
		}
		used := float64(u.total-u.free) / float64(u.total)
		s := fmt.Sprintf("%s %3d%%", devices.UsageBar(barW, used), int(used*100+0.5))
		if used >= 0.9 {
			s += " almost full"
		}
		s += "  " + ui.HumanSize(u.free) + " free of " + ui.HumanSize(u.total)
		if v.Label != "" { // the name column shows the label, not the folder
			s += "  " + a.display(v.MountPoint())
		}
		return s + ro
	case v.FSType == "":
		return "No filesystem"
	case !v.Mountable():
		return "Cannot be mounted here"
	}
	return "Not mounted"
}

func (a *App) line(r row, w int) string {
	nameW := min(max(w-(2+1+devW+1+fsW+1)-56, 12), 28)
	if r.place != "" {
		return "  " + term.PadRight(r.label, nameW) + " " + a.display(r.place)
	}
	v := a.vols[r.vol]
	fs := v.FSType
	if fs == "" {
		fs = "-"
	}
	return "  " + term.PadRight(a.display(v.Title()), nameW) + " " +
		term.PadRight(a.display(v.Dev), devW) + " " + term.PadRight(fs, fsW) + " " + a.status(v)
}

func (a *App) Draw(s wm.Surface, focused bool) {
	w, h := s.Size()
	a.vw = w
	a.bodyH = max(h-1-a.chrome.HintRows(), 0)
	a.offset = min(max(a.offset, 0), max(len(a.rows)-max(a.bodyH, 1), 0))
	s.Clear(wm.StyleBody)
	a.chrome.DrawBar(s)
	a.chrome.DrawHints(s)

	textW := w
	if len(a.rows) > a.bodyH && a.bodyH > 0 {
		textW = w - 1
		ui.DrawScrollbar(s.Sub(wm.Rect{X: w - 1, Y: 1, W: 1, H: a.bodyH}), len(a.rows), a.offset, a.bodyH)
	}
	for y := 0; y < a.bodyH && a.offset+y < len(a.rows); y++ {
		i := a.offset + y
		r := a.rows[i]
		line := s.Sub(wm.Rect{X: 0, Y: 1 + y, W: textW, H: 1})
		switch {
		case r.header != "":
			line.Clear(wm.StyleHeader)
			line.Text(1, 0, r.header, wm.StyleHeader)
		case r.note != "":
			line.Text(2, 0, r.note, wm.StyleBodyDim)
		case r.selectable():
			st := wm.StyleBody
			if i == a.sel {
				st = wm.StyleMenu
				if focused {
					st = wm.StyleSelected
				}
			}
			line.Clear(st)
			line.Text(0, 0, a.line(r, textW), st)
		}
	}
}
