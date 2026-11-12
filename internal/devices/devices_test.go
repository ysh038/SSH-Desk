package devices

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// A server with one SATA disk (EFI, /, swap, an fstab data partition), a
// snap loop image, a USB stick with a Hangul label that is not mounted, a
// mounted USB disk without a partition table, a blank USB stick and an
// empty optical drive. Shape of util-linux 2.37+ (Ubuntu 22.04/24.04).
const lsblkNew = `{
   "blockdevices": [
      {"name":"loop0","path":"/dev/loop0","type":"loop","size":66560000,"fstype":"squashfs","label":null,"uuid":null,"partuuid":null,"mountpoints":["/snap/core22/1122"],"rm":false,"hotplug":false,"tran":null,"model":null,"vendor":null,"ro":true},
      {"name":"sda","path":"/dev/sda","type":"disk","size":500107862016,"fstype":null,"label":null,"uuid":null,"partuuid":null,"mountpoints":[null],"rm":false,"hotplug":false,"tran":"sata","model":"Samsung SSD 870","vendor":"ATA     ","ro":false,
         "children": [
            {"name":"sda1","path":"/dev/sda1","type":"part","size":1127219200,"fstype":"vfat","label":null,"uuid":"AAAA-BBBB","partuuid":"p-1","mountpoints":["/boot/efi"],"rm":false,"hotplug":false,"tran":null,"model":null,"vendor":null,"ro":false},
            {"name":"sda2","path":"/dev/sda2","type":"part","size":400000000000,"fstype":"ext4","label":null,"uuid":"root-uuid","partuuid":"p-2","mountpoints":["/"],"rm":false,"hotplug":false,"tran":null,"model":null,"vendor":null,"ro":false},
            {"name":"sda3","path":"/dev/sda3","type":"part","size":8000000000,"fstype":"swap","label":null,"uuid":"swap-uuid","partuuid":"p-3","mountpoints":["[SWAP]"],"rm":false,"hotplug":false,"tran":null,"model":null,"vendor":null,"ro":false},
            {"name":"sda4","path":"/dev/sda4","type":"part","size":90000000000,"fstype":"ext4","label":"data","uuid":"data-uuid","partuuid":"p-4","mountpoints":["/srv/data"],"rm":false,"hotplug":false,"tran":null,"model":null,"vendor":null,"ro":false},
            {"name":"sda5","path":"/dev/sda5","type":"part","size":1048576,"fstype":null,"label":null,"uuid":null,"partuuid":"p-5","mountpoints":[null],"rm":false,"hotplug":false,"tran":null,"model":null,"vendor":null,"ro":false}
         ]
      },
      {"name":"sdb","path":"/dev/sdb","type":"disk","size":30752636928,"fstype":null,"label":null,"uuid":null,"partuuid":null,"mountpoints":[null],"rm":true,"hotplug":true,"tran":"usb","model":"Ultra Fit","vendor":"SanDisk ","ro":false,
         "children": [
            {"name":"sdb1","path":"/dev/sdb1","type":"part","size":30751588352,"fstype":"vfat","label":"한글 USB","uuid":"1234-ABCD","partuuid":"p-b1","mountpoints":[null],"rm":true,"hotplug":true,"tran":null,"model":null,"vendor":null,"ro":false}
         ]
      },
      {"name":"sdc","path":"/dev/sdc","type":"disk","size":2000398934016,"fstype":"exfat","label":"DATA","uuid":"C0DE-0001","partuuid":null,"mountpoints":["/media/kim/DATA"],"rm":false,"hotplug":true,"tran":"usb","model":"Elements 25A2","vendor":"WD      ","ro":false},
      {"name":"sdd","path":"/dev/sdd","type":"disk","size":8000000000,"fstype":null,"label":null,"uuid":null,"partuuid":null,"mountpoints":[null],"rm":true,"hotplug":true,"tran":"usb","model":"Flash Disk","vendor":"Generic ","ro":false},
      {"name":"sr0","path":"/dev/sr0","type":"rom","size":1073741312,"fstype":null,"label":null,"uuid":null,"partuuid":null,"mountpoints":[null],"rm":true,"hotplug":true,"tran":"sata","model":"DVD-RW","vendor":"HL-DT-ST","ro":false}
   ]
}`

