package fsops

import (
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

// Permission cells in display order: owner r w x, group r w x, others r w x.
var permBits = [9]fs.FileMode{0o400, 0o200, 0o100, 0o040, 0o020, 0o010, 0o004, 0o002, 0o001}

// PermBit returns the mode bit for cell i (0..8).
func PermBit(i int) fs.FileMode { return permBits[i] }

// ParseOctal parses "755", "0644" or "4755" into permission and special bits.
func ParseOctal(s string) (fs.FileMode, error) {
	s = strings.TrimSpace(s)
	if len(s) < 3 || len(s) > 4 {
		return 0, fmt.Errorf("octal mode must have 3 or 4 digits: %q", s)
	}
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid octal mode %q", s)
	}
	m := fs.FileMode(v & 0o777)
	if v&0o4000 != 0 {
		m |= fs.ModeSetuid
	}
	if v&0o2000 != 0 {
		m |= fs.ModeSetgid
	}
	if v&0o1000 != 0 {
		m |= fs.ModeSticky
	}
	return m, nil
}

// Octal formats permission and special bits as 3 digits (4 if special bits are set).
func Octal(m fs.FileMode) string {
	v := uint32(m.Perm())
	if m&fs.ModeSetuid != 0 {
		v |= 0o4000
	}
	if m&fs.ModeSetgid != 0 {
		v |= 0o2000
	}
	if m&fs.ModeSticky != 0 {
		v |= 0o1000
	}
	if v > 0o777 {
		return fmt.Sprintf("%04o", v)
	}
	return fmt.Sprintf("%03o", v)
}

// Symbolic formats the 9 permission characters like ls: "rwxr-xr-x",
// with s/S and t/T for setuid, setgid and sticky.
func Symbolic(m fs.FileMode) string {
	b := []byte("rwxrwxrwx")
	for i, bit := range permBits {
		if m&bit == 0 {
			b[i] = '-'
		}
	}
	special := func(i int, set bool, on, off byte) {
		if !set {
			return
		}
		if b[i] == 'x' {
			b[i] = on
		} else {
			b[i] = off
		}
	}
	special(2, m&fs.ModeSetuid != 0, 's', 'S')
	special(5, m&fs.ModeSetgid != 0, 's', 'S')
	special(8, m&fs.ModeSticky != 0, 't', 'T')
	return string(b)
}

// Tri is the state of one permission cell across several files.
type Tri uint8

const (
	Off Tri = iota
	On
	Mixed
)

// Aggregate returns the per-cell state over modes (Mixed where they differ).
func Aggregate(modes []fs.FileMode) [9]Tri {
	var out [9]Tri
	for i, bit := range permBits {
		on, off := false, false
		for _, m := range modes {
			if m&bit != 0 {
				on = true
			} else {
				off = true
			}
		}
		switch {
		case on && off:
			out[i] = Mixed
		case on:
			out[i] = On
		}
	}
	return out
}

// Change sets and clears permission bits, leaving all other bits alone.
type Change struct{ Set, Clear fs.FileMode }

// Apply returns m with the change applied.
func (c Change) Apply(m fs.FileMode) fs.FileMode { return (m &^ c.Clear) | c.Set }

// Empty reports whether the change does nothing.
func (c Change) Empty() bool { return c.Set == 0 && c.Clear == 0 }

// ChangeFrom builds a change from the cells the user edited: only cells whose
// state differs between before and after are applied, so mixed cells left
// untouched keep each file's own value.
func ChangeFrom(before, after [9]Tri) Change {
	var c Change
	for i, bit := range permBits {
		if before[i] == after[i] || after[i] == Mixed {
			continue
		}
		if after[i] == On {
			c.Set |= bit
		} else {
			c.Clear |= bit
		}
	}
	return c
}

// ExactChange turns m into a change that sets the permission bits exactly
// (used for an octal value typed for all selected files).
func ExactChange(m fs.FileMode) Change {
	return Change{Set: m.Perm(), Clear: fs.ModePerm &^ m.Perm()}
}

// PlusX adds execute permission to every class that can read (like chmod
// +x for a typical umask): rw-r--r-- becomes rwxr-xr-x, rw------- becomes rwx------.
func PlusX(m fs.FileMode) fs.FileMode {
	for c := 0; c < 3; c++ {
		if m&permBits[c*3] != 0 {
			m |= permBits[c*3+2]
		}
	}
	return m
}

// MinusX removes every execute bit.
func MinusX(m fs.FileMode) fs.FileMode { return m &^ 0o111 }

// Warnings lists risky aspects of changing old to new.
func Warnings(old, new fs.FileMode) []string {
	var w []string
	if old&0o400 != 0 && new&0o400 == 0 {
		w = append(w, "owner loses read permission")
	}
	if old&0o200 != 0 && new&0o200 == 0 && new.IsDir() {
		w = append(w, "owner loses write permission on a folder")
	}
	if new&0o002 != 0 && old&0o002 == 0 {
		w = append(w, "everyone can write (o+w)")
	}
	return w
}
