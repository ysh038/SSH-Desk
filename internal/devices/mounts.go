package devices

import (
	"strings"
)

// Mount is one line of /proc/self/mountinfo.
type Mount struct {
	Source  string // "/dev/sdb1", "nas:/export"
	Dir     string
	FSType  string
	Options string // "rw,nosuid,nodev,relatime"
}

// ReadOnly reports the "ro" mount option.
func (m Mount) ReadOnly() bool {
	for _, o := range strings.Split(m.Options, ",") {
		if o == "ro" {
			return true
		}
	}
	return false
}

// unescape decodes the octal escapes the kernel uses in mountinfo and fstab
// for space (\040), tab (\011), newline (\012) and backslash (\134).
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) && isOctal(s[i+1]) && isOctal(s[i+2]) && isOctal(s[i+3]) {
			b.WriteByte((s[i+1]-'0')<<6 | (s[i+2]-'0')<<3 | (s[i+3] - '0'))
			i += 3
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isOctal(c byte) bool { return c >= '0' && c <= '7' }

// ParseMountInfo reads /proc/self/mountinfo. Lines it cannot read are
// skipped.
//
//	36 35 98:0 / /mnt/usb rw,noatime shared:1 - vfat /dev/sdb1 rw,uid=1000
func ParseMountInfo(data string) []Mount {
	var out []Mount
	for _, line := range strings.Split(data, "\n") {
		left, right, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		l, r := strings.Fields(left), strings.Fields(right)
		if len(l) < 6 || len(r) < 2 {
			continue
		}
		out = append(out, Mount{Source: unescape(r[1]), Dir: unescape(l[4]), FSType: r[0], Options: l[5]})
	}
	return out
}

// FstabEntry is one filesystem listed in /etc/fstab.
type FstabEntry struct {
	Spec string // "UUID=...", "LABEL=...", "/dev/sda2"
	Dir  string
}

// ParseFstab reads /etc/fstab, skipping comments and swap's "none" folder.
func ParseFstab(data string) []FstabEntry {
	var out []FstabEntry
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		e := FstabEntry{Spec: unescape(f[0]), Dir: unescape(f[1])}
		if !strings.HasPrefix(e.Dir, "/") {
			e.Dir = "" // "none", "swap"
		}
		out = append(out, e)
	}
	return out
}

var networkFS = map[string]bool{
	"nfs": true, "nfs4": true, "cifs": true, "smb3": true,
	"fuse.sshfs": true, "ceph": true, "fuse.glusterfs": true,
}

// networkVolumes turns network mounts into volumes, one per folder.
func networkVolumes(mounts []Mount) []Volume {
	var out []Volume
	seen := map[string]bool{}
	for _, m := range mounts {
		if !networkFS[m.FSType] || seen[m.Dir] {
			continue
		}
		seen[m.Dir] = true
		out = append(out, Volume{
			Name: m.Source, Dev: m.Source, FSType: m.FSType, Mounts: []string{m.Dir},
			Options: m.Options, ReadOnly: m.ReadOnly(), Kind: Network,
		})
	}
	return out
}