const fstab = `# /etc/fstab: static file system information.
UUID=root-uuid /               ext4    errors=remount-ro 0       1
UUID=AAAA-BBBB  /boot/efi       vfat    umask=0077      0       1
UUID=swap-uuid none            swap    sw              0       0
LABEL=data /srv/data ext4 defaults 0 2
`

const mountinfo = `30 1 8:2 / / rw,relatime shared:1 - ext4 /dev/sda2 rw
35 30 8:1 / /boot/efi rw,relatime shared:20 - vfat /dev/sda1 rw,fmask=0077
90 30 7:0 / /snap/core22/1122 ro,nodev,relatime shared:50 - squashfs /dev/loop0 ro
700 30 8:32 / /media/kim/DATA ro,nosuid,nodev,relatime shared:400 - exfat /dev/sdc ro,uid=1000
620 30 0:52 / /mnt/nas\040share rw,relatime shared:330 - nfs4 10.0.0.5:/export/share rw,vers=4.2
621 30 0:53 / /mnt/win rw,relatime - cifs //fs01/공유 rw,vers=3.0
broken line
`

func byName(t *testing.T, vols []Volume, name string) Volume {
	t.Helper()
	for _, v := range vols {
		if v.Name == name {
			return v
		}
	}
	t.Fatalf("no volume %q in %v", name, names(vols))
	return Volume{}
}

func names(vols []Volume) []string {
	var out []string
	for _, v := range vols {
		out = append(out, v.Name)
	}
	return out
}

func TestBuild(t *testing.T) {
	vols, err := Build([]byte(lsblkNew), fstab, mountinfo)
	if err != nil {
		t.Fatal(err)
	}
	// Fixed, Removable, Network, Virtual; lsblk's order inside each. The
	// partitioned disks, the unused sda5 and the empty sr0 are not volumes.
	want := []string{"sda1", "sda2", "sda4", "sdb1", "sdc", "sdd", "10.0.0.5:/export/share", "//fs01/공유", "loop0", "sda3"}
	if got := names(vols); !reflect.DeepEqual(got, want) {
		t.Fatalf("volumes:\n got %q\nwant %q", got, want)
	}

	root := byName(t, vols, "sda2")
	if root.Kind != Fixed || !root.Protected || root.CanUnmount() || root.Ejectable || root.Mountable() {
		t.Errorf("root must be protected: %+v", root)
	}
	if root.Model != "ATA Samsung SSD 870" || root.Disk != "/dev/sda" || root.Title() != "/" {
		t.Errorf("root details: %+v", root)
	}
	if efi := byName(t, vols, "sda1"); !efi.Protected {
		t.Errorf("/boot/efi must be protected: %+v", efi)
	}
	if data := byName(t, vols, "sda4"); !data.Protected || data.Title() != "data" {
		t.Errorf("fstab volume must be protected: %+v", data)
	}
	if swap := byName(t, vols, "sda3"); !swap.Swap || swap.Kind != Virtual || !swap.Protected || swap.Mounted() || swap.Mountable() {
		t.Errorf("swap: %+v", swap)
	}
	if snap := byName(t, vols, "loop0"); snap.Kind != Virtual || snap.CanUnmount() || snap.Ejectable {
		t.Errorf("snap image: %+v", snap)
	}

	usb := byName(t, vols, "sdb1")
	if usb.Kind != Removable || usb.Label != "한글 USB" || usb.Mounted() || !usb.Mountable() || usb.CanUnmount() || !usb.Ejectable {
		t.Errorf("unmounted USB stick: %+v", usb)
	}
	if usb.Model != "SanDisk Ultra Fit" || usb.DiskName != "sdb" || usb.Tran != "usb" || usb.Size != 30751588352 {
		t.Errorf("USB stick details: %+v", usb)
	}

	// hotplug without rm (USB disks), filesystem on the whole disk.
	disk := byName(t, vols, "sdc")
	if disk.Kind != Removable || !disk.CanUnmount() || !disk.Ejectable || disk.Mountable() || disk.Disk != "/dev/sdc" {
		t.Errorf("mounted USB disk: %+v", disk)
	}
	if !disk.ReadOnly || disk.Options != "ro,nosuid,nodev,relatime" {
		t.Errorf("mount options from mountinfo: %+v", disk)
	}

	if blank := byName(t, vols, "sdd"); blank.FSType != "" || blank.Mountable() || !blank.Ejectable || blank.Title() != "sdd" {
		t.Errorf("blank stick: %+v", blank)
	}

	nfs := byName(t, vols, "10.0.0.5:/export/share")
	if nfs.Kind != Network || nfs.MountPoint() != "/mnt/nas share" || nfs.CanUnmount() || nfs.Mountable() {
		t.Errorf("nfs: %+v", nfs)
	}

	if s := names(Siblings(vols, disk)); !reflect.DeepEqual(s, []string{"sdc"}) {
		t.Errorf("siblings of sdc: %q", s)
	}
	if s := Siblings(vols, usb); len(s) != 0 {
		t.Errorf("nothing mounted on sdb: %q", names(s))
	}
}

