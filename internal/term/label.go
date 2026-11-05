package term

import (
	"strings"
	"sync/atomic"
)

// LabelStyle selects how shortcuts are shown to the user (ADR 0003 A).
type LabelStyle int32

const (
	LabelPC  LabelStyle = iota // Ctrl+S, Alt+↓, PgDn, F2
	LabelMac                   // ⌃S, ⌥↓, fn↓, fn F2
)

var labelStyle atomic.Int32

// SetLabelStyle sets the display style used by KeyLabel.
func SetLabelStyle(s LabelStyle) { labelStyle.Store(int32(s)) }

// CurrentLabelStyle returns the display style.
func CurrentLabelStyle() LabelStyle { return LabelStyle(labelStyle.Load()) }

var arrowGlyph = map[string]string{"Up": "↑", "Down": "↓", "Left": "←", "Right": "→"}

// KeyLabel formats a binding such as "Ctrl+S" or "Alt+Down" for display in
// the current style. Several bindings separated by " / " are formatted each.
func KeyLabel(binding string) string { return FormatLabel(binding, CurrentLabelStyle()) }

// FormatLabel formats binding in style s.
func FormatLabel(binding string, s LabelStyle) string {
	if binding == "" {
		return ""
	}
	if strings.Contains(binding, " / ") {
		parts := strings.Split(binding, " / ")
		for i, p := range parts {
			parts[i] = FormatLabel(p, s)
		}
		return strings.Join(parts, " / ")
	}
	tokens := strings.Split(binding, "+")
	if strings.HasSuffix(binding, "++") { // "Ctrl++"
		tokens = append(strings.Split(strings.TrimSuffix(binding, "++"), "+"), "+")
	}
	key := tokens[len(tokens)-1]
	mods := tokens[:len(tokens)-1]
	if s == LabelPC {
		if g, ok := arrowGlyph[key]; ok {
			key = g
		}
		return strings.Join(append(mods, key), "+")
	}
	var b strings.Builder
	for _, m := range mods {
		switch m {
		case "Ctrl":
			b.WriteString("⌃")
		case "Alt":
			b.WriteString("⌥")
		case "Shift":
			b.WriteString("⇧")
		case "Meta":
			b.WriteString("⌘")
		}
	}
	switch {
	case arrowGlyph[key] != "":
		b.WriteString(arrowGlyph[key])
	case key == "Home":
		b.WriteString("fn←")
	case key == "End":
		b.WriteString("fn→")
	case key == "PgUp":
		b.WriteString("fn↑")
	case key == "PgDn":
		b.WriteString("fn↓")
	case key == "Delete":
		b.WriteString("fn⌫")
	case key == "Backspace":
		b.WriteString("⌫")
	case key == "Enter":
		b.WriteString("↩")
	case key == "Tab":
		b.WriteString("⇥")
	case key == "Esc":
		b.WriteString("esc")
	case len(key) >= 2 && key[0] == 'F' && key[1] >= '0' && key[1] <= '9':
		return "fn " + b.String() + key
	default:
		b.WriteString(key)
	}
	return b.String()
}
