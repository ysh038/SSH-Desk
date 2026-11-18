package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"tuidesk/internal/apps/files"
	"tuidesk/internal/apps/mypc"
	"tuidesk/internal/config"
	"tuidesk/internal/devices"
	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// fakePC is a machine with a system disk and one USB stick ("한글 USB",
// vfat). sudo mount/umount act on a temporary media folder, never on real
// devices (CLAUDE.md rule 6).
type fakePC struct {
	mu      sync.Mutex
	plugged bool
	mounted map[string]string // device → folder
	opts    string            // options of the last mount
	calls   []string
	stdins  []string
	ejected []string
	busy    bool // umount fails with "target is busy"
	// stale is a folder still mounted from /dev/sdz1, a drive that was
	// pulled out without Eject ("" = none).
	stale    string
	nopasswd bool
}

func (p *fakePC) lsblk(context.Context, string, []string, []byte) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	js := `{"blockdevices":[
 {"name":"sda","path":"/dev/sda","type":"disk","size":500000000000,"rm":false,"hotplug":false,"tran":"sata","model":"SSD","vendor":"ATA","children":[
   {"name":"sda2","path":"/dev/sda2","type":"part","size":400000000000,"fstype":"ext4","uuid":"root-uuid","mountpoints":["/"]}]}`
	if p.plugged {
		mp := "null"
		if d, ok := p.mounted["/dev/sdb1"]; ok {
			mp = strconv.Quote(d)
		}
		js += `,
 {"name":"sdb","path":"/dev/sdb","type":"disk","size":30752636928,"rm":true,"hotplug":true,"tran":"usb","model":"Ultra Fit","vendor":"SanDisk","children":[
   {"name":"sdb1","path":"/dev/sdb1","type":"part","size":30751588352,"fstype":"vfat","label":"한글 USB","uuid":"1234-ABCD","mountpoints":[` + mp + `]}]}`
	}
	return []byte(js + "]}"), nil
}

func (p *fakePC) readFile(path string) ([]byte, error) {
	if path == "/etc/fstab" {
		return []byte("UUID=root-uuid / ext4 defaults 0 1\n"), nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if path == "/proc/self/mountinfo" && p.stale != "" {
		return []byte("40 25 8:200 / " + p.stale + " rw,nosuid - vfat /dev/sdz1 rw\n"), nil
	}
	return nil, errors.New("no such file")
}

func (p *fakePC) usage(dir string) (int64, int64, error) {
	if dir == "/" {
		return 400e9, 20e9, nil // 95% used
	}
	return 30e9, 12e9, nil
}

func (p *fakePC) sudo(_ context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, name+" "+strings.Join(args, " "))
	p.stdins = append(p.stdins, string(stdin))
	if name != "sudo" {
		return nil, errors.New("unexpected program " + name)
	}
	if args[0] == "-n" && !p.nopasswd {
		return []byte("sudo: a password is required\n"), &exitErr{}
	}
	c := -1
	for i, a := range args {
		if a == "-c" {
			c = i
			break
		}
	}
	if c < 0 || c+1 >= len(args) {
		return nil, errors.New("no script")
	}
	script := args[c+1]
	var rest []string
	if c+3 <= len(args) {
		rest = args[c+3:]
	}
	switch {
	case strings.Contains(script, "umount"):
		dir := rest[0]
		if p.busy {
			return []byte("umount: " + dir + ": target is busy.\n"), &exitErr{}
		}
		for dev, d := range p.mounted {
			if d == dir {
				delete(p.mounted, dev)
			}
		}
		if dir == p.stale {
			if !strings.Contains(script, "umount -l") {
				return []byte("umount: " + dir + ": target is busy.\n"), &exitErr{}
			}
			p.stale = ""
		}
		os.Remove(filepath.Join(dir, "사진.txt"))
		if strings.HasPrefix(dir, rest[1]+"/") { // like the script: only our own folders
			os.Remove(dir)
		}
	case strings.Contains(script, "mount -o"):
		dir, opts, dev := rest[0], rest[1], rest[2]
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return []byte(err.Error()), err
		}
		os.WriteFile(filepath.Join(dir, "사진.txt"), []byte("x"), 0o644)
		p.mounted[dev], p.opts = dir, opts
	case strings.Contains(script, "device/delete"):
		p.ejected = append(p.ejected, rest[0])
	}
	return nil, nil
}

