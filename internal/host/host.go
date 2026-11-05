// Package host defines the services the desktop offers to apps (SPEC 6
// "event bus"): posting work back to the UI goroutine, modals, menus,
// clipboards, undo, long-running operations and drag and drop.
package host

import (
	"context"

	"tuidesk/internal/config"
	"tuidesk/internal/fsops"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

// Clipboard holds files cut or copied in a file manager.
type Clipboard struct {
	Paths []string
	Cut   bool
}

// Drag is a drag-and-drop payload started by an app.
type Drag struct {
	Paths  []string
	SrcDir string
}

// DropTarget is implemented by apps that accept dropped files.
type DropTarget interface {
	// DropDir returns the folder that a drop at local (x, y) goes into.
	DropDir(x, y int) (dir string, ok bool)
	// DropHover highlights the target under local (x, y); on=false clears it.
	DropHover(x, y int, on bool)
}

// Refresher is implemented by apps that show files and must reload after
// file operations.
type Refresher interface{ Refresh() }

// Work is a long operation run off the UI goroutine. It must stop when ctx
// is cancelled and may report progress.
type Work func(ctx context.Context, progress func(fsops.Progress)) error

// Host is what apps can ask the desktop to do. All methods must be called on
// the UI goroutine except Post.
type Host interface {
	// Post runs fn on the UI goroutine (safe to call from any goroutine).
	Post(fn func())

	ShowModal(m ui.Modal)
	Notify(text string, isErr bool)
	ShowError(title, msg, details string)
	// Confirm shows a dialog; onChoose gets the button index (the last
	// button is chosen by Esc).
	Confirm(title, msg string, buttons []string, onChoose func(int))
	// Prompt asks for a line of text.
	Prompt(p *ui.Prompt)
	// OpenMenu opens a popup menu for app at its local (x, y).
	OpenMenu(app wm.App, x, y int, items []ui.MenuItem, onChoose func(action string))
	// OpenBarMenu is OpenMenu for a menu bar: Left/Right call side(-1/+1).
	OpenBarMenu(app wm.App, x, y int, items []ui.MenuItem, onChoose func(action string), side func(dir int))
	// ShowPalette opens the command search for cmds.
	ShowPalette(cmds *ui.Commands)

	OpenWindow(app wm.App) *wm.Window
	// SetClipboardText copies text to the internal clipboard and, through
	// OSC 52, to the user's local clipboard.
	SetClipboardText(s string)
	Files() *Clipboard
	BeginDrag(d Drag)
	// Console reports a Linux console (TERM=linux): no Hangul glyphs.
	Console() bool

	FS() *fsops.FS
	PushUndo(rec *fsops.Record)
	PopUndo() *fsops.Record
	// FilesChanged makes every Refresher reload.
	FilesChanged()
	// Go runs work in the background with a progress popup (shown only if
	// it takes a while) and calls done on the UI goroutine.
	Go(title string, work Work, done func(error))

	// Config returns the current settings.
	Config() config.Config
}
