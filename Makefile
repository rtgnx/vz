GO_BUILD_FLAGS := -trimpath -buildvcs=true

VERSION ?= $(shell git describe --tags --exact-match HEAD 2>/dev/null || shell git branch --show-current 2>/dev/null || echo develop)
COMMIT ?= $(shell git rev-parse --short HEAD)

LD_FLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT)


all: bin/vz bin/vzd

tests:
	go test -v ./...

clean:
	rm bin/vz bin/vzd || exit 0

bin/vz: cmd/vz/main.go
	CGO_ENABLED=1 go build \
	    $(GO_BUILD_FLAGS) -ldflags="$(LD_FLAGS)" -o bin/vz cmd/vz/main.go

bin/vzd: cmd/vzd/main.go
	CGO_ENABLED=1 go build \
	    $(GO_BUILD_FLAGS) -ldflags="$(LD_FLAGS)" -o bin/vzd cmd/vzd/main.go

	codesign --force --sign - --entitlements virtualization.entitlements bin/vzd
