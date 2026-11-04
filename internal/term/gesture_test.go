package term

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func at(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }

func kinds(gs []Gesture) []GestureKind {
	out := make([]GestureKind, len(gs))
	for i, g := range gs {
		out[i] = g.Kind
	}
	return out
}

func feedAll(d *Detector, ins ...MouseInput) []Gesture {
	var out []Gesture
	for _, in := range ins {
		out = append(out, d.Feed(in)...)
	}
	return out
}

func clicks(gs []Gesture) []int {
	var out []int
	for _, g := range gs {
		if g.Kind == GestureClick {
			out = append(out, g.Count)
		}
	}
	return out
}

func eqInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func click(x, y, ms int, b Button) []MouseInput {
	return []MouseInput{
		{X: x, Y: y, Button: b, Time: at(ms)},
		{X: x, Y: y, Button: ButtonNone, Time: at(ms + 30)},
	}
}

func TestSingleClick(t *testing.T) {
	d := NewDetector()
	gs := feedAll(d, click(5, 3, 0, ButtonLeft)...)
	want := []GestureKind{GesturePress, GestureRelease, GestureClick}
	if got := kinds(gs); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	c := gs[2]
	if c.Count != 1 || c.Button != ButtonLeft || c.X != 5 || c.Y != 3 {
		t.Fatalf("click = %+v", c)
	}
}

func TestDoubleAndTripleClick(t *testing.T) {
	d := NewDetector()
	var ins []MouseInput
	ins = append(ins, click(5, 3, 0, ButtonLeft)...)
	ins = append(ins, click(5, 3, 200, ButtonLeft)...)
	ins = append(ins, click(5, 3, 400, ButtonLeft)...)
	ins = append(ins, click(5, 3, 600, ButtonLeft)...) // wraps back to single
	if got := clicks(feedAll(d, ins...)); !eqInts(got, []int{1, 2, 3, 1}) {
		t.Fatalf("counts = %v", got)
	}
}

func TestSlowClicksAreSingles(t *testing.T) {
	d := NewDetector()
	var ins []MouseInput
	ins = append(ins, click(5, 3, 0, ButtonLeft)...)
	ins = append(ins, click(5, 3, 1000, ButtonLeft)...)
	if got := clicks(feedAll(d, ins...)); !eqInts(got, []int{1, 1}) {
		t.Fatalf("counts = %v", got)
	}
}

func TestClicksAtDifferentCellsAreSingles(t *testing.T) {
	d := NewDetector()
	var ins []MouseInput
	ins = append(ins, click(5, 3, 0, ButtonLeft)...)
	ins = append(ins, click(6, 3, 100, ButtonLeft)...)
	if got := clicks(feedAll(d, ins...)); !eqInts(got, []int{1, 1}) {
		t.Fatalf("counts = %v", got)
	}
}

func TestDifferentButtonsDoNotCombine(t *testing.T) {
	d := NewDetector()
	var ins []MouseInput
	ins = append(ins, click(5, 3, 0, ButtonLeft)...)
	ins = append(ins, click(5, 3, 100, ButtonRight)...)
	gs := feedAll(d, ins...)
	if got := clicks(gs); !eqInts(got, []int{1, 1}) {
		t.Fatalf("counts = %v", got)
	}
	if gs[len(gs)-1].Button != ButtonRight {
		t.Fatalf("last click button = %v", gs[len(gs)-1].Button)
	}
}

func TestDrag(t *testing.T) {
	d := NewDetector()
	gs := feedAll(d,
		MouseInput{X: 2, Y: 2, Button: ButtonLeft, Time: at(0)},
		MouseInput{X: 3, Y: 2, Button: ButtonLeft, Time: at(10)},
		MouseInput{X: 3, Y: 2, Button: ButtonLeft, Time: at(20)}, // no movement: no event
		MouseInput{X: 6, Y: 4, Button: ButtonLeft, Time: at(30)},
		MouseInput{X: 6, Y: 4, Button: ButtonNone, Time: at(40)},
	)
	want := []GestureKind{GesturePress, GestureDragStart, GestureDrag, GestureDrag, GestureRelease, GestureDragEnd}
	got := kinds(gs)
	if len(got) != len(want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", got, want)
		}
	}
	end := gs[len(gs)-1]
	if end.StartX != 2 || end.StartY != 2 || end.X != 6 || end.Y != 4 {
		t.Fatalf("drag end = %+v", end)
	}
	if len(clicks(gs)) != 0 {
		t.Fatal("drag must not produce a click")
	}
}

