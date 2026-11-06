package fsops

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SPEC 7.2: tar is the only archive format the servers use, so only .tar,
// .tar.gz and .tgz are handled, with the standard library (no tar binary,
// no network, fine for the lite build).

var tarSuffixes = []string{".tar.gz", ".tgz", ".tar"}

// IsTarName reports whether name looks like a tar archive by its extension.
func IsTarName(name string) bool {
	l := strings.ToLower(name)
	for _, s := range tarSuffixes {
		if strings.HasSuffix(l, s) && len(l) > len(s) {
			return true
		}
	}
	return false
}

// ArchiveStem is name without its tar extension: "app-1.0.tar.gz" -> "app-1.0".
func ArchiveStem(name string) string {
	l := strings.ToLower(name)
	for _, s := range tarSuffixes {
		if strings.HasSuffix(l, s) && len(l) > len(s) {
			return name[:len(name)-len(s)]
		}
	}
	return name
}

// ErrUnsafeMember is returned for archive members that would be written
// outside the extraction folder ("../x", or through a symlink).
var ErrUnsafeMember = errors.New("unsafe path in archive")

// Extract unpacks each tar archive into dstDir (like tar -xzf). Each archive
// is unpacked into a hidden staging folder first; then its single top-level
// item, or a folder named after the archive holding several items, is moved
// into dstDir under a free name. Nothing in dstDir is ever overwritten. The
// Record lists the created items, so Undo moves them to the trash.
func (f *FS) Extract(ctx context.Context, archives []string, dstDir string, prog func(Progress)) (*Record, error) {
	rec := &Record{Label: "Extract"}
	var total int64
	for _, a := range archives {
		if info, err := os.Stat(a); err == nil {
			total += info.Size()
		}
	}
	p := &progress{fn: prog, total: total}
	for _, a := range archives {
		if err := ctx.Err(); err != nil {
			return rec, err
		}
		out, err := extractOne(ctx, a, dstDir, p)
		if err != nil {
			return rec, fmt.Errorf("%s: %w", filepath.Base(a), err)
		}
		rec.steps = append(rec.steps, step{kind: stepCreated, to: out})
	}
	return rec, nil
}

func extractOne(ctx context.Context, archive, dstDir string, p *progress) (string, error) {
	fh, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer fh.Close()
	staging, err := os.MkdirTemp(dstDir, ".tuidesk-extract-")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(staging)
		}
	}()

	r, err := tarReader(&countingReader{r: fh, p: p, name: filepath.Base(archive)})
	if err != nil {
		return "", err
	}
	if err := untar(ctx, r, staging); err != nil {
		return "", err
	}

	des, err := os.ReadDir(staging)
	if err != nil {
		return "", err
	}
	var src, name string
	switch len(des) {
	case 0:
		return "", errors.New("the archive is empty")
	case 1:
		src, name = filepath.Join(staging, des[0].Name()), des[0].Name()
	default:
		src, name = staging, ArchiveStem(filepath.Base(archive))
	}
	dst := filepath.Join(dstDir, UniqueName(dstDir, name))
	if src == staging {
		if err := os.Chmod(staging, 0o755); err != nil { // MkdirTemp made it 0700
			return "", err
		}
	}
	if err := os.Rename(src, dst); err != nil {
		return "", err
	}
	if src == staging {
		ok = true
	}
	return dst, nil
}

// tarReader opens r as a tar stream, gunzipping it when it starts with the
// gzip magic bytes (whatever the file is called).
func tarReader(r io.Reader) (*tar.Reader, error) {
	br := bufio.NewReader(r)
	if magic, _ := br.Peek(2); len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		zr, err := gzip.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("not a valid .tar.gz file: %w", err)
		}
		return tar.NewReader(zr), nil
	}
	return tar.NewReader(br), nil
}

type countingReader struct {
	r    io.Reader
	p    *progress
	name string
}

func (c *countingReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	if n > 0 {
		c.p.add(int64(n), c.name)
	}
	return n, err
}

// memberPath turns a tar member name into a clean relative path. Like GNU
// tar it drops a leading "/" and refuses "..". It returns "" for the root
// ("./").
func memberPath(name string) (string, error) {
	name = strings.TrimLeft(name, "/")
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", fmt.Errorf("%w: %s", ErrUnsafeMember, name)
		}
	}
	clean := path.Clean(name)
	if clean == "." {
		return "", nil
	}
	return clean, nil
}

// safeParent makes sure every folder between root and rel's parent exists
// and is a real folder, not a symlink (a member may not be written through
// a symlink that an earlier member created).
func safeParent(root, rel string) error {
	dir := root
	parts := strings.Split(path.Dir(rel), "/")
	for _, part := range parts {
		if part == "." {
			continue
		}
		dir = filepath.Join(dir, part)
		info, err := os.Lstat(dir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if err := os.Mkdir(dir, 0o755); err != nil {
				return err
			}
		case err != nil:
			return err
		case info.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("%w: %s goes through a symlink", ErrUnsafeMember, rel)
		case !info.IsDir():
			return fmt.Errorf("%s: %s is not a folder", rel, path.Join(parts...))
		}
	}
	return nil
}

// clearTarget removes a non-folder already at p, so a later member with the
// same name replaces it (tar semantics) instead of writing through it.
func clearTarget(p string) error {
	info, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s: a folder with this name already exists", filepath.Base(p))
	}
	return os.Remove(p)
}

