// Package fsops implements file operations for the file manager: listing,
// copy, move, trash (freedesktop spec), rename, create, duplicate, chmod and
// undo. Overwritten files go to the trash instead of being deleted, and every
// operation returns a Record that Undo can reverse.
//
// Tests must only touch t.TempDir() (CLAUDE.md rule 6).
package fsops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

// Entry is one directory entry.
type Entry struct {
	Name    string
	Path    string
	Size    int64
	Mode    fs.FileMode // includes fs.ModeDir / fs.ModeSymlink
	ModTime time.Time
	IsDir   bool // true also for symlinks that point to a directory
	IsLink  bool
	UID     uint32
	GID     uint32
}

// Hidden reports whether the entry is a dotfile.
func (e Entry) Hidden() bool { return strings.HasPrefix(e.Name, ".") }

func entryFor(path string, info fs.FileInfo) Entry {
	e := Entry{
		Name:    info.Name(),
		Path:    path,
		Size:    info.Size(),
		Mode:    info.Mode(),
		ModTime: info.ModTime(),
		IsDir:   info.IsDir(),
		IsLink:  info.Mode()&fs.ModeSymlink != 0,
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		e.UID, e.GID = st.Uid, st.Gid
	}
	if e.IsLink {
		if t, err := os.Stat(path); err == nil && t.IsDir() {
			e.IsDir = true
		}
	}
	return e
}

// Stat returns the entry for one path (not following a final symlink).
func Stat(path string) (Entry, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Entry{}, err
	}
	return entryFor(path, info), nil
}

// ReadDir lists dir. It checks ctx periodically so huge directories can be
// abandoned when the user navigates away. Entries that vanish while listing
// are skipped.
func ReadDir(ctx context.Context, dir string) ([]Entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(des))
	for i, de := range des {
		if i%1024 == 0 && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		p := filepath.Join(dir, de.Name())
		info, err := os.Lstat(p)
		if err != nil {
			continue
		}
		out = append(out, entryFor(p, info))
	}
	return out, nil
}

// SortKey selects the sort column.
type SortKey int

const (
	SortName SortKey = iota
	SortSize
	SortTime
)

// Sort orders entries with folders first, then by key (name ties broken by name).
func Sort(es []Entry, key SortKey, reverse bool) {
	less := func(a, b Entry) bool {
		switch key {
		case SortSize:
			if a.Size != b.Size {
				return a.Size < b.Size
			}
		case SortTime:
			if !a.ModTime.Equal(b.ModTime) {
				return a.ModTime.Before(b.ModTime)
			}
		}
		la, lb := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if la != lb {
			return la < lb
		}
		return a.Name < b.Name
	}
	sort.SliceStable(es, func(i, j int) bool {
		a, b := es[i], es[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		if reverse {
			return less(b, a)
		}
		return less(a, b)
	})
}

// IsText guesses whether path is a text file: valid UTF-8 with no NUL bytes
// in the first 8 KiB.
func IsText(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]
	if strings.IndexByte(string(buf), 0) >= 0 {
		return false
	}
	// A multi-byte rune may be cut at the end of the buffer.
	for i := 0; i < 3 && len(buf) > 0 && !utf8.Valid(buf); i++ {
		buf = buf[:len(buf)-1]
	}
	return utf8.Valid(buf)
}

// ---- operations ----

// Policy decides what happens when a destination name already exists.
type Policy int

const (
	Skip     Policy = iota // leave the existing item, do not copy/move this one
	Replace                // move the existing item to the trash, then copy/move
	KeepBoth               // give the new item a unique name: "name (2).ext"
)

// Progress reports bytes processed.
type Progress struct {
	Done, Total int64
	Current     string
}

// FS performs operations; TrashDir is the freedesktop trash directory
// (normally ~/.local/share/Trash).
type FS struct {
	TrashDir string
	Now      func() time.Time
}

// New returns an FS using $XDG_DATA_HOME/Trash or ~/.local/share/Trash.
func New() *FS {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return &FS{TrashDir: filepath.Join(base, "Trash"), Now: time.Now}
}

func (f *FS) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

// Conflicts returns the base names of srcs that already exist in dstDir
// (excluding sources that are already in dstDir).
func Conflicts(srcs []string, dstDir string) []string {
	var out []string
	for _, s := range srcs {
		if filepath.Dir(s) == filepath.Clean(dstDir) {
			continue
		}
		if _, err := os.Lstat(filepath.Join(dstDir, filepath.Base(s))); err == nil {
			out = append(out, filepath.Base(s))
		}
	}
	return out
}

// UniqueName returns name, or "name (2).ext", "name (3).ext"… so that it
// does not exist in dir.
func UniqueName(dir, name string) string {
	if _, err := os.Lstat(filepath.Join(dir, name)); errors.Is(err, fs.ErrNotExist) {
		return name
	}
	stem, ext := splitExt(name)
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s (%d)%s", stem, i, ext)
		if _, err := os.Lstat(filepath.Join(dir, cand)); errors.Is(err, fs.ErrNotExist) {
			return cand
		}
	}
}