func TestDragThreshold(t *testing.T) {
	d := NewDetector()
	d.DragThreshold = 3
	gs := feedAll(d,
		MouseInput{X: 2, Y: 2, Button: ButtonLeft, Time: at(0)},
		MouseInput{X: 4, Y: 2, Button: ButtonLeft, Time: at(10)},
		MouseInput{X: 4, Y: 2, Button: ButtonNone, Time: at(20)},
	)
	if got := clicks(gs); !eqInts(got, []int{1}) {
		t.Fatalf("small movement should still click, got kinds %v", kinds(gs))
	}
}

func TestDragResetsMultiClick(t *testing.T) {
	d := NewDetector()
	var ins []MouseInput
	ins = append(ins, click(5, 3, 0, ButtonLeft)...)
	ins = append(ins,
		MouseInput{X: 5, Y: 3, Button: ButtonLeft, Time: at(100)},
		MouseInput{X: 8, Y: 3, Button: ButtonLeft, Time: at(110)},
		MouseInput{X: 8, Y: 3, Button: ButtonNone, Time: at(120)},
	)
	ins = append(ins, click(5, 3, 200, ButtonLeft)...)
	if got := clicks(feedAll(d, ins...)); !eqInts(got, []int{1, 1}) {
		t.Fatalf("counts = %v", got)
	}
}

func TestWheelDoesNotAffectButtonState(t *testing.T) {
	d := NewDetector()
	gs := feedAll(d,
		MouseInput{X: 1, Y: 1, Button: ButtonLeft, Time: at(0)},
		MouseInput{X: 1, Y: 1, Wheel: WheelDown, Time: at(10)},
		MouseInput{X: 1, Y: 1, Button: ButtonNone, Time: at(20)},
	)
	want := []GestureKind{GesturePress, GestureWheel, GestureRelease, GestureClick}
	got := kinds(gs)
	if len(got) != len(want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", got, want)
		}
	}
}

func TestButtonSwitchWhileHeld(t *testing.T) {
	d := NewDetector()
	gs := feedAll(d,
		MouseInput{X: 1, Y: 1, Button: ButtonLeft, Time: at(0)},
		MouseInput{X: 1, Y: 1, Button: ButtonRight, Time: at(10)},
		MouseInput{X: 1, Y: 1, Button: ButtonNone, Time: at(20)},
	)
	want := []GestureKind{GesturePress, GestureRelease, GesturePress, GestureRelease, GestureClick}
	got := kinds(gs)
	if len(got) != len(want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	if gs[len(gs)-1].Button != ButtonRight {
		t.Fatalf("click button = %v", gs[len(gs)-1].Button)
	}
}

func TestMoveOnlyWhenPositionChanges(t *testing.T) {
	d := NewDetector()
	gs := feedAll(d,
		MouseInput{X: 1, Y: 1, Time: at(0)},
		MouseInput{X: 1, Y: 1, Time: at(10)},
		MouseInput{X: 2, Y: 1, Time: at(20)},
	)
	if got := kinds(gs); len(got) != 2 || got[0] != GestureMove || got[1] != GestureMove {
		t.Fatalf("kinds = %v", got)
	}
}

func TestModString(t *testing.T) {
	cases := map[Mod]string{0: "-", ModShift: "Shift", ModCtrl | ModShift: "Ctrl+Shift", ModAlt | ModMeta: "Alt+Meta"}
	for m, want := range cases {
		if got := m.String(); got != want {
			t.Errorf("%d: got %q want %q", m, got, want)
		}
	}
}
