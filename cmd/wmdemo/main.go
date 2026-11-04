// Command wmdemo is the Phase 0 completion demo: several overlapping windows
// that can be moved with the mouse (title bar drag) or the keyboard.
//
// Mouse: drag a title bar to move, click a window to bring it to the front.
// Keyboard: Tab or F6 = next window, Shift+Tab = previous window,
// arrow keys = move the front window,
// F10 or Ctrl+Q = quit.
package main

import (
	"fmt"
	"os"

	"github.com/gdamore/tcell/v2"

	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

type app struct {
	s      tcell.Screen
	det    *term.Detector
	m      *wm.Manager
	canvas *wm.Canvas
	styles map[wm.StyleID]tcell.Style
	quit   bool
}

func newApp(s tcell.Screen) *app {
	w, h := s.Size()
	a := &app{
		s:      s,
		det:    term.NewDetector(),
		m:      &wm.Manager{Desktop: wm.Rect{X: 0, Y: 1, W: w, H: h - 2}},
		canvas: wm.NewCanvas(w, h),
	}
	a.det.MultiClickInterval = 500_000_000 // 500ms, see compat-matrix finding 7
	a.styles = stylesFor(s.Colors())

	a.m.Add(&wm.Window{ID: 1, Title: "Files", Rect: wm.Rect{X: 3, Y: 3, W: 40, H: 12}, Body: []string{
		" [d] nginx/",
		" [d] backup/",
		"    nginx.conf          2.1K",
		"    한글 파일명.txt       512",
		"    access.log         18.4M",
	}})
	a.m.Add(&wm.Window{ID: 2, Title: "Editor - nginx.conf", Rect: wm.Rect{X: 22, Y: 7, W: 44, H: 12}, Body: []string{
		" server {",
		"     listen 80;",
		"     server_name example.com;  # example",
		"     location / {",
		"         proxy_pass http://app;",
		"     }",
		" }",
	}})
	a.m.Add(&wm.Window{ID: 3, Title: "Help", Rect: wm.Rect{X: 50, Y: 2, W: 34, H: 10}, Body: []string{
		" Mouse: drag a title bar to move",
		"        click a window to raise",
		" Keys:  Tab/F6 next window",
		"        Shift+Tab previous window",
		"        arrows move front window",
		" Quit:  F10 or Ctrl+Q",
	}})
	return a
}

// stylesFor maps window styles to colors that also work on an 8-color
// Linux console.
func stylesFor(colors int) map[wm.StyleID]tcell.Style {
	base := tcell.StyleDefault
	st := map[wm.StyleID]tcell.Style{
		wm.StyleDesktop:      base.Background(tcell.ColorNavy).Foreground(tcell.ColorSilver),
		wm.StyleBar:          base.Background(tcell.ColorSilver).Foreground(tcell.ColorBlack),
		wm.StyleFrame:        base.Background(tcell.ColorBlack).Foreground(tcell.ColorGray),
		wm.StyleFrameFocused: base.Background(tcell.ColorBlack).Foreground(tcell.ColorAqua).Bold(true),
		wm.StyleTitle:        base.Background(tcell.ColorBlack).Foreground(tcell.ColorSilver),
		wm.StyleTitleFocused: base.Background(tcell.ColorTeal).Foreground(tcell.ColorWhite).Bold(true),
		wm.StyleBody:         base.Background(tcell.ColorBlack).Foreground(tcell.ColorSilver),
	}
	if colors < 16 {
		st[wm.StyleFrame] = base.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)
	}
	return st
}

func (a *app) resize() {
	w, h := a.s.Size()
	a.canvas.Resize(w, h)
	a.m.Desktop = wm.Rect{X: 0, Y: 1, W: w, H: max(h-2, 1)}
	a.m.ClampAll()
}

func (a *app) handle(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		a.resize()
		a.s.Sync()
	case *tcell.EventKey:
		a.handleKey(ev)
	case *tcell.EventMouse:
		a.handleMouse(ev)
	}
}