func (p *fakePC) sudoCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

type pcHarness struct {
	*fmHarness
	pc    *fakePC
	media string
}

func newPC(t *testing.T) *pcHarness {
	f := newFM(t)
	pc := &fakePC{plugged: true, mounted: map[string]string{}}
	h := &pcHarness{fmHarness: f, pc: pc, media: filepath.Join(f.root, "media", "kim")}
	if err := os.MkdirAll(h.media, 0o755); err != nil {
		t.Fatal(err)
	}
	f.d.NewMyPC = func() wm.App {
		return mypc.New(f.d, mypc.Deps{
			Source: devices.Source{Run: pc.lsblk, ReadFile: pc.readFile},
			Run:    pc.sudo, Usage: pc.usage,
			Exists:    func(p string) bool { _, err := os.Lstat(p); return err == nil },
			MediaBase: h.media, User: "kim", UID: 1000, GID: 1000, Home: f.root,
		})
	}
	return h
}

func (h *pcHarness) window() *wm.Window {
	h.t.Helper()
	for _, w := range h.d.M.Windows() {
		if _, ok := w.App.(*mypc.App); ok {
			return w
		}
	}
	h.t.Fatal("no My PC window")
	return nil
}

func (h *pcHarness) filesWindow() (*files.App, *wm.Window) {
	h.t.Helper()
	for _, w := range h.d.M.Windows() {
		if a, ok := w.App.(*files.App); ok {
			return a, w
		}
	}
	h.t.Fatal("no file window")
	return nil, nil
}

// rowY finds the screen row that shows text.
func (h *pcHarness) rowY(text string) int {
	h.t.Helper()
	c := h.draw()
	for y := 0; y < c.H; y++ {
		if strings.Contains(c.Row(y), text) {
			return y
		}
	}
	h.t.Fatalf("screen does not show %q", text)
	return 0
}

func (h *pcHarness) password(pw string) {
	h.t.Helper()
	h.settle()
	if !h.d.DialogOpen() {
		h.t.Fatal("no password prompt")
	}
	h.typeText(pw)
	h.key("Enter", 0)
	h.settle()
}

// SPEC scenario S4 with the mouse: open My PC from the launcher, double-click
// the USB stick (mounts it and opens it in a file window), then right-click
// it and Eject. The only typing is the sudo password.
func TestScenarioS4MouseMountAndEject(t *testing.T) {
	h := newPC(t)
	h.draw()
	h.click(2, 0) // ≡ Menu
	h.clickMenuItem("My PC")
	h.settle()
	w := h.window()
	w.Rect = wm.Rect{X: 1, Y: 1, W: 98, H: 14}
	h.settle()

	for _, want := range []string{"Places", "Drives", "Removable devices", "/dev/sda2", "almost full", "한글 USB", "/dev/sdb1", "vfat", "Not mounted"} {
		if !h.screenHas(want) {
			t.Fatalf("My PC does not show %q", want)
		}
	}

	// Double-click the stick: password, mount, file window.
	y := h.rowY("/dev/sdb1")
	h.click(w.Inner().X+5, y)
	h.click(w.Inner().X+5, y)
	h.password("비밀pw")
	dir := filepath.Join(h.media, "한글 USB")
	if h.pc.mounted["/dev/sdb1"] != dir {
		t.Fatalf("mounted %v, calls %q", h.pc.mounted, h.pc.calls)
	}
	if h.pc.opts != "nosuid,nodev,uid=1000,gid=1000,utf8" {
		t.Errorf("mount options %q", h.pc.opts)
	}
	last := len(h.pc.stdins) - 1
	if h.pc.stdins[last] != "비밀pw\n" || !strings.HasPrefix(h.pc.calls[last], "sudo -S -k -p  sh -c ") {
		t.Errorf("sudo call %q stdin %q", h.pc.calls[last], h.pc.stdins[last])
	}
	for _, c := range h.pc.calls {
		if strings.Contains(c, "비밀pw") {
			t.Fatal("the password must never be an argument")
		}
	}
	fa, fw := h.filesWindow()
	if fa.Dir() != dir || h.d.M.Focused() != fw {
		t.Fatalf("file window at %q", fa.Dir())
	}
	fw.Rect = wm.Rect{X: 1, Y: 16, W: 98, H: 12}
	h.settle()
	if !h.screenHas("사진.txt") || !h.screenHas("12.0G free of 30.0G") && !h.screenHas("free of") {
		t.Fatal("the stick's files or its usage are not shown")
	}

	// Right-click the stick in My PC → Eject.
	y = h.rowY("/dev/sdb1")
	h.rclick(w.Inner().X+5, y)
	h.clickMenuItem("Eject (safe to remove)")
	h.password("비밀pw")
	if len(h.pc.mounted) != 0 || len(h.pc.ejected) != 1 || h.pc.ejected[0] != "sdb" {
		t.Fatalf("after eject: mounted %v ejected %v", h.pc.mounted, h.pc.ejected)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("the mount folder was left behind")
	}
	if fa.Dir() != h.media {
		t.Errorf("the file window stayed in %q", fa.Dir())
	}
	if !h.screenHas("Safe to remove SanDisk Ultra Fit") || !h.screenHas("Not mounted") {
		t.Error("no confirmation that the stick can be removed")
	}
}

