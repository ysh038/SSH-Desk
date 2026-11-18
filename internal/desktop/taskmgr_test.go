package desktop

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"tuidesk/internal/apps/mypc"
	"tuidesk/internal/apps/taskmgr"
	"tuidesk/internal/devices"
	"tuidesk/internal/procs"
	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// fakeProcFS is a /proc held in a map: path → contents, or "->target" for
// a link. No real process is ever read or signalled (CLAUDE.md rule 6).
type fakeProcFS map[string]string

func (f fakeProcFS) source() procs.Source {
	return procs.Source{
		Root: "/proc",
		ReadFile: func(p string) ([]byte, error) {
			v, ok := f[p]
			if !ok || strings.HasPrefix(v, "->") {
				return nil, errors.New("no such file")
			}
			return []byte(v), nil
		},
		ReadDir: func(p string) ([]string, error) {
			seen := map[string]bool{}
			for k := range f {
				if rest, ok := strings.CutPrefix(k, p+"/"); ok {
					name, _, _ := strings.Cut(rest, "/")
					seen[name] = true
				}
			}
			var out []string
			for n := range seen {
				out = append(out, n)
			}
			sort.Strings(out)
			if len(out) == 0 {
				return nil, errors.New("no such folder")
			}
			return out, nil
		},
		Readlink: func(p string) (string, error) {
			if v, ok := f[p]; ok && strings.HasPrefix(v, "->") {
				return v[2:], nil
			}
			return "", errors.New("not a link")
		},
		UserName: func(uid int) string { return map[int]string{0: "root", 1000: "kim", 33: "www-data"}[uid] },
	}
}

// add puts a process into the fake /proc.
func (f fakeProcFS) add(pid, ppid, uid int, name, cmd string, ticks uint64, rssKB int) {
	d := "/proc/" + strconv.Itoa(pid)
	f[d+"/stat"] = fmt.Sprintf("%d (%s) S %d 1 1 0 -1 4194304 100 0 0 0 %d 0 0 0 20 0 1 0 100 0 0", pid, name, ppid, ticks)
	f[d+"/status"] = fmt.Sprintf("Uid:\t%d\t%d\t%d\t%d\nVmRSS:\t%d kB\n", uid, uid, uid, uid, rssKB)
	f[d+"/cmdline"] = strings.ReplaceAll(cmd, " ", "\x00") + "\x00"
}

type tmHarness struct {
	*fmHarness
	fs     fakeProcFS
	killed []string // "pid:signal"
	sudo   []string
	killFn func(pid int, sig syscall.Signal) error
}

// newTM is a server where kim (uid 1000) runs tuidesk (PID 950) from a
// shell (901) inside an ssh session (900).
func newTM(t *testing.T) *tmHarness {
	f := newFM(t)
	h := &tmHarness{fmHarness: f, fs: fakeProcFS{
		"/proc/meminfo": "MemTotal: 8000000 kB\nMemAvailable: 6000000 kB\n",
		"/proc/stat":    "cpu  1000 0 500 8000 500 0 0 0 0 0\ncpu0 1 1 1 1 1\ncpu1 1 1 1 1 1\n",
		"/proc/loadavg": "0.52 0.40 0.31 1/212 950\n",
		"/proc/uptime":  "273600.50 1.0\n",
	}}
	h.fs.add(1, 0, 0, "systemd", "/sbin/init", 100, 12000)
	h.fs.add(700, 1, 33, "nginx", "nginx: worker process", 40, 30000)
	h.fs.add(900, 1, 1000, "sshd", "sshd: kim@pts/0", 20, 5000)
	h.fs.add(901, 900, 1000, "bash", "-bash", 25, 4000)
	h.fs.add(950, 901, 1000, "tuidesk", "/home/kim/bin/tuidesk", 60, 20000)
	h.fs.add(960, 901, 1000, "python3", "python3 한글 보고서.py", 400, 204800)
	h.fs["/proc/960/cwd"] = "->" + f.root
	h.fs["/proc/2/stat"] = "2 (kthreadd) S 0 0 0 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 1 0 0"
	h.fs["/proc/2/status"] = "Uid:\t0\t0\t0\t0\n"
	h.fs["/proc/2/cmdline"] = ""
	f.d.NewTaskManager = func() wm.App {
		return taskmgr.New(f.d, taskmgr.Deps{
			Source: h.fs.source(), User: "kim", UID: 1000, Self: 950,
			Kill: func(pid int, sig syscall.Signal) error {
				if h.killFn != nil {
					if err := h.killFn(pid, sig); err != nil {
						return err
					}
				}
				h.killed = append(h.killed, fmt.Sprintf("%d:%d", pid, sig))
				return nil
			},
			Sudo: func(_ context.Context, name string, args []string, _ []byte) ([]byte, error) {
				if cmd := name + " " + strings.Join(args, " "); strings.Contains(cmd, "kill") {
					h.sudo = append(h.sudo, cmd) // the rest is the NOPASSWD probe
				}
				return nil, nil
			},
		})
	}
	return h
}

