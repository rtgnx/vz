GO_BUILD_FLAGS := -trimpath -buildvcs=true

VERSION ?= $(shell git describe --tags --exact-match HEAD 2>/dev/null || shell git branch --show-current 2>/dev/null || echo develop)
COMMIT ?= $(shell git rev-parse --short HEAD)

LD_FLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT)


all: bin/vz bin/vzd bin/initd initramfs

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

bin/initd: cmd/init/main.go
	GOOS=linux GOARCH=arm64 go build -o bin/initd cmd/init/main.go

bin/Image: kernel/Dockerfile
	container build -c 6 -m 4 -o type=local,dest=./bin/ kernel/

initramfs: bin/initd
	mkdir -p bin/initramfs/{dev,proc,sys,newroot}
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/initramfs/init ./cmd/init
	chmod 0755 bin/initramfs/init
	(cd bin/initramfs && find . -print | cpio -o --format newc) \
        | gzip -c > initramfs.cpio.gz && cp initramfs.cpio.gz ~/.vm/kernel/oci-7.3.4.rd