// A removable disk the system boots from or lists in fstab is not offered
// for Eject, and neither are the other partitions on it.
func TestProtectedRemovableDisk(t *testing.T) {
	const js = `{"blockdevices":[{"name":"sdb","path":"/dev/sdb","type":"disk","rm":true,"hotplug":true,"tran":"usb","children":[
		{"name":"sdb1","path":"/dev/sdb1","type":"part","fstype":"ext4","uuid":"u1","mountpoints":["/backup"]},
		{"name":"sdb2","path":"/dev/sdb2","type":"part","fstype":"vfat","uuid":"u2","mountpoints":["/media/kim/EXTRA"]}]}]}`
	vols, err := Build([]byte(js), "/dev/disk/by-uuid/u1 /backup ext4 defaults 0 2\n", "")
	if err != nil {
		t.Fatal(err)
	}
	a, b := byName(t, vols, "sdb1"), byName(t, vols, "sdb2")
	if a.Kind != Fixed || !a.Protected || a.CanUnmount() || a.Ejectable {
		t.Errorf("fstab partition: %+v", a)
	}
	if b.Kind != Removable || !b.CanUnmount() || b.Ejectable {
		t.Errorf("other partition may be unmounted but the disk not ejected: %+v", b)
	}
}

// util-linux 2.34 (Ubuntu 20.04): "mountpoint" instead of "mountpoints";
// even older versions print every value as a string.
func TestParseLsblkOld(t *testing.T) {
	const js = `lsblk: warning before the JSON
{"blockdevices": [
  {"name":"sdb","type":"disk","size":"16008609792","fstype":null,"mountpoint":null,"rm":"1","hotplug":"1","tran":"usb","model":"DataTraveler","vendor":"Kingston","ro":"0",
   "children":[{"name":"sdb1","type":"part","size":"16007561216","fstype":"vfat","label":"KINGSTON","uuid":"9C33-6BBD","mountpoint":"/mnt/usb","rm":"1","ro":"0"}]}
]}`
	vols, err := ParseLsblk([]byte(js))
	if err != nil {
		t.Fatal(err)
	}
	if len(vols) != 1 {
		t.Fatalf("volumes: %q", names(vols))
	}
	v := vols[0]
	if v.Dev != "/dev/sdb1" || v.Disk != "/dev/sdb" || v.Size != 16007561216 || v.Kind != Removable || v.MountPoint() != "/mnt/usb" || v.ReadOnly {
		t.Errorf("%+v", v)
	}
	if _, err := ParseLsblk([]byte("lsblk: unknown column")); err == nil {
		t.Error("garbage must be an error")
	}
}

// A root filesystem that is also bind-mounted elsewhere (WSL, containers)
// is still "/".
func TestMountPointPrefersShortest(t *testing.T) {
	const js = `{"blockdevices":[{"name":"sdd","path":"/dev/sdd","type":"disk","fstype":"ext4","mountpoints":["/mnt/wslg/distro","/"]}]}`
	vols, err := Build([]byte(js), "", "30 1 8:48 / / rw,relatime - ext4 /dev/sdd rw\n")
	if err != nil {
		t.Fatal(err)
	}
	if v := vols[0]; v.MountPoint() != "/" || v.Title() != "/" || !v.Protected || v.Options != "rw,relatime" {
		t.Errorf("%+v", v)
	}
}

