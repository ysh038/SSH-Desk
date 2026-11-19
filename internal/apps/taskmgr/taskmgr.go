// Package taskmgr is the Task Manager window (SPEC 7.9, ADR 0006): the
// programs running on this server, how much CPU and memory they use, and a
// safe way to end one without typing ps and kill.
package taskmgr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"tuidesk/internal/config"
	"tuidesk/internal/devices"
	"tuidesk/internal/host"
	"tuidesk/internal/procs"
	"tuidesk/internal/sudo"
	"tuidesk/internal/term"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

// Deps is everything the window needs from the machine; tests replace it.
type Deps struct {
	Source procs.Source
	// Kill sends a signal to a process.
	Kill func(pid int, sig syscall.Signal) error
	Sudo sudo.Runner
	User string
	UID  int
	// Self is the PID of tuidesk.
	Self int
	// Auto starts the background sampler. Tests leave it off and call
	// Sample themselves.
	Auto bool
}

// SystemDeps reads the real machine as the current user.
func SystemDeps() Deps {
	d := Deps{Source: procs.System(), Kill: syscall.Kill, Sudo: sudo.Exec, User: "user", UID: os.Getuid(), Self: os.Getpid(), Auto: true}
	if u, err := user.Current(); err == nil {
		d.User = u.Username
	}
	return d
}

const (
	summaryY = 1
	filterY  = 2
	headY    = 3
	listY    = 4
)

// column positions of the list.
const (
	colPID  = 0
	colUser = 8
	colCPU  = 19
	colMem  = 26
	colSt   = 36
	colCmd  = 46
)

type column struct {
	x     int
	title string
	key   procs.SortKey
}

var columns = []column{
	{colPID, "    PID", procs.ByPID}, {colUser, "User", procs.ByUser}, {colCPU, " CPU %", procs.ByCPU},
	{colMem, "   Memory", procs.ByMemory}, {colSt, "State", -1}, {colCmd, "Command", procs.ByName},
}

// App is the Task Manager window.
type App struct {
	h   host.Host
	dep Deps

	prev    *procs.Snapshot
	snap    procs.Snapshot
	rows    []procs.Proc
	loaded  bool
	err     error
	sel     int // PID of the selected process (0: none)
	offset  int
	bodyH   int
	sortKey procs.SortKey
	reverse bool
	all     bool // processes of every user
	kernel  bool // kernel threads too
	filter  *ui.Input

	interval atomic.Int64 // seconds between samples (0: paused)
	kick     chan struct{}
	done     chan struct{}
	closed   bool

	cmds   ui.Commands
	chrome *host.Chrome
}

// New opens the window and, with Deps.Auto, starts sampling.
func New(h host.Host, dep Deps) *App {
	a := &App{h: h, dep: dep, filter: ui.NewInput(""), kick: make(chan struct{}, 1), done: make(chan struct{})}
	a.interval.Store(int64(h.Config().TaskSeconds))
	a.buildCommands()
	a.chrome = host.NewChrome(h, a, &a.cmds)
	if dep.Auto {
		go a.loop()
	} else {
		a.Sample()
	}
	return a
}

func (a *App) Title() string       { return "Task Manager" }
func (a *App) MinSize() (int, int) { return 60, 10 }

func (a *App) Close() error {
	if !a.closed {
		a.closed = true
		close(a.done)
	}
	return nil
}

// loop samples in the background and hands each snapshot to the UI
// goroutine. Only this window's timer runs, and only while it is open
// (CLAUDE.md rule 9).
func (a *App) loop() {
	for {
		snap, err := a.dep.Source.Read()
		a.h.Post(func() { a.apply(snap, err) })
		var tick <-chan time.Time
		var t *time.Timer
		if sec := a.interval.Load(); sec > 0 {
			t = time.NewTimer(time.Duration(sec) * time.Second)
			tick = t.C
		}
		select {
		case <-a.done:
			if t != nil {
				t.Stop()
			}
			return
		case <-a.kick:
			if t != nil {
				t.Stop()
			}
		case <-tick:
		}
	}
}