// The same with the keyboard only (Linux console): F9, m, End, Enter to
// mount and open; back in My PC, Ctrl+U unmounts.
func TestMyPCKeyboardMountAndUnmount(t *testing.T) {
	h := newPC(t)
	h.pc.nopasswd = true // NOPASSWD sudoers: no prompt at all
	h.key("F9", 0)
	h.rune('m', 0)
	h.settle()
	w := h.window()
	h.key("End", 0) // the stick is the last row
	h.key("Enter", 0)
	h.settle()
	dir := filepath.Join(h.media, "한글 USB")
	if h.pc.mounted["/dev/sdb1"] != dir || h.d.DialogOpen() {
		t.Fatalf("mounted %v dialog %v", h.pc.mounted, h.d.DialogOpen())
	}
	if last := h.pc.calls[len(h.pc.calls)-1]; !strings.HasPrefix(last, "sudo -n -k sh -c ") {
		t.Errorf("sudo call %q", last)
	}
	for i := 0; i < 4 && h.d.M.Focused() != w; i++ {
		h.key("F6", 0)
	}
	if h.d.M.Focused() != w {
		t.Fatal("F6 does not reach My PC")
	}
	h.rune('u', term.ModCtrl)
	h.settle()
	if len(h.pc.mounted) != 0 || len(h.pc.ejected) != 0 {
		t.Fatalf("after unmount: mounted %v ejected %v", h.pc.mounted, h.pc.ejected)
	}
	if !h.screenHas("Unmounted 한글 USB") {
		t.Error("no toast")
	}
	// Opening My PC again raises the same window.
	h.key("F9", 0)
	h.rune('m', 0)
	h.settle()
	n := 0
	for _, x := range h.d.M.Windows() {
		if _, ok := x.App.(*mypc.App); ok {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d My PC windows", n)
	}
}

type editorStub struct{ wm.App }

func (editorStub) UsesDir(dir string) string { return "1 file open in the editor" }

// Safety: system volumes are never unmounted, a busy stick stays mounted
// with an explanation, and our own windows that cannot leave block it.
func TestMyPCSafety(t *testing.T) {
	h := newPC(t)
	h.pc.nopasswd = true
	dir := filepath.Join(h.media, "한글 USB")
	os.MkdirAll(dir, 0o755)
	h.pc.mounted["/dev/sdb1"] = dir
	h.key("F9", 0)
	h.rune('m', 0)
	h.settle()
	w := h.window()
	w.Rect = wm.Rect{X: 1, Y: 1, W: 98, H: 14}

	// "/" (the row above the stick): Unmount and Eject do nothing.
	h.key("End", 0)
	h.key("Up", 0)
	h.rune('u', term.ModCtrl)
	h.rune('e', term.ModCtrl)
	h.settle()
	if n := h.pc.sudoCalls(); n != 0 || h.d.DialogOpen() {
		t.Fatalf("system volume: %d sudo calls %q", n, h.pc.calls)
	}

	// Busy: error popup, still mounted, nothing powered off.
	h.pc.busy = true
	h.key("End", 0)
	h.rune('e', term.ModCtrl)
	h.settle()
	if !h.d.DialogOpen() || !h.screenHas("is in use") || h.pc.mounted["/dev/sdb1"] != dir || len(h.pc.ejected) != 0 {
		t.Fatalf("busy eject: dialog %v mounted %v ejected %v", h.d.DialogOpen(), h.pc.mounted, h.pc.ejected)
	}
	h.key("Enter", 0) // OK
	h.pc.busy = false

	// A window that holds files on the stick blocks before sudo runs.
	before := h.pc.sudoCalls()
	stub := h.d.OpenWindow(editorStub{h.d.NewApp(9)})
	h.d.M.Raise(w)
	h.rune('u', term.ModCtrl)
	h.settle()
	if !h.d.DialogOpen() || !h.screenHas("1 file open in the editor") || h.pc.sudoCalls() != before {
		t.Fatalf("blocked unmount: dialog %v calls %q", h.d.DialogOpen(), h.pc.calls[before:])
	}
	h.key("Enter", 0)
	h.d.M.Remove(stub)

	// Unplugged: the row disappears after F5 and the hint is shown.
	h.pc.plugged = false
	h.d.M.Raise(w)
	h.key("F5", 0)
	h.settle()
	if h.screenHas("/dev/sdb1") || !h.screenHas("Plug in a USB drive") {
		t.Error("the unplugged stick is still listed")
	}
}

func (p *fakePC) plug(on bool) {
	p.mu.Lock()
	p.plugged = on
	p.mu.Unlock()
}

// Hot-plug: the monitor announces a stick that appears or disappears and
// the open My PC window follows without F5. A burst of kernel events
// causes one scan.
func TestHotPlugNotifiesAndRefreshes(t *testing.T) {
	h := newPC(t)
	h.pc.plug(false)
	scans := 0
	src := devices.Source{ReadFile: h.pc.readFile, Run: func(ctx context.Context, n string, a []string, in []byte) ([]byte, error) {
		scans++
		return h.pc.lsblk(ctx, n, a, in)
	}}
	mon := mypc.NewMonitor(h.d, src)
	mon.Check() // what is connected at startup is not news
	h.settle()
	h.key("F9", 0)
	h.rune('m', 0)
	h.settle()
	h.window().Rect = wm.Rect{X: 1, Y: 1, W: 98, H: 14}
	if h.screenHas("connected") || h.screenHas("/dev/sdb1") {
		t.Fatal("nothing is plugged in yet")
	}

	h.pc.plug(true)
	mon.Check()
	h.settle()
	if !h.screenHas("USB drive connected: SanDisk Ultra Fit (한글 USB)") || !h.screenHas("/dev/sdb1") {
		t.Fatal("plugging in is not announced or not listed")
	}

	h.pc.plug(false)
	mon.Check()
	h.settle()
	if !h.screenHas("Drive disconnected: SanDisk Ultra Fit (한글 USB)") || h.screenHas("/dev/sdb1") {
		t.Fatal("unplugging is not announced or still listed")
	}

	// Three events in a row → one scan after the delay.
	mon.Delay = 20 * time.Millisecond
	before := scans
	mon.Trigger()
	mon.Trigger()
	mon.Trigger()
	time.Sleep(150 * time.Millisecond)
	h.settle()
	if scans != before+1 {
		t.Errorf("%d scans for one burst", scans-before)
	}
}

// File manager: File > Eject this drive ejects the stick the folder is on
// and Go > My PC opens the window.
func TestFilesEjectThisDrive(t *testing.T) {
	h := newPC(t)
	h.pc.nopasswd = true
	dir := filepath.Join(h.media, "한글 USB")
	os.MkdirAll(filepath.Join(dir, "photos"), 0o755)
	h.pc.mounted["/dev/sdb1"] = dir
	fa, _ := h.open(filepath.Join(dir, "photos"), wm.Rect{X: 1, Y: 14, W: 98, H: 14})

	h.rune('k', term.ModCtrl)
	h.typeText("eject")
	h.key("Enter", 0)
	h.settle()
	h.settle()
	if len(h.pc.mounted) != 0 || len(h.pc.ejected) != 1 {
		t.Fatalf("mounted %v ejected %v calls %q", h.pc.mounted, h.pc.ejected, h.pc.calls)
	}
	if fa.Dir() != h.media {
		t.Errorf("file window stayed in %q", fa.Dir())
	}
	h.window() // My PC opened to show what happened

	// A folder on the system disk: nothing runs, a notice explains.
	before := h.pc.sudoCalls()
	fb, w2 := h.open(h.root, wm.Rect{X: 1, Y: 14, W: 98, H: 14})
	_ = fb
	h.d.M.Raise(w2)
	h.rune('k', term.ModCtrl)
	h.typeText("eject")
	h.key("Enter", 0)
	h.settle()
	if h.pc.sudoCalls() != before || !h.screenHas("Cannot eject: ") {
		t.Errorf("system folder: %d sudo calls", h.pc.sudoCalls()-before)
	}
}

// fixedDir sets Settings › Storage & USB › "Mount USB drives at" to a
// folder like the server's /mnt/offline.
func (h *pcHarness) fixedDir(create bool) string {
	h.t.Helper()
	dir := filepath.Join(h.root, "mnt", "offline")
	if create {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			h.t.Fatal(err)
		}
	}
	h.d.UpdateConfig(func(c *config.Config) { c.MountDir = dir })
	return dir
}

