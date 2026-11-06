GO      ?= go
LDFLAGS := -s -w
DIST    := dist
# Version baked into tuidesk (--version) and package names. Tag a release
# (git tag v0.1.0) to get a clean version; otherwise the commit is used.
# Source tarballs carry a VERSION file instead of .git.
VERSION ?= $(shell cat VERSION 2>/dev/null || git describe --tags --always --dirty 2>/dev/null || echo dev)
TD_LDFLAGS := $(LDFLAGS) -X main.version=$(VERSION)

.PHONY: check-lite test vet package tuidesk tuidesk-linux evlog evlog-linux wmdemo wmdemo-linux clean

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

# CLAUDE.md rule 3: the lite build has no network code. net/url and
# net/netip (pure parsing, pulled in by x/ansi for OSC 8 links) are allowed.
check-lite:
	@if GOOS=linux $(GO) list -deps ./cmd/tuidesk | grep -E '^(net|net/http|net/rpc|net/smtp|net/mail|crypto/tls)(/|$$)' | grep -vE '^net/(url|netip)$$'; then \
		echo "lite build imports network packages (above)"; exit 1; else echo "lite build: no network packages"; fi

tuidesk:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(TD_LDFLAGS)" -o $(DIST)/tuidesk ./cmd/tuidesk

# Static Linux binaries of the desktop (lite build).
tuidesk-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(TD_LDFLAGS)" -o $(DIST)/tuidesk-linux-amd64 ./cmd/tuidesk
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(TD_LDFLAGS)" -o $(DIST)/tuidesk-linux-arm64 ./cmd/tuidesk

# Offline install packages for closed-network servers (SPEC 11):
# dist/release/tuidesk-<version>-linux-{amd64,arm64}.tar.gz, a source
# tarball with vendored modules, and SHA256SUMS.
package:
	VERSION=$(VERSION) scripts/package.sh

# Local (macOS) build of the Phase 0 event logger.
evlog:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/evlog ./cmd/evlog

# Static Linux binaries to scp onto the Ubuntu test server.
evlog-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/evlog-linux-amd64 ./cmd/evlog
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/evlog-linux-arm64 ./cmd/evlog

wmdemo:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/wmdemo ./cmd/wmdemo

wmdemo-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/wmdemo-linux-amd64 ./cmd/wmdemo
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/wmdemo-linux-arm64 ./cmd/wmdemo

clean:
	rm -rf $(DIST)
