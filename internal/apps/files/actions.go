package files

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"tuidesk/internal/apps/viewer"
	"tuidesk/internal/fsops"
	"tuidesk/internal/host"
	"tuidesk/internal/term"
	"tuidesk/internal/ui"
)

func plural(n int, one string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %ss", n, one)
}

func (a *App) up() {
	parent := filepath.Dir(a.dir)
	if parent == a.dir {
		return
	}
	a.navigate(parent, filepath.Base(a.dir))
}

// open enters folders, views text files and describes everything else.
func (a *App) open(e fsops.Entry) {
	switch {
	case e.IsDir:
		a.navigate(e.Path, "")
	case e.Mode.IsRegular() && fsops.IsText(e.Path):
		v, err := viewer.Open(e.Path)
		if err != nil {
			a.h.ShowError("Cannot open file", a.display(e.Name)+": "+errText(err), "")
			return
		}
		a.h.OpenWindow(v)
	default:
		kind := "binary file"
		if e.Mode&0o111 != 0 {
			kind = "executable"
		}
		a.h.Confirm("Open", fmt.Sprintf("%s\n\nType: %s\nSize: %s\n\nRunning programs will be available with the terminal window (later phase).",
			a.display(e.Name), kind, ui.HumanSize(e.Size)), []string{"OK"}, nil)
	}
}

// ---- rename (inline) ----

func (a *App) startRename() {
	sel := a.Selection()
	if len(sel) != 1 {
		a.h.Notify("Select one item to rename", true)
		return
	}
	a.renameName = sel[0].Name
	a.moveCursorTo(a.renameName)
	a.renaming = ui.NewInput(sel[0].Name)
	// Select the stem like most file managers: cursor before the extension.
	if ext := filepath.Ext(sel[0].Name); ext != "" && ext != sel[0].Name && !sel[0].IsDir {
		a.renaming.Cursor = len([]rune(sel[0].Name)) - len([]rune(ext))
	}
}

func (a *App) handleRename(ev term.Event) bool {
	switch ev := ev.(type) {
	case term.KeyEvent:
		switch a.renaming.HandleKey(ev.Key) {
		case ui.InputSubmit:
			a.commitRename()
		case ui.InputCancel:
			a.renaming = nil
		}
		return true
	case term.PasteEvent:
		a.renaming.Paste(ev.Text)
		return true
	case term.MouseEvent:
		if ev.Gesture.Kind == term.GestureClick {
			a.commitRename()
		}
		return true
	}
	return false
}

func (a *App) commitRename() {
	in := a.renaming
	newName := strings.TrimSpace(in.Value())
	oldPath := filepath.Join(a.dir, a.renameName)
	if newName == a.renameName || newName == "" {
		a.renaming = nil
		return
	}
	rec, err := a.h.FS().Rename(oldPath, newName)
	if err != nil {
		a.h.Notify("Rename failed: "+errText(err), true)
		return // keep editing
	}
	a.renaming = nil
	a.h.PushUndo(rec)
	a.wantCur = newName
	a.h.FilesChanged()
}

// ---- prompts ----

func (a *App) promptPath() {
	p := ui.NewPrompt("Go to folder", "Path (~ for home):", a.dir, func(v string) { a.navigate(expand(v), "") })
	p.Validate = func(v string) error {
		info, err := os.Stat(expand(v))
		if err != nil {
			return errors.New(errText(err))
		}
		if !info.IsDir() {
			return errors.New("not a folder")
		}
		return nil
	}
	a.h.Prompt(p)
}

func expand(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = home + p[1:]
		}
	}
	return p
}

func (a *App) validNew(v string) error {
	if v == "" || v == "." || v == ".." || strings.ContainsRune(v, '/') {
		return errors.New("enter a name without '/'")
	}
	if _, err := os.Lstat(filepath.Join(a.dir, v)); err == nil {
		return fmt.Errorf("%q already exists", v)
	}
	return nil
}

