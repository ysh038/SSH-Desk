package procs

import (
	"errors"
	"sort"
	"strings"
	"testing"
	"time"
)

// fakeProc is a /proc held in a map: path → contents (files) or link
// target (links, prefixed with "->").
type fakeProc map[string]string

func (f fakeProc) source() Source {
	return Source{
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
			if len(seen) == 0 {
				return nil, errors.New("no such folder")
			}
			var out []string
			for n := range seen {
				out = append(out, n)
			}
			sort.Strings(out)
			return out, nil
		},
		Readlink: func(p string) (string, error) {
			if v, ok := f[p]; ok && strings.HasPrefix(v, "->") {
				return v[2:], nil
			}
			return "", errors.New("not a link")
		},
		UserName: func(uid int) string { return map[int]string{0: "root", 1000: "kim"}[uid] },
	}
}

func stat(pid int, name string, state string, ppid int, utime, stime uint64, threads int) string {
	return strings.Join([]string{
		itoa(pid), "(" + name + ")", state, itoa(ppid), "1 1 0 -1 4194304 100 0 0 0",
		itoa(int(utime)), itoa(int(stime)), "0 0 20 0", itoa(threads), "0 100 0 0",
	}, " ")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s, neg := "", n < 0
	if neg {
		n = -n
	}
	for ; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	if neg {
		s = "-" + s
	}
	return s
}

func machine() fakeProc {
	return fakeProc{
		"/proc/1/stat":      stat(1, "systemd", "S", 0, 50, 50, 1),
		"/proc/1/status":    "Name:\tsystemd\nUid:\t0\t0\t0\t0\nVmRSS:\t   12000 kB\n",
		"/proc/1/cmdline":   "/sbin/init\x00",
		"/proc/2/stat":      stat(2, "kthreadd", "S", 0, 0, 0, 1),
		"/proc/2/status":    "Name:\tkthreadd\nUid:\t0\t0\t0\t0\n",
		"/proc/2/cmdline":   "",
		"/proc/900/stat":    stat(900, "sshd", "S", 1, 10, 10, 1),
		"/proc/900/status":  "Uid:\t1000\t1000\t1000\t1000\nVmRSS:\t 5000 kB\n",
		"/proc/900/cmdline": "sshd: kim@pts/0\x00",
		"/proc/901/stat":    stat(901, "bash", "S", 900, 20, 5, 1),
		"/proc/901/status":  "Uid:\t1000\t1000\t1000\t1000\nVmRSS:\t 4000 kB\n",
		"/proc/901/cmdline": "-bash\x00",
		"/proc/901/cwd":     "->/mnt/offline/작업",
		// A name with spaces and parentheses, and a Hangul argument.
		"/proc/950/stat":    stat(950, "my (odd) prog", "R", 901, 300, 100, 4),
		"/proc/950/status":  "Uid:\t1000\t1000\t1000\t1000\nVmRSS:\t 204800 kB\n",
		"/proc/950/cmdline": "/opt/my prog\x00--file\x00한글 보고서.txt\x00",
		"/proc/950/cwd":     "->/home/kim",
		"/proc/950/fd/0":    "->/dev/pts/0",
		"/proc/950/fd/3":    "->/mnt/offline/data.bin",
		"/proc/meminfo":     "MemTotal:        8000000 kB\nMemFree:  100 kB\nMemAvailable:    6000000 kB\nSwapTotal: 2000000 kB\nSwapFree: 1500000 kB\n",
		"/proc/stat":        "cpu  1000 0 500 8000 500 0 0 0 0 0\ncpu0 500 0 250 4000 250 0 0 0 0 0\ncpu1 500 0 250 4000 250 0 0 0 0 0\nintr 1\n",
		"/proc/loadavg":     "0.52 0.40 0.31 1/212 950\n",
		"/proc/uptime":      "273600.50 500000.00\n",
		"/proc/self/stat":   "ignored",
	}
}

func byPID(ps []Proc, pid int) Proc {
	for _, p := range ps {
		if p.PID == pid {
			return p
		}
	}
	return Proc{}
}

func TestReadSnapshot(t *testing.T) {
	snap, err := machine().source().Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Procs) != 5 {
		t.Fatalf("%d processes: %+v", len(snap.Procs), snap.Procs)
	}
	odd := byPID(snap.Procs, 950)
	if odd.Name != "my (odd) prog" || odd.State != 'R' || odd.PPID != 901 || odd.Ticks != 400 || odd.Threads != 4 {
		t.Fatalf("stat: %+v", odd)
	}
	if odd.Cmd != "/opt/my prog --file 한글 보고서.txt" || odd.RSS != 204800*1024 || odd.User != "kim" || odd.Kernel {
		t.Fatalf("details: %+v", odd)
	}
	if k := byPID(snap.Procs, 2); !k.Kernel || k.Title() != "[kthreadd]" || k.User != "root" {
		t.Fatalf("kernel thread: %+v", k)
	}
	s := snap.Sys
	if s.MemTotal != 8000000*1024 || s.MemAvail != 6000000*1024 || s.SwapFree != 1500000*1024 || s.CPUs != 2 ||
		s.Load != [3]float64{0.52, 0.40, 0.31} || s.Uptime.Round(time.Hour) != 76*time.Hour {
		t.Fatalf("system: %+v", s)
	}
}

