package devices

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MediaBase is where volumes are mounted for a user: /media/<user>.
func MediaBase(user string) string { return "/media/" + cleanName(user) }

// maxDirBytes keeps mount folder names well under NAME_MAX (255 bytes); a
// Hangul syllable takes 3 bytes.
const maxDirBytes = 100

// cleanName makes s safe as a single folder name: no slashes, control
// characters or broken UTF-8, no leading dots (so never "." or "..", never
// hidden), not longer than maxDirBytes.
func cleanName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '/' || r < 0x20 || r == 0x7f || r == utf8.RuneError {
			continue
		}
		b.WriteRune(r)
	}
	s = strings.TrimLeft(strings.TrimSpace(b.String()), ". ")
	if len(s) > maxDirBytes {
		cut := maxDirBytes
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = strings.TrimSpace(s[:cut])
	}
	return s
}

// MountDirName is the folder name a volume is mounted under: its label,
// else its UUID, else the kernel name.
func MountDirName(v Volume) string {
	for _, s := range []string{v.Label, v.UUID, v.Name} {
		if n := cleanName(s); n != "" {
			return n
		}
	}
	return "volume"
}

// UniqueDir returns base/name, or base/"name (2)", "name (3)"... when that
// path is taken (exists reports it).
func UniqueDir(base, name string, exists func(path string) bool) string {
	p := base + "/" + name
	for i := 2; exists(p); i++ {
		p = fmt.Sprintf("%s/%s (%d)", base, name, i)
	}
	return p
}

// ownerFS are filesystems without Unix owners: uid= and gid= decide who
// may write.
var ownerFS = map[string]bool{
	"vfat": true, "msdos": true, "exfat": true, "ntfs": true, "ntfs3": true,
	"ntfs-3g": true, "iso9660": true, "udf": true, "hfsplus": true,
}

// HasOwners reports whether files on fstype keep their own Unix owners
// (ext4, xfs, btrfs...): mounting does not make them writable for the user.
func HasOwners(fstype string) bool { return !ownerFS[fstype] }

// MountOptions is the -o value for mounting fstype for the user uid:gid.
// nosuid and nodev always apply: nothing on a stick may gain privileges.
func MountOptions(fstype string, uid, gid int, readOnly bool) string {
	opts := []string{"nosuid", "nodev"}
	if readOnly {
		opts = append([]string{"ro"}, opts...)
	}
	if ownerFS[fstype] {
		opts = append(opts, fmt.Sprintf("uid=%d", uid), fmt.Sprintf("gid=%d", gid))
	}
	if fstype == "vfat" || fstype == "msdos" {
		opts = append(opts, "utf8") // Hangul file names
	}
	return strings.Join(opts, ",")
}

// mountScript makes the folder $1 when it is missing, mounts $3 on it with
// the options $2 and removes the folder again if it made it and the mount
// failed.
const mountScript = `made=0
if [ ! -d "$1" ]; then mkdir -p -- "$1" || exit 1; made=1; fi
mount -o "$2" -- "$3" "$1"; rc=$?
if [ "$rc" -ne 0 ] && [ "$made" = 1 ]; then rmdir -- "$1" 2>/dev/null; fi
exit "$rc"`

// unmountScript unmounts $1 and removes the emptied folder when it is one
// of ours (directly under $2, the user's media folder). No -f, no -l: a busy
// volume stays mounted.
const unmountScript = `umount -- "$1" || exit $?
case "$1" in "$2"/*) rmdir -- "$1" 2>/dev/null;; esac
exit 0`

// cleanUpScript removes the mount $1 of a drive that is already gone. A
// plain umount does it when nothing has the folder open; otherwise the
// mount is detached (-l) and disappears when the last program lets go.
// Only for stale mounts: on a living drive -l would hide unwritten data.
const cleanUpScript = `umount -- "$1" 2>/dev/null || umount -l -- "$1" || exit $?
case "$1" in "$2"/*) rmdir -- "$1" 2>/dev/null;; esac
exit 0`

// CleanUpCommand is the command (to run as root) that removes the stale
// mount on dir. base is MediaBase of the user.
func CleanUpCommand(dir, base string) []string {
	return []string{"sh", "-c", cleanUpScript, "sh", dir, base}
}

// powerOffScript flushes and detaches the disk $1 ("sdb") so it is safe to
// pull out. Devices without the control file (SD readers, NVMe) are left
// alone: unmounting was enough.
const powerOffScript = `f="/sys/block/$1/device/delete"
[ -w "$f" ] || exit 0
sync
echo 1 > "$f"`

// MountCommand is the command (to run as root) that mounts dev on dir.
func MountCommand(dev, dir, options string) []string {
	return []string{"sh", "-c", mountScript, "sh", dir, options, dev}
}

// UnmountCommand is the command (to run as root) that unmounts dir. base is
// MediaBase of the user.
func UnmountCommand(dir, base string) []string {
	return []string{"sh", "-c", unmountScript, "sh", dir, base}
}

// PowerOffCommand is the command (to run as root) that powers off the disk
// with the kernel name diskName. ok is false when the name is not a plain
// kernel name.
func PowerOffCommand(diskName string) (cmd []string, ok bool) {
	if diskName == "" {
		return nil, false
	}
	for _, r := range diskName {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return nil, false
		}
	}
	return []string{"sh", "-c", powerOffScript, "sh", diskName}, true
}
