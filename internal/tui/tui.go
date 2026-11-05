// Package tui is the only place that talks to tcell for the desktop: it
// turns tcell events into normalized term events, schedules timer ticks and
// pushes changed canvas cells to the screen.
package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"

	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// Handler is what the loop drives (the desktop).
type Handler interface {
	Handle(ev term.Event)
	Draw(c *wm.Canvas)
	Resize(w, h int)
	NextWake() time.Time
	Done() bool
}

// poster is implemented by handlers that accept work from goroutines.
type poster interface {
	SetPoster(wake func())
	Flush() bool
}

// clipboarder is implemented by handlers that copy text to the user's clipboard.
type clipboarder interface{ SetClipboardWriter(func(string)) }

// Loop owns the tcell screen and the render state.
type Loop struct {
	S       tcell.Screen
	H       Handler
	Profile term.KeyProfile
	Det     *term.Detector
	Styles  [wm.NumStyles]tcell.Style
	Now     func() time.Time

	canvas  *wm.Canvas
	prev    *wm.Canvas
	pasting bool
	paste   strings.Builder
	timer   *time.Timer
	wakeAt  time.Time

	// Stats for tests and diagnostics.
	Frames, CellsWritten int
}

type tickData struct{}
type flushData struct{}

// NewLoop prepares a loop for an initialized screen.
func NewLoop(s tcell.Screen, h Handler) *Loop {
	w, ht := s.Size()
	l := &Loop{
		S:       s,
		H:       h,
		Profile: term.ProfileFor(os.Getenv("TERM")),
		Det:     term.NewDetector(),
		Styles:  StylesFor(s.Colors()),
		Now:     time.Now,
		canvas:  wm.NewCanvas(w, ht),
	}
	l.Det.MultiClickInterval = 500 * time.Millisecond // compat-matrix finding 7
	if p, ok := h.(poster); ok {
		p.SetPoster(func() { _ = s.PostEvent(tcell.NewEventInterrupt(flushData{})) })
	}
	if c, ok := h.(clipboarder); ok {
		c.SetClipboardWriter(func(text string) { s.SetClipboard([]byte(text)) })
	}
	return l
}

// Run processes events until the handler is done or the screen closes.
func (l *Loop) Run() {
	l.render()
	for !l.H.Done() {
		ev := l.S.PollEvent()
		if ev == nil {
			return
		}
		l.Dispatch(ev)
	}
	if l.timer != nil {
		l.timer.Stop()
	}
}

// Dispatch handles one tcell event and re-renders.
func (l *Loop) Dispatch(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		w, h := ev.Size()
		l.canvas.Resize(w, h)
		l.prev = nil // repaint everything
		l.H.Resize(w, h)
		l.S.Sync()
	case *tcell.EventPaste:
		if ev.Start() {
			l.pasting = true
			l.paste.Reset()
			return
		}
		l.pasting = false
		l.H.Handle(term.PasteEvent{Text: l.paste.String()})
	case *tcell.EventKey:
		if l.pasting {
			l.collectPaste(ev)
			return
		}
		l.H.Handle(term.KeyEvent{Key: l.Profile.Normalize(KeyFromTcell(ev))})
	case *tcell.EventMouse:
		for _, g := range l.Det.Feed(MouseFromTcell(ev)) {
			l.H.Handle(term.MouseEvent{Gesture: g})
		}
	case *tcell.EventInterrupt:
		switch ev.Data().(type) {
		case tickData:
			l.H.Handle(term.TickEvent{Now: l.Now()})
		case flushData:
			if p, ok := l.H.(poster); !ok || !p.Flush() {
				return
			}
		default:
			return
		}
	default:
		return
	}
	if !l.H.Done() {
		l.render()
	}
}

func (l *Loop) collectPaste(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyRune:
		l.paste.WriteRune(ev.Rune())
	case tcell.KeyEnter, tcell.KeyLF:
		l.paste.WriteByte('\n')
	case tcell.KeyTab:
		l.paste.WriteByte('\t')
	}
}

// render draws the handler into the canvas and writes only changed cells.
func (l *Loop) render() {
	l.H.Draw(l.canvas)
	c := l.canvas
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			cell := c.At(x, y)
			if l.prev != nil && l.prev.At(x, y) == cell {
				continue
			}
			if cell.Width == 0 {
				continue // right half of a wide glyph: tcell skips it
			}
			l.S.SetContent(x, y, cell.Rune, nil, l.Styles[cell.Style])
			l.CellsWritten++
		}
	}
	if l.prev == nil || l.prev.W != c.W || l.prev.H != c.H {
		l.prev = wm.NewCanvas(c.W, c.H)
	}
	l.prev.CopyFrom(c)
	l.Frames++
	l.S.Show()
	l.schedule()
}

