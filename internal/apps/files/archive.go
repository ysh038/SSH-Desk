package files

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"tuidesk/internal/fsops"
	"tuidesk/internal/ui"
)

// SPEC 7.2: tar only (.tar, .tar.gz, .tgz).

// archives returns the selected tar archives.
func (a *App) archives() []string {
	var out []string
	for _, e := range a.Selection() {
		if !e.IsDir && e.Mode.IsRegular() && fsops.IsTarName(e.Name) {
			out = append(out, e.Path)
		}
	}
	return out
}

// extract unpacks the selected archives into the folder shown (tar -xzf).
func (a *App) extract() {
	ps := a.archives()
	if len(ps) == 0 {
		a.h.Notify("Select a .tar, .tar.gz or .tgz file to extract", true)
		return
	}
	a.extractPaths(ps)
}

func (a *App) extractPaths(ps []string) {
	dir := a.dir
	var rec *fsops.Record
	a.h.Go("Extracting "+plural(len(ps), "archive"), func(ctx context.Context, prog func(fsops.Progress)) error {
		var err error
		rec, err = a.h.FS().Extract(ctx, ps, dir, prog)
		return err
	}, func(err error) {
		a.h.PushUndo(rec)
		out := rec.Paths()
		if len(out) > 0 && a.dir == dir {
			a.selected = map[string]bool{}
			for _, p := range out {
				a.selected[filepath.Base(p)] = true
			}
			a.wantCur = filepath.Base(out[len(out)-1])
		}
		a.h.FilesChanged()
		if err != nil {
			if errors.Is(err, fsops.ErrUnsafeMember) {
				a.h.ShowError("Extract refused", "The archive has a path that would be written outside this folder,\nso nothing was extracted from it.\n\n"+a.display(errText(err)), err.Error())
				return
			}
			a.opFailed("Extract", err)
			return
		}
		what := plural(len(out), "item")
		if len(out) == 1 {
			what = a.display(filepath.Base(out[0]))
		}
		a.h.Notify("Extracted "+what+" · Ctrl+Z to undo", false)
	})
}

// compress packs the selection into one .tar.gz after asking for its name
// (tar -czf).
func (a *App) compress() {
	ps := paths(a.Selection())
	if len(ps) == 0 {
		return
	}
	dir := a.dir
	initial := fsops.CompressName(dir, ps)
	p := ui.NewPrompt("Compress", fmt.Sprintf("Archive name (%s):", plural(len(ps), "item")), initial, func(v string) {
		name := strings.TrimSpace(v)
		dst := filepath.Join(dir, name)
		var rec *fsops.Record
		a.h.Go("Compressing "+plural(len(ps), "item"), func(ctx context.Context, prog func(fsops.Progress)) error {
			var err error
			rec, err = a.h.FS().Compress(ctx, ps, dst, prog)
			return err
		}, func(err error) {
			a.h.PushUndo(rec)
			if err == nil && a.dir == dir {
				a.selected = map[string]bool{name: true}
				a.wantCur = name
			}
			a.h.FilesChanged()
			if err != nil {
				a.opFailed("Compress", err)
				return
			}
			a.h.Notify("Created "+a.display(name)+" · Ctrl+Z to undo", false)
		})
	})
	p.Validate = func(v string) error {
		v = strings.TrimSpace(v)
		if err := a.validNew(v); err != nil {
			return err
		}
		if !IsGzipTarName(v) {
			return errors.New("name must end in .tar.gz or .tgz")
		}
		return nil
	}
	a.h.Prompt(p)
}

// IsGzipTarName reports whether name ends in .tar.gz or .tgz.
func IsGzipTarName(name string) bool {
	l := strings.ToLower(name)
	return strings.HasSuffix(l, ".tar.gz") || strings.HasSuffix(l, ".tgz")
}
