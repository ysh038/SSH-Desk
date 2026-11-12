// Package devices lists the storage devices of this machine and builds the
// commands that mount and unmount them (SPEC 7.8, ADR 0005). Parsing and
// classification are pure functions; nothing here touches the screen.
package devices

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Kind is the section of the My PC window a volume is shown in.
type Kind int

const (
	Fixed     Kind = iota // internal disks, and anything the system depends on
	Removable             // USB sticks and other hot-pluggable media
	Network               // NFS, CIFS, sshfs: shown and opened, never unmounted
	Virtual               // loop images (snap) and swap: hidden by default
)

// Volume is one thing that can hold files: a partition, a whole disk with a
// filesystem on it, or a network mount.
type Volume struct {
	Name     string // kernel name: "sdb1"
	Dev      string // "/dev/sdb1"; for network mounts the source "nas:/export"
	Disk     string // whole disk: "/dev/sdb" ("" when not on a disk)
	DiskName string // "sdb"
	FSType   string // "" when there is no filesystem
	Label    string
	UUID     string
	PartUUID string
	Size     int64 // bytes
	Mounts   []string
	Options  string // mount options of the first mount point
	Model    string // vendor and model of the disk
	Tran     string // "usb", "sata", "nvme", ...
	ReadOnly bool
	Swap     bool
	Kind     Kind
	// Protected volumes belong to the system (/, /boot, swap, /etc/fstab):
	// Unmount and Eject are not offered.
	Protected bool
	// Ejectable is true when the whole disk can be powered off: removable
	// and no protected volume on it.
	Ejectable bool
}

// Mounted reports whether the volume is mounted somewhere.
func (v Volume) Mounted() bool { return len(v.Mounts) > 0 }

// MountPoint is the main mount point, or "": the shortest path when the
// volume is mounted in several places ("/" rather than a bind mount of it).
func (v Volume) MountPoint() string {
	best := ""
	for _, m := range v.Mounts {
		if best == "" || len(m) < len(best) {
			best = m
		}
	}
	return best
}

// containers hold other volumes, not files.
var containers = map[string]bool{"LVM2_member": true, "crypto_LUKS": true, "linux_raid_member": true, "zfs_member": true}

// Mountable reports whether Mount can be offered.
func (v Volume) Mountable() bool {
	return v.FSType != "" && !v.Swap && !containers[v.FSType] && !v.Mounted() &&
		(v.Kind == Fixed || v.Kind == Removable)
}

// CanUnmount reports whether Unmount can be offered.
func (v Volume) CanUnmount() bool {
	return v.Mounted() && !v.Protected && (v.Kind == Fixed || v.Kind == Removable)
}

// Title is the name shown for the volume: its label, else where it is
// mounted, else the device.
func (v Volume) Title() string {
	switch {
	case v.Label != "":
		return v.Label
	case v.Mounted():
		return v.MountPoint()
	case v.Kind == Network:
		return v.Dev
	}
	return v.Name
}

// ---- lsblk --json ----

// Old lsblk versions print numbers and booleans as strings ("1", "0").
type flexBool bool

func (b *flexBool) UnmarshalJSON(d []byte) error {
	s := strings.Trim(string(d), `"`)
	*b = s == "true" || s == "1"
	return nil
}

type flexInt int64

func (n *flexInt) UnmarshalJSON(d []byte) error {
	v, _ := strconv.ParseInt(strings.Trim(string(d), `"`), 10, 64) // null or "1.5G": 0
	*n = flexInt(v)
	return nil
}

type node struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Type        string   `json:"type"`
	Size        flexInt  `json:"size"`
	FSType      string   `json:"fstype"`
	Label       string   `json:"label"`
	UUID        string   `json:"uuid"`
	PartUUID    string   `json:"partuuid"`
	Mountpoint  string   `json:"mountpoint"`  // util-linux < 2.37
	Mountpoints []string `json:"mountpoints"` // util-linux >= 2.37, may hold nulls
	RM          flexBool `json:"rm"`
	Hotplug     flexBool `json:"hotplug"`
	Tran        string   `json:"tran"`
	Model       string   `json:"model"`
	Vendor      string   `json:"vendor"`
	RO          flexBool `json:"ro"`
	Children    []node   `json:"children"`
}

