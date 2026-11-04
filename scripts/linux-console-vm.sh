#!/usr/bin/env bash
# Linux console (TERM=linux) test VM: Alpine Linux x86_64 under QEMU.
#
# The kernel VT reproduces what a server's physical / IPMI console shows:
# no Hangul glyphs, 8 colors, VT-switching Alt+arrows, lost Shift/Ctrl on arrows.
#
#   scripts/linux-console-vm.sh start [--window]  boot the VM (--window: show a QEMU window)
#   scripts/linux-console-vm.sh setup             log in, network, copy dist/{evlog,wmdemo}-linux-amd64
#   scripts/linux-console-vm.sh run [PROG [ARGS]] run PROG (evlog|wmdemo, default evlog) on VT7
#   scripts/linux-console-vm.sh shot FILE.png     screenshot of the console
#   scripts/linux-console-vm.sh stop              power off
#
# Keys can be sent with: python3 -I scripts/vmctl.py keys ctrl-c f2 shift-up ...
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VM_DIR="$ROOT/.cache/vm"
ALPINE_VER="3.24.3"
ISO="alpine-virt-${ALPINE_VER}-x86_64.iso"
ISO_SHA256="cdc1bc0efc64d8d611bf3733f54868172a545888dfd726f518bf279bcb0ff1af"
ISO_URL="https://dl-cdn.alpinelinux.org/alpine/v${ALPINE_VER%.*}/releases/x86_64/${ISO}"
HTTP_PORT=8099
VMCTL=(python3 -I "$ROOT/scripts/vmctl.py")
export VM_DIR

die() { echo "error: $*" >&2; exit 1; }

cmd_start() {
	command -v qemu-system-x86_64 >/dev/null || die "qemu not installed (brew install qemu)"
	mkdir -p "$VM_DIR"
	if [[ -f "$VM_DIR/qemu.pid" ]] && kill -0 "$(cat "$VM_DIR/qemu.pid")" 2>/dev/null; then
		echo "VM already running (pid $(cat "$VM_DIR/qemu.pid"))"; return
	fi
	if [[ ! -f "$VM_DIR/$ISO" ]]; then
		echo "downloading $ISO_URL"
		curl -fsSLo "$VM_DIR/$ISO.part" "$ISO_URL"
		mv "$VM_DIR/$ISO.part" "$VM_DIR/$ISO"
	fi
	echo "$ISO_SHA256  $VM_DIR/$ISO" | shasum -a 256 -c - >/dev/null || die "ISO checksum mismatch"

	local display=(-display none)
	[[ "${1:-}" == "--window" ]] && display=(-display cocoa)
	rm -f "$VM_DIR/mon.sock" "$VM_DIR/serial.sock"
	qemu-system-x86_64 -m 512 -smp 2 -cdrom "$VM_DIR/$ISO" -boot d -vga std "${display[@]}" \
		-monitor "unix:$VM_DIR/mon.sock,server,nowait" \
		-serial "unix:$VM_DIR/serial.sock,server,nowait" \
		-nic user,model=virtio-net-pci \
		-daemonize -pidfile "$VM_DIR/qemu.pid"
	echo "VM started (pid $(cat "$VM_DIR/qemu.pid")). Next: $0 setup"
}

cmd_setup() {
	(cd "$ROOT" && make evlog-linux wmdemo-linux >/dev/null)
	"${VMCTL[@]}" login
	python3 -I -m http.server "$HTTP_PORT" --bind 127.0.0.1 --directory "$ROOT/dist" >/dev/null 2>&1 &
	local http_pid=$!
	trap 'kill $http_pid 2>/dev/null' RETURN
	sleep 1
	local prog
	for prog in evlog wmdemo; do
		# The marker is computed in the guest so the echoed command line cannot match it.
		"${VMCTL[@]}" serial "ip link set eth0 up; ip -4 addr show eth0 | grep -q inet || udhcpc -i eth0 -q -n >/dev/null 2>&1; for i in 1 2 3 4 5 6 7 8 9 10; do wget -q -O /root/$prog http://10.0.2.2:$HTTP_PORT/$prog-linux-amd64 && break; sleep 1; done; [ -s /root/$prog ] && chmod +x /root/$prog && echo COPY_\$((40+2))" \
			--expect 'COPY_42' --timeout 90 >/dev/null || die "could not copy $prog into the VM"
	done
	echo "evlog and wmdemo copied to /root. Next: $0 run [evlog|wmdemo]"
}

cmd_run() {
	local prog="${1:-evlog}"
	shift || true
	local args="$*"
	[[ "$prog" == evlog && -z "$args" ]] && args="-label linux-console -mouse=off"
	"${VMCTL[@]}" serial "openvt -c 7 -f -s -- env -i TERM=linux HOME=/root sh -c 'cd /root && ./$prog $args'; echo RUN_\$((40+2))" \
		--expect 'RUN_42' --timeout 30 >/dev/null || die "could not start $prog"
	echo "$prog running on VT7"
}

cmd_shot() {
	[[ -n "${1:-}" ]] || die "usage: $0 shot FILE.png"
	"${VMCTL[@]}" shot "$1"
	echo "saved $1"
}

cmd_stop() {
	if [[ -f "$VM_DIR/qemu.pid" ]]; then
		kill "$(cat "$VM_DIR/qemu.pid")" 2>/dev/null || true
		rm -f "$VM_DIR/qemu.pid"
	fi
	echo "VM stopped"
}

case "${1:-}" in
start) shift; cmd_start "$@" ;;
setup) cmd_setup ;;
run) shift; cmd_run "$@" ;;
shot) shift; cmd_shot "$@" ;;
stop) cmd_stop ;;
*) sed -n '2,13p' "$0"; exit 2 ;;
esac
