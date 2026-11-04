package iscsi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	nbd "github.com/pojntfx/go-nbd/pkg/server"
	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/volume/types"
	"github.com/tmc/apple/foundation"
	vz "github.com/tmc/apple/virtualization"
)

type nbdBackend struct{ device *BlockDevice }

func (b nbdBackend) ReadAt(p []byte, off int64) (int, error)  { return b.device.ReadAt(p, off) }
func (b nbdBackend) WriteAt(p []byte, off int64) (int, error) { return b.device.WriteAt(p, off) }
func (b nbdBackend) Size() (int64, error)                     { return b.device.Size(), nil }
func (b nbdBackend) Sync() error                              { return b.device.Sync() }

type Volume struct {
	device     *BlockDevice
	listener   net.Listener
	attachment vz.VZStorageDeviceAttachment
	readOnly   bool
	mu         sync.Mutex
	conns      map[net.Conn]struct{}
	closed     bool
	wg         sync.WaitGroup
	closeOnce  sync.Once
	closeErr   error
}

func openVolume(ctx context.Context, cfg config.Volume) (types.Volume, error) {
	device, err := newBlockDevice(ctx, cfg)
	if err != nil {
		return nil, errors.Join(types.ErrVolumeOpen, err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = device.Close()
		return nil, errors.Join(types.ErrVolumeOpen, err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	rawURL := fmt.Sprintf("nbd://127.0.0.1:%d/", addr.Port)
	u := foundation.NewURLWithString(rawURL)
	if u.ID == 0 {
		_ = listener.Close()
		_ = device.Close()
		return nil, errors.Join(types.ErrVolumeOpen, errors.New("invalid NBD server URL"))
	}
	attachment, err := vz.NewNetworkBlockDeviceStorageDeviceAttachmentWithURLTimeoutForcedReadOnlySynchronizationModeError(
		u, 10, cfg.ReadOnly, vz.VZDiskSynchronizationModeFull,
	)
	if err != nil {
		_ = listener.Close()
		_ = device.Close()
		return nil, errors.Join(types.ErrVolumeOpen, err)
	}
	v := &Volume{device: device, listener: listener, attachment: attachment.VZStorageDeviceAttachment, conns: make(map[net.Conn]struct{}), readOnly: cfg.ReadOnly}
	v.wg.Add(1)
	go v.serve()
	return v, nil
}

// New opens an iSCSI target and exposes its LUN to Virtualization.framework through a local NBD endpoint.
func New(ctx context.Context, addr, target string) (types.Volume, error) {
	return openVolume(ctx, config.Volume{Address: addr, Target: target})
}

func (v *Volume) serve() {
	defer v.wg.Done()
	for {
		conn, err := v.listener.Accept()
		if err != nil {
			return
		}
		v.mu.Lock()
		if v.closed {
			v.mu.Unlock()
			_ = conn.Close()
			return
		}
		v.conns[conn] = struct{}{}
		v.wg.Add(1)
		v.mu.Unlock()
		go func(conn net.Conn) {
			defer v.wg.Done()
			defer func() { v.mu.Lock(); delete(v.conns, conn); v.mu.Unlock(); _ = conn.Close() }()
			_ = nbd.Handle(conn, []*nbd.Export{{Name: "", Backend: nbdBackend{device: v.device}}}, &nbd.Options{ReadOnly: v.readOnly, SupportsMultiConn: false})
		}(conn)
	}
}

func (v *Volume) Close() error {
	v.closeOnce.Do(func() {
		v.mu.Lock()
		v.closed = true
		v.mu.Unlock()
		_ = v.listener.Close()
		v.mu.Lock()
		for conn := range v.conns {
			_ = conn.Close()
		}
		v.mu.Unlock()
		v.wg.Wait()
		v.closeErr = v.device.Close()
	})
	return v.closeErr
}

func (v *Volume) Attachment() vz.VZStorageDeviceAttachment { return v.attachment }
