#!/usr/bin/env bash
# Ubuntu server test VM (Lima, vz + Rosetta): log in over real ssh as a normal
# user, the way tuidesk is used on a closed-network server.
#
#   scripts/ubuntu-vm.sh up VER       create/start the VM, add the test user, deploy tuidesk
#   scripts/ubuntu-vm.sh deploy VER   rebuild and copy dist/tuidesk-linux-* to ~tester/bin
#   scripts/ubuntu-vm.sh ssh VER [U]  ssh in as U (default tester) and run a shell
#   scripts/ubuntu-vm.sh stop VER     stop the VM
#   scripts/ubuntu-vm.sh delete VER   delete the VM
#   scripts/ubuntu-vm.sh list         list the test VMs
#
# VER is an Ubuntu release with a Lima template, e.g. 22.04 or 24.04.
#
# What the VM looks like:
#   - no host directories mounted, no containerd: files live only in the VM
#   - user "tester", password "tuidesk": sudo asks for the password (like a real server)
#   - the Lima user (your Mac user name) keeps passwordless sudo (NOPASSWD servers)
#   - locales en_US.UTF-8 (default) and ko_KR.UTF-8. The Mac's LANG is sent but
#     pam_env replaces it with /etc/default/locale, as on a stock Ubuntu server
#   - tuidesk is the amd64 binary run through Rosetta; arm64 when Rosetta is missing
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TEST_USER="tester"
TEST_PW="tuidesk" # test-only password for the throwaway VM user

die() { echo "error: $*" >&2; exit 1; }

need_ver() {
	[[ -n "${1:-}" ]] || die "missing Ubuntu version (e.g. 22.04 or 24.04)"
	[[ "$1" =~ ^[0-9]{2}\.[0-9]{2}$ ]] || die "version must look like 24.04, got '$1'"
}
vm_name() { echo "tuidesk-ubuntu-${1//./}"; }
vm_status() { limactl list --format '{{.Status}}' "$1" 2>/dev/null || true; }
vm_port() { limactl list --format '{{.SSHLocalPort}}' "$1"; }
guest_root() { limactl shell --workdir / "$1" sudo bash -s; }

cmd_up() {
	need_ver "${1:-}"
	command -v limactl >/dev/null || die "lima not installed (brew install lima)"
	local name; name="$(vm_name "$1")"
	case "$(vm_status "$name")" in
	Running) echo "$name already running" ;;
	Stopped) limactl start -y "$name" ;;
	"")
		echo "creating $name from template:ubuntu-$1"
		# --mount-none, not --plain: --plain also drops the Rosetta mount.
		# Lima 2.2.0 can spin forever converting a freshly downloaded image when
		# stdout is not a terminal, so run it under a pseudo-terminal then.
		local tty=()
		[[ -t 1 ]] || tty=(script -q /dev/null)
		"${tty[@]+"${tty[@]}"}" limactl start -y --name "$name" --vm-type vz --rosetta --mount-none \
			--set '.containerd.system=false | .containerd.user=false' \
			--cpus 2 --memory 2 --disk 10 "template:ubuntu-$1"
		;;
	*) die "$name is in state $(vm_status "$name")" ;;
	esac
	provision "$name"
	cmd_deploy "$1"
}

provision() {
	echo "provisioning $1 (user $TEST_USER, locales)"
	guest_root "$1" <<EOF
set -e
if ! id $TEST_USER >/dev/null 2>&1; then
	useradd -m -s /bin/bash -G sudo $TEST_USER
	echo '$TEST_USER:$TEST_PW' | chpasswd
fi
# Same ssh key as the Lima user, so the Mac can log in as $TEST_USER.
install -d -m 700 -o $TEST_USER -g $TEST_USER /home/$TEST_USER/.ssh
install -m 600 -o $TEST_USER -g $TEST_USER "\$(getent passwd \$SUDO_USER | cut -d: -f6)/.ssh/authorized_keys" /home/$TEST_USER/.ssh/authorized_keys
install -d -o $TEST_USER -g $TEST_USER /home/$TEST_USER/bin
if ! locale -a | grep -qi '^ko_KR.utf8$'; then
	locale-gen en_US.UTF-8 ko_KR.UTF-8 >/dev/null
	update-locale LANG=en_US.UTF-8
fi
EOF
}

cmd_deploy() {
	need_ver "${1:-}"
	local name; name="$(vm_name "$1")"
	[[ "$(vm_status "$name")" == Running ]] || die "$name is not running (run: $0 up $1)"
	(cd "$ROOT" && make tuidesk-linux >/dev/null)
	local arch
	for arch in amd64 arm64; do
		limactl copy "$ROOT/dist/tuidesk-linux-$arch" "$name:/tmp/tuidesk-linux-$arch"
	done
	guest_root "$name" <<EOF
set -e
for a in amd64 arm64; do
	install -m 755 -o $TEST_USER -g $TEST_USER /tmp/tuidesk-linux-\$a /home/$TEST_USER/bin/tuidesk-linux-\$a
	rm -f /tmp/tuidesk-linux-\$a
done
if [ -e /proc/sys/fs/binfmt_misc/rosetta ]; then a=amd64; else a=arm64; fi
ln -sfn tuidesk-linux-\$a /home/$TEST_USER/bin/tuidesk
chown -h $TEST_USER:$TEST_USER /home/$TEST_USER/bin/tuidesk
echo "deployed: ~$TEST_USER/bin/tuidesk -> tuidesk-linux-\$a"
EOF
}

cmd_ssh() {
	need_ver "${1:-}"
	local name; name="$(vm_name "$1")"
	[[ "$(vm_status "$name")" == Running ]] || die "$name is not running (run: $0 up $1)"
	# Plain ssh (not limactl shell, not ssh -F): reads the Mac's ssh_config
	# (SendEnv LANG LC_*), so the login goes through the same path as a real server.
	exec ssh -p "$(vm_port "$name")" -i "$HOME/.lima/_config/user" \
		-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR \
		"${2:-$TEST_USER}@127.0.0.1"
}

cmd_stop() { need_ver "${1:-}"; limactl stop "$(vm_name "$1")"; }
cmd_delete() { need_ver "${1:-}"; limactl delete --force "$(vm_name "$1")"; }
cmd_list() { limactl list | awk 'NR == 1 || $1 ~ /^tuidesk-ubuntu-/'; }

case "${1:-}" in
up) shift; cmd_up "$@" ;;
deploy) shift; cmd_deploy "$@" ;;
ssh) shift; cmd_ssh "$@" ;;
stop) shift; cmd_stop "$@" ;;
delete) shift; cmd_delete "$@" ;;
list) cmd_list ;;
*) sed -n '2,21p' "$0"; exit 2 ;;
esac