func TestParseMountInfoAndFstab(t *testing.T) {
	ms := ParseMountInfo(mountinfo)
	if len(ms) != 6 {
		t.Fatalf("mounts: %+v", ms)
	}
	if m := ms[4]; m.Dir != "/mnt/nas share" || m.FSType != "nfs4" || m.Source != "10.0.0.5:/export/share" || m.ReadOnly() {
		t.Errorf("nfs line: %+v", m)
	}
	if !ms[3].ReadOnly() {
		t.Errorf("ro option: %+v", ms[3])
	}
	fs := ParseFstab(fstab + "/dev/sdx1 /mnt/my\\040disk ext4 defaults 0 0\nshort\n")
	want := []FstabEntry{
		{"UUID=root-uuid", "/"}, {"UUID=AAAA-BBBB", "/boot/efi"}, {"UUID=swap-uuid", ""},
		{"LABEL=data", "/srv/data"}, {"/dev/sdx1", "/mnt/my disk"},
	}
	if !reflect.DeepEqual(fs, want) {
		t.Errorf("fstab:\n got %+v\nwant %+v", fs, want)
	}
}

func TestMountDirName(t *testing.T) {
	cases := []struct {
		v    Volume
		want string
	}{
		{Volume{Label: "한글 USB", UUID: "1234-ABCD", Name: "sdb1"}, "한글 USB"},
		{Volume{Label: "../../etc", UUID: "u", Name: "sdb1"}, "etc"},
		{Volume{Label: " .. ", UUID: "1234-ABCD", Name: "sdb1"}, "1234-ABCD"},
		{Volume{Label: "a/b\x00c\nd", Name: "sdb1"}, "abcd"},
		{Volume{Label: "bad\xffutf8", Name: "sdb1"}, "badutf8"},
		{Volume{Name: "sdb1"}, "sdb1"},
		{Volume{}, "volume"},
	}
	for _, c := range cases {
		if got := MountDirName(c.v); got != c.want {
			t.Errorf("MountDirName(%q) = %q, want %q", c.v.Label, got, c.want)
		}
	}
	// Long Hangul labels are cut on a character boundary.
	long := MountDirName(Volume{Label: strings.Repeat("가", 60)})
	if long != strings.Repeat("가", 33) {
		t.Errorf("long label: %d bytes %q", len(long), long)
	}
	if got := MediaBase("kim"); got != "/media/kim" {
		t.Errorf("MediaBase: %q", got)
	}
}

func TestUniqueDir(t *testing.T) {
	taken := map[string]bool{"/media/kim/USB": true, "/media/kim/USB (2)": true}
	exists := func(p string) bool { return taken[p] }
	if got := UniqueDir("/media/kim", "USB", exists); got != "/media/kim/USB (3)" {
		t.Errorf("got %q", got)
	}
	if got := UniqueDir("/media/kim", "한글 USB", exists); got != "/media/kim/한글 USB" {
		t.Errorf("got %q", got)
	}
}

func TestMountOptions(t *testing.T) {
	cases := []struct {
		fs   string
		ro   bool
		want string
	}{
		{"vfat", false, "nosuid,nodev,uid=1000,gid=1001,utf8"},
		{"exfat", false, "nosuid,nodev,uid=1000,gid=1001"},
		{"ntfs", true, "ro,nosuid,nodev,uid=1000,gid=1001"},
		{"ext4", false, "nosuid,nodev"},
		{"xfs", true, "ro,nosuid,nodev"},
	}
	for _, c := range cases {
		if got := MountOptions(c.fs, 1000, 1001, c.ro); got != c.want {
			t.Errorf("MountOptions(%s, ro=%v) = %q, want %q", c.fs, c.ro, got, c.want)
		}
	}
	if HasOwners("vfat") || !HasOwners("ext4") {
		t.Error("HasOwners")
	}
}