// refreshNow asks for a new snapshot right away.
func (a *App) refreshNow() {
	if !a.dep.Auto {
		a.Sample()
		return
	}
	select {
	case a.kick <- struct{}{}:
	default:
	}
}

// Sample reads the processes now, on the calling goroutine (tests).
func (a *App) Sample() {
	snap, err := a.dep.Source.Read()
	a.apply(snap, err)
}

func (a *App) apply(snap procs.Snapshot, err error) {
	if a.closed {
		return
	}
	a.loaded, a.err = true, err
	if err != nil {
		return
	}
	procs.Diff(a.prev, &snap)
	a.snap = snap
	a.prev = &a.snap
	a.rebuild()
}

// rebuild filters and sorts the snapshot and keeps the selection on the
// same process (or, when it ended, on the same line).
func (a *App) rebuild() {
	at := a.index()
	a.rows = procs.Filter(a.snap.Procs, a.dep.UID, a.all, a.kernel, a.filter.Value())
	procs.Sort(a.rows, a.sortKey, a.reverse)
	if a.index() < 0 && len(a.rows) > 0 {
		a.sel = a.rows[min(max(at, 0), len(a.rows)-1)].PID
	}
	a.ensureVisible()
}

// Rows returns the listed processes (tests).
func (a *App) Rows() []procs.Proc { return a.rows }

// Selected returns the PID of the selected process (tests).
func (a *App) Selected() int { return a.sel }

// index is the row of the selected process (-1: not listed).
func (a *App) index() int {
	for i, p := range a.rows {
		if p.PID == a.sel {
			return i
		}
	}
	return -1
}

func (a *App) current() (procs.Proc, bool) {
	if i := a.index(); i >= 0 {
		return a.rows[i], true
	}
	return procs.Proc{}, false
}

func (a *App) move(delta int) {
	if len(a.rows) == 0 {
		return
	}
	i := min(max(max(a.index(), 0)+delta, 0), len(a.rows)-1)
	a.sel = a.rows[i].PID
	a.ensureVisible()
}

func (a *App) ensureVisible() {
	h, i := max(a.bodyH, 1), a.index()
	if i >= 0 && i < a.offset {
		a.offset = i
	}
	if i >= a.offset+h {
		a.offset = i - h + 1
	}
	a.offset = min(max(a.offset, 0), max(len(a.rows)-h, 0))
}

func (a *App) display(s string) string {
	if a.h.Console() {
		return term.EscapeNonASCII(s)
	}
	return s
}

func (a *App) name(p procs.Proc) string {
	return a.display(p.Name) + " (PID " + strconv.Itoa(p.PID) + ")"
}

// ---- commands ----

func (a *App) setSort(k procs.SortKey) {
	if a.sortKey == k {
		a.reverse = !a.reverse
	} else {
		a.sortKey, a.reverse = k, false
	}
	a.rebuild()
}

func (a *App) setInterval(sec int) {
	a.h.UpdateConfig(func(c *config.Config) { c.TaskSeconds = sec })
	a.interval.Store(int64(a.h.Config().TaskSeconds))
	a.refreshNow() // also wakes a paused sampler so it picks the new pace up
	if sec == 0 {
		a.h.Notify("Task Manager is paused: "+term.KeyLabel("F5")+" refreshes", false)
	}
}

