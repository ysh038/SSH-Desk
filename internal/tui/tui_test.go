package tui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"tuidesk/internal/desktop"
	"tuidesk/internal/term"
)

func newSim(t *testing.T) (*Loop, *desktop.Desktop, tcell.SimulationScreen) {
	t.Helper()
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	s.SetSize(100, 30)
	t.Cleanup(s.Fini)
	d := desktop.New(100, 30, "host", time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC))
	d.OpenWindow(d.NewApp(1))
	d.OpenWindow(d.NewApp(2))
	l := NewLoop(s, d)
	l.Now = func() time.Time { return d.Now.Add(time.Minute) }
	l.render()
	return l, d, s
}

func TestOnlyChangedCellsAreWritten(t *testing.T) {
	l, _, _ := newSim(t)
	first := l.CellsWritten
	if first < 100*30/2 {
		t.Fatalf("first frame wrote only %d cells", first)
	}
	// A click on the empty desktop changes nothing on screen.
	l.Dispatch(tcell.NewEventMouse(90, 25, tcell.Button1, tcell.ModNone))
	l.Dispatch(tcell.NewEventMouse(90, 25, tcell.ButtonNone, tcell.ModNone))
	if l.CellsWritten != first {
		t.Fatalf("idle frame wrote %d cells", l.CellsWritten-first)
	}
	// Moving a window writes far fewer cells than a full repaint.
	l.Dispatch(tcell.NewEventKey(tcell.KeyF7, 0, tcell.ModNone))
	l.Dispatch(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if n := l.CellsWritten - first; n == 0 || n > 100*30/2 {
		t.Fatalf("move wrote %d cells", n)
	}
}

func TestKoreanIMECtrlKeysAreNormalized(t *testing.T) {
	l, d, _ := newSim(t)
	l.Dispatch(tcell.NewEventKey(tcell.KeyRune, 'ㅈ', tcell.ModCtrl)) // Ctrl+W with the Korean IME on
	if len(d.M.Windows()) != 1 {
		t.Fatalf("Ctrl+ㅈ should close a window, have %d", len(d.M.Windows()))
	}
}

func TestCtrlLettersAndBacktab(t *testing.T) {
	cases := []struct {
		ev   *tcell.EventKey
		want string
	}{
		{tcell.NewEventKey(tcell.KeyCtrlW, 'w', tcell.ModCtrl), "Ctrl+W"},
		{tcell.NewEventKey(tcell.KeyCtrlSpace, 0, tcell.ModCtrl), "Ctrl+Space"},
		{tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModShift), "Shift+Tab"},
		{tcell.NewEventKey(tcell.KeyF20, 0, tcell.ModNone), "F20"},
		{tcell.NewEventKey(tcell.KeyBackspace2, 0, tcell.ModNone), "Backspace"},
		{tcell.NewEventKey(tcell.KeyRune, '한', tcell.ModNone), "한"},
		{tcell.NewEventKey(tcell.KeyCtrlUnderscore, 0, tcell.ModCtrl), "Ctrl+/"},
		{tcell.NewEventKey(tcell.KeyCtrlRightSq, 0, tcell.ModCtrl), "Ctrl+]"},
	}
	for _, c := range cases {
		if got := KeyFromTcell(c.ev).String(); got != c.want {
			t.Errorf("%s → %q, want %q", c.ev.Name(), got, c.want)
		}
	}
	lin := term.ProfileFor("linux")
	if got := lin.Normalize(KeyFromTcell(tcell.NewEventKey(tcell.KeyF20, 0, tcell.ModNone))).String(); got != "Shift+F10" {
		t.Fatalf("console F20 → %s", got)
	}
}

func TestPasteIsCollected(t *testing.T) {
	l, d, _ := newSim(t)
	l.Dispatch(tcell.NewEventPaste(true))
	for _, r := range "가나" {
		l.Dispatch(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
	l.Dispatch(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	l.Dispatch(tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone))
	windows := len(d.M.Windows())
	l.Dispatch(tcell.NewEventPaste(false))
	// Enter inside a paste must not reach the desktop as a key.
	if len(d.M.Windows()) != windows || d.MenuOpen() {
		t.Fatal("paste content leaked as keys")
	}
}

func TestTickUpdatesClock(t *testing.T) {
	l, d, s := newSim(t)
	l.Dispatch(tcell.NewEventInterrupt(tickData{}))
	if d.Now.Format("15:04") != "09:01" {
		t.Fatalf("now = %v", d.Now)
	}
	cells, w, _ := s.GetContents()
	row := ""
	for x := 0; x < w; x++ {
		row += string(cells[x].Runes)
	}
	if want := "09:01"; !contains(row, want) {
		t.Fatalf("top bar = %q", row)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Regression: work posted before the loop installs its wake-up hook (a folder
// loaded at startup) must still be delivered without further input.
func TestWorkPostedBeforeLoopIsDelivered(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	s.SetSize(80, 24)
	defer s.Fini()
	d := desktop.New(80, 24, "host", time.Now())
	ran := false
	d.Post(func() { ran = true })
	l := NewLoop(s, d)
	deadline := time.Now().Add(2 * time.Second)
	for !ran && time.Now().Before(deadline) {
		ev := s.PollEvent()
		l.Dispatch(ev)
	}
	if !ran {
		t.Fatal("queued work was never flushed")
	}
}
