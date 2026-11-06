#!/bin/sh
# tuidesk @VERSION@ installer (linux-@ARCH@). No network access needed.
#
#   ./install.sh                 install (root: /usr/local/bin, others: ~/.local/bin)
#   ./install.sh --user          install to ~/.local/bin/tuidesk
#   sudo ./install.sh --system   install to /usr/local/bin/tuidesk
#   ./install.sh --prefix DIR    install to DIR/bin/tuidesk
#   ./install.sh --uninstall     remove the binary (add the same --user/--system/--prefix)
#
# Settings in ~/.config/tuidesk are never overwritten or removed.
set -eu

VERSION="@VERSION@"
ARCH="@ARCH@"
HERE=$(cd "$(dirname "$0")" && pwd)

die() { echo "install.sh: $*" >&2; exit 1; }
usage() { sed -n '4,10p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

mode=""
prefix=""
uninstall=0
while [ $# -gt 0 ]; do
	case "$1" in
	--user) mode=user ;;
	--system) mode=system ;;
	--prefix)
		[ $# -ge 2 ] || die "--prefix needs a directory"
		mode=prefix; prefix=$2; shift ;;
	--prefix=*) mode=prefix; prefix=${1#--prefix=} ;;
	--uninstall) uninstall=1 ;;
	-h|--help) usage 0 ;;
	*) echo "install.sh: unknown option: $1" >&2; usage 2 ;;
	esac
	shift
done
if [ -z "$mode" ]; then
	if [ "$(id -u)" -eq 0 ]; then mode=system; else mode=user; fi
fi
case "$mode" in
user) bindir="$HOME/.local/bin" ;;
system) bindir="/usr/local/bin" ;;
prefix) bindir="${prefix%/}/bin" ;;
esac
target="$bindir/tuidesk"

if [ "$uninstall" -eq 1 ]; then
	if [ -e "$target" ]; then
		rm -f "$target" || die "cannot remove $target (try: sudo $0 --uninstall --$mode)"
		echo "Removed $target"
	else
		echo "Not installed: $target"
	fi
	if [ "$mode" != system ]; then
		echo "Settings were kept: ${XDG_CONFIG_HOME:-$HOME/.config}/tuidesk"
	fi
	exit 0
fi

# 1. Right package for this machine?
case "$(uname -s)/$(uname -m)" in
Linux/x86_64|Linux/amd64) machine=amd64 ;;
Linux/aarch64|Linux/arm64) machine=arm64 ;;
*) machine="$(uname -s)/$(uname -m)" ;;
esac
[ "$machine" = "$ARCH" ] || die "this package is for linux-$ARCH, but this machine is $machine"

# 2. Files intact after the transfer?
cd "$HERE"
if command -v sha256sum >/dev/null 2>&1; then
	sha256sum -c --quiet SHA256SUMS || die "checksum mismatch: copy the package again"
else
	echo "warning: sha256sum not found; skipping the checksum check" >&2
fi

# 3. Does the binary run here?
"$HERE/tuidesk" --version >/dev/null || die "the tuidesk binary does not run on this machine"

# 4. Install: copy next to the target, then rename, so a running tuidesk is
#    not disturbed and a failed copy never leaves a broken binary.
mkdir -p "$bindir" 2>/dev/null || die "cannot create $bindir (try: sudo $0 --system)"
tmp="$bindir/.tuidesk.new.$$"
trap 'rm -f "$tmp"' EXIT
cp "$HERE/tuidesk" "$tmp" 2>/dev/null || die "cannot write to $bindir (try: sudo $0 --system)"
chmod 755 "$tmp"
mv -f "$tmp" "$target"
trap - EXIT
echo "Installed $("$target" --version) to $target"

# 5. Settings folder for the user who runs it (never touches config.toml;
#    tuidesk writes it on first run).
if [ "$mode" != system ]; then
	confdir="${XDG_CONFIG_HOME:-$HOME/.config}/tuidesk"
	mkdir -p "$confdir/apps" "$confdir/snippets"
	echo "Settings folder: $confdir"
	echo "  Example app launchers: cp $HERE/examples/apps/*.toml $confdir/apps/"
fi

# 6. Can the shell find it?
case ":$PATH:" in
*":$bindir:"*) echo "Run: tuidesk" ;;
*)
	echo "Note: $bindir is not in PATH."
	if [ "$mode" = user ]; then
		echo "  Log out and in again (Ubuntu adds ~/.local/bin to PATH at login),"
		echo "  or run: export PATH=\"\$HOME/.local/bin:\$PATH\""
	fi
	echo "  Until then run: $target"
	;;
esac
