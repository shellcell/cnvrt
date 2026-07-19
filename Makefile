GO ?= go
APP := cnvrt
CMD := ./cmd/cnvrt
DIST := dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf dev)
GOARM ?= 7
LDFLAGS := -s -w -X main.version=$(VERSION)

MANPAGE := docs/cnvrt.1
COMPLETIONS := completions/cnvrt.bash completions/cnvrt.fish completions/_cnvrt

RELEASE_TARGETS := \
	linux-386 \
	linux-amd64 \
	linux-arm \
	linux-arm64 \
	darwin-amd64 \
	darwin-arm64

.PHONY: build build-debug test vet mandoc clean release build-release checksums $(RELEASE_TARGETS)

build:
	$(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(APP) $(CMD)

build-debug:
	$(GO) build -o $(APP) $(CMD)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

mandoc:
	@command -v mandoc >/dev/null 2>&1 || { printf 'mandoc is required to validate %s\n' "$(MANPAGE)"; exit 1; }
	mandoc -Tlint "$(MANPAGE)"

release: clean mandoc build-release checksums

build-release: $(RELEASE_TARGETS)

$(DIST):
	mkdir -p "$(DIST)"

define release_target
$(1): | $(DIST)
	@printf 'building %s\n' "$(APP)-$(VERSION)-$(1)"
	rm -rf "$(DIST)/$(APP)-$(VERSION)-$(1)"
	mkdir -p "$(DIST)/$(APP)-$(VERSION)-$(1)/man/man1" "$(DIST)/$(APP)-$(VERSION)-$(1)/completions"
	CGO_ENABLED=0 GOOS=$(2) GOARCH=$(3) $(if $(4),GOARM=$(4),) $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o "$(DIST)/$(APP)-$(VERSION)-$(1)/$(APP)" $(CMD)
	cp LICENSE README.md "$(DIST)/$(APP)-$(VERSION)-$(1)/"
	cp "$(MANPAGE)" "$(DIST)/$(APP)-$(VERSION)-$(1)/man/man1/$(APP).1"
	cp $(COMPLETIONS) "$(DIST)/$(APP)-$(VERSION)-$(1)/completions/"
	tar -C "$(DIST)" -czf "$(DIST)/$(APP)-$(VERSION)-$(1).tar.gz" "$(APP)-$(VERSION)-$(1)"
endef

$(eval $(call release_target,linux-386,linux,386,))
$(eval $(call release_target,linux-amd64,linux,amd64,))
$(eval $(call release_target,linux-arm,linux,arm,$(GOARM)))
$(eval $(call release_target,linux-arm64,linux,arm64,))
$(eval $(call release_target,darwin-amd64,darwin,amd64,))
$(eval $(call release_target,darwin-arm64,darwin,arm64,))

checksums: build-release
	@if command -v shasum >/dev/null 2>&1; then \
		cd "$(DIST)" && shasum -a 256 *.tar.gz > checksums.txt; \
	else \
		cd "$(DIST)" && sha256sum *.tar.gz > checksums.txt; \
	fi

clean:
	rm -rf "$(DIST)"
