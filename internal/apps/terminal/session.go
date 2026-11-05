package terminal

import (
	"errors"
	"image/color"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

const (
	drainChunk   = 256 << 10 // bytes fed to the emulator per UI turn
	maxPending   = 1 << 20   // reader blocks above this (backpressure)
	scrollbackSz = 5000      // lines kept above the screen
)

// Spec says what to run.
type Spec struct {
	Argv []string // empty: the user's shell
	Dir  string
	Env  []string // extra KEY=VALUE pairs
}

// Session is a program on a pseudo-terminal plus its emulator. The emulator
// is only touched on the UI goroutine: output is handed over through post.
type Session struct {
	emu   *vt.Emulator
	pty   *os.File
	cmd   *exec.Cmd
	modes Modes
	title string
	cwd   string

	post     func(func())
	onOutput func() // UI goroutine, after output was processed
	onExit   func(code int, err error)

	in chan []byte // to the pty (keys, paste, emulator replies)

	mu       sync.Mutex
	cond     *sync.Cond
	pending  [][]byte
	pendingN int
	posted   bool
	closed   bool
}

// Shell returns the user's login shell, falling back to /bin/sh.
func Shell() string {
	if s := os.Getenv("SHELL"); s != "" {
		if _, err := os.Stat(s); err == nil {
			return s
		}
	}
	for _, s := range []string{"/bin/bash", "/bin/sh"} {
		if _, err := os.Stat(s); err == nil {
			return s
		}
	}
	return "/bin/sh"
}

// childEnv is the environment of programs in a terminal window.
func childEnv(extra []string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "TERM", "COLORTERM", "LINES", "COLUMNS", "TERM_PROGRAM", "TERM_PROGRAM_VERSION":
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "TERM=xterm-256color", "COLORTERM=truecolor", "TUIDESK=1")
	return append(env, extra...)
}

// Start runs spec on a new pty of w×h cells.
func Start(spec Spec, w, h int, post func(func())) (*Session, error) {
	argv := spec.Argv
	if len(argv) == 0 {
		argv = []string{Shell()}
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = childEnv(spec.Env)
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(max(w, 1)), Rows: uint16(max(h, 1))})
	if err != nil {
		return nil, err
	}
	s := &Session{
		emu:  vt.NewEmulator(max(w, 1), max(h, 1)),
		pty:  f,
		cmd:  cmd,
		post: post,
		in:   make(chan []byte, 1024),
		cwd:  spec.Dir,
	}
	s.cond = sync.NewCond(&s.mu)
	s.emu.SetScrollbackSize(scrollbackSz)
	s.emu.SetCallbacks(vt.Callbacks{
		Title:            func(t string) { s.title = t },
		WorkingDirectory: func(d string) { s.cwd = d },
		EnableMode:       func(m ansi.Mode) { s.setMode(m, true) },
		DisableMode:      func(m ansi.Mode) { s.setMode(m, false) },
	})
	go s.writeLoop()
	go s.replyLoop()
	go s.readLoop()
	return s, nil
}

func (s *Session) setMode(m ansi.Mode, on bool) {
	switch m {
	case ansi.ModeCursorKeys:
		s.modes.AppCursor = on
	case ansi.ModeMouseX10:
		s.modes.MouseX10 = on
	case ansi.ModeMouseNormal:
		s.modes.MouseNormal = on
	case ansi.ModeMouseButtonEvent:
		s.modes.MouseButton = on
	case ansi.ModeMouseAnyEvent:
		s.modes.MouseAny = on
	case ansi.ModeMouseExtSgr:
		s.modes.MouseSGR = on
	case ansi.ModeBracketedPaste:
		s.modes.BracketedPaste = on
	}
}

// Modes returns the modes the program has set.
func (s *Session) Modes() Modes { return s.modes }

// Send queues bytes for the program.
func (s *Session) Send(b []byte) {
	if len(b) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.in <- b:
	default:
		// The program is not reading and the queue is full: drop rather
		// than freeze the whole desktop.
	}
}

func (s *Session) writeLoop() {
	for b := range s.in {
		if _, err := s.pty.Write(b); err != nil {
			return
		}
	}
}

// replyLoop forwards the emulator's answers (cursor position reports,
// device attributes) to the program.
func (s *Session) replyLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := s.emu.Read(buf)
		if n > 0 {
			s.Send(append([]byte(nil), buf[:n]...))
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) readLoop() {
	buf := make([]byte, 32<<10)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.queue(append([]byte(nil), buf[:n]...))
		}
		if err != nil {
			break
		}
	}
	err := s.cmd.Wait()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
		err = nil
	}
	s.post(func() {
		s.drain() // whatever is still pending
		if s.onExit != nil {
			s.onExit(code, err)
		}
	})
}

func (s *Session) queue(b []byte) {
	s.mu.Lock()
	for s.pendingN > maxPending && !s.closed {
		s.cond.Wait()
	}
	s.pending = append(s.pending, b)
	s.pendingN += len(b)
	post := !s.posted
	s.posted = true
	s.mu.Unlock()
	if post {
		s.post(s.drain)
	}
}

// drain feeds pending output to the emulator (UI goroutine), at most
// drainChunk bytes per turn so keys stay responsive during a flood.
func (s *Session) drain() {
	s.mu.Lock()
	var batch [][]byte
	n := 0
	for len(s.pending) > 0 && n < drainChunk {
		batch = append(batch, s.pending[0])
		n += len(s.pending[0])
		s.pending = s.pending[1:]
	}
	s.pendingN -= n
	more := len(s.pending) > 0
	s.posted = more
	s.cond.Broadcast()
	s.mu.Unlock()
	for _, b := range batch {
		_, _ = s.emu.Write(b)
	}
	if more {
		s.post(s.drain)
	}
	if n > 0 && s.onOutput != nil {
		s.onOutput()
	}
}

// Resize changes the emulator and pty size (the program gets SIGWINCH).
func (s *Session) Resize(w, h int) {
	if w < 1 || h < 1 || (w == s.emu.Width() && h == s.emu.Height()) {
		return
	}
	s.emu.Resize(w, h)
	_ = pty.Setsize(s.pty, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
}

// Close hangs up the program (SIGHUP to its process group) and releases the pty.
func (s *Session) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.cond.Broadcast()
	close(s.in)
	s.mu.Unlock()
	if p := s.cmd.Process; p != nil {
		_ = syscall.Kill(-p.Pid, syscall.SIGHUP)
	}
	_ = s.pty.Close()
	_ = s.emu.Close()
}

// Pid of the program (0 if unknown).
func (s *Session) Pid() int {
	if s.cmd.Process == nil {
		return 0
	}
	return s.cmd.Process.Pid
}

// colorOf converts an emulator color to a tuidesk terminal color.
func colorOf(c color.Color) uint32 {
	switch v := c.(type) {
	case nil:
		return 0
	case ansi.BasicColor:
		return 1<<24 | uint32(v)
	case ansi.IndexedColor:
		return 1<<24 | uint32(v)
	}
	r, g, b, _ := c.RGBA()
	return 2<<24 | (r>>8)<<16 | (g>>8)<<8 | b>>8
}
