package host

import (
	"tuidesk/internal/term"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

// Chrome gives an app the shared ADR 0003 UX: a menu bar on its first row
// (F10 or click), command search (Ctrl+K) and the hint bar, all driven by
// one command registry.
type Chrome struct {
	H    Host
	App  wm.App
	Cmds *ui.Commands
	Bar  *ui.MenuBar
}

// NewChrome wires a registry to an app.
func NewChrome(h Host, app wm.App, cmds *ui.Commands) *Chrome {
	return &Chrome{H: h, App: app, Cmds: cmds, Bar: ui.NewMenuBar()}
}

// OpenMenu opens menu bar group i (wrapping); Left/Right move between groups.
func (c *Chrome) OpenMenu(i int) {
	menus := c.Cmds.Menus()
	if len(menus) == 0 {
		return
	}
	i = (i%len(menus) + len(menus)) % len(menus)
	c.H.OpenBarMenu(c.App, c.Bar.X(i), 1, c.Cmds.MenuItems(menus[i]),
		func(action string) { c.Cmds.Run(action) },
		func(dir int) { c.OpenMenu(i + dir) })
}

// HandleKey handles F10, Ctrl+K and every registered shortcut.
func (c *Chrome) HandleKey(k term.Key) bool {
	switch k.String() {
	case "F10":
		c.OpenMenu(0)
		return true
	case "Ctrl+K":
		c.H.ShowPalette(c.Cmds)
		return true
	}
	return c.Cmds.HandleKey(k)
}

// ClickBar opens the menu under local column x of the bar row.
func (c *Chrome) ClickBar(x int) bool {
	if i := c.Bar.At(x, c.Cmds.Menus()); i >= 0 {
		c.OpenMenu(i)
		return true
	}
	return false
}

// HintRows is 1 when the hint bar is enabled.
func (c *Chrome) HintRows() int {
	if c.H.Config().HintBar {
		return 1
	}
	return 0
}

// DrawBar paints the menu bar on row 0 of s.
func (c *Chrome) DrawBar(s wm.Surface) { c.Bar.Draw(s, c.Cmds.Menus()) }

// DrawHints paints the hint bar on the last row of s when enabled.
func (c *Chrome) DrawHints(s wm.Surface) {
	if c.HintRows() == 0 {
		return
	}
	w, h := s.Size()
	ui.DrawHints(s.Sub(wm.Rect{X: 0, Y: h - 1, W: w, H: 1}), c.Cmds.All())
}
