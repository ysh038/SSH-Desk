package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"golang.org/x/sys/unix"

	"tuidesk/internal/host"
	"tuidesk/internal/term"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

// MenuKey opens the terminal menu. Every other key goes to the program, so
// this is how the keyboard reaches the desktop from a terminal (ADR 0004).
const MenuKey = "Ctrl+]"

// App is a terminal window.
type App struct {
	h    host.Host
	spec Spec
	name string // "Terminal" or the app's name
	// Hold keeps the window open after the program exits (running a file).
	Hold bool

	s        *Session
	startErr error
	exited   bool
	code     int

	cols, rows int
	scroll     int  // lines scrolled back (0 = live screen)
	scrollMode bool // keyboard scrollback (from the menu)

	// mouse selection in screen coordinates (only when the program does not
	// use the mouse)
	selecting bool
	selA      [2]int
	selB      [2]int
	hasSel    bool
}

// New starts spec in a terminal window. name is the fallback title.
func New(h host.Host, spec Spec, name string) *App {
	if name == "" {
		name = "Terminal"
	}
	a := &App{h: h, spec: spec, name: name, cols: 80, rows: 24}
	a.start()
	return a
}

func (a *App) start() {
	a.exited, a.startErr, a.scroll, a.hasSel = false, nil, 0, false
	s, err := Start(a.spec, a.cols, a.rows, a.h.Post)
	if err != nil {
		a.startErr = err
		return
	}
	s.onExit = func(code int, err error) {
		if a.s == s { // not a session replaced by Restart
			a.onExit(code, err)
		}
	}
	a.s = s
}

func (a *App) onExit(code int, err error) {
	if a.exited {
		return
	}
	a.exited, a.code = true, code
	if err == nil && code == 0 && !a.Hold {
		a.h.CloseApp(a)
	}
}

// Session exposes the running session (tests).
func (a *App) Session() *Session { return a.s }

// Exited reports that the program has ended and its exit code.
func (a *App) Exited() (bool, int) { return a.exited, a.code }

func (a *App) Title() string {
	t := a.name
	if a.s != nil && a.s.title != "" {
		t = a.s.title
	} else if a.spec.Dir != "" && len(a.spec.Argv) == 0 {
		t = a.name + " - " + tildePath(a.spec.Dir)
	}
	if a.h.Console() {
		t = term.EscapeNonASCII(t)
	}
	return t
}

func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func (a *App) MinSize() (int, int) { return 20, 4 }

func (a *App) Close() error {
	if a.s != nil {
		a.s.Close()
	}
	return nil
}

// Running names the program in the foreground if it is not the shell
// itself (so closing the window would kill it), else "".
func (a *App) Running() string {
	if a.s == nil || a.exited {
		return ""
	}
	pid := a.s.Pid()
	pgrp := 0
	if rc, err := a.s.pty.SyscallConn(); err == nil {
		_ = rc.Control(func(fd uintptr) { pgrp, _ = unix.IoctlGetInt(int(fd), unix.TIOCGPGRP) })
	}
	if len(a.spec.Argv) == 0 && (pgrp == 0 || pgrp == pid) {
		return "" // just the shell prompt
	}
	if pgrp <= 0 {
		pgrp = pid
	}
	if b, err := os.ReadFile("/proc/" + strconv.Itoa(pgrp) + "/comm"); err == nil {
		return strings.TrimSpace(string(b))
	}
	if len(a.spec.Argv) > 0 && pgrp == pid {
		return filepath.Base(a.spec.Argv[0])
	}
	return "a program"
}

// ---- drawing ----

