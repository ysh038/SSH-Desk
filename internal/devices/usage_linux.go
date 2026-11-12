//go:build linux

package devices

import "syscall"

// Usage returns the size of the filesystem mounted at dir and the space a
// normal user can still write, in bytes.
func Usage(dir string) (total, free int64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, 0, err
	}
	return int64(st.Blocks) * int64(st.Bsize), int64(st.Bavail) * int64(st.Bsize), nil
}