func (h *tmHarness) open() (*taskmgr.App, *wm.Window) {
	h.t.Helper()
	h.key("F9", 0)
	h.rune('k', 0)
	for _, w := range h.d.M.Windows() {
		if a, ok := w.App.(*taskmgr.App); ok {
			w.Rect = wm.Rect{X: 0, Y: 1, W: 100, H: 20}
			return a, w
		}
	}
	h.t.Fatal("no Task Manager window")
	return nil, nil
}

func pidsOf(a *taskmgr.App) string {
	var out []string
	for _, p := range a.Rows() {
		out = append(out, strconv.Itoa(p.PID))
	}
	return strings.Join(out, ",")
}

func (h *tmHarness) selectPID(a *taskmgr.App, pid int) {
	h.t.Helper()
	h.key("Home", 0)
	for i := 0; i < len(a.Rows()) && a.Selected() != pid; i++ {
		h.key("Down", 0)
	}
	if a.Selected() != pid {
		h.t.Fatalf("PID %d is not listed (%s)", pid, pidsOf(a))
	}
}

// Keyboard only: open the Task Manager, find a program by typing, end it.
func TestTaskManagerKeyboard(t *testing.T) {
	h := newTM(t)
	a, _ := h.open()
	if got := pidsOf(a); got != "900,901,950,960" {
		t.Fatalf("my processes, in PID order while nothing uses the CPU: %s", got)
	}
	for _, want := range []string{"Memory", "of 7.6G", "Load 0.52", "Up 3d 4h", "python3 한글 보고서.py", "kim", "200.0M", "4 of 7 · my processes"} {
		if !h.screenHas(want) {
			t.Fatalf("screen does not show %q", want)
		}
	}

	// A second sample: python3 used 100 of the 200 ticks of 2 cores = 100 %.
	h.fs.add(960, 901, 1000, "python3", "python3 한글 보고서.py", 500, 204800)
	h.fs["/proc/stat"] = "cpu  1100 0 550 8050 500 0 0 0 0 0\ncpu0 1 1 1 1 1\ncpu1 1 1 1 1 1\n"
	h.key("F5", 0)
	if a.Rows()[0].PID != 960 || !h.screenHas(" 100.0") || !h.screenHas(" 75%") {
		t.Fatalf("after a second sample: first row %d", a.Rows()[0].PID)
	}

	h.typeText("보고서")
	if got := pidsOf(a); got != "960" || !h.screenHas("filter: 보고서") {
		t.Fatalf("filter: %s", got)
	}
	h.key("Delete", 0)
	if !h.d.DialogOpen() || !h.screenHas("End python3 (PID 960)?") {
		t.Fatal("End task must ask first")
	}
	h.key("Esc", 0) // Cancel
	if len(h.killed) != 0 {
		t.Fatalf("cancelled, but signalled %v", h.killed)
	}
	h.key("Delete", 0)
	h.key("Enter", 0)
	if len(h.killed) != 1 || h.killed[0] != fmt.Sprintf("960:%d", syscall.SIGTERM) {
		t.Fatalf("signals %v", h.killed)
	}
	h.key("Delete", term.ModShift)
	if !h.screenHas("Try End task first") {
		t.Fatal("Force quit must say what it does")
	}
	h.key("Enter", 0)
	if len(h.killed) != 2 || h.killed[1] != fmt.Sprintf("960:%d", syscall.SIGKILL) {
		t.Fatalf("signals %v", h.killed)
	}

	// The process is gone at the next sample; Esc clears the filter.
	for k := range h.fs {
		if strings.HasPrefix(k, "/proc/960/") {
			delete(h.fs, k)
		}
	}
	h.key("F5", 0)
	if !h.screenHas("No process matches") {
		t.Fatal("an empty list is not explained")
	}
	h.key("Esc", 0)
	if got := pidsOf(a); got != "900,901,950" {
		t.Fatalf("after Esc: %s", got)
	}

	// All users, kernel threads from the menu.
	h.rune('a', term.ModCtrl)
	if got := pidsOf(a); got != "1,700,900,901,950" || !h.screenHas("www-data") {
		t.Fatalf("all users: %s", got)
	}
}