func TestDiffGivesCPUPercent(t *testing.T) {
	m := machine()
	prev, _ := m.source().Read()
	// One second later on 2 cores at 100 ticks/s: 200 ticks in all, of
	// which 150 were busy; process 950 used 100 of them = one full core.
	m["/proc/stat"] = "cpu  1100 0 550 8050 500 0 0 0 0 0\ncpu0 1 1 1 1 1\ncpu1 1 1 1 1 1\n"
	m["/proc/950/stat"] = stat(950, "my (odd) prog", "R", 901, 380, 120, 4)
	delete(m, "/proc/901/stat") // bash ended
	m["/proc/999/stat"] = stat(999, "new", "R", 1, 5, 5, 1)
	cur, _ := m.source().Read()
	Diff(&prev, &cur)
	if got := cur.Sys.CPU; got < 74.9 || got > 75.1 {
		t.Errorf("system CPU %.1f, want 75", got)
	}
	if got := byPID(cur.Procs, 950).CPU; got < 99.9 || got > 100.1 {
		t.Errorf("process CPU %.1f, want 100", got)
	}
	if got := byPID(cur.Procs, 999).CPU; got != 0 {
		t.Errorf("a new process has no CPU share yet, got %.1f", got)
	}
	Diff(nil, &cur) // no previous snapshot: nothing to do, no panic
}

func TestSortAndFilter(t *testing.T) {
	snap, _ := machine().source().Read()
	ps := snap.Procs
	Sort(ps, ByMemory, false)
	if ps[0].PID != 950 {
		t.Fatalf("largest memory first: %d", ps[0].PID)
	}
	Sort(ps, ByPID, true)
	if ps[0].PID != 950 || ps[len(ps)-1].PID != 1 {
		t.Fatalf("reverse PID order: %d..%d", ps[0].PID, ps[len(ps)-1].PID)
	}
	Sort(ps, ByName, false)
	if ps[0].Title() != "-bash" {
		t.Fatalf("by name: %q", ps[0].Title())
	}

	pids := func(ps []Proc) string {
		var out []string
		for _, p := range ps {
			out = append(out, itoa(p.PID))
		}
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	if got := pids(Filter(ps, 1000, false, false, "")); got != "900,901,950" {
		t.Errorf("my processes: %s", got)
	}
	if got := pids(Filter(ps, 1000, true, false, "")); got != "1,900,901,950" {
		t.Errorf("all users, no kernel threads: %s", got)
	}
	if got := pids(Filter(ps, 1000, true, true, "")); got != "1,2,900,901,950" {
		t.Errorf("with kernel threads: %s", got)
	}
	for query, want := range map[string]string{"보고서": "950", "BASH": "901", "root": "1", "90": "900,901", "nothing": ""} {
		if got := pids(Filter(ps, 1000, true, false, query)); got != want {
			t.Errorf("filter %q: %s, want %s", query, got, want)
		}
	}
}

func TestAncestors(t *testing.T) {
	snap, _ := machine().source().Read()
	got := Ancestors(snap.Procs, 950)
	if len(got) != 3 || got[0] != 901 || got[1] != 900 || got[2] != 1 {
		t.Fatalf("ancestors of 950: %v", got)
	}
	if got := Ancestors(snap.Procs, 4242); len(got) != 0 {
		t.Fatalf("unknown process: %v", got)
	}
}

func TestUsingDirFindsWhoKeepsADriveBusy(t *testing.T) {
	got := machine().source().UsingDir("/mnt/offline")
	if len(got) != 2 {
		t.Fatalf("users: %+v", got)
	}
	if got[0].PID != 901 || got[0].Name != "bash" || got[0].What != "working folder" || got[0].Path != "/mnt/offline/작업" {
		t.Errorf("shell: %+v", got[0])
	}
	if got[1].PID != 950 || got[1].What != "open file" || got[1].Path != "/mnt/offline/data.bin" {
		t.Errorf("program: %+v", got[1])
	}
	// /mnt/offline2 is another folder, not below /mnt/offline.
	if got := machine().source().UsingDir("/mnt/off"); len(got) != 0 {
		t.Errorf("prefix of a name must not match: %+v", got)
	}
}

func TestBrokenInputIsSkipped(t *testing.T) {
	for _, bad := range []string{"", "12 bash S 1", "x (bash) S 1", "12 (bash) S"} {
		if _, ok := ParseStat(bad); ok {
			t.Errorf("ParseStat(%q) accepted", bad)
		}
	}
	if got := ParseCmdline("a\x00b\nc\x1b[31m\x00"); got != "a b c [31m" {
		t.Errorf("cmdline: %q", got)
	}
	if _, err := (fakeProc{}).source().Read(); err == nil {
		t.Error("an empty /proc must be an error")
	}
}
