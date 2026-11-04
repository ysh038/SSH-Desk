package term

import "time"

// Event is a normalized input event delivered to the desktop and apps.
// Concrete types: KeyEvent, MouseEvent, PasteEvent, TickEvent.
type Event interface{ isEvent() }

// KeyEvent is a normalized key press.
type KeyEvent struct{ Key Key }

// MouseEvent is a derived mouse gesture (click, drag, wheel …).
type MouseEvent struct{ Gesture Gesture }

// PasteEvent carries bracketed-paste text.
type PasteEvent struct{ Text string }

// TickEvent is delivered by timers (clock, toast expiry).
type TickEvent struct{ Now time.Time }

func (KeyEvent) isEvent()   {}
func (MouseEvent) isEvent() {}
func (PasteEvent) isEvent() {}
func (TickEvent) isEvent()  {}