func (a *App) newItem(folder bool) {
	title, label, initial := "New file", "File name:", "untitled.txt"
	if folder {
		title, label, initial = "New folder", "Folder name:", "New folder"
	}
	initial = fsops.UniqueName(a.dir, initial)
	p := ui.NewPrompt(title, label, initial, func(v string) {
		var rec *fsops.Record
		var err error
		if folder {
			_, rec, err = a.h.FS().Mkdir(a.dir, v)
		} else {
			_, rec, err = a.h.FS().CreateFile(a.dir, v)
		}
		if err != nil {
			a.h.ShowError(title+" failed", errText(err), "")
			return
		}
		a.h.PushUndo(rec)
		a.selected = map[string]bool{v: true}
		a.wantCur = v
		a.h.FilesChanged()
	})
	p.Validate = a.validNew
	a.h.Prompt(p)
}

// ---- clipboard ----

func (a *App) clip(cut bool) {
	sel := a.Selection()
	if len(sel) == 0 {
		return
	}
	*a.h.Files() = host.Clipboard{Paths: paths(sel), Cut: cut}
	verb := "Copied"
	if cut {
		verb = "Cut"
	}
	a.h.Notify(fmt.Sprintf("%s %s · Ctrl+V to paste", verb, plural(len(sel), "item")), false)
}

func (a *App) paste() {
	cb := a.h.Files()
	if len(cb.Paths) == 0 {
		a.h.Notify("Clipboard is empty", true)
		return
	}
	ps, cut := append([]string{}, cb.Paths...), cb.Cut
	if cut {
		*cb = host.Clipboard{} // a cut can be pasted once
	}
	a.Transfer(cut, ps, a.dir)
}

func (a *App) copyPath() {
	sel := a.Selection()
	text := a.dir
	if len(sel) > 0 {
		text = strings.Join(paths(sel), "\n")
	}
	a.h.SetClipboardText(text)
	a.h.Notify("Path copied", false)
}

// ---- operations ----

// Transfer moves or copies paths into dir, asking about name conflicts first.
// Used by paste and by drag and drop.
func (a *App) Transfer(move bool, ps []string, dir string) {
	run := func(pol fsops.Policy) {
		verb, done := "Copying", "Copied"
		if move {
			verb, done = "Moving", "Moved"
		}
		var rec *fsops.Record
		a.h.Go(fmt.Sprintf("%s %s", verb, plural(len(ps), "item")), func(ctx context.Context, prog func(fsops.Progress)) error {
			var err error
			if move {
				rec, err = a.h.FS().Move(ctx, ps, dir, pol, prog)
			} else {
				rec, err = a.h.FS().Copy(ctx, ps, dir, pol, prog)
			}
			return err
		}, func(err error) {
			a.h.PushUndo(rec)
			a.h.FilesChanged()
			if err != nil {
				a.opFailed(verb, err)
				return
			}
			a.h.Notify(fmt.Sprintf("%s %s to %s · Ctrl+Z to undo", done, plural(rec.Len(), "item"), a.display(filepath.Base(dir))+"/"), false)
		})
	}
	conflicts := fsops.Conflicts(ps, dir)
	if len(conflicts) == 0 {
		run(fsops.Skip)
		return
	}
	list := conflicts
	if len(list) > 5 {
		list = append(append([]string{}, list[:5]...), fmt.Sprintf("… and %d more", len(conflicts)-5))
	}
	for i := range list {
		list[i] = a.display(list[i])
	}
	msg := fmt.Sprintf("%s already in %s:\n  %s\n\nReplace moves the existing items to the Trash.",
		plural(len(conflicts), "item"), a.display(filepath.Base(dir))+"/", strings.Join(list, "\n  "))
	a.h.Confirm("Name conflict", msg, []string{"Replace", "Skip", "Keep both", "Cancel"}, func(choice int) {
		switch choice {
		case 0:
			run(fsops.Replace)
		case 1:
			run(fsops.Skip)
		case 2:
			run(fsops.KeepBoth)
		}
	})
}

