package files

import (
	"os"

	"tuidesk/internal/fsops"
	"tuidesk/internal/ui"
)

// buildCommands registers every file manager action once; keys, the menu
// bar, the command search and the hint bar all come from here (ADR 0003).
func (a *App) buildCommands() {
	hasSel := func() bool { return len(a.Selection()) > 0 }
	one := func() bool { return len(a.Selection()) == 1 }
	hasClip := func() bool { return len(a.h.Files().Paths) > 0 }
	openCursor := func() {
		if e, ok := a.cursorEntry(); ok {
			a.open(e)
		}
	}
	sortBy := func(k fsops.SortKey) func() {
		return func() {
			if a.sortKey == k {
				a.reverse = !a.reverse
			} else {
				a.sortKey, a.reverse = k, false
			}
			fsops.Sort(a.entries, a.sortKey, a.reverse)
			a.rebuild()
		}
	}
	c := func(id, menu, title, hint string, keys []string, mac string, en func() bool, run func()) *ui.Command {
		return &ui.Command{ID: id, Menu: menu, Title: title, Hint: hint, Keys: keys, MacKey: mac, Enabled: en, Run: run}
	}
	sep := func(menu string) *ui.Command { return &ui.Command{ID: "", Menu: menu, Title: "-"} }

	a.cmds.Add(
		// File
		c("open", "File", "Open", "Open", []string{"Enter", "Alt+Down"}, "", hasSel, openCursor),
		c("view", "File", "View read-only", "", nil, "", func() bool {
			e, ok := a.cursorEntry()
			return ok && e.Mode.IsRegular()
		}, func() {
			if e, ok := a.cursorEntry(); ok {
				a.viewReadOnly(e)
			}
		}),
		c("newfile", "File", "New file...", "", nil, "", nil, func() { a.newItem(false) }),
		c("newfolder", "File", "New folder...", "", nil, "", nil, func() { a.newItem(true) }),
		sep("File"),
		c("rename", "File", "Rename", "Rename", []string{"F2"}, "", one, a.startRename),
		c("duplicate", "File", "Duplicate", "", []string{"Ctrl+D"}, "", hasSel, a.duplicate),
		c("copypath", "File", "Copy path", "", nil, "", nil, a.copyPath),
		sep("File"),
		c("perms", "File", "Permissions...", "", []string{"Ctrl+P", "Alt+Enter"}, "", hasSel, a.permissions),
		c("plusx", "File", "Make executable (+x)", "", nil, "", hasSel, func() { a.setExec(true) }),
		c("minusx", "File", "Remove executable (-x)", "", nil, "", hasSel, func() { a.setExec(false) }),
		sep("File"),
		c("trash", "File", "Move to Trash", "Trash", []string{"Delete"}, "", hasSel, a.trash),
		c("delete", "File", "Delete permanently...", "", []string{"Shift+Delete"}, "", hasSel, a.deletePermanently),

		// Edit
		c("undo", "Edit", "Undo", "Undo", []string{"Ctrl+Z"}, "", nil, a.undo),
		sep("Edit"),
		c("cut", "Edit", "Cut", "", []string{"Ctrl+X"}, "", hasSel, func() { a.clip(true) }),
		c("copy", "Edit", "Copy", "Copy", []string{"Ctrl+C"}, "", hasSel, func() { a.clip(false) }),
		c("paste", "Edit", "Paste", "Paste", []string{"Ctrl+V"}, "", hasClip, a.paste),
		sep("Edit"),
		c("selectall", "Edit", "Select all", "", []string{"Ctrl+A"}, "", nil, func() { a.selectRange(0, len(a.view)-1) }),

		// Go
		c("up", "Go", "Parent folder", "", []string{"Alt+Up"}, "", nil, a.up),
		c("goto", "Go", "Go to folder...", "", []string{"Ctrl+L"}, "", nil, a.promptPath),
		c("home", "Go", "Home folder", "", nil, "", nil, func() {
			if h, err := os.UserHomeDir(); err == nil {
				a.navigate(h, "")
			}
		}),
		c("root", "Go", "Root folder (/)", "", nil, "", nil, func() { a.navigate("/", "") }),

		// View
		c("refresh", "View", "Refresh", "", []string{"F5"}, "", nil, a.Refresh),
		c("hidden", "View", "Show / hide hidden files", "", []string{"Alt+."}, "", nil, func() {
			a.showHidden = !a.showHidden
			a.rebuild()
		}),
		sep("View"),
		c("sortname", "View", "Sort by name", "", nil, "", nil, sortBy(fsops.SortName)),
		c("sortsize", "View", "Sort by size", "", nil, "", nil, sortBy(fsops.SortSize)),
		c("sorttime", "View", "Sort by date", "", nil, "", nil, sortBy(fsops.SortTime)),
		sep("View"),
		c("palette", "View", "Command search...", "Commands", []string{"Ctrl+K"}, "", nil, func() { a.h.ShowPalette(&a.cmds) }),
	)
	a.cmds.ByID("palette").HintPinned = true
}
