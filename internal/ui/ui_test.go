package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

func key(name string) term.KeyEvent { return term.KeyEvent{Key: term.Key{Name: name}} }
func runeKey(r rune) term.Key       { return term.Key{Name: "Rune", Rune: r} }

func click(x, y, count int) term.Gesture {
	return term.Gesture{Kind: term.GestureClick, Button: term.ButtonLeft, X: x, Y: y, StartX: x, StartY: y, Count: count}
}

func sampleMenu() *Menu {
	m := NewMenu([]MenuItem{
		{Label: "새 창", Hotkey: 'n', Shortcut: "Ctrl+N", Action: "new"},
		{Separator: true},
		{Label: "비활성", Action: "off", Disabled: true},
		{Label: "종료", Hotkey: 'q', Action: "quit"},
	})
	w, h := m.Size()
	m.Rect = PlacePopup(wm.Rect{X: 0, Y: 0, W: 80, H: 24}, 5, 3, w, h)
	return m
}

func TestMenuKeyboardSkipsSeparatorsAndDisabled(t *testing.T) {
	m := sampleMenu()
	if m.Selected != 0 {
		t.Fatalf("initial selection %d", m.Selected)
	}
	m.HandleKey(key("Down").Key)
	if m.Selected != 3 {
		t.Fatalf("down → %d, want 3", m.Selected)
	}
	m.HandleKey(key("Down").Key) // wraps
	if m.Selected != 0 {
		t.Fatalf("wrap → %d", m.Selected)
	}
	m.HandleKey(key("Up").Key)
	if m.Selected != 3 {
		t.Fatalf("up → %d", m.Selected)
	}
	if r := m.HandleKey(key("Enter").Key); r != MenuActivated || m.Chosen.Action != "quit" {
		t.Fatalf("enter → %v %q", r, m.Chosen.Action)
	}
	if r := m.HandleKey(key("Esc").Key); r != MenuClosed {
		t.Fatalf("esc → %v", r)
	}
}

func TestMenuHotkeyWorksWithKoreanIME(t *testing.T) {
	m := sampleMenu()
	if r := m.HandleKey(runeKey('ㅂ')); r != MenuActivated || m.Chosen.Action != "quit" { // ㅂ is the Q key
		t.Fatalf("ㅂ → %v %q", r, m.Chosen.Action)
	}
	m = sampleMenu()
	if r := m.HandleKey(runeKey('N')); r != MenuActivated || m.Chosen.Action != "new" {
		t.Fatalf("N → %v %q", r, m.Chosen.Action)
	}
}

func TestMenuMouse(t *testing.T) {
	m := sampleMenu()
	// Rows start at Rect.Y+1; item 3 ("종료") is at Rect.Y+4.
	if r := m.HandleMouse(click(m.Rect.X+2, m.Rect.Y+4, 1)); r != MenuActivated || m.Chosen.Action != "quit" {
		t.Fatalf("click → %v %q", r, m.Chosen.Action)
	}
	m = sampleMenu()
	if r := m.HandleMouse(click(m.Rect.X+2, m.Rect.Y+3, 1)); r != MenuNone {
		t.Fatalf("click on disabled → %v", r)
	}
	press := term.Gesture{Kind: term.GesturePress, Button: term.ButtonLeft, X: 70, Y: 20}
	if r := m.HandleMouse(press); r != MenuClosed {
		t.Fatalf("press outside → %v", r)
	}
}

func TestMenuDraw(t *testing.T) {
	m := sampleMenu()
	c := wm.NewCanvas(80, 24)
	m.Draw(c)
	row := c.Row(m.Rect.Y + 1)
	if !strings.Contains(row, "새 창(N)") || !strings.Contains(row, "Ctrl+N") {
		t.Fatalf("row = %q", row)
	}
	if !strings.Contains(c.Row(m.Rect.Y+2), "├") {
		t.Fatalf("separator row = %q", c.Row(m.Rect.Y+2))
	}
}

func TestPlacePopupStaysOnScreen(t *testing.T) {
	scr := wm.Rect{X: 0, Y: 1, W: 80, H: 22}
	r := PlacePopup(scr, 75, 20, 20, 8)
	if r.X+r.W > 80 || r.Y+r.H > 23 || r.Y < 1 {
		t.Fatalf("r = %+v", r)
	}
	if r := PlacePopup(scr, 3, 2, 20, 8); r.X != 3 || r.Y != 2 {
		t.Fatalf("r = %+v", r)
	}
}

func TestDialog(t *testing.T) {
	d := NewDialog("quit", "종료", "tuidesk를 종료할까요?", "종료", "취소")
	d.Center(wm.Rect{X: 0, Y: 0, W: 80, H: 24})
	if d.HandleEvent(key("Right")) != DialogNone || d.Focused != 1 {
		t.Fatal("right should focus 취소")
	}
	if d.HandleEvent(key("Esc")) != DialogDone || d.Chosen != 1 {
		t.Fatalf("esc → %d", d.Chosen)
	}
	d.Focused = 0
	if d.HandleEvent(key("Enter")) != DialogDone || d.Chosen != 0 {
		t.Fatal("enter should choose 종료")
	}
	br := d.buttonRects()[1]
	if d.HandleEvent(term.MouseEvent{Gesture: click(br.X+1, br.Y, 1)}) != DialogDone || d.Chosen != 1 {
		t.Fatal("click 취소")
	}
	c := wm.NewCanvas(80, 24)
	d.Draw(c)
	if !strings.Contains(c.Row(d.Rect.Y+2), "tuidesk를 종료할까요?") {
		t.Fatalf("message row = %q", c.Row(d.Rect.Y+2))
	}
}