func splitExt(name string) (stem, ext string) {
	ext = filepath.Ext(name)
	if ext == name || ext == "" { // ".bashrc" or no extension
		return name, ""
	}
	return strings.TrimSuffix(name, ext), ext
}

// ErrIntoItself is returned when copying or moving a folder into itself.
var ErrIntoItself = errors.New("cannot copy or move a folder into itself")

func inside(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && (rel == "." || !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// Copy copies srcs into dstDir.
func (f *FS) Copy(ctx context.Context, srcs []string, dstDir string, pol Policy, prog func(Progress)) (*Record, error) {
	return f.transfer(ctx, srcs, dstDir, pol, prog, false)
}

// Move moves srcs into dstDir (rename, or copy+delete across file systems).
func (f *FS) Move(ctx context.Context, srcs []string, dstDir string, pol Policy, prog func(Progress)) (*Record, error) {
	return f.transfer(ctx, srcs, dstDir, pol, prog, true)
}

func (f *FS) transfer(ctx context.Context, srcs []string, dstDir string, pol Policy, prog func(Progress), move bool) (*Record, error) {
	rec := &Record{}
	if move {
		rec.Label = "Move"
	} else {
		rec.Label = "Copy"
	}
	var total int64
	for _, s := range srcs {
		if inside(dstDir, s) {
			return rec, fmt.Errorf("%s: %w", filepath.Base(s), ErrIntoItself)
		}
		total += treeSize(s)
	}
	p := &progress{fn: prog, total: total}
	for _, src := range srcs {
		if err := ctx.Err(); err != nil {
			return rec, err
		}
		name := filepath.Base(src)
		if move && filepath.Dir(src) == filepath.Clean(dstDir) {
			continue // already there
		}
		dst := filepath.Join(dstDir, name)
		if _, err := os.Lstat(dst); err == nil {
			switch pol {
			case Skip:
				continue
			case KeepBoth:
				dst = filepath.Join(dstDir, UniqueName(dstDir, name))
			case Replace:
				r, err := f.Trash([]string{dst})
				rec.steps = append(rec.steps, r.steps...)
				if err != nil {
					return rec, err
				}
			}
		}
		if move {
			err := os.Rename(src, dst)
			if err == nil {
				rec.steps = append(rec.steps, step{kind: stepMoved, from: src, to: dst})
				p.add(treeSize(dst), name)
				continue
			}
			if !errors.Is(err, syscall.EXDEV) {
				return rec, err
			}
			// Different file system: copy, then delete the source.
			if err := copyTree(ctx, src, dst, p); err != nil {
				_ = os.RemoveAll(dst)
				return rec, err
			}
			if err := os.RemoveAll(src); err != nil {
				return rec, err
			}
			rec.steps = append(rec.steps, step{kind: stepMoved, from: src, to: dst})
			continue
		}
		if err := copyTree(ctx, src, dst, p); err != nil {
			_ = os.RemoveAll(dst)
			return rec, err
		}
		rec.steps = append(rec.steps, step{kind: stepCreated, to: dst})
	}
	return rec, nil
}

type progress struct {
	fn          func(Progress)
	done, total int64
	last        time.Time
}

func (p *progress) add(n int64, cur string) {
	p.done += n
	if p.fn != nil && (time.Since(p.last) > 50*time.Millisecond || p.done == p.total) {
		p.last = time.Now()
		p.fn(Progress{Done: p.done, Total: p.total, Current: cur})
	}
}

func treeSize(path string) int64 {
	var n int64
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

func copyTree(ctx context.Context, src, dst string, p *progress) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case info.IsDir():
		if err := os.Mkdir(dst, info.Mode().Perm()|0o700); err != nil {
			return err
		}
		des, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, de := range des {
			if err := copyTree(ctx, filepath.Join(src, de.Name()), filepath.Join(dst, de.Name()), p); err != nil {
				return err
			}
		}
		if err := os.Chmod(dst, info.Mode().Perm()); err != nil {
			return err
		}
		return os.Chtimes(dst, info.ModTime(), info.ModTime())
	case info.Mode().IsRegular():
		return copyFile(ctx, src, dst, info, p)
	default:
		return fmt.Errorf("%s: unsupported file type", src)
	}
}

func copyFile(ctx context.Context, src, dst string, info fs.FileInfo, p *progress) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm()|0o200)
	if err != nil {
		return err
	}
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			out.Close()
			return err
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return werr
			}
			p.add(int64(n), filepath.Base(src))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return rerr
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(dst, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

// Rename renames path to newName within the same folder.
func (f *FS) Rename(path, newName string) (*Record, error) {
	if err := validName(newName); err != nil {
		return nil, err
	}
	dst := filepath.Join(filepath.Dir(path), newName)
	if dst == path {
		return &Record{Label: "Rename"}, nil
	}
	if _, err := os.Lstat(dst); err == nil {
		return nil, fmt.Errorf("%q already exists", newName)
	}
	if err := os.Rename(path, dst); err != nil {
		return nil, err
	}
	return &Record{Label: "Rename", steps: []step{{kind: stepMoved, from: path, to: dst}}}, nil
}

func validName(name string) error {
	switch {
	case name == "" || name == "." || name == "..":
		return fmt.Errorf("invalid name %q", name)
	case strings.ContainsRune(name, '/') || strings.ContainsRune(name, 0):
		return fmt.Errorf("name must not contain '/': %q", name)
	}
	return nil
}

// Mkdir creates folder name in dir.
func (f *FS) Mkdir(dir, name string) (string, *Record, error) {
	if err := validName(name); err != nil {
		return "", nil, err
	}
	p := filepath.Join(dir, name)
	if err := os.Mkdir(p, 0o755); err != nil {
		return "", nil, err
	}
	return p, &Record{Label: "New folder", steps: []step{{kind: stepCreated, to: p}}}, nil
}

// CreateFile creates an empty file name in dir.
func (f *FS) CreateFile(dir, name string) (string, *Record, error) {
	if err := validName(name); err != nil {
		return "", nil, err
	}
	p := filepath.Join(dir, name)
	fh, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", nil, err
	}
	if err := fh.Close(); err != nil {
		return "", nil, err
	}
	return p, &Record{Label: "New file", steps: []step{{kind: stepCreated, to: p}}}, nil
}