func untar(ctx context.Context, r *tar.Reader, root string) error {
	type dirMeta struct {
		path  string
		mode  fs.FileMode
		mtime time.Time
	}
	var dirs []dirMeta
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("not a valid tar archive: %w", err)
		}
		rel, err := memberPath(h.Name)
		if err != nil {
			return err
		}
		if rel == "" {
			continue
		}
		if err := safeParent(root, rel); err != nil {
			return err
		}
		target := filepath.Join(root, filepath.FromSlash(rel))
		perm := fs.FileMode(h.Mode) & 0o777
		switch h.Typeflag {
		case tar.TypeDir:
			info, err := os.Lstat(target)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				if err := os.Mkdir(target, 0o700); err != nil {
					return err
				}
			case err != nil:
				return err
			case !info.IsDir():
				if err := os.Remove(target); err != nil {
					return err
				}
				if err := os.Mkdir(target, 0o700); err != nil {
					return err
				}
			default:
				if err := os.Chmod(target, 0o700); err != nil {
					return err
				}
			}
			dirs = append(dirs, dirMeta{target, perm, h.ModTime})
		case tar.TypeReg, tar.TypeRegA, tar.TypeGNUSparse:
			if err := clearTarget(target); err != nil {
				return err
			}
			if err := writeMember(ctx, r, target, perm, h.ModTime); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := clearTarget(target); err != nil {
				return err
			}
			if err := os.Symlink(h.Linkname, target); err != nil {
				return err
			}
		case tar.TypeLink:
			lrel, err := memberPath(h.Linkname)
			if err != nil || lrel == "" {
				return fmt.Errorf("%w: hard link %s -> %s", ErrUnsafeMember, rel, h.Linkname)
			}
			if err := safeParent(root, lrel); err != nil {
				return err
			}
			if err := clearTarget(target); err != nil {
				return err
			}
			if err := os.Link(filepath.Join(root, filepath.FromSlash(lrel)), target); err != nil {
				return err
			}
		default:
			// Devices, FIFOs and other special files: skipped (a normal
			// user cannot create most of them anyway).
		}
	}
	// Folder modes and times last, deepest first, so writing into them
	// worked and their times are not bumped by the files inside.
	sort.SliceStable(dirs, func(i, j int) bool { return len(dirs[i].path) > len(dirs[j].path) })
	for _, d := range dirs {
		if err := os.Chmod(d.path, d.mode); err != nil {
			return err
		}
		_ = os.Chtimes(d.path, d.mtime, d.mtime)
	}
	return nil
}

func writeMember(ctx context.Context, r io.Reader, target string, perm fs.FileMode, mtime time.Time) error {
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm|0o200)
	if err != nil {
		return err
	}
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			out.Close()
			return err
		}
		n, rerr := r.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return werr
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return fmt.Errorf("not a valid tar archive: %w", rerr)
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(target, perm); err != nil {
		return err
	}
	return os.Chtimes(target, mtime, mtime)
}

// CompressName is the default archive name for srcs (all in dir):
// "<name>.tar.gz" for one item, "<folder>.tar.gz" for several, free in dir.
func CompressName(dir string, srcs []string) string {
	base := filepath.Base(dir)
	if len(srcs) == 1 {
		base = filepath.Base(srcs[0])
	}
	if base == "/" || base == "." {
		base = "archive"
	}
	return UniqueName(dir, base+".tar.gz")
}

// Compress packs srcs into the gzipped tar archive dst (like tar -czf dst
// -C <parent> <names...>): members are stored under their base names,
// symlinks as symlinks. The archive is written to a hidden temporary file
// and renamed into place when complete, so a cancelled or failed run
// leaves nothing behind. dst must not exist.
func (f *FS) Compress(ctx context.Context, srcs []string, dst string, prog func(Progress)) (*Record, error) {
	rec := &Record{Label: "Compress"}
	if err := validName(filepath.Base(dst)); err != nil {
		return rec, err
	}
	if _, err := os.Lstat(dst); err == nil {
		return rec, fmt.Errorf("%q already exists", filepath.Base(dst))
	}
	var total int64
	for _, s := range srcs {
		total += treeSize(s)
	}
	p := &progress{fn: prog, total: total}

	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".part-")
	if err != nil {
		return rec, err
	}
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	bw := bufio.NewWriterSize(tmp, 1<<20)
	zw := gzip.NewWriter(bw)
	tw := tar.NewWriter(zw)
	for _, src := range srcs {
		if err := addTree(ctx, tw, src, p); err != nil {
			return rec, err
		}
	}
	for _, c := range []io.Closer{tw, zw} {
		if err := c.Close(); err != nil {
			return rec, err
		}
	}
	if err := bw.Flush(); err != nil {
		return rec, err
	}
	if err := tmp.Close(); err != nil {
		return rec, err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { // CreateTemp made it 0600
		return rec, err
	}
	if _, err := os.Lstat(dst); err == nil {
		return rec, fmt.Errorf("%q already exists", filepath.Base(dst))
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return rec, err
	}
	ok = true
	rec.steps = append(rec.steps, step{kind: stepCreated, to: dst})
	return rec, nil
}

func addTree(ctx context.Context, tw *tar.Writer, src string, p *progress) error {
	parent := filepath.Dir(src)
	return filepath.WalkDir(src, func(fp string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(parent, fp)
		if err != nil {
			return err
		}
		link := ""
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			if link, err = os.Readlink(fp); err != nil {
				return err
			}
		case info.IsDir(), info.Mode().IsRegular():
		default:
			return nil // sockets, devices, FIFOs: skipped
		}
		h, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			h.Name += "/"
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		in, err := os.Open(fp)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(tw, &ctxReader{ctx: ctx, r: &countingReader{r: in, p: p, name: d.Name()}})
		return err
	})
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(b []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(b)
}
