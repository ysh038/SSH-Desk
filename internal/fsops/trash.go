package fsops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Trash moves paths into the freedesktop trash (files/ + info/*.trashinfo).
// Items on another file system are copied into the home trash and then
// removed.
func (f *FS) Trash(paths []string) (*Record, error) {
	rec := &Record{Label: "Move to Trash"}
	filesDir := filepath.Join(f.TrashDir, "files")
	infoDir := filepath.Join(f.TrashDir, "info")
	if err := os.MkdirAll(filesDir, 0o700); err != nil {
		return rec, err
	}
	if err := os.MkdirAll(infoDir, 0o700); err != nil {
		return rec, err
	}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return rec, err
		}
		if inside(f.TrashDir, abs) {
			return rec, fmt.Errorf("cannot move the trash into itself")
		}
		name, infoPath, err := f.reserve(infoDir, filepath.Base(abs), abs)
		if err != nil {
			return rec, err
		}
		dst := filepath.Join(filesDir, name)
		err = os.Rename(abs, dst)
		if errors.Is(err, syscall.EXDEV) {
			err = copyTree(context.Background(), abs, dst, &progress{})
			if err == nil {
				err = os.RemoveAll(abs)
			} else {
				_ = os.RemoveAll(dst)
			}
		}
		if err != nil {
			_ = os.Remove(infoPath)
			return rec, err
		}
		rec.steps = append(rec.steps, step{kind: stepTrashed, from: abs, to: dst, info: infoPath})
	}
	return rec, nil
}

// reserve creates info/<name>.trashinfo exclusively, picking a free name.
func (f *FS) reserve(infoDir, base, orig string) (name, infoPath string, err error) {
	body := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n", escapePath(orig), f.now().Format("2006-01-02T15:04:05"))
	stem, ext := splitExt(base)
	for i := 1; i < 10000; i++ {
		name = base
		if i > 1 {
			name = fmt.Sprintf("%s.%d%s", stem, i, ext)
		}
		infoPath = filepath.Join(infoDir, name+".trashinfo")
		fh, err := os.OpenFile(infoPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		if _, err := fh.WriteString(body); err != nil {
			fh.Close()
			os.Remove(infoPath)
			return "", "", err
		}
		if err := fh.Close(); err != nil {
			return "", "", err
		}
		if _, err := os.Lstat(filepath.Join(filepath.Dir(infoDir), "files", name)); err == nil {
			os.Remove(infoPath) // stale file without info: try the next name
			continue
		}
		return name, infoPath, nil
	}
	return "", "", fmt.Errorf("trash: no free name for %s", base)
}

// escapePath percent-encodes a path as the trash spec requires (RFC 2396),
// keeping '/' and unreserved characters. Implemented here to keep the lite
// build free of net/* packages.
func escapePath(p string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("/-_.~", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}

// ---- undo ----

type stepKind int

const (
	stepMoved   stepKind = iota // from -> to (rename, move)
	stepCreated                 // to was created (copy, new file/folder, duplicate)
	stepTrashed                 // from was moved to the trash at to; info file at info
	stepChmod                   // to had mode before
)

type step struct {
	kind     stepKind
	from, to string
	info     string
	mode     fs.FileMode
}

// Record describes a completed operation so it can be undone.
type Record struct {
	Label string
	steps []step
}

// Len is the number of recorded steps.
func (r *Record) Len() int {
	if r == nil {
		return 0
	}
	return len(r.steps)
}

// Paths returns the paths the operation produced (destinations).
func (r *Record) Paths() []string {
	if r == nil {
		return nil
	}
	var out []string
	for _, s := range r.steps {
		if s.kind != stepTrashed {
			out = append(out, s.to)
		}
	}
	return out
}

// Undo reverses rec, newest step first. Created items go to the trash
// (never deleted), moved items move back, trashed items are restored and
// modes are reset.
func (f *FS) Undo(rec *Record) error {
	if rec == nil {
		return nil
	}
	var errs []error
	for i := len(rec.steps) - 1; i >= 0; i-- {
		s := rec.steps[i]
		var err error
		switch s.kind {
		case stepMoved:
			if _, e := os.Lstat(s.from); e == nil {
				err = fmt.Errorf("cannot undo: %s exists again", s.from)
				break
			}
			_, err = f.Move(context.Background(), []string{s.to}, filepath.Dir(s.from), Skip, nil)
			if err == nil && filepath.Base(s.to) != filepath.Base(s.from) {
				err = os.Rename(filepath.Join(filepath.Dir(s.from), filepath.Base(s.to)), s.from)
			}
		case stepCreated:
			_, err = f.Trash([]string{s.to})
		case stepTrashed:
			if _, e := os.Lstat(s.from); e == nil {
				err = fmt.Errorf("cannot restore: %s exists", s.from)
				break
			}
			if err = os.MkdirAll(filepath.Dir(s.from), 0o755); err == nil {
				err = os.Rename(s.to, s.from)
				if errors.Is(err, syscall.EXDEV) {
					if err = copyTree(context.Background(), s.to, s.from, &progress{}); err == nil {
						err = os.RemoveAll(s.to)
					}
				}
			}
			if err == nil {
				_ = os.Remove(s.info)
			}
		case stepChmod:
			err = os.Chmod(s.to, chmodBits(s.mode))
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