// schedule arms a single timer for the handler's next wake-up.
func (l *Loop) schedule() {
	at := l.H.NextWake()
	if at.IsZero() || at.Equal(l.wakeAt) {
		return
	}
	l.wakeAt = at
	if l.timer != nil {
		l.timer.Stop()
	}
	l.timer = time.AfterFunc(max(time.Until(at), 0), func() {
		_ = l.S.PostEvent(tcell.NewEventInterrupt(tickData{}))
	})
}

// KeyFromTcell converts a tcell key event (not yet normalized).
func KeyFromTcell(ev *tcell.EventKey) term.Key {
	mod := ModFromTcell(ev.Modifiers())
	k := ev.Key()
	switch {
	case k == tcell.KeyRune:
		return term.Key{Name: "Rune", Rune: ev.Rune(), Mod: mod}
	case k == tcell.KeyCtrlSpace:
		return term.Key{Name: "Rune", Rune: ' ', Mod: mod | term.ModCtrl}
	case k >= tcell.KeyCtrlA && k <= tcell.KeyCtrlZ:
		return term.Key{Name: "Rune", Rune: rune('a' + (k - tcell.KeyCtrlA)), Mod: mod | term.ModCtrl}
	case k >= tcell.KeyF1 && k <= tcell.KeyF64:
		return term.Key{Name: fmt.Sprintf("F%d", int(k-tcell.KeyF1)+1), Mod: mod}
	case k == tcell.KeyCtrlUnderscore: // terminals send Ctrl+/ as ^_ (0x1F)
		return term.Key{Name: "Rune", Rune: '/', Mod: mod | term.ModCtrl}
	case k == tcell.KeyBacktab:
		return term.Key{Name: "Tab", Mod: mod | term.ModShift}
	}
	if name, ok := keyNames[k]; ok {
		return term.Key{Name: name, Mod: mod}
	}
	return term.Key{Name: ev.Name(), Mod: mod}
}

var keyNames = map[tcell.Key]string{
	tcell.KeyUp: "Up", tcell.KeyDown: "Down", tcell.KeyLeft: "Left", tcell.KeyRight: "Right",
	tcell.KeyHome: "Home", tcell.KeyEnd: "End", tcell.KeyPgUp: "PgUp", tcell.KeyPgDn: "PgDn",
	tcell.KeyInsert: "Insert", tcell.KeyDelete: "Delete", tcell.KeyEnter: "Enter",
	tcell.KeyTab: "Tab", tcell.KeyEsc: "Esc", tcell.KeyBackspace: "Backspace",
	tcell.KeyBackspace2: "Backspace",
}

