package devices

import (
	"context"
	"fmt"
	"os"
	"strings"

	"tuidesk/internal/sudo"
)

// columns asked from lsblk. MOUNTPOINTS needs util-linux 2.37 (Ubuntu
// 22.04); older versions only know MOUNTPOINT.
const columns = "NAME,PATH,TYPE,SIZE,FSTYPE,LABEL,UUID,PARTUUID,MOUNTPOINTS,RM,HOTPLUG,TRAN,MODEL,VENDOR,RO"

// Source is where the device list comes from. Tests replace both fields.
type Source struct {
	Run      sudo.Runner
	ReadFile func(path string) ([]byte, error)
}

// System reads the real machine.
func System() Source { return Source{Run: sudo.Exec, ReadFile: os.ReadFile} }

func lsblkArgs(cols string) []string {
	return []string{"--json", "--bytes", "--output", cols}
}

// List returns the volumes of this machine. A missing /etc/fstab or
// mountinfo only loses protection hints and network mounts.
func (s Source) List(ctx context.Context) ([]Volume, error) {
	out, err := s.Run(ctx, "lsblk", lsblkArgs(columns), nil)
	if err != nil {
		var err2 error
		out, err2 = s.Run(ctx, "lsblk", lsblkArgs(strings.Replace(columns, "MOUNTPOINTS", "MOUNTPOINT", 1)), nil)
		if err2 != nil {
			return nil, fmt.Errorf("lsblk failed: %w: %s", err2, firstLine(out))
		}
	}
	fstab, _ := s.ReadFile("/etc/fstab")
	mountinfo, _ := s.ReadFile("/proc/self/mountinfo")
	return Build(out, string(fstab), string(mountinfo))
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
