package host

import (
	"context"
	"strings"

	"tuidesk/internal/fsops"
	"tuidesk/internal/sudo"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

// DirUser is implemented by apps that work inside folders. My PC asks every
// window before a drive is unmounted (SPEC 7.8).
type DirUser interface {
	// UsesDir says what the app has open in dir or below it ("2 files in the
	// editor"), or "" when nothing.
	UsesDir(dir string) string
}

// DirLeaver is a DirUser that can simply move out of the folder (a file
// manager goes to the parent) instead of blocking the unmount.
type DirLeaver interface {
	LeaveDir(dir string)
}

// Inside reports whether path is dir or below it.
func Inside(dir, path string) bool {
	dir = strings.TrimRight(dir, "/")
	return path == dir || strings.HasPrefix(path, dir+"/") || dir == ""
}

// DirUsers asks every window what it has open under any of dirs. Windows
// that can leave are returned as leavers; the others as descriptions.
func DirUsers(apps []wm.App, dirs []string) (leavers []DirLeaver, blocked []string) {
	for _, app := range apps {
		u, ok := app.(DirUser)
		if !ok {
			continue
		}
		for _, dir := range dirs {
			what := u.UsesDir(dir)
			if what == "" {
				continue
			}
			if l, ok := app.(DirLeaver); ok {
				leavers = append(leavers, l)
			} else {
				blocked = append(blocked, what)
			}
			break
		}
	}
	return leavers, blocked
}

// RunSudo runs cmds in order as root, stopping at the first failure, and
// calls done on the UI goroutine with their combined output. It first
// checks whether sudo needs a password and asks for it only then (SPEC
// 7.2.1: shown as *, passed once on stdin, wiped). Cancelling the password
// prompt runs nothing and done is not called.
func RunSudo(h Host, run sudo.Runner, user, title string, cmds [][]string, done func(out []byte, err error)) {
	exec := func(pw []rune) {
		var out []byte
		h.Go(title, func(ctx context.Context, _ func(fsops.Progress)) error {
			defer sudo.WipeRunes(pw)
			for _, c := range cmds {
				in := sudo.Input(pw, nil)
				o, err := run(ctx, "sudo", sudo.Args(pw != nil, c...), in)
				sudo.Wipe(in)
				out = append(out, o...)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if err != nil {
					return err
				}
			}
			return nil
		}, func(err error) { done(out, err) })
	}
	h.Go("Checking sudo", func(ctx context.Context, _ func(fsops.Progress)) error {
		_, err := run(ctx, "sudo", sudo.ProbeArgs(), nil)
		return err
	}, func(err error) {
		if err == nil {
			exec(nil) // NOPASSWD: sudo would not read a password
			return
		}
		p := ui.NewPrompt("sudo", "Password for "+user+" (sudo):", "", nil)
		p.Input.Password = true
		p.OnSecret = exec
		h.Prompt(p)
	})
}