// ModFromTcell converts tcell modifiers.
func ModFromTcell(m tcell.ModMask) term.Mod {
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

// MouseFromTcell converts a raw mouse report.
func MouseFromTcell(ev *tcell.EventMouse) term.MouseInput {
	x, y := ev.Position()
	in := term.MouseInput{X: x, Y: y, Mod: ModFromTcell(ev.Modifiers()), Time: ev.When()}
	b := ev.Buttons()
	switch {
	case b&tcell.WheelUp != 0:
		in.Wheel = term.WheelUp
	case b&tcell.WheelDown != 0:
		in.Wheel = term.WheelDown
	case b&tcell.WheelLeft != 0:
		in.Wheel = term.WheelLeft
	case b&tcell.WheelRight != 0:
		in.Wheel = term.WheelRight
	case b&tcell.Button1 != 0:
		in.Button = term.ButtonLeft
	case b&tcell.Button2 != 0:
		in.Button = term.ButtonRight
	case b&tcell.Button3 != 0:
		in.Button = term.ButtonMiddle
	}
	return in
}

// StylesFor maps style IDs to colors. Below 16 colors (Linux console) only
// the 8 basic colors are used.
func StylesFor(colors int) [wm.NumStyles]tcell.Style {
	b := tcell.StyleDefault
	var st [wm.NumStyles]tcell.Style
	st[wm.StyleDesktop] = b.Background(tcell.ColorNavy).Foreground(tcell.ColorSilver)
	st[wm.StyleBar] = b.Background(tcell.ColorSilver).Foreground(tcell.ColorBlack)
	st[wm.StyleBarActive] = b.Background(tcell.ColorTeal).Foreground(tcell.ColorWhite).Bold(true)
	st[wm.StyleFrame] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorGray)
	st[wm.StyleFrameFocused] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorAqua).Bold(true)
	st[wm.StyleTitle] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorSilver)
	st[wm.StyleTitleFocused] = b.Background(tcell.ColorTeal).Foreground(tcell.ColorWhite).Bold(true)
	st[wm.StyleButton] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorYellow)
	st[wm.StyleBody] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorSilver)
	st[wm.StyleBodyDim] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorGray)
	st[wm.StyleMenu] = b.Background(tcell.ColorSilver).Foreground(tcell.ColorBlack)
	st[wm.StyleMenuSelected] = b.Background(tcell.ColorTeal).Foreground(tcell.ColorWhite).Bold(true)
	st[wm.StyleMenuHotkey] = b.Background(tcell.ColorSilver).Foreground(tcell.ColorMaroon).Underline(true)
	st[wm.StyleMenuSelectedHotkey] = b.Background(tcell.ColorTeal).Foreground(tcell.ColorYellow).Bold(true)
	st[wm.StyleMenuDisabled] = b.Background(tcell.ColorSilver).Foreground(tcell.ColorGray)
	st[wm.StyleDialog] = b.Background(tcell.ColorSilver).Foreground(tcell.ColorBlack)
	st[wm.StyleDialogButton] = b.Background(tcell.ColorSilver).Foreground(tcell.ColorBlack)
	st[wm.StyleDialogButtonFocused] = b.Background(tcell.ColorTeal).Foreground(tcell.ColorWhite).Bold(true)
	st[wm.StyleToast] = b.Background(tcell.ColorGreen).Foreground(tcell.ColorBlack)
	st[wm.StyleToastError] = b.Background(tcell.ColorMaroon).Foreground(tcell.ColorWhite).Bold(true)
	st[wm.StyleScrollbar] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorGray)
	st[wm.StyleScrollThumb] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorSilver)
	st[wm.StyleSelected] = b.Background(tcell.ColorTeal).Foreground(tcell.ColorWhite)
	st[wm.StyleInput] = b.Background(tcell.ColorNavy).Foreground(tcell.ColorWhite)
	st[wm.StyleInputCursor] = b.Background(tcell.ColorWhite).Foreground(tcell.ColorNavy)
	st[wm.StyleDialogError] = b.Background(tcell.ColorSilver).Foreground(tcell.ColorMaroon).Bold(true)
	st[wm.StyleHeader] = b.Background(tcell.ColorNavy).Foreground(tcell.ColorWhite)
	st[wm.StyleMarked] = b.Background(tcell.ColorOlive).Foreground(tcell.ColorBlack)
	st[wm.StyleDropTarget] = b.Background(tcell.ColorGreen).Foreground(tcell.ColorBlack).Bold(true)
	st[wm.StyleGutter] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorGray)
	st[wm.StyleCurLine] = b.Background(tcell.NewRGBColor(30, 30, 40)).Foreground(tcell.ColorSilver)
	st[wm.StyleSelection] = b.Background(tcell.ColorNavy).Foreground(tcell.ColorWhite)
	st[wm.StyleBracket] = b.Background(tcell.ColorOlive).Foreground(tcell.ColorBlack)
	st[wm.StyleSynComment] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorGray)
	st[wm.StyleSynString] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorGreen)
	st[wm.StyleSynKeyword] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorAqua)
	st[wm.StyleSynNumber] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorFuchsia)
	st[wm.StyleSynVar] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorYellow)
	st[wm.StyleSynKey] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorAqua)
	st[wm.StyleSynHeader] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorYellow).Bold(true)
	if colors < 256 {
		st[wm.StyleCurLine] = st[wm.StyleBody] // no subtle grey on 8/16 colors
	}
	if colors < 16 {
		// Bright colors render as bold/blink on 8-color consoles; keep to the base 8.
		st[wm.StyleFrame] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)
		st[wm.StyleBodyDim] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)
		st[wm.StyleMenuDisabled] = b.Background(tcell.ColorSilver).Foreground(tcell.ColorNavy)
		st[wm.StyleScrollbar] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)
		st[wm.StyleSynComment] = b.Background(tcell.ColorBlack).Foreground(tcell.ColorOlive) // grey is invisible here
	}
	return st
}
