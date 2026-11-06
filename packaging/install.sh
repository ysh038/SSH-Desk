#!/bin/sh
# tuidesk @VERSION@ installer (linux-@ARCH@). No network access needed.
#
#   ./install.sh                 install (root: /usr/local/bin, others: ~/.local/bin)
#   ./install.sh --user          install to ~/.local/bin/tuidesk
#   sudo ./install.sh --system   install to /usr/local/bin/tuidesk
#   ./install.sh --prefix DIR    install to DIR/bin/tuidesk
#   ./install.sh --rollback      undo the last install (same as ./rollback.sh)
#   ./install.sh --uninstall     remove the binary
#   (--rollback and --uninstall take the same --user/--system/--prefix)
#
# Settings in ~/.config/tuidesk are never overwritten or removed.
set -eu

VERSION="@VERSION@"
ARCH="@ARCH@"
HERE=$(cd "$(dirname "$0")" && pwd)

die() { echo "install.sh: $*" >&2; exit 1; }
usage() { sed -n '4,12p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

mode=""
prefix=""
action=install
while [ $# -gt 0 ]; do
	case "$1" in
	--user) mode=user ;;
	--system) mode=system ;;
	--prefix)
		[ $# -ge 2 ] || die "--prefix needs a directory"
		mode=prefix; prefix=$2; shift ;;
	--prefix=*) mode=prefix; prefix=${1#--prefix=} ;;
	--uninstall) action=uninstall ;;
	--rollback) action=rollback ;;
	-h|--help) usage 0 ;;
	*) echo "install.sh: unknown option: $1" >&2; usage 2 ;;
	esac
	shift
done
if [ -z "$mode" ]; then
	if [ "$(id -u)" -eq 0 ]; then mode=system; else mode=user; fi
fi
case "$mode" in
user)
	bindir="$HOME/.local/bin"
	statedir="${XDG_STATE_HOME:-$HOME/.local/state}/tuidesk" ;;
system)
	bindir="/usr/local/bin"
	statedir="/var/lib/tuidesk" ;;
prefix)
	bindir="${prefix%/}/bin"
	statedir="${prefix%/}/lib/tuidesk" ;;
esac
target="$bindir/tuidesk"
confdir="${XDG_CONFIG_HOME:-$HOME/.config}/tuidesk"
modeflag="--$mode"
[ "$mode" = prefix ] && modeflag="--prefix $prefix"

# The install record: what the last install replaced and which folders it
# created, so --rollback can put things back.
#   $statedir/install.state   key=value lines
#   $statedir/previous        the binary that was replaced (upgrades only)
state="$statedir/install.state"
state_get() { sed -n "s/^$1=//p" "$state"; }

# atomic_copy SRC: copy SRC over $target via a temp file + rename, so a
# running tuidesk is not disturbed and a failed copy leaves no broken binary.
atomic_copy() {
	tmp="$bindir/.tuidesk.new.$$"
	trap 'rm -f "$tmp"' EXIT
	cp "$1" "$tmp" 2>/dev/null || die "cannot write to $bindir (try: sudo $0 $modeflag)"
	chmod 755 "$tmp"
	mv -f "$tmp" "$target"
	trap - EXIT
}

# ---- uninstall ----
if [ "$action" = uninstall ]; then
	if [ -e "$target" ]; then
		rm -f "$target" || die "cannot remove $target (try: sudo $0 --uninstall $modeflag)"
		echo "Removed $target"
	else
		echo "Not installed: $target"
	fi
	rm -rf "$statedir" 2>/dev/null || true
	[ "$mode" = system ] || echo "Settings were kept: $confdir"
	exit 0
fi

# ---- rollback ----
if [ "$action" = rollback ]; then
	[ -f "$state" ] || die "nothing to roll back: no install record in $statedir"
	[ -w "$statedir" ] || die "cannot change $statedir (try: sudo $0 --rollback $modeflag)"
	[ "$(state_get target)" = "$target" ] || die "the install record is for $(state_get target), not $target"
	if [ "$(state_get previous)" = yes ]; then
		"$statedir/previous" --version >/dev/null 2>&1 || die "the saved previous binary does not run: $statedir/previous"
		atomic_copy "$statedir/previous"
		echo "Rolled back: $("$target" --version) restored to $target"
	else
		rm -f "$target" || die "cannot remove $target (try: sudo $0 --rollback $modeflag)"
		echo "Rolled back: removed $target (there was no tuidesk before)"
	fi
	created=$(state_get created)
	rm -rf "$statedir"
	# Remove folders the install created, deepest first, only while empty.
	printf '%s\n' "$created" | awk 'NF { a[++n] = $0 } END { for (i = n; i > 0; i--) print a[i] }' |
		while IFS= read -r d; do rmdir "$d" 2>/dev/null || true; done
	if [ "$mode" != system ] && [ -d "$confdir" ]; then
		echo "Settings were kept: $confdir"
	fi
	exit 0
fi

# ---- install ----

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

# 4. Note the folders this install will create (for --rollback).
created=""
note_new() { # note_new DIR: record DIR and its missing parents, top-down
	d=$1
	new=""
	while [ ! -e "$d" ]; do
		new="$d
$new"
		d=$(dirname "$d")
	done
	created="$created$new"
}
note_new "$bindir"
note_new "$statedir"
if [ "$mode" != system ]; then
	note_new "$confdir/apps"
	note_new "$confdir/snippets"
fi

# 5. Install, keeping the replaced binary for --rollback. Reinstalling the
#    same binary keeps the earlier record, so rollback still goes back to
#    the version before it.
mkdir -p "$bindir" 2>/dev/null || die "cannot create $bindir (try: sudo $0 --system)"
same=no
if [ -e "$target" ] && cmp -s "$HERE/tuidesk" "$target"; then
	same=yes
fi
if [ "$same" = no ]; then
	mkdir -p "$statedir" 2>/dev/null || die "cannot create $statedir (try: sudo $0 $modeflag)"
	previous=no
	previous_version=""
	rm -f "$statedir/previous"
	if [ -e "$target" ]; then
		cp -p "$target" "$statedir/previous.tmp" || die "cannot back up $target"
		mv -f "$statedir/previous.tmp" "$statedir/previous"
		previous=yes
		previous_version=$("$target" --version 2>/dev/null || echo unknown)
	fi
	atomic_copy "$HERE/tuidesk"
	{
		echo "version=$VERSION"
		echo "target=$target"
		echo "previous=$previous"
		echo "previous_version=$previous_version"
		printf '%s' "$created" | awk 'NF && !seen[$0]++ { print "created=" $0 }'
	} > "$state.tmp"
	mv -f "$state.tmp" "$state"
fi
if [ "$same" = yes ]; then
	echo "Already installed: $("$target" --version) at $target"
elif [ "$previous" = yes ]; then
	echo "Installed $("$target" --version) to $target (replaced: $previous_version)"
else
	echo "Installed $("$target" --version) to $target"
fi

# 6. Settings folder for the user who runs it (never touches config.toml;
#    tuidesk writes it on first run).
if [ "$mode" != system ]; then
	mkdir -p "$confdir/apps" "$confdir/snippets"
	echo "Settings folder: $confdir"
	echo "  Example app launchers: cp $HERE/examples/apps/*.toml $confdir/apps/"
fi

# 7. Can the shell find it?
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
if [ -f "$state" ]; then
	sudo=""
	[ "$mode" = system ] && sudo="sudo "
	echo "To undo this install: $sudo$HERE/rollback.sh $modeflag"
fi