// DuplicateName returns "name copy.ext", "name copy 2.ext"… free in dir.
func DuplicateName(dir, name string) string {
	stem, ext := splitExt(name)
	for i := 1; ; i++ {
		cand := stem + " copy" + ext
		if i > 1 {
			cand = fmt.Sprintf("%s copy %d%s", stem, i, ext)
		}
		if _, err := os.Lstat(filepath.Join(dir, cand)); errors.Is(err, fs.ErrNotExist) {
			return cand
		}
	}
}

// Duplicate copies each path next to itself as "name copy".
func (f *FS) Duplicate(ctx context.Context, paths []string) (*Record, error) {
	rec := &Record{Label: "Duplicate"}
	for _, src := range paths {
		dir := filepath.Dir(src)
		dst := filepath.Join(dir, DuplicateName(dir, filepath.Base(src)))
		if err := copyTree(ctx, src, dst, &progress{}); err != nil {
			_ = os.RemoveAll(dst)
			return rec, err
		}
		rec.steps = append(rec.steps, step{kind: stepCreated, to: dst})
	}
	return rec, nil
}

// DeletePermanently removes paths for good (no undo). Callers must confirm twice.
func (f *FS) DeletePermanently(paths []string) error {
	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return nil
}

// ModeChange is a planned chmod.
type ModeChange struct {
	Path string
	Mode fs.FileMode // new permission and special bits
}

// PlanChmod builds the list of changes. For each selected path the change c
// is applied; with contents=true, folders and files below selected folders
// get dirMode / fileMode exactly (nil = leave them alone).
func PlanChmod(paths []string, c Change, contents bool, dirMode, fileMode *fs.FileMode) ([]ModeChange, error) {
	var out []ModeChange
	for _, p := range paths {
		info, err := os.Lstat(p)
		if err != nil {
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			continue // chmod follows links; never change a link target implicitly
		}
		if !c.Empty() {
			out = append(out, ModeChange{Path: p, Mode: c.Apply(info.Mode())})
		}
		if !contents || !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(p, func(q string, d fs.DirEntry, err error) error {
			if err != nil || q == p || d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			switch {
			case d.IsDir() && dirMode != nil:
				out = append(out, ModeChange{Path: q, Mode: ExactChange(*dirMode).Apply(info.Mode())})
			case !d.IsDir() && fileMode != nil:
				out = append(out, ModeChange{Path: q, Mode: ExactChange(*fileMode).Apply(info.Mode())})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Chmod applies planned changes and records the old modes for undo. On the
// first error it stops; the record covers what was changed.
//
// Deeper paths are changed first: if a folder loses its execute bit, its
// contents would otherwise become unreachable halfway through. Undo runs in
// reverse, so parents regain access before their children are restored.
func (f *FS) Chmod(changes []ModeChange) (*Record, error) {
	rec := &Record{Label: "Permissions"}
	changes = append([]ModeChange(nil), changes...)
	sort.SliceStable(changes, func(i, j int) bool {
		return strings.Count(changes[i].Path, string(filepath.Separator)) > strings.Count(changes[j].Path, string(filepath.Separator))
	})
	for _, ch := range changes {
		info, err := os.Lstat(ch.Path)
		if err != nil {
			return rec, err
		}
		if err := os.Chmod(ch.Path, chmodBits(ch.Mode)); err != nil {
			return rec, err
		}
		rec.steps = append(rec.steps, step{kind: stepChmod, to: ch.Path, mode: info.Mode()})
	}
	return rec, nil
}

// chmodBits keeps only what os.Chmod understands.
func chmodBits(m fs.FileMode) fs.FileMode {
	return m & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky)
}