func TestToastsExpire(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var ts Toasts
	ts.Push(t0, "저장했습니다", false)
	ts.Push(t0.Add(time.Second), "nginx -t 실패", true)
	if got := ts.NextDeadline(); !got.Equal(t0.Add(4 * time.Second)) {
		t.Fatalf("deadline %v", got)
	}
	if ts.Tick(t0.Add(3 * time.Second)) {
		t.Fatal("nothing should expire yet")
	}
	if !ts.Tick(t0.Add(4*time.Second)) || len(ts.Items) != 1 {
		t.Fatal("first toast should expire")
	}
	c := wm.NewCanvas(40, 5)
	ts.Draw(c, wm.Rect{X: 0, Y: 0, W: 40, H: 5})
	if !strings.HasSuffix(c.Row(4), " nginx -t 실패 ") {
		t.Fatalf("row = %q", c.Row(4))
	}
}

type bigSource int

func (b bigSource) Len() int          { return int(b) }
func (b bigSource) Item(i int) string { return fmt.Sprintf("파일-%06d.log", i) }

func TestListVirtualScrolling(t *testing.T) {
	l := &List{Src: bigSource(100000), Height: 10}
	l.HandleEvent(key("End"))
	if l.Selected != 99999 || l.Offset != 99990 {
		t.Fatalf("end → sel %d off %d", l.Selected, l.Offset)
	}
	l.HandleEvent(key("PgUp"))
	if l.Selected != 99990 {
		t.Fatalf("pgup → %d", l.Selected)
	}
	l.HandleEvent(key("Home"))
	if l.Selected != 0 || l.Offset != 0 {
		t.Fatal("home")
	}
	l.HandleEvent(term.MouseEvent{Gesture: term.Gesture{Kind: term.GestureWheel, Wheel: term.WheelDown}})
	if l.Offset != 3 || l.Selected != 0 {
		t.Fatalf("wheel → off %d sel %d", l.Offset, l.Selected)
	}
	if r := l.HandleEvent(term.MouseEvent{Gesture: click(2, 4, 2)}); r != ListActivated || l.Selected != 7 {
		t.Fatalf("double click → %v sel %d", r, l.Selected)
	}

	c := wm.NewCanvas(30, 10)
	l.Draw(wm.NewSurface(c, wm.Rect{X: 0, Y: 0, W: 30, H: 10}), true)
	if !strings.HasPrefix(c.Row(0), "파일-000003.log") {
		t.Fatalf("row0 = %q", c.Row(0))
	}
	if r := []rune(c.Row(0)); r[len(r)-1] != '█' {
		t.Fatalf("scrollbar thumb missing: %q", c.Row(0))
	}
}

func TestThumbRange(t *testing.T) {
	if s, l := ThumbRange(100, 0, 10, 10); s != 0 || l != 1 {
		t.Fatalf("top %d %d", s, l)
	}
	if s, l := ThumbRange(100, 90, 10, 10); s != 9 || l != 1 {
		t.Fatalf("bottom %d %d", s, l)
	}
	if s, l := ThumbRange(5, 0, 10, 10); s != 0 || l != 10 {
		t.Fatalf("fits %d %d", s, l)
	}
}

func TestMenuHotkeyHighlightedInPlace(t *testing.T) {
	m := NewMenu([]MenuItem{{Label: "Maximize", Hotkey: 'x', Action: "max"}, {Label: "새 창", Hotkey: 'n', Action: "new"}})
	w, h := m.Size()
	m.Rect = wm.Rect{X: 0, Y: 0, W: w, H: h}
	c := wm.NewCanvas(40, 5)
	m.Draw(c)
	if row := c.Row(1); !strings.Contains(row, "Maximize") || strings.Contains(row, "(X)") {
		t.Fatalf("row1 = %q", row)
	}
	// 'x' is the 3rd letter: label starts at column 2 (frame + padding).
	if got := c.At(4, 1); got.Rune != 'x' || got.Style != wm.StyleMenuSelectedHotkey {
		t.Fatalf("hotkey cell = %+v", got)
	}
	if row := c.Row(2); !strings.Contains(row, "새 창(N)") {
		t.Fatalf("label without the letter keeps the suffix: %q", row)
	}
}

func TestListJumpAliases(t *testing.T) {
	l := &List{Src: bigSource(1000), Height: 20}
	l.HandleEvent(term.KeyEvent{Key: term.Key{Name: "Down", Mod: term.ModAlt}}) // Option+↓
	if l.Selected != 999 {
		t.Fatalf("Option+Down → %d", l.Selected)
	}
	l.HandleEvent(term.KeyEvent{Key: term.Key{Name: "Home", Mod: term.ModCtrl}})
	if l.Selected != 0 {
		t.Fatalf("Ctrl+Home → %d", l.Selected)
	}
	l.HandleEvent(key("PgDn")) // one screen minus one line of overlap
	if l.Selected != 19 {
		t.Fatalf("PgDn → %d", l.Selected)
	}
}