func (a *App) opFailed(verb string, err error) {
	if errors.Is(err, context.Canceled) {
		a.h.Notify(verb+" cancelled · finished items can be undone with Ctrl+Z", true)
		return
	}
	title := verb + " failed"
	msg := errText(err)
	if errors.Is(err, fs.ErrPermission) {
		title = "Permission denied"
		msg = "You do not have permission for this operation.\n" + msg + "\n\n(Retrying with sudo comes in a later phase.)"
	}
	a.h.ShowError(title, msg, err.Error())
}

func (a *App) duplicate() {
	ps := paths(a.Selection())
	if len(ps) == 0 {
		return
	}
	var rec *fsops.Record
	a.h.Go("Duplicating", func(ctx context.Context, _ func(fsops.Progress)) error {
		var err error
		rec, err = a.h.FS().Duplicate(ctx, ps)
		return err
	}, func(err error) {
		a.h.PushUndo(rec)
		a.h.FilesChanged()
		if err != nil {
			a.opFailed("Duplicate", err)
		}
	})
}

func (a *App) trash() {
	sel := a.Selection()
	if len(sel) == 0 {
		return
	}
	rec, err := a.h.FS().Trash(paths(sel))
	a.h.PushUndo(rec)
	a.selected = map[string]bool{}
	a.h.FilesChanged()
	if err != nil {
		a.opFailed("Move to Trash", err)
		return
	}
	a.h.Notify(fmt.Sprintf("Moved %s to Trash · Ctrl+Z to undo", plural(len(sel), "item")), false)
}

func (a *App) deletePermanently() {
	sel := a.Selection()
	if len(sel) == 0 {
		return
	}
	what := a.display(sel[0].Name)
	if len(sel) > 1 {
		what = plural(len(sel), "item")
	}
	a.h.Confirm("Delete permanently", fmt.Sprintf("Delete %s permanently?\nThis cannot be undone. Use Move to Trash to keep a copy.", what),
		[]string{"Delete", "Cancel"}, func(c int) {
			if c != 0 {
				return
			}
			a.h.Confirm("Are you sure?", fmt.Sprintf("%s will be gone for good.", what), []string{"Yes, delete", "Cancel"}, func(c int) {
				if c != 0 {
					return
				}
				err := a.h.FS().DeletePermanently(paths(sel))
				a.selected = map[string]bool{}
				a.h.FilesChanged()
				if err != nil {
					a.opFailed("Delete", err)
					return
				}
				a.h.Notify("Deleted "+what, false)
			})
		})
}

func (a *App) undo() {
	rec := a.h.PopUndo()
	if rec == nil {
		a.h.Notify("Nothing to undo", true)
		return
	}
	err := a.h.FS().Undo(rec)
	a.h.FilesChanged()
	if err != nil {
		a.h.ShowError("Undo failed", "Undo of \""+rec.Label+"\" did not complete.", err.Error())
		return
	}
	a.h.Notify("Undone: "+rec.Label, false)
}

func (a *App) setExec(on bool) {
	var changes []fsops.ModeChange
	for _, e := range a.Selection() {
		if e.IsLink {
			continue
		}
		m := fsops.MinusX(e.Mode)
		if on {
			m = fsops.PlusX(e.Mode)
		}
		changes = append(changes, fsops.ModeChange{Path: e.Path, Mode: m})
	}
	rec, err := a.h.FS().Chmod(changes)
	a.h.PushUndo(rec)
	a.h.FilesChanged()
	if err != nil {
		a.opFailed("Permissions", err)
		return
	}
	verb := "Removed execute permission from"
	if on {
		verb = "Made executable:"
	}
	a.h.Notify(fmt.Sprintf("%s %s · Ctrl+Z to undo", verb, plural(len(changes), "item")), false)
}