func (h *pcHarness) openMyPC() *wm.Window {
	h.key("F9", 0)
	h.rune('m', 0)
	h.settle()
	w := h.window()
	w.Rect = wm.Rect{X: 1, Y: 1, W: 98, H: 16}
	return w
}

// A fixed mount folder (the server's /mnt/offline): the stick is mounted
// there, and Eject empties the folder but never removes it.
func TestMountAtTheConfiguredFolder(t *testing.T) {
	h := newPC(t)
	h.pc.nopasswd = true
	fixed := h.fixedDir(true)
	h.openMyPC()
	if !h.screenHas("Removable devices  (mounted at ") {
		t.Fatal("My PC does not say where drives are mounted")
	}
	h.key("End", 0)
	h.key("Enter", 0)
	h.settle()
	if h.pc.mounted["/dev/sdb1"] != fixed {
		t.Fatalf("mounted %v, want %s", h.pc.mounted, fixed)
	}
	fa, _ := h.filesWindow()
	if fa.Dir() != fixed {
		t.Fatalf("file window shows %q", fa.Dir())
	}
	h.d.M.Raise(h.window())
	h.rune('e', term.ModCtrl)
	h.settle()
	if len(h.pc.mounted) != 0 || len(h.pc.ejected) != 1 {
		t.Fatalf("after eject: mounted %v, ejected %v", h.pc.mounted, h.pc.ejected)
	}
	if fi, err := os.Stat(fixed); err != nil || !fi.IsDir() {
		t.Fatal("Eject removed the configured mount folder")
	}
	for _, c := range h.pc.calls {
		if strings.Contains(c, "umount") && !strings.HasSuffix(c, " "+fixed+" "+h.media) {
			t.Errorf("umount must name the folder and the media base: %q", c)
		}
	}
}

