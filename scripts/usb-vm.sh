#!/usr/bin/env bash
# USB test VM: the Ubuntu test VM of scripts/ubuntu-vm.sh, but under QEMU, so
# a disk image can be plugged in and pulled out as a USB memory stick
# (SPEC 7.8 My PC, scenario S4). The vz VMs cannot do that.
#
#   scripts/usb-vm.sh up [VER]          create/start the VM (default 24.04), deploy tuidesk
#   scripts/usb-vm.sh deploy [VER]      rebuild and copy tuidesk
#   scripts/usb-vm.sh ssh [VER] [U]     ssh in as U (default tester, password "tuidesk")
#   scripts/usb-vm.sh plug [FS] [VER]   plug in the USB stick; FS: vfat (default), exfat
#   scripts/usb-vm.sh unplug [VER]      pull it out
#   scripts/usb-vm.sh status [VER]      plugged or unplugged
#   scripts/usb-vm.sh stop|delete [VER]
#
# The stick is a 64 MB image in .cache/usb/ with one partition (label TUIDESK),
# made by hdiutil the first time and kept afterwards, so files copied to it are
# still there after unplug + plug. Delete the image to get an empty stick.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IMG_DIR="$ROOT/.cache/usb"
DEFAULT_VER="24.04"
export TUIDESK_VM_KIND=usb

die() { echo "error: $*" >&2; exit 1; }
is_ver() { [[ "${1:-}" =~ ^[0-9]{2}\.[0-9]{2}$ ]]; }
sock() {
	local s="$HOME/.lima/tuidesk-usb-${1//./}/qmp.sock"
	[[ -S "$s" ]] || die "the USB VM is not running (run: $0 up $1)"
	echo "$s"
}
qmp() { python3 -I "$ROOT/scripts/qmp.py" "$@"; }

image() { # image FS -> path, created on first use
	local fs="$1" img="$IMG_DIR/stick-$1.img" hfs
	case "$fs" in
	vfat) hfs="MS-DOS FAT32" ;;
	exfat) hfs="ExFAT" ;;
	*) die "unknown filesystem '$fs' (vfat or exfat)" ;;
	esac
	if [[ ! -f "$img" ]]; then
		mkdir -p "$IMG_DIR"
		hdiutil create -quiet -size 64m -fs "$hfs" -volname TUIDESK -layout MBRSPUD "$IMG_DIR/stick-$fs" >&2
		mv "$IMG_DIR/stick-$fs.dmg" "$img" # a read/write .dmg is a raw disk image
	fi
	echo "$img"
}

cmd="${1:-}"
shift || true
case "$cmd" in
up | deploy | stop | delete)
	exec "$ROOT/scripts/ubuntu-vm.sh" "$cmd" "${1:-$DEFAULT_VER}"
	;;
ssh)
	ver="$DEFAULT_VER"
	if is_ver "${1:-}"; then ver="$1"; shift; fi
	exec "$ROOT/scripts/ubuntu-vm.sh" ssh "$ver" "$@"
	;;
plug)
	fs="vfat"
	if [[ -n "${1:-}" ]] && ! is_ver "$1"; then fs="$1"; shift; fi
	ver="${1:-$DEFAULT_VER}"
	s="$(sock "$ver")"
	qmp "$s" plug "$(image "$fs")"
	;;
unplug | status)
	qmp "$(sock "${1:-$DEFAULT_VER}")" "$cmd"
	;;
*)
	sed -n '2,17p' "$0"
	exit 2
	;;
esac