// What must not be ended is refused; what is risky is said clearly;
// another user's program goes through sudo.
func TestTaskManagerSafety(t *testing.T) {
	h := newTM(t)
	a, _ := h.open()
	h.rune('a', term.ModCtrl)

	h.selectPID(a, 1)
	h.key("Delete", 0)
	if !h.screenHas("is the system itself") || len(h.killed) != 0 {
		t.Fatal("PID 1 must be refused")
	}
	h.key("Enter", 0)

	h.selectPID(a, 950)
	h.key("Delete", term.ModShift)
	if !h.screenHas("That is tuidesk") {
		t.Fatal("tuidesk itself must be refused")
	}
	h.key("Enter", 0)

	h.selectPID(a, 901) // the shell tuidesk was started from
	h.key("Delete", 0)
	if !h.screenHas("WARNING: tuidesk runs inside this program") {
		t.Fatal("ending the own session must carry a warning")
	}
	h.key("Esc", 0)

	h.selectPID(a, 700) // nginx of www-data
	h.key("Delete", 0)
	if !h.screenHas("belongs to www-data") {
		t.Fatal("another user's program: sudo must be announced")
	}
	h.key("Enter", 0)
	h.settle()
	if len(h.killed) != 0 || len(h.sudo) != 1 || !strings.HasSuffix(h.sudo[0], "kill -TERM 700") {
		t.Fatalf("killed %v, sudo %q", h.killed, h.sudo)
	}

	// A program that ended meanwhile is not an error.
	h.killFn = func(int, syscall.Signal) error { return syscall.ESRCH }
	h.selectPID(a, 960)
	h.key("Delete", 0)
	h.key("Enter", 0)
	if h.d.DialogOpen() || !h.screenHas("has already ended") {
		t.Fatal("ESRCH must be a plain notice")
	}
}

// Mouse only: sort by clicking a column, right-click to end, double-click
// for the properties.
func TestTaskManagerMouse(t *testing.T) {
	h := newTM(t)
	h.draw()
	h.click(2, 0)
	h.clickMenuItem("Task Manager")
	var a *taskmgr.App
	var w *wm.Window
	for _, x := range h.d.M.Windows() {
		if app, ok := x.App.(*taskmgr.App); ok {
			a, w = app, x
		}
	}
	w.Rect = wm.Rect{X: 0, Y: 1, W: 100, H: 20}
	h.draw()
	in := w.Inner()

	h.click(in.X+30, in.Y+3) // the Memory column
	h.wait()
	if a.Rows()[0].PID != 960 {
		t.Fatalf("largest memory first: %s", pidsOf(a))
	}
	h.click(in.X+30, in.Y+3) // again: smallest first
	h.wait()
	if a.Rows()[0].PID != 901 {
		t.Fatalf("smallest memory first: %s", pidsOf(a))
	}

	y, ok := 0, false
	c := h.draw()
	for row := 0; row < c.H; row++ {
		if strings.Contains(c.Row(row), "한글 보고서.py") {
			y, ok = row, true
		}
	}
	if !ok {
		t.Fatal("python3 is not on screen")
	}
	h.rclick(in.X+50, y)
	if a.Selected() != 960 || !h.d.MenuOpen() {
		t.Fatal("right click selects and opens the menu")
	}
	h.clickMenuItem("End task")
	h.key("Enter", 0)
	if len(h.killed) != 1 {
		t.Fatalf("signals %v", h.killed)
	}

	h.wait()
	h.click(in.X+50, y)
	h.click(in.X+50, y)
	for _, want := range []string{"Properties - python3 (PID 960)", "Parent:", "bash (PID 901)", "Folder:"} {
		if !h.screenHas(want) {
			t.Fatalf("properties do not show %q", want)
		}
	}
}

// My PC names the programs that keep a drive busy (SPEC 7.8).
func TestBusyDriveNamesThePrograms(t *testing.T) {
	h := newPC(t)
	h.pc.nopasswd = true
	dir := h.media + "/한글 USB"
	h.pc.mounted["/dev/sdb1"] = dir
	fs := fakeProcFS{}
	fs.add(901, 900, 1000, "bash", "-bash", 1, 1)
	fs["/proc/901/cwd"] = "->" + dir + "/작업"
	h.d.NewMyPC = func() wm.App {
		return mypc.New(h.d, mypc.Deps{
			Source: devices.Source{Run: h.pc.lsblk, ReadFile: h.pc.readFile},
			Run:    h.pc.sudo, Usage: h.pc.usage, Exists: func(string) bool { return false },
			MediaBase: h.media, User: "kim", UID: 1000, GID: 1000, Home: h.root,
			Users: fs.source().UsingDir,
		})
	}
	h.openMyPC()
	h.pc.busy = true
	h.key("End", 0)
	h.rune('e', term.ModCtrl)
	h.settle()
	for _, want := range []string{"is in use", "In use by:", "bash (PID 901): working folder", "Task Manager"} {
		if !h.screenHas(want) {
			t.Fatalf("busy message does not show %q", want)
		}
	}
}