// The folder is never created for the user: a typing mistake in the
// settings file must not make root create folders.
func TestConfiguredFolderMissing(t *testing.T) {
	h := newPC(t)
	h.pc.nopasswd = true
	fixed := h.fixedDir(false)
	h.openMyPC()
	h.key("End", 0)
	before := h.pc.sudoCalls()
	h.key("Enter", 0)
	h.settle()
	if !h.d.DialogOpen() || !h.screenHas("Cannot mount") || h.pc.sudoCalls() != before {
		t.Fatalf("missing folder: dialog %v, sudo calls %d -> %d", h.d.DialogOpen(), before, h.pc.sudoCalls())
	}
	if _, err := os.Stat(fixed); err == nil {
		t.Fatal("the folder was created")
	}
}

// The user's case: a stick was pulled out while mounted on /mnt/offline, so
// the folder is still held (and the next stick gets another device name).
// My PC shows the dead mount, and mounting the next stick cleans it up
// first, with one password.
func TestRemovedDriveIsCleanedUpBeforeTheNextMount(t *testing.T) {
	h := newPC(t)
	fixed := h.fixedDir(true)
	h.pc.stale = fixed
	h.openMyPC()
	if !h.screenHas("/dev/sdz1") || !h.screenHas("Removed without Eject") {
		t.Fatal("the dead mount is not shown")
	}

	y := h.rowY("/dev/sdb1")
	h.click(h.window().Inner().X+5, y)
	h.key("Enter", 0)
	h.settle()
	if !h.screenHas("Clean up and mount") {
		t.Fatal("no question about the folder that is still held")
	}
	h.key("Enter", 0) // Clean up and mount
	h.password("pw")
	if h.pc.stale != "" || h.pc.mounted["/dev/sdb1"] != fixed {
		t.Fatalf("stale %q, mounted %v, calls %q", h.pc.stale, h.pc.mounted, h.pc.calls)
	}
	asked := 0
	for _, in := range h.pc.stdins {
		if in == "pw\n" {
			asked++
		}
	}
	if asked != 2 { // one prompt, used for the two commands
		t.Errorf("password used %d times, want 2 (clean up, mount)", asked)
	}
	if fi, err := os.Stat(fixed); err != nil || !fi.IsDir() {
		t.Fatal("cleaning up removed the configured folder")
	}
}

