// Package procs reads the processes of this server from /proc (SPEC 7.9,
// ADR 0006): what the Task Manager lists and what My PC names when a drive
// is busy. Everything goes through Source, so the parsing is tested with
// fixtures and never needs a real /proc.
package procs

import (
	"errors"
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Source is where process information comes from.
type Source struct {
	// Root is the proc filesystem ("/proc").
	Root     string
	ReadFile func(path string) ([]byte, error)
	// ReadDir lists the names in a folder.
	ReadDir  func(path string) ([]string, error)
	Readlink func(path string) (string, error)
	// UserName turns a user id into a name ("" = unknown).
	UserName func(uid int) string
}

// System reads the real /proc.
func System() Source {
	names := map[int]string{}
	return Source{
		Root:     "/proc",
		ReadFile: os.ReadFile,
		ReadDir: func(p string) ([]string, error) {
			f, err := os.Open(p)
			if err != nil {
				return nil, err
			}
			defer f.Close()
			return f.Readdirnames(-1)
		},
		Readlink: os.Readlink,
		// Only called from one goroutine at a time (the sampler).
		UserName: func(uid int) string {
			if n, ok := names[uid]; ok {
				return n
			}
			n := ""
			if u, err := user.LookupId(strconv.Itoa(uid)); err == nil {
				n = u.Username
			}
			names[uid] = n
			return n
		},
	}
}

// Proc is one process.
type Proc struct {
	PID, PPID int
	UID       int
	User      string
	Name      string // short name ("bash")
	Cmd       string // full command line ("" for kernel threads)
	State     byte   // R, S, D, Z, T, I...
	Ticks     uint64 // CPU time used so far, in clock ticks
	RSS       int64  // memory in use, bytes
	Threads   int
	Kernel    bool // a kernel thread: no command line, cannot be ended
	// CPU is the share of one CPU core used since the previous snapshot,
	// in percent (100 = one core fully busy). Filled in by Diff.
	CPU float64
}

// Title is what the Command column shows: the command line, else the name.
func (p Proc) Title() string {
	if p.Cmd != "" {
		return p.Cmd
	}
	return "[" + p.Name + "]"
}

// StateName spells the state letter out.
func (p Proc) StateName() string {
	switch p.State {
	case 'R':
		return "Running"
	case 'S', 'I':
		return "Sleeping"
	case 'D':
		return "Waiting"
	case 'Z':
		return "Zombie"
	case 'T', 't':
		return "Stopped"
	}
	return string(p.State)
}

// Sys is the state of the whole machine.
type Sys struct {
	MemTotal, MemAvail  int64 // bytes
	SwapTotal, SwapFree int64
	Load                [3]float64
	Uptime              time.Duration
	CPUs                int
	busy, total         uint64 // CPU ticks since boot
	// CPU is how busy all cores together were since the previous snapshot
	// (0-100). Filled in by Diff.
	CPU float64
}

// Snapshot is everything read at one moment.
type Snapshot struct {
	Procs []Proc
	Sys   Sys
}

// ParseStat reads /proc/<pid>/stat. The name is in parentheses and may
// itself contain spaces and parentheses, so the fields are counted from
// the last ")".
//
//	1234 (my prog) S 1 1234 1234 0 -1 4194304 ... utime stime ... threads ...
func ParseStat(data string) (p Proc, ok bool) {
	open, end := strings.IndexByte(data, '('), strings.LastIndexByte(data, ')')
	if open < 0 || end < open {
		return p, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(data[:open]))
	if err != nil {
		return p, false
	}
	f := strings.Fields(data[end+1:])
	if len(f) < 18 {
		return p, false
	}
	p.PID, p.Name, p.State = pid, data[open+1:end], f[0][0]
	p.PPID, _ = strconv.Atoi(f[1])
	ut, _ := strconv.ParseUint(f[11], 10, 64)
	st, _ := strconv.ParseUint(f[12], 10, 64)
	p.Ticks = ut + st
	p.Threads, _ = strconv.Atoi(f[17])
	return p, true
}

// kb reads a "   1234 kB" value as bytes.
func kb(v string) int64 {
	f := strings.Fields(v)
	if len(f) == 0 {
		return 0
	}
	n, _ := strconv.ParseInt(f[0], 10, 64)
	return n * 1024
}

// ParseStatus reads the user id and the memory in use from
// /proc/<pid>/status. Kernel threads have no VmRSS line.
func ParseStatus(data string) (uid int, rss int64, hasMem bool) {
	for _, line := range strings.Split(data, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch k {
		case "Uid":
			if f := strings.Fields(v); len(f) > 0 {
				uid, _ = strconv.Atoi(f[0])
			}
		case "VmRSS":
			rss, hasMem = kb(v), true
		}
	}
	return uid, rss, hasMem
}

// ParseCmdline turns the NUL-separated /proc/<pid>/cmdline into one line.
// Control characters would break the list and are replaced by spaces.
func ParseCmdline(data string) string {
	data = strings.TrimRight(data, "\x00")
	var b strings.Builder
	for _, r := range data {
		if r < 0x20 || r == 0x7f {
			r = ' '
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// ParseMeminfo reads /proc/meminfo.
func ParseMeminfo(data string, s *Sys) {
	for _, line := range strings.Split(data, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch k {
		case "MemTotal":
			s.MemTotal = kb(v)
		case "MemAvailable":
			s.MemAvail = kb(v)
		case "SwapTotal":
			s.SwapTotal = kb(v)
		case "SwapFree":
			s.SwapFree = kb(v)
		}
	}
}

// ParseCPU reads /proc/stat: the busy and total ticks of all cores and the
// number of cores.
//
//	cpu  user nice system idle iowait irq softirq steal ...
func ParseCPU(data string, s *Sys) {
	for _, line := range strings.Split(data, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		if f[0] != "cpu" {
			s.CPUs++
			continue
		}
		var total, idle uint64
		for i, v := range f[1:] {
			n, _ := strconv.ParseUint(v, 10, 64)
			if i >= 8 {
				break // guest time is already counted in user
			}
			total += n
			if i == 3 || i == 4 { // idle, iowait
				idle += n
			}
		}
		s.total, s.busy = total, total-idle
	}
	if s.CPUs == 0 {
		s.CPUs = 1
	}
}

// Read takes a snapshot. Processes that end while they are being read are
// skipped. It fails only when the process list itself cannot be read.
func (s Source) Read() (Snapshot, error) {
	var snap Snapshot
	names, err := s.ReadDir(s.Root)
	if err != nil {
		return snap, err
	}
	for _, n := range names {
		pid, err := strconv.Atoi(n)
		if err != nil {
			continue
		}
		dir := s.Root + "/" + n
		stat, err := s.ReadFile(dir + "/stat")
		if err != nil {
			continue
		}
		p, ok := ParseStat(string(stat))
		if !ok || p.PID != pid {
			continue
		}
		hasMem := false
		if status, err := s.ReadFile(dir + "/status"); err == nil {
			p.UID, p.RSS, hasMem = ParseStatus(string(status))
		}
		if cmd, err := s.ReadFile(dir + "/cmdline"); err == nil {
			p.Cmd = ParseCmdline(string(cmd))
		}
		p.Kernel = p.Cmd == "" && !hasMem
		if s.UserName != nil {
			p.User = s.UserName(p.UID)
		}
		if p.User == "" {
			p.User = strconv.Itoa(p.UID)
		}
		snap.Procs = append(snap.Procs, p)
	}
	if len(snap.Procs) == 0 {
		return snap, errors.New("no processes found in " + s.Root)
	}
	if b, err := s.ReadFile(s.Root + "/meminfo"); err == nil {
		ParseMeminfo(string(b), &snap.Sys)
	}
	if b, err := s.ReadFile(s.Root + "/stat"); err == nil {
		ParseCPU(string(b), &snap.Sys)
	}
	if b, err := s.ReadFile(s.Root + "/loadavg"); err == nil {
		f := strings.Fields(string(b))
		for i := 0; i < 3 && i < len(f); i++ {
			snap.Sys.Load[i], _ = strconv.ParseFloat(f[i], 64)
		}
	}
	if b, err := s.ReadFile(s.Root + "/uptime"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			sec, _ := strconv.ParseFloat(f[0], 64)
			snap.Sys.Uptime = time.Duration(sec * float64(time.Second))
		}
	}
	return snap, nil
}

// Diff fills in the CPU percentages of cur from the time that passed since
// prev. Without a previous snapshot they stay 0.
func Diff(prev, cur *Snapshot) {
	if prev == nil || cur.Sys.total <= prev.Sys.total {
		return
	}
	dt := float64(cur.Sys.total - prev.Sys.total)
	cur.Sys.CPU = 100 * float64(cur.Sys.busy-min(prev.Sys.busy, cur.Sys.busy)) / dt
	old := make(map[int]uint64, len(prev.Procs))
	for _, p := range prev.Procs {
		old[p.PID] = p.Ticks
	}
	for i := range cur.Procs {
		p := &cur.Procs[i]
		if before, ok := old[p.PID]; ok && p.Ticks >= before {
			// All cores together tick dt times: one core is dt/CPUs.
			p.CPU = 100 * float64(p.Ticks-before) * float64(cur.Sys.CPUs) / dt
		}
	}
}

// SortKey is a column of the process list.
type SortKey int

const (
	ByCPU SortKey = iota
	ByMemory
	ByPID
	ByName
	ByUser
)

// Sort orders ps by key. CPU and memory start with the largest; reverse
// flips the order. Ties are broken by PID so the list does not jump.
func Sort(ps []Proc, key SortKey, reverse bool) {
	less := func(a, b Proc) bool {
		switch key {
		case ByCPU:
			if a.CPU != b.CPU {
				return a.CPU > b.CPU
			}
		case ByMemory:
			if a.RSS != b.RSS {
				return a.RSS > b.RSS
			}
		case ByName:
			if x, y := strings.ToLower(a.Title()), strings.ToLower(b.Title()); x != y {
				return x < y
			}
		case ByUser:
			if a.User != b.User {
				return a.User < b.User
			}
		}
		return a.PID < b.PID
	}
	sort.SliceStable(ps, func(i, j int) bool {
		if reverse {
			return less(ps[j], ps[i])
		}
		return less(ps[i], ps[j])
	})
}

// Filter keeps the processes the user wants to see: only those of uid
// unless all is set, no kernel threads unless kernel is set, and only
// those whose name, command line, user or PID contain query.
func Filter(ps []Proc, uid int, all, kernel bool, query string) []Proc {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []Proc
	for _, p := range ps {
		if p.Kernel && !kernel {
			continue
		}
		if !all && p.UID != uid {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(p.Name+" "+p.Cmd+" "+p.User+" "+strconv.Itoa(p.PID)), q) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// Ancestors returns the PIDs above pid, nearest first: the shell and the
// sshd session a program runs in.
func Ancestors(ps []Proc, pid int) []int {
	parent := make(map[int]int, len(ps))
	for _, p := range ps {
		parent[p.PID] = p.PPID
	}
	var out []int
	for i := 0; i < 64; i++ {
		pp, ok := parent[pid]
		if !ok || pp <= 0 {
			break
		}
		out = append(out, pp)
		pid = pp
	}
	return out
}

// User is a process that keeps a folder busy.
type User struct {
	PID  int
	Name string
	What string // "working folder" or "open file"
	Path string
}

// maxFDs bounds how many open files of one process are looked at.
const maxFDs = 4096

func inside(dir, path string) bool {
	dir = strings.TrimRight(dir, "/")
	return path == dir || strings.HasPrefix(path, dir+"/")
}

// UsingDir finds the processes whose working folder or open files are in
// dir or below it: why a drive cannot be unmounted. Processes of other
// users cannot be inspected without root and are silently left out.
func (s Source) UsingDir(dir string) []User {
	names, err := s.ReadDir(s.Root)
	if err != nil {
		return nil
	}
	var out []User
	for _, n := range names {
		pid, err := strconv.Atoi(n)
		if err != nil {
			continue
		}
		base := s.Root + "/" + n
		u := User{PID: pid}
		if cwd, err := s.Readlink(base + "/cwd"); err == nil && inside(dir, cwd) {
			u.What, u.Path = "working folder", cwd
		} else if fds, err := s.ReadDir(base + "/fd"); err == nil {
			for i, fd := range fds {
				if i >= maxFDs {
					break
				}
				if p, err := s.Readlink(base + "/fd/" + fd); err == nil && inside(dir, p) {
					u.What, u.Path = "open file", p
					break
				}
			}
		}
		if u.What == "" {
			continue
		}
		if stat, err := s.ReadFile(base + "/stat"); err == nil {
			if p, ok := ParseStat(string(stat)); ok {
				u.Name = p.Name
			}
		}
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out
}
