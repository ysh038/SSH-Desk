GO      ?= go
LDFLAGS := -s -w
DIST    := dist

.PHONY: test vet tuidesk tuidesk-linux evlog evlog-linux wmdemo wmdemo-linux clean

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

tuidesk:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/tuidesk ./cmd/tuidesk

# Static Linux binaries of the desktop (lite build).
tuidesk-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/tuidesk-linux-amd64 ./cmd/tuidesk
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/tuidesk-linux-arm64 ./cmd/tuidesk

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
