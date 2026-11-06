#!/usr/bin/env bash
# Builds the offline packages for closed-network servers (SPEC 11) into
# dist/release:
#
#   tuidesk-<version>-linux-amd64.tar.gz   binary, install.sh, README.txt, SHA256SUMS, ...
#   tuidesk-<version>-linux-arm64.tar.gz
#   tuidesk-<version>-src.tar.gz           source with vendor/ for building offline
#   SHA256SUMS                             checksums of the three archives
#
# VERSION defaults to `git describe` (tag a release: git tag v0.1.0).
# Usage: scripts/package.sh   (or: make package)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
OUT="$ROOT/dist/release"
STAGE="$OUT/stage"
GO="${GO:-go}"

die() { echo "error: $*" >&2; exit 1; }
[[ "$VERSION" =~ ^[A-Za-z0-9._+-]+$ ]] || die "bad VERSION '$VERSION'"

# Archives made on macOS must not carry Apple metadata (._ files, xattrs):
# GNU tar on the server would warn about every file.
targz() { # targz ARCHIVE DIR_IN_STAGE
	COPYFILE_DISABLE=1 tar --no-mac-metadata --no-xattrs --no-acls \
		--uid 0 --gid 0 --uname root --gname root \
		-czf "$1" -C "$STAGE" "$2"
}

make -s check-lite # CLAUDE.md rule 3: no network code in the lite build
if [[ -n "$(git status --porcelain)" ]]; then
	echo "warning: uncommitted changes; binaries include them, the source tarball (HEAD) does not" >&2
fi

rm -rf "$STAGE"
mkdir -p "$STAGE"
rm -f "$OUT"/tuidesk-*.tar.gz "$OUT/SHA256SUMS"
archives=()

for arch in amd64 arm64; do
	name="tuidesk-$VERSION-linux-$arch"
	dir="$STAGE/$name"
	mkdir -p "$dir/examples/apps"
	CGO_ENABLED=0 GOOS=linux GOARCH="$arch" "$GO" build -trimpath \
		-ldflags "-s -w -X main.version=$VERSION" -o "$dir/tuidesk" ./cmd/tuidesk
	for f in install.sh rollback.sh README.txt; do
		sed -e "s/@VERSION@/$VERSION/g" -e "s/@ARCH@/$arch/g" "packaging/$f" > "$dir/$f"
	done
	chmod 755 "$dir/install.sh" "$dir/rollback.sh"
	cp LICENSE THIRD_PARTY_NOTICES.md "$dir/"
	cp docs/examples/apps/*.toml "$dir/examples/apps/"
	(cd "$dir" && find . -type f ! -name SHA256SUMS | sed 's|^\./||' | LC_ALL=C sort |
		xargs shasum -a 256 > SHA256SUMS)
	targz "$OUT/$name.tar.gz" "$name"
	archives+=("$name.tar.gz")
done

# Source tarball: committed tree + vendored modules, so a Go toolchain inside
# the closed network can build without a module proxy.
src="tuidesk-$VERSION-src"
git archive --format=tar --prefix="$src/" HEAD | tar -x -C "$STAGE"
(cd "$STAGE/$src" && "$GO" mod vendor)
echo "$VERSION" > "$STAGE/$src/VERSION"
targz "$OUT/$src.tar.gz" "$src"
archives+=("$src.tar.gz")

(cd "$OUT" && shasum -a 256 "${archives[@]}" > SHA256SUMS)
rm -rf "$STAGE"

echo "packages in ${OUT#"$ROOT"/}:"
(cd "$OUT" && ls -l "${archives[@]}" SHA256SUMS | awk '{printf "  %8s  %s\n", $5, $9}')