func (a *app) handleKey(ev *tcell.EventKey) {
	mod := toMod(ev.Modifiers())
	if ev.Key() == tcell.KeyF10 || ev.Key() == tcell.KeyCtrlQ ||
		(ev.Key() == tcell.KeyRune && mod&term.ModCtrl != 0 && term.ShortcutRune(ev.Rune(), mod) == 'q') {
		a.quit = true
		return
	}
	f := a.m.Focused()
	switch ev.Key() {
	case tcell.KeyTab, tcell.KeyF6:
		a.m.FocusNext()
	case tcell.KeyBacktab:
		a.m.FocusPrev()
	case tcell.KeyUp:
		a.m.MoveBy(f, 0, -1)
	case tcell.KeyDown:
		a.m.MoveBy(f, 0, 1)
	case tcell.KeyLeft:
		a.m.MoveBy(f, -2, 0)
	case tcell.KeyRight:
		a.m.MoveBy(f, 2, 0)
	}
}

func (a *app) handleMouse(ev *tcell.EventMouse) {
	x, y := ev.Position()
	in := term.MouseInput{X: x, Y: y, Mod: toMod(ev.Modifiers()), Time: ev.When()}
	switch b := ev.Buttons(); {
	case b&(tcell.WheelUp|tcell.WheelDown|tcell.WheelLeft|tcell.WheelRight) != 0:
		return
	case b&tcell.Button1 != 0:
		in.Button = term.ButtonLeft
	case b&tcell.Button2 != 0:
		in.Button = term.ButtonRight
	case b&tcell.Button3 != 0:
		in.Button = term.ButtonMiddle
	}
	for _, g := range a.det.Feed(in) {
		if g.Button != term.ButtonLeft {
			continue
		}
		switch g.Kind {
		case term.GesturePress:
			a.m.PointerDown(g.X, g.Y)
		case term.GestureDrag:
			a.m.PointerDrag(g.X, g.Y)
		case term.GestureRelease:
			a.m.PointerUp()
		}
	}
}

func toMod(m tcell.ModMask) term.Mod {
	var out term.Mod
	if m&tcell.ModShift != 0 {
		out |= term.ModShift
	}
	if m&tcell.ModCtrl != 0 {
		out |= term.ModCtrl
	}
	if m&tcell.ModAlt != 0 {
		out |= term.ModAlt
	}
	if m&tcell.ModMeta != 0 {
		out |= term.ModMeta
	}
	return out
}

func (a *app) draw() {
	c := a.canvas
	full := wm.Rect{X: 0, Y: 0, W: c.W, H: c.H}
	c.Fill(wm.Rect{X: 0, Y: 0, W: c.W, H: 1}, ' ', wm.StyleBar)
	c.Text(full, 1, 0, "tuidesk wmdemo │ drag title to move · Tab/Shift+Tab switch · arrows move · F10 quit", wm.StyleBar)
	a.m.Draw(c)

	status := "ready"
	if f := a.m.Focused(); f != nil {
		status = fmt.Sprintf("front: %s  at (%d,%d)", f.Title, f.Rect.X, f.Rect.Y)
		if a.m.Dragging() {
			status = "moving · " + status
		}
	}
	c.Fill(wm.Rect{X: 0, Y: c.H - 1, W: c.W, H: 1}, ' ', wm.StyleBar)
	c.Text(full, 1, c.H-1, status, wm.StyleBar)

	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			cell := c.At(x, y)
			if cell.Width == 0 {
				continue // right half of a wide glyph: the terminal fills it
			}
			a.s.SetContent(x, y, cell.Rune, nil, a.styles[cell.Style])
		}
	}
	a.s.Show()
}

func main() {
	s, err := tcell.NewScreen()
	if err == nil {
		err = s.Init()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wmdemo: cannot initialize the terminal:", err)
		os.Exit(1)
	}
	s.EnableMouse(tcell.MouseDragEvents)
	s.HideCursor()

	a := newApp(s)
	a.draw()
	for !a.quit {
		ev := s.PollEvent()
		if ev == nil {
			break
		}
		a.handle(ev)
		if !a.quit {
			a.draw()
		}
	}
	s.Fini()
}