// Without a new stick: Ctrl+U on the dead mount cleans it up, and the
// monitor says so when it appears.
func TestCleanUpByKeyAndNotification(t *testing.T) {
	h := newPC(t)
	h.pc.nopasswd = true
	h.pc.plug(false)
	src := devices.Source{ReadFile: h.pc.readFile, Run: h.pc.lsblk}
	mon := mypc.NewMonitor(h.d, src)
	mon.Check()
	h.settle()
	h.d.UpdateConfig(func(c *config.Config) { c.NotifyDrives = false })

	stale := filepath.Join(h.root, "mnt", "old")
	os.MkdirAll(stale, 0o755)
	h.pc.mu.Lock()
	h.pc.stale = stale
	h.pc.mu.Unlock()
	mon.Check()
	h.settle()
	if !h.screenHas("Drive removed without Eject") {
		t.Fatal("a dead mount must be announced even with drive notifications off")
	}
	h.openMyPC()
	h.key("End", 0)
	h.rune('u', term.ModCtrl)
	h.settle()
	if h.pc.stale != "" || !h.screenHas("Cleaned up") || h.screenHas("/dev/sdz1") {
		t.Fatalf("not cleaned up: stale %q", h.pc.stale)
	}
	if last := h.pc.calls[len(h.pc.calls)-1]; !strings.Contains(last, "umount -l") {
		t.Errorf("clean-up call %q", last)
	}

	// Plugging in is not announced while the setting is off.
	h.pc.plug(true)
	mon.Check()
	h.settle()
	if h.screenHas("USB drive connected") {
		t.Fatal("drive notifications are off")
	}
}

// Settings › Storage & USB › read-only: Enter mounts read-only, and the
// menu still offers a writable mount.
func TestMountReadOnlyByDefault(t *testing.T) {
	h := newPC(t)
	h.pc.nopasswd = true
	h.d.UpdateConfig(func(c *config.Config) { c.MountReadOnly = true })
	w := h.openMyPC()
	h.key("End", 0)
	h.key("Enter", 0)
	h.settle()
	if !strings.HasPrefix(h.pc.opts, "ro,") {
		t.Fatalf("mount options %q", h.pc.opts)
	}
	h.d.M.Raise(w)
	h.rune('u', term.ModCtrl)
	h.settle()
	h.d.M.Raise(w)
	h.key("F10", term.ModShift)
	if !slices.Contains(menuLabels(h.fmHarness), "Mount for writing") {
		t.Fatalf("menu: %v", menuLabels(h.fmHarness))
	}
}