func TestCommands(t *testing.T) {
	m := MountCommand("/dev/sdb1", "/media/kim/한글 USB", "nosuid,nodev")
	if len(m) != 7 || m[0] != "sh" || m[1] != "-c" || !reflect.DeepEqual(m[3:], []string{"sh", "/media/kim/한글 USB", "nosuid,nodev", "/dev/sdb1"}) {
		t.Errorf("mount: %q", m)
	}
	u := UnmountCommand("/media/kim/한글 USB", "/media/kim")
	if !reflect.DeepEqual(u[3:], []string{"sh", "/media/kim/한글 USB", "/media/kim"}) {
		t.Errorf("unmount: %q", u)
	}
	// Never force or lazy unmount.
	if strings.Contains(unmountScript, "-f") || strings.Contains(unmountScript, "-l") {
		t.Error("unmount script must not force")
	}
	if p, ok := PowerOffCommand("sdb"); !ok || p[len(p)-1] != "sdb" {
		t.Errorf("power off: %q %v", p, ok)
	}
	for _, bad := range []string{"", "../sda", "sdb/..", "sd b", "sdb;reboot"} {
		if _, ok := PowerOffCommand(bad); ok {
			t.Errorf("PowerOffCommand(%q) must be refused", bad)
		}
	}
}

func TestUsageBar(t *testing.T) {
	cases := []struct {
		w    int
		used float64
		want string
	}{
		{12, 0, "[----------]"},
		{12, 0.001, "[#---------]"},
		{12, 0.61, "[######----]"},
		{12, 0.999, "[#########-]"},
		{12, 1, "[##########]"},
		{12, 1.5, "[##########]"},
		{12, -1, "[----------]"},
		{2, 0.5, ""},
	}
	for _, c := range cases {
		if got := UsageBar(c.w, c.used); got != c.want {
			t.Errorf("UsageBar(%d, %v) = %q, want %q", c.w, c.used, got, c.want)
		}
	}
}

// lsblk on Ubuntu 20.04 rejects MOUNTPOINTS: List asks again with
// MOUNTPOINT.
func TestListFallsBackToOldColumn(t *testing.T) {
	var calls []string
	src := Source{
		Run: func(_ context.Context, name string, args []string, _ []byte) ([]byte, error) {
			cols := args[len(args)-1]
			calls = append(calls, name+" "+cols)
			if strings.Contains(cols, "MOUNTPOINTS") {
				return []byte("lsblk: unknown column: MOUNTPOINTS"), errors.New("exit status 1")
			}
			return []byte(`{"blockdevices":[{"name":"sda","type":"disk","fstype":"ext4","mountpoint":"/"}]}`), nil
		},
		ReadFile: func(path string) ([]byte, error) {
			if path == "/etc/fstab" {
				return []byte(fstab), nil
			}
			return nil, errors.New("no such file")
		},
	}
	vols, err := src.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || !strings.HasSuffix(calls[1], ",MOUNTPOINT,RM,HOTPLUG,TRAN,MODEL,VENDOR,RO") {
		t.Errorf("calls: %q", calls)
	}
	if len(vols) != 1 || !vols[0].Protected {
		t.Errorf("volumes: %+v", vols)
	}

	src.Run = func(context.Context, string, []string, []byte) ([]byte, error) {
		return []byte("sh: lsblk: not found\nmore"), errors.New("exit status 127")
	}
	if _, err := src.List(context.Background()); err == nil || !strings.Contains(err.Error(), "lsblk: not found") || strings.Contains(err.Error(), "more") {
		t.Errorf("error: %v", err)
	}
}

func TestDiffDisks(t *testing.T) {
	vols, err := Build([]byte(lsblkNew), fstab, mountinfo)
	if err != nil {
		t.Fatal(err)
	}
	cur := RemovableDisks(vols)
	want := map[string]string{"/dev/sdb": "SanDisk Ultra Fit (한글 USB)", "/dev/sdc": "WD Elements 25A2 (DATA)", "/dev/sdd": "Generic Flash Disk"}
	if !reflect.DeepEqual(cur, want) {
		t.Fatalf("removable disks %q", cur)
	}
	old := map[string]string{"/dev/sdc": "WD Elements 25A2 (DATA)", "/dev/sdx": "Old Stick"}
	added, removed := DiffDisks(old, cur)
	if !reflect.DeepEqual(added, []string{"Generic Flash Disk", "SanDisk Ultra Fit (한글 USB)"}) || !reflect.DeepEqual(removed, []string{"Old Stick"}) {
		t.Errorf("added %q removed %q", added, removed)
	}
	if a, r := DiffDisks(cur, cur); len(a)+len(r) != 0 {
		t.Errorf("no change: %q %q", a, r)
	}
}
