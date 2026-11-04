// Package term normalizes raw terminal input into higher-level events.
//
// Everything in this file is independent of tcell so it can be unit tested
// with synthetic input. Terminals only report "button state at position",
// so clicks, multi-clicks and drags have to be derived here.
package term

import (
	"fmt"
	"time"
)

// Button identifies a mouse button.
type Button uint8

const (
	ButtonNone Button = iota
	ButtonLeft
	ButtonRight
	ButtonMiddle
)

func (b Button) String() string {
	switch b {
	case ButtonLeft:
		return "Left"
	case ButtonRight:
		return "Right"
	case ButtonMiddle:
		return "Middle"
	default:
		return "None"
	}
}

// Wheel identifies a wheel impulse.
type Wheel uint8

const (
	WheelNone Wheel = iota
	WheelUp
	WheelDown
	WheelLeft
	WheelRight
)

func (w Wheel) String() string {
	switch w {
	case WheelUp:
		return "Up"
	case WheelDown:
		return "Down"
	case WheelLeft:
		return "Left"
	case WheelRight:
		return "Right"
	default:
		return "None"
	}
}

// Mod is a bit set of keyboard modifiers reported with a mouse event.
type Mod uint8

const (
	ModShift Mod = 1 << iota
	ModCtrl
	ModAlt
	ModMeta
)

func (m Mod) String() string {
	if m == 0 {
		return "-"
	}
	s := ""
	for _, p := range []struct {
		bit  Mod
		name string
	}{{ModCtrl, "Ctrl"}, {ModAlt, "Alt"}, {ModMeta, "Meta"}, {ModShift, "Shift"}} {
		if m&p.bit != 0 {
			if s != "" {
				s += "+"
			}
			s += p.name
		}
	}
	return s
}

// MouseInput is one raw mouse report: the button currently held (if any),
// an optional wheel impulse, modifiers and position.
type MouseInput struct {
	X, Y   int
	Button Button
	Wheel  Wheel
	Mod    Mod
	Time   time.Time
}

// GestureKind is the type of a derived mouse gesture.
type GestureKind uint8

const (
	GesturePress GestureKind = iota
	GestureRelease
	GestureClick
	GestureDragStart
	GestureDrag
	GestureDragEnd
	GestureWheel
	GestureMove
)

func (k GestureKind) String() string {
	return [...]string{"Press", "Release", "Click", "DragStart", "Drag", "DragEnd", "Wheel", "Move"}[k]
}

// Gesture is a higher-level mouse event derived from MouseInput.
type Gesture struct {
	Kind   GestureKind
	Button Button
	Wheel  Wheel
	Mod    Mod
	X, Y   int
	// StartX/StartY is where the button went down (drag gestures, clicks).
	StartX, StartY int
	// Count is 1, 2 or 3 for single/double/triple clicks.
	Count int
}

func (g Gesture) String() string {
	switch g.Kind {
	case GestureClick:
		names := [...]string{"", "single", "double", "triple"}
		return fmt.Sprintf("Click %s %s (%d,%d)", names[g.Count], g.Button, g.X, g.Y)
	case GestureDragStart, GestureDrag, GestureDragEnd:
		return fmt.Sprintf("%s %s (%d,%d)->(%d,%d)", g.Kind, g.Button, g.StartX, g.StartY, g.X, g.Y)
	case GestureWheel:
		return fmt.Sprintf("Wheel %s (%d,%d)", g.Wheel, g.X, g.Y)
	default:
		return fmt.Sprintf("%s %s (%d,%d)", g.Kind, g.Button, g.X, g.Y)
	}
}

// Default tuning values. SPEC 9: multi-click is time based, default 400ms.
const (
	DefaultMultiClickInterval = 400 * time.Millisecond
	DefaultDragThreshold      = 1
)