func (a *App) buildCommands() {
	has := func() bool { _, ok := a.current(); return ok }
	c := func(id, menu, title, hint string, keys []string, en func() bool, run func()) *ui.Command {
		return &ui.Command{ID: id, Menu: menu, Title: title, Hint: hint, Keys: keys, Enabled: en, Run: run}
	}
	sep := func(menu string) *ui.Command { return &ui.Command{Menu: menu, Title: "-"} }
	withProc := func(fn func(procs.Proc)) func() {
		return func() {
			if p, ok := a.current(); ok {
				fn(p)
			}
		}
	}
	pace := func(sec int, title string) *ui.Command {
		return c("pace"+strconv.Itoa(sec), "View", title, "", nil, nil, func() { a.setInterval(sec) })
	}
	a.cmds.Add(
		c("end", "Process", "End task", "End task", []string{"Delete"}, has, withProc(func(p procs.Proc) { a.end(p, false) })),
		c("kill", "Process", "Force quit", "", []string{"Shift+Delete"}, has, withProc(func(p procs.Proc) { a.end(p, true) })),
		sep("Process"),
		c("props", "Process", "Properties...", "Properties", []string{"Ctrl+P", "Alt+Enter", "Enter"}, has, withProc(a.properties)),
		c("folder", "Process", "Open its working folder", "", nil, has, withProc(a.openFolder)),
		c("copypid", "Process", "Copy PID", "", nil, has, withProc(func(p procs.Proc) {
			a.h.SetClipboardText(strconv.Itoa(p.PID))
			a.h.Notify("Copied PID "+strconv.Itoa(p.PID), false)
		})),
		c("copycmd", "Process", "Copy command line", "", nil, has, withProc(func(p procs.Proc) {
			a.h.SetClipboardText(p.Title())
			a.h.Notify("Copied the command line", false)
		})),

		c("refresh", "View", "Refresh now", "Refresh", []string{"F5"}, nil, a.refreshNow),
		sep("View"),
		c("sortcpu", "View", "Sort by CPU", "", nil, nil, func() { a.setSort(procs.ByCPU) }),
		c("sortmem", "View", "Sort by memory", "", nil, nil, func() { a.setSort(procs.ByMemory) }),
		c("sortpid", "View", "Sort by PID", "", nil, nil, func() { a.setSort(procs.ByPID) }),
		c("sortname", "View", "Sort by command", "", nil, nil, func() { a.setSort(procs.ByName) }),
		c("sortuser", "View", "Sort by user", "", nil, nil, func() { a.setSort(procs.ByUser) }),
		sep("View"),
		c("all", "View", "Show processes of all users", "All users", []string{"Ctrl+A"}, nil, func() {
			a.all = !a.all
			a.rebuild()
		}),
		c("kernel", "View", "Show kernel threads", "", nil, nil, func() {
			a.kernel = !a.kernel
			if a.kernel {
				a.all = true // they all belong to root
			}
			a.rebuild()
		}),
		sep("View"),
		pace(1, "Update every second"), pace(2, "Update every 2 seconds"), pace(5, "Update every 5 seconds"), pace(0, "Pause updates"),
		sep("View"),
		c("palette", "View", "Command search...", "Commands", []string{"Ctrl+K"}, nil, func() { a.h.ShowPalette(&a.cmds) }),
	)
	a.cmds.ByID("palette").HintPinned = true
}

// ---- ending a process ----

// refuse explains why p must not be ended here ("" = it may be).
func (a *App) refuse(p procs.Proc) string {
	switch {
	case p.PID == 1:
		return a.name(p) + " is the system itself: ending it would stop the server."
	case p.PID == a.dep.Self:
		return "That is tuidesk. Use " + term.KeyLabel("Ctrl+Q") + " to quit it."
	case p.Kernel:
		return a.name(p) + " is part of the Linux kernel and cannot be ended."
	}
	return ""
}