func (a *App) permissions() {
	sel := a.Selection()
	if len(sel) == 0 {
		return
	}
	a.h.ShowModal(newPermDialog(a, sel))
}

// ---- context menu ----

func (a *App) contextMenu(x, y int, onItems bool) {
	cb := a.h.Files()
	var items []ui.MenuItem
	if onItems {
		sel := a.Selection()
		one := len(sel) == 1
		items = []ui.MenuItem{
			{Label: "Open", Hotkey: 'o', Shortcut: "Enter", Action: "open", Disabled: !one},
			{Label: "Rename", Hotkey: 'r', Shortcut: "F2", Action: "rename", Disabled: !one},
			{Separator: true},
			{Label: "Cut", Hotkey: 'u', Shortcut: "Ctrl+X", Action: "cut"},
			{Label: "Copy", Hotkey: 'c', Shortcut: "Ctrl+C", Action: "copy"},
			{Label: "Paste", Hotkey: 'p', Shortcut: "Ctrl+V", Action: "paste", Disabled: len(cb.Paths) == 0},
			{Label: "Duplicate", Hotkey: 'd', Shortcut: "Ctrl+D", Action: "duplicate"},
			{Separator: true},
			{Label: "Move to Trash", Hotkey: 't', Shortcut: "Del", Action: "trash"},
			{Label: "Delete permanently", Hotkey: 'e', Shortcut: "Shift+Del", Action: "delete"},
			{Separator: true},
			{Label: "Permissions...", Hotkey: 's', Shortcut: "Ctrl+P", Action: "perms"},
			{Label: "Make executable (+x)", Hotkey: 'x', Action: "plusx"},
			{Label: "Remove executable (-x)", Hotkey: 'v', Action: "minusx"},
			{Label: "Copy path", Hotkey: 'a', Action: "copypath"},
			{Separator: true},
			{Label: "New file", Hotkey: 'f', Action: "newfile"},
			{Label: "New folder", Hotkey: 'n', Action: "newfolder"},
			{Label: "Open terminal here", Action: "terminal", Disabled: true},
		}
	} else {
		hidden := "Show hidden files"
		if a.showHidden {
			hidden = "Hide hidden files"
		}
		items = []ui.MenuItem{
			{Label: "New file", Hotkey: 'f', Action: "newfile"},
			{Label: "New folder", Hotkey: 'n', Action: "newfolder"},
			{Label: "Paste", Hotkey: 'p', Shortcut: "Ctrl+V", Action: "paste", Disabled: len(cb.Paths) == 0},
			{Separator: true},
			{Label: "Refresh", Hotkey: 'r', Shortcut: "F5", Action: "refresh"},
			{Label: hidden, Hotkey: 'h', Shortcut: "Alt+.", Action: "hidden"},
			{Label: "Go to folder...", Hotkey: 'g', Shortcut: "Ctrl+L", Action: "goto"},
			{Label: "Copy path", Hotkey: 'a', Action: "copypath"},
			{Label: "Open terminal here", Action: "terminal", Disabled: true},
		}
	}
	a.h.OpenMenu(a, x, y, items, a.runAction)
}

func (a *App) runAction(action string) {
	switch action {
	case "open":
		if e, ok := a.cursorEntry(); ok {
			a.open(e)
		}
	case "rename":
		a.startRename()
	case "cut":
		a.clip(true)
	case "copy":
		a.clip(false)
	case "paste":
		a.paste()
	case "duplicate":
		a.duplicate()
	case "trash":
		a.trash()
	case "delete":
		a.deletePermanently()
	case "perms":
		a.permissions()
	case "plusx":
		a.setExec(true)
	case "minusx":
		a.setExec(false)
	case "copypath":
		a.copyPath()
	case "newfile":
		a.newItem(false)
	case "newfolder":
		a.newItem(true)
	case "refresh":
		a.Refresh()
	case "hidden":
		a.showHidden = !a.showHidden
		a.rebuild()
	case "goto":
		a.promptPath()
	}
}