// Detector turns a stream of MouseInput into Gestures.
// The zero value is not ready; use NewDetector.
type Detector struct {
	MultiClickInterval time.Duration
	// DragThreshold is the Chebyshev distance in cells the pointer must move
	// with a button held before a drag starts.
	DragThreshold int
	// RightDrag enables drags with the right button. It is off by default:
	// the right button only opens context menus, and a hand wobble of a few
	// cells during a right click must not turn it into a drag.
	RightDrag bool

	held             Button
	downX, downY     int
	downTime         time.Time
	dragging         bool
	lastX, lastY     int
	lastClick        time.Time
	lastClickX       int
	lastClickY       int
	lastClickButton  Button
	lastClickCount   int
	havePrevPosition bool
}

// NewDetector returns a Detector with default settings.
func NewDetector() *Detector {
	return &Detector{
		MultiClickInterval: DefaultMultiClickInterval,
		DragThreshold:      DefaultDragThreshold,
	}
}

// Feed processes one raw input and returns the gestures it produced (possibly none).
func (d *Detector) Feed(in MouseInput) []Gesture {
	var out []Gesture
	g := func(k GestureKind) Gesture {
		return Gesture{Kind: k, Button: d.held, Mod: in.Mod, X: in.X, Y: in.Y, StartX: d.downX, StartY: d.downY}
	}

	defer func() {
		d.lastX, d.lastY = in.X, in.Y
		d.havePrevPosition = true
	}()

	if in.Wheel != WheelNone {
		return []Gesture{{Kind: GestureWheel, Wheel: in.Wheel, Mod: in.Mod, X: in.X, Y: in.Y, StartX: in.X, StartY: in.Y}}
	}

	// A different button while one is held: end the old one without a click.
	if d.held != ButtonNone && in.Button != ButtonNone && in.Button != d.held {
		out = append(out, d.release(in, false)...)
	}

	switch {
	case d.held == ButtonNone && in.Button != ButtonNone:
		d.held = in.Button
		d.downX, d.downY = in.X, in.Y
		d.downTime = in.Time
		d.dragging = false
		out = append(out, g(GesturePress))

	case d.held != ButtonNone && in.Button == d.held:
		if d.held == ButtonRight && !d.RightDrag {
			break // motion is ignored; release becomes a click at the press position
		}
		if !d.dragging && chebyshev(in.X-d.downX, in.Y-d.downY) >= d.DragThreshold {
			d.dragging = true
			out = append(out, g(GestureDragStart), g(GestureDrag))
		} else if d.dragging && (in.X != d.lastX || in.Y != d.lastY) {
			out = append(out, g(GestureDrag))
		}

	case d.held != ButtonNone && in.Button == ButtonNone:
		out = append(out, d.release(in, true)...)

	case d.held == ButtonNone && in.Button == ButtonNone:
		if !d.havePrevPosition || in.X != d.lastX || in.Y != d.lastY {
			out = append(out, Gesture{Kind: GestureMove, Mod: in.Mod, X: in.X, Y: in.Y, StartX: in.X, StartY: in.Y})
		}
	}
	return out
}

// release ends the current press. If allowClick and no drag happened, a Click
// gesture with the multi-click count is emitted.
func (d *Detector) release(in MouseInput, allowClick bool) []Gesture {
	base := Gesture{Button: d.held, Mod: in.Mod, X: in.X, Y: in.Y, StartX: d.downX, StartY: d.downY}
	rel := base
	rel.Kind = GestureRelease
	out := []Gesture{rel}

	switch {
	case d.dragging:
		end := base
		end.Kind = GestureDragEnd
		out = append(out, end)
		d.lastClickCount = 0
	case allowClick:
		count := 1
		if d.lastClickCount > 0 && d.lastClickCount < 3 &&
			d.held == d.lastClickButton &&
			d.downX == d.lastClickX && d.downY == d.lastClickY &&
			d.downTime.Sub(d.lastClick) <= d.MultiClickInterval {
			count = d.lastClickCount + 1
		}
		click := base
		click.Kind = GestureClick
		click.X, click.Y = d.downX, d.downY
		click.Count = count
		out = append(out, click)
		d.lastClick = in.Time
		d.lastClickX, d.lastClickY = d.downX, d.downY
		d.lastClickButton = d.held
		d.lastClickCount = count
	}

	d.held = ButtonNone
	d.dragging = false
	return out
}

func chebyshev(dx, dy int) int {
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}
