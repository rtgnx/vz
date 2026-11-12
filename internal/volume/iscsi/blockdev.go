//go:build darwin

package iscsi

import (
	"context"
	"errors"
	"io"
	"math"
	"sync"

	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/volume/types"
	"github.com/uiscsi/uiscsi"
)

func newBlockDevice(ctx context.Context, cfg config.Volume) (*BlockDevice, error) {
	session, err := dial(ctx, cfg)
	if err != nil {
		return nil, err
	}
	capacity, err := session.SCSI().ReadCapacity(ctx, cfg.LUN)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	if capacity.BlockSize == 0 || capacity.LogicalBlocks == 0 || capacity.LogicalBlocks > uint64(math.MaxInt64)/uint64(capacity.BlockSize) {
		_ = session.Close()
		return nil, errors.New("target returned invalid or unsupported capacity")
	}
	return &BlockDevice{ctx: context.Background(), conn: session, lun: cfg.LUN, blockSize: capacity.BlockSize, size: int64(capacity.LogicalBlocks * uint64(capacity.BlockSize))}, nil
}

var _ types.BlockDevice = &BlockDevice{}

type BlockDevice struct {
	ctx       context.Context
	conn      *uiscsi.Session
	size      int64
	lun       uint64
	blockSize uint32
	closeOnce sync.Once
	closeErr  error
}

func (b *BlockDevice) Close() error {
	b.closeOnce.Do(func() { b.closeErr = b.conn.Close() })
	return b.closeErr
}

func (b *BlockDevice) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative iSCSI read offset")
	}
	if len(p) == 0 {
		return 0, nil
	}
	if off >= b.size {
		return 0, io.EOF
	}
	want := len(p)
	if int64(want) > b.size-off {
		want = int(b.size - off)
	}
	bs := int64(b.blockSize)
	start := off / bs
	end := (off + int64(want) + bs - 1) / bs
	data, err := b.conn.SCSI().ReadBlocks(b.ctx, b.lun, uint64(start), uint32(end-start), b.blockSize)
	if err != nil {
		return 0, err
	}
	if int64(len(data)) < (end-start)*bs {
		return 0, io.ErrUnexpectedEOF
	}
	from := off - start*bs
	n := copy(p[:want], data[int(from):int(from)+want])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (b *BlockDevice) Size() int64 { return b.size }

func (b *BlockDevice) Sync() error {
	return b.conn.SCSI().SynchronizeCache(b.ctx, b.lun)
}

func (b *BlockDevice) WriteAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative iSCSI write offset")
	}
	if len(p) == 0 {
		return 0, nil
	}
	if off >= b.size || int64(len(p)) > b.size-off {
		return 0, io.ErrShortWrite
	}
	bs := int64(b.blockSize)
	start := off / bs
	end := (off + int64(len(p)) + bs - 1) / bs
	if end-start > math.MaxUint32 {
		return 0, errors.New("iSCSI request exceeds block count limit")
	}
	aligned := off%bs == 0 && int64(len(p))%bs == 0
	data := p
	if !aligned {
		var err error
		data, err = b.conn.SCSI().ReadBlocks(b.ctx, b.lun, uint64(start), uint32(end-start), b.blockSize)
		if err != nil {
			return 0, err
		}
		if int64(len(data)) < (end-start)*bs {
			return 0, io.ErrUnexpectedEOF
		}
		from := off - start*bs
		copy(data[int(from):int(from)+len(p)], p)
	}
	if err := b.conn.SCSI().WriteBlocks(b.ctx, b.lun, uint64(start), uint32(end-start), b.blockSize, data); err != nil {
		return 0, err
	}
	return len(p), nil
}