func styleOf(c *uv.Cell) wm.TermStyle {
	st := c.Style
	t := wm.TermStyle{FG: wm.TermColor(colorOf(st.Fg)), BG: wm.TermColor(colorOf(st.Bg))}
	at := st.Attrs
	if at&uv.AttrBold != 0 {
		t.Attr |= wm.AttrBold
	}
	if at&uv.AttrFaint != 0 {
		t.Attr |= wm.AttrDim
	}
	if at&uv.AttrItalic != 0 {
		t.Attr |= wm.AttrItalic
	}
	if at&uv.AttrBlink != 0 {
		t.Attr |= wm.AttrBlink
	}
	if at&uv.AttrReverse != 0 {
		t.Attr |= wm.AttrReverse
	}
	if at&uv.AttrStrikethrough != 0 {
		t.Attr |= wm.AttrStrike
	}
	if st.Underline != uv.UnderlineNone {
		t.Attr |= wm.AttrUnderline
	}
	if at&uv.AttrConceal != 0 {
		t.FG = t.BG
	}
	return t
}

// cellAt returns the emulator cell shown at screen (x, y), counting the
// scrollback offset.
func (a *App) cellAt(x, y int) *uv.Cell {
	e := a.s.emu
	sb := e.ScrollbackLen()
	v := sb - a.scroll + y
	if v < sb {
		return e.ScrollbackCellAt(x, v)
	}
	return e.CellAt(x, v-sb)
}

func (a *App) Draw(s wm.Surface, focused bool) {
	w, h := s.Size()
	if a.startErr != nil {
		s.Clear(wm.StyleBody)
		s.Text(1, 0, "Cannot start "+a.cmdName()+": "+a.startErr.Error(), wm.StyleDialogError)
		return
	}
	if w != a.cols || h != a.rows {
		a.cols, a.rows = w, h
		a.s.Resize(w, h)
	}
	a.scroll = min(a.scroll, a.s.emu.ScrollbackLen())
	cur := a.s.emu.CursorPosition()
	showCursor := focused && !a.s.emu.CursorHidden() && a.scroll == 0 && !a.exited
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := a.cellAt(x, y)
			if c == nil {
				s.SetTerm(x, y, ' ', wm.TermStyle{})
				continue
			}
			if c.Width == 0 && c.Content == "" {
				continue // right half of a wide glyph drawn at x-1
			}
			r := ' '
			if c.Content != "" {
				r = []rune(c.Content)[0]
			}
			if a.h.Console() && r > 0x7e && term.RuneWidth(r) == 2 {
				r = '?' // no wide glyphs on the Linux console font
			}
			t := styleOf(c)
			if (showCursor && x == cur.X && y == cur.Y) || a.selected(x, y) {
				t.Attr ^= wm.AttrReverse
			}
			s.SetTerm(x, y, r, t)
		}
	}
	switch {
	case a.exited:
		msg := fmt.Sprintf(" Process exited (code %d) · Enter closes · %s menu ", a.code, term.KeyLabel(MenuKey))
		s.Text(max(w-term.Width(msg), 0), h-1, msg, wm.StyleToast)
	case a.scrollMode || a.scroll > 0:
		msg := fmt.Sprintf(" Scrollback -%d · arrows/PgUp/PgDn · Esc ", a.scroll)
		s.Text(max(w-term.Width(msg), 0), 0, msg, wm.StyleBarActive)
	}
}

func (a *App) cmdName() string {
	if len(a.spec.Argv) > 0 {
		return a.spec.Argv[0]
	}
	return "shell"
}

// ---- selection ----

func order(p, q [2]int) ([2]int, [2]int) {
	if p[1] > q[1] || (p[1] == q[1] && p[0] > q[0]) {
		return q, p
	}
	return p, q
}

func (a *App) selected(x, y int) bool {
	if !a.hasSel {
		return false
	}
	p, q := order(a.selA, a.selB)
	if y < p[1] || y > q[1] {
		return false
	}
	if y == p[1] && x < p[0] {
		return false
	}
	if y == q[1] && x > q[0] {
		return false
	}
	return true
}