func (a *App) end(p procs.Proc, force bool) {
	if why := a.refuse(p); why != "" {
		a.h.ShowError("Cannot end this task", why, "")
		return
	}
	title, button := "End task", "End task"
	msg := "End " + a.name(p) + "?\nIt is asked to close. Work in it that was not saved may be lost."
	sig := syscall.SIGTERM
	if force {
		title, button, sig = "Force quit", "Force quit", syscall.SIGKILL
		msg = "Force " + a.name(p) + " to quit?\nIt is stopped at once and cannot save anything. Try End task first."
	}
	for _, pid := range procs.Ancestors(a.snap.Procs, a.dep.Self) {
		if pid == p.PID {
			msg += "\n\nWARNING: tuidesk runs inside this program. Ending it closes tuidesk and may disconnect you."
		}
	}
	if p.UID != a.dep.UID {
		msg += "\n\nIt belongs to " + a.display(p.User) + ": your sudo password will be asked."
	}
	a.h.Confirm(title, msg, []string{button, "Cancel"}, func(choice int) {
		if choice == 0 {
			a.signal(p, sig, force)
		}
	})
}

func (a *App) signal(p procs.Proc, sig syscall.Signal, force bool) {
	done := "Asked " + a.name(p) + " to close"
	if force {
		done = "Forced " + a.name(p) + " to quit"
	}
	err := error(syscall.EPERM)
	if p.UID == a.dep.UID {
		err = a.dep.Kill(p.PID, sig)
	}
	switch {
	case err == nil:
		a.h.Notify(done, false)
		a.refreshNow()
	case errors.Is(err, syscall.ESRCH):
		a.h.Notify(a.name(p)+" has already ended", false)
		a.refreshNow()
	case errors.Is(err, syscall.EPERM):
		flag := "-TERM"
		if force {
			flag = "-KILL"
		}
		host.RunSudo(a.h, a.dep.Sudo, a.dep.User, "Ending "+a.name(p), [][]string{{"kill", flag, strconv.Itoa(p.PID)}}, func(out []byte, err error) {
			a.refreshNow()
			switch {
			case err == nil:
				a.h.Notify(done, false)
			case err == context.Canceled:
				a.h.Notify("Cancelled", true)
			case strings.Contains(string(out), "No such process"):
				a.h.Notify(a.name(p)+" has already ended", false)
			default:
				msg := sudo.Explain(out, a.dep.User)
				if msg == "" {
					msg = strings.TrimSpace(string(out))
				}
				if msg == "" {
					msg = err.Error()
				}
				a.h.ShowError("Cannot end "+a.name(p), msg, strings.TrimSpace(string(out)+"\n"+err.Error()))
			}
		})
	default:
		a.h.ShowError("Cannot end "+a.name(p), err.Error(), "")
	}
}

func (a *App) cwd(p procs.Proc) string {
	dir, err := a.dep.Source.Readlink(a.dep.Source.Root + "/" + strconv.Itoa(p.PID) + "/cwd")
	if err != nil {
		return ""
	}
	return dir
}

func (a *App) openFolder(p procs.Proc) {
	if dir := a.cwd(p); dir != "" {
		a.h.OpenFolder(dir)
		return
	}
	a.h.Notify("The folder of "+a.name(p)+" cannot be read (it belongs to "+a.display(p.User)+")", true)
}

func (a *App) properties(p procs.Proc) {
	var b strings.Builder
	line := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%-9s %s\n", k+":", a.display(v))
		}
	}
	line("Name", p.Name)
	line("PID", strconv.Itoa(p.PID))
	for _, o := range a.snap.Procs {
		if o.PID == p.PPID {
			line("Parent", o.Name+" (PID "+strconv.Itoa(o.PID)+")")
		}
	}
	line("User", p.User)
	line("State", p.StateName())
	line("CPU", fmt.Sprintf("%.1f %% of one core", p.CPU))
	if !p.Kernel {
		line("Memory", ui.HumanSize(p.RSS))
	}
	line("Threads", strconv.Itoa(p.Threads))
	line("Folder", a.cwd(p))
	exe, _ := a.dep.Source.Readlink(a.dep.Source.Root + "/" + strconv.Itoa(p.PID) + "/exe")
	line("Program", exe)
	line("Command", p.Cmd)
	a.h.Confirm("Properties - "+a.name(p), strings.TrimRight(b.String(), "\n"), []string{"Close"}, func(int) {})
}