// ParseLsblk reads the output of `lsblk --json --bytes` into volumes, in
// lsblk's order. Protection and mount options are filled in by Build.
func ParseLsblk(data []byte) ([]Volume, error) {
	if i := bytes.IndexByte(data, '{'); i > 0 {
		data = data[i:] // warnings printed before the JSON
	}
	var doc struct {
		Blockdevices []node `json:"blockdevices"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("cannot read the lsblk output: %w", err)
	}
	var out []Volume
	for i := range doc.Blockdevices {
		top := &doc.Blockdevices[i]
		collect(top, top, &out)
	}
	return out, nil
}

func devPath(n *node) string {
	if n.Path != "" {
		return n.Path
	}
	return "/dev/" + n.Name
}

// collect adds the leaves under n. A node with children (a partitioned
// disk, an LVM or LUKS container) is not a volume itself.
func collect(n, top *node, out *[]Volume) {
	if len(n.Children) > 0 {
		for i := range n.Children {
			collect(&n.Children[i], top, out)
		}
		return
	}
	removable := bool(top.RM) || bool(top.Hotplug) || top.Tran == "usb"
	if n.FSType == "" && (!removable || n.Type == "rom") {
		return // unused internal partitions, an empty optical drive
	}
	v := Volume{
		Name: n.Name, Dev: devPath(n), FSType: n.FSType, Label: n.Label,
		UUID: n.UUID, PartUUID: n.PartUUID, Size: int64(n.Size),
		Model:    strings.Join(strings.Fields(top.Vendor+" "+top.Model), " "),
		Tran:     top.Tran,
		ReadOnly: bool(n.RO),
		Swap:     n.FSType == "swap",
	}
	if top.Type == "disk" {
		v.Disk, v.DiskName = devPath(top), top.Name
	}
	for _, m := range append(n.Mountpoints[:len(n.Mountpoints):len(n.Mountpoints)], n.Mountpoint) {
		switch m {
		case "":
		case "[SWAP]":
			v.Swap = true
		default:
			v.Mounts = append(v.Mounts, m)
		}
	}
	switch {
	case n.Type == "loop" || n.FSType == "squashfs" || v.Swap:
		v.Kind = Virtual
	case removable:
		v.Kind = Removable
	default:
		v.Kind = Fixed
	}
	*out = append(*out, v)
}

// ---- classification ----

func systemMount(mounts []string) bool {
	for _, m := range mounts {
		if m == "/" || m == "/boot" || strings.HasPrefix(m, "/boot/") {
			return true
		}
	}
	return false
}

func inFstab(v Volume, fstab []FstabEntry) bool {
	for _, e := range fstab {
		switch e.Spec {
		case v.Dev:
			return true
		case "UUID=" + v.UUID, "/dev/disk/by-uuid/" + v.UUID:
			if v.UUID != "" {
				return true
			}
		case "LABEL=" + v.Label, "/dev/disk/by-label/" + v.Label:
			if v.Label != "" {
				return true
			}
		case "PARTUUID=" + v.PartUUID, "/dev/disk/by-partuuid/" + v.PartUUID:
			if v.PartUUID != "" {
				return true
			}
		}
		for _, m := range v.Mounts {
			if e.Dir == m {
				return true
			}
		}
	}
	return false
}

// Build puts everything together: the lsblk volumes, protection from
// /etc/fstab, mount options and network mounts from /proc/self/mountinfo.
// The result is ordered by Kind, keeping lsblk's order inside each.
func Build(lsblk []byte, fstab, mountinfo string) ([]Volume, error) {
	vols, err := ParseLsblk(lsblk)
	if err != nil {
		return nil, err
	}
	entries := ParseFstab(fstab)
	mounts := ParseMountInfo(mountinfo)
	byDir := make(map[string]Mount, len(mounts))
	for _, m := range mounts {
		byDir[m.Dir] = m // later mounts on the same folder hide earlier ones
	}

	busy := map[string]bool{}
	for i := range vols {
		v := &vols[i]
		if m, ok := byDir[v.MountPoint()]; ok {
			v.Options = m.Options
			v.ReadOnly = v.ReadOnly || m.ReadOnly()
		}
		v.Protected = v.Swap || systemMount(v.Mounts) || inFstab(*v, entries)
		if v.Protected {
			if v.Kind == Removable {
				v.Kind = Fixed // the system boots from it or expects it
			}
			if v.Disk != "" {
				busy[v.Disk] = true
			}
		}
	}
	for i := range vols {
		v := &vols[i]
		v.Ejectable = v.Kind == Removable && v.Disk != "" && !busy[v.Disk]
	}
	vols = append(vols, networkVolumes(mounts)...)
	sort.SliceStable(vols, func(i, j int) bool { return vols[i].Kind < vols[j].Kind })
	return vols, nil
}

// Siblings returns the mounted volumes on the same disk as v (v included):
// what Eject has to unmount first.
func Siblings(vols []Volume, v Volume) []Volume {
	var out []Volume
	for _, o := range vols {
		if o.Disk != "" && o.Disk == v.Disk && o.Mounted() {
			out = append(out, o)
		}
	}
	return out
}

// UsageBar draws used (0..1) as an ASCII bar of the given total width:
// "[######----]". Any use at all shows at least one mark.
func UsageBar(width int, used float64) string {
	inner := width - 2
	if inner < 1 {
		return ""
	}
	if used < 0 {
		used = 0
	}
	if used > 1 {
		used = 1
	}
	n := int(used*float64(inner) + 0.5)
	if n == 0 && used > 0 {
		n = 1
	}
	if n == inner && used < 1 {
		n = inner - 1 // only a full disk fills the bar
	}
	if n < 0 {
		n = 0
	}
	return "[" + strings.Repeat("#", n) + strings.Repeat("-", inner-n) + "]"
}