// screenText returns the text of screen rows [y0, y1] (trailing blanks cut).
func (a *App) lineText(y, x0, x1 int) string {
	var b strings.Builder
	for x := x0; x <= x1 && x < a.cols; x++ {
		c := a.cellAt(x, y)
		switch {
		case c == nil:
			b.WriteByte(' ')
		case c.Width == 0 && c.Content == "":
		case c.Content == "":
			b.WriteByte(' ')
		default:
			b.WriteString(c.Content)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// SelectionText is the selected text (or "" without a selection).
func (a *App) SelectionText() string {
	if !a.hasSel {
		return ""
	}
	p, q := order(a.selA, a.selB)
	var lines []string
	for y := p[1]; y <= q[1]; y++ {
		x0, x1 := 0, a.cols-1
		if y == p[1] {
			x0 = p[0]
		}
		if y == q[1] {
			x1 = q[0]
		}
		lines = append(lines, a.lineText(y, x0, x1))
	}
	return strings.Join(lines, "\n")
}

// ScreenText is the visible text, one line per row (trailing blank rows cut).
func (a *App) ScreenText() string {
	if a.s == nil {
		return ""
	}
	var lines []string
	for y := 0; y < a.rows; y++ {
		lines = append(lines, a.lineText(y, 0, a.cols-1))
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func (a *App) copySelection() {
	if t := a.SelectionText(); t != "" {
		a.h.SetClipboardText(t)
		a.h.Notify(fmt.Sprintf("Copied %d characters", len([]rune(t))), false)
	}
}

// ---- events ----

func (a *App) HandleEvent(ev term.Event) bool {
	switch ev := ev.(type) {
	case term.KeyEvent:
		return a.handleKey(ev.Key)
	case term.PasteEvent:
		a.paste(ev.Text)
		return true
	case term.MouseEvent:
		return a.handleMouse(ev.Gesture)
	}
	return false
}

func (a *App) paste(text string) {
	if a.s == nil || a.exited || text == "" {
		return
	}
	a.scroll, a.scrollMode = 0, false
	a.s.Send(EncodePaste(text, a.s.Modes()))
}

func (a *App) handleKey(k term.Key) bool {
	name := k.String()
	if name == MenuKey {
		a.menu(1, 0)
		return true
	}
	if a.s == nil || a.startErr != nil {
		if name == "Enter" || name == "Esc" {
			a.h.CloseApp(a)
		}
		return true
	}
	if a.scrollMode {
		a.scrollKey(name)
		return true
	}
	if a.exited {
		switch name {
		case "Enter", "Esc":
			a.h.CloseApp(a)
		}
		return true
	}
	if !a.s.emu.IsAltScreen() {
		switch name {
		case "Shift+PgUp":
			a.scrollBy(a.rows - 1)
			return true
		case "Shift+PgDn":
			a.scrollBy(-(a.rows - 1))
			return true
		}
	}
	a.scroll, a.hasSel = 0, false
	a.s.Send(EncodeKey(k, a.s.Modes()))
	return true
}

func (a *App) scrollBy(n int) {
	a.scroll = min(max(a.scroll+n, 0), a.s.emu.ScrollbackLen())
}

func (a *App) scrollKey(name string) {
	switch name {
	case "Up":
		a.scrollBy(1)
	case "Down":
		a.scrollBy(-1)
	case "PgUp", "Shift+PgUp":
		a.scrollBy(a.rows - 1)
	case "PgDn", "Shift+PgDn":
		a.scrollBy(-(a.rows - 1))
	case "Home":
		a.scrollBy(a.s.emu.ScrollbackLen())
	case "End":
		a.scroll = 0
	default: // Esc, q, Enter, anything else: back to the live screen
		a.scrollMode, a.scroll = false, 0
	}
}

func (a *App) handleMouse(g term.Gesture) bool {
	if a.s == nil {
		return true
	}
	m := a.s.Modes()
	if m.MouseOn() && a.scroll == 0 && !a.exited {
		ev := MouseEvent{X: g.X, Y: g.Y, Button: g.Button, Mod: g.Mod}
		switch g.Kind {
		case term.GesturePress:
			a.s.Send(EncodeMouse(ev, m))
		case term.GestureDrag:
			ev.Motion = true
			a.s.Send(EncodeMouse(ev, m))
		case term.GestureRelease:
			ev.Release = true
			a.s.Send(EncodeMouse(ev, m))
		case term.GestureClick:
			if g.Button != term.ButtonLeft { // the desktop does not pass right/middle presses
				a.s.Send(EncodeMouse(ev, m))
				ev.Release = true
				a.s.Send(EncodeMouse(ev, m))
			}
		case term.GestureWheel:
			ev.Wheel = g.Wheel
			a.s.Send(EncodeMouse(ev, m))
		}
		return true
	}
	switch g.Kind {
	case term.GestureWheel:
		up := g.Wheel == term.WheelUp
		if a.s.emu.IsAltScreen() && !a.exited {
			// Full-screen programs without mouse support (less, man):
			// the wheel sends arrow keys, like most terminals do.
			key := term.Key{Name: "Down"}
			if up {
				key.Name = "Up"
			}
			for range 3 {
				a.s.Send(EncodeKey(key, m))
			}
		} else if up {
			a.scrollBy(3)
		} else {
			a.scrollBy(-3)
		}
	case term.GesturePress:
		a.selecting, a.hasSel = true, false
		a.selA = [2]int{g.X, g.Y}
	case term.GestureDragStart, term.GestureDrag:
		if a.selecting {
			a.selB = [2]int{min(max(g.X, 0), a.cols-1), min(max(g.Y, 0), a.rows-1)}
			a.hasSel = true
		}
	case term.GestureRelease, term.GestureDragEnd:
		if a.selecting && a.hasSel {
			a.copySelection()
		}
		a.selecting = false
	case term.GestureClick:
		if g.Button == term.ButtonRight {
			a.menu(g.X, g.Y)
		} else {
			a.hasSel = false
		}
	}
	return true
}

// menu is the terminal menu (Ctrl+] or right click): the keyboard's way to
// the desktop while every other key goes to the program.
func (a *App) menu(x, y int) {
	running := a.s != nil && !a.exited && a.startErr == nil
	items := []ui.MenuItem{
		{Label: "Paste", Hotkey: 'v', Action: "paste", Disabled: !running || a.h.ClipboardText() == ""},
		{Label: "Copy selection", Hotkey: 'c', Action: "copy", Disabled: !a.hasSel},
		{Label: "Copy screen text", Hotkey: 't', Action: "copyscreen", Disabled: a.s == nil},
		{Label: "Scroll back...", Hotkey: 's', Action: "scroll", Disabled: a.s == nil || a.s.emu.ScrollbackLen() == 0},
		{Separator: true},
		{Label: "Next window", Hotkey: 'n', Shortcut: "F6", Action: "next"},
		{Label: "Previous window", Hotkey: 'p', Shortcut: "Shift+F6", Action: "prev"},
		{Label: "Desktop menu", Hotkey: 'm', Shortcut: "F9", Action: "launcher"},
		{Label: "Window menu", Hotkey: 'w', Shortcut: "Shift+F10", Action: "windowmenu"},
		{Separator: true},
		{Label: "Send " + term.KeyLabel(MenuKey) + " to the program", Hotkey: ']', Action: "sendmenu", Disabled: !running},
		{Label: "Restart", Hotkey: 'r', Action: "restart", Disabled: running},
		{Label: "Close terminal", Hotkey: 'l', Action: "close"},
	}
	a.h.OpenMenu(a, x, y, items, a.runMenu)
}

func (a *App) runMenu(action string) {
	switch action {
	case "paste":
		a.paste(a.h.ClipboardText())
	case "copy":
		a.copySelection()
	case "copyscreen":
		a.h.SetClipboardText(a.ScreenText())
		a.h.Notify("Copied the screen text", false)
	case "scroll":
		a.scrollMode = true
		a.scrollBy(a.rows - 1)
	case "sendmenu":
		a.s.Send([]byte{0x1d})
	case "restart":
		if a.s != nil {
			a.s.Close()
		}
		a.start()
	case "close":
		a.h.DesktopAction("close")
	default:
		a.h.DesktopAction(action)
	}
}