// ---- events ----

func (a *App) HandleEvent(ev term.Event) bool {
	switch ev := ev.(type) {
	case term.KeyEvent:
		return a.handleKey(ev.Key)
	case term.MouseEvent:
		return a.handleMouse(ev.Gesture)
	case term.PasteEvent:
		a.filter.Paste(ev.Text)
		a.rebuild()
		return true
	}
	return false
}

func (a *App) handleKey(k term.Key) bool {
	page := max(a.bodyH-1, 1)
	name := k.String()
	switch name {
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
		a.contextMenu(colCmd, listY+max(a.index(), 0)-a.offset+1)
	case "Esc":
		if a.filter.Value() == "" {
			return false
		}
		a.filter.Clear()
		a.rebuild()
	case "Backspace":
		if a.filter.Value() == "" {
			return false
		}
		a.filter.HandleKey(k)
		a.rebuild()
	default:
		if a.chrome.HandleKey(k) {
			return true
		}
		// Typing filters the list, like in a file window.
		if k.IsRune() && k.Mod&(term.ModCtrl|term.ModAlt|term.ModMeta) == 0 && k.Rune >= ' ' {
			a.filter.HandleKey(term.Key{Name: "End"})
			a.filter.HandleKey(k)
			a.rebuild()
			return true
		}
		return false
	}
	return true
}

func (a *App) contextMenu(x, y int) {
	a.h.OpenMenu(a, x, y, a.cmds.MenuItems("Process"), func(id string) { a.cmds.Run(id) })
}

func (a *App) rowAt(y int) int {
	if y < listY || y >= listY+a.bodyH {
		return -1
	}
	if i := a.offset + y - listY; i < len(a.rows) {
		return i
	}
	return -1
}

func (a *App) handleMouse(g term.Gesture) bool {
	switch g.Kind {
	case term.GestureWheel:
		n := ui.WheelLines
		if g.Wheel == term.WheelUp {
			n = -n
		}
		a.offset = min(max(a.offset+n, 0), max(len(a.rows)-max(a.bodyH, 1), 0))
		return true
	case term.GesturePress:
		if i := a.rowAt(g.Y); i >= 0 {
			a.sel = a.rows[i].PID
		}
		return true
	case term.GestureClick:
		switch {
		case g.Y == 0:
			return g.Button == term.ButtonLeft && a.chrome.ClickBar(g.X)
		case g.Y == headY && g.Button == term.ButtonLeft:
			for i := len(columns) - 1; i >= 0; i-- {
				if g.X >= columns[i].x {
					if columns[i].key >= 0 {
						a.setSort(columns[i].key)
					}
					break
				}
			}
			return true
		}
		i := a.rowAt(g.Y)
		if i < 0 {
			return true
		}
		a.sel = a.rows[i].PID
		switch {
		case g.Button == term.ButtonRight:
			a.contextMenu(g.X, g.Y)
		case g.Button == term.ButtonLeft && g.Count == 2:
			a.properties(a.rows[i])
		}
		return true
	}
	return false
}

// ---- drawing ----

func uptime(d time.Duration) string {
	days, hours, mins := int(d.Hours())/24, int(d.Hours())%24, int(d.Minutes())%60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func clipTo(s string, w int) string {
	if term.Width(s) <= w {
		return s
	}
	var out []rune
	cw := 0
	for _, r := range s {
		rw := term.RuneWidth(r)
		if cw+rw > w-1 {
			break
		}
		out = append(out, r)
		cw += rw
	}
	return string(out) + "~"
}

func (a *App) summary() string {
	s := a.snap.Sys
	used := s.MemTotal - s.MemAvail
	frac := 0.0
	if s.MemTotal > 0 {
		frac = float64(used) / float64(s.MemTotal)
	}
	out := fmt.Sprintf(" CPU %s %3.0f%%   Memory %s %s of %s   Load %.2f   Up %s",
		devices.UsageBar(10, s.CPU/100), s.CPU, devices.UsageBar(10, frac), ui.HumanSize(used), ui.HumanSize(s.MemTotal), s.Load[0], uptime(s.Uptime))
	if s.SwapTotal > 0 && s.SwapFree < s.SwapTotal {
		out += "   Swap " + ui.HumanSize(s.SwapTotal-s.SwapFree)
	}
	return out
}

func (a *App) Draw(s wm.Surface, focused bool) {
	w, h := s.Size()
	a.bodyH = max(h-listY-a.chrome.HintRows(), 0)
	a.offset = min(max(a.offset, 0), max(len(a.rows)-max(a.bodyH, 1), 0))
	s.Clear(wm.StyleBody)
	a.chrome.DrawBar(s)
	a.chrome.DrawHints(s)

	switch {
	case a.err != nil:
		s.Text(1, summaryY, "The Task Manager works on Linux: it reads /proc, which this system does not have.", wm.StyleBodyDim)
		s.Text(1, filterY, "("+a.err.Error()+")", wm.StyleBodyDim)
		return
	case !a.loaded:
		s.Text(1, summaryY, "Reading processes...", wm.StyleBodyDim)
		return
	}
	s.Text(0, summaryY, a.summary(), wm.StyleBody)

	// Filter line: what is shown, and the text typed to narrow it.
	scope := "my processes"
	if a.all {
		scope = "all users"
	}
	info := fmt.Sprintf(" %d of %d · %s", len(a.rows), len(a.snap.Procs), scope)
	if a.interval.Load() == 0 {
		info += " · paused"
	}
	if q := a.filter.Value(); q != "" {
		info += " · filter: " + a.display(q) + "  (" + term.KeyLabel("Esc") + " clears)"
	} else {
		info += " · type to filter"
	}
	s.Text(0, filterY, info, wm.StyleBodyDim)

	head := s.Sub(wm.Rect{X: 0, Y: headY, W: w, H: 1})
	head.Clear(wm.StyleHeader)
	for _, c := range columns {
		t := c.title
		if c.key == a.sortKey && c.key >= 0 {
			arrow := " v"
			if a.reverse != (c.key == procs.ByPID || c.key == procs.ByName || c.key == procs.ByUser) {
				arrow = " ^"
			}
			t += arrow
		}
		head.Text(c.x+1, 0, t, wm.StyleHeader)
	}

	textW := w
	if len(a.rows) > a.bodyH && a.bodyH > 0 {
		textW = w - 1
		ui.DrawScrollbar(s.Sub(wm.Rect{X: w - 1, Y: listY, W: 1, H: a.bodyH}), len(a.rows), a.offset, a.bodyH)
	}
	if len(a.rows) == 0 && a.bodyH > 0 {
		s.Text(2, listY, "No process matches. Press "+term.KeyLabel("Esc")+" to clear the filter, or "+term.KeyLabel("Ctrl+A")+" to see all users.", wm.StyleBodyDim)
	}
	for y := 0; y < a.bodyH && a.offset+y < len(a.rows); y++ {
		p := a.rows[a.offset+y]
		st := wm.StyleBody
		if p.PID == a.sel {
			st = wm.StyleMenu
			if focused {
				st = wm.StyleSelected
			}
		}
		row := s.Sub(wm.Rect{X: 0, Y: listY + y, W: textW, H: 1})
		if st == wm.StyleBody && row.Hovered() {
			st = wm.StyleHover
		}
		row.Clear(st)
		mem := "-"
		if !p.Kernel {
			mem = ui.HumanSize(p.RSS)
		}
		row.Text(1, 0, fmt.Sprintf("%7d %s %6.1f %9s %s %s", p.PID, term.PadRight(clipTo(a.display(p.User), 10), 10),
			p.CPU, mem, term.PadRight(p.StateName(), 9), a.display(p.Title())), st)
	}
}
