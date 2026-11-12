//go:build darwin

package types

import (
	"errors"
	"io"

	vz "github.com/tmc/apple/virtualization"
)

const (
	MiB uint64 = 1 << 20
	GiB uint64 = 1 << 30
)

var (
	// ErrVolumeOpen indicates that a volume could not be opened.
	ErrVolumeOpen = errors.New("volume open error")
	// ErrVolumeCreate indicates that a volume could not be created.
	ErrVolumeCreate = errors.New("volume create error")
	// ErrVolumeDelete indicates that a volume could not be deleted.
	ErrVolumeDelete = errors.New("volume delete error")
	// ErrVolumeExists indicates that a volume with the requested name already exists.
	ErrVolumeExists = errors.New("volume already exists")
	// ErrVolumeNotExists indicates that the requested volume does not exist.
	ErrVolumeNotExists = errors.New("volume does not exist")
	// ErrUnsupportedAction indicates that feature is not supported by the driver
	ErrUnsupportedAction = errors.New("unsupported action")
)

// Stat describes a volume reported by a [Driver].
type Stat struct {
	// Name is the volume's driver-specific name.
	Name string
	// Format identifies the volume format or backing driver.
	Format string
	// SizeGB is the configured volume capacity in gigabytes. Its value may be
	// zero when the driver does not report a configured capacity.
	SizeGB uint64
	// SizeOnDiskGB is the volume's current on-disk size in gigabytes. Its value
	// may be zero when the driver does not report on-disk usage.
	SizeOnDiskGB uint64
}

// Driver manages volumes in a particular storage format.
type Driver interface {
	// Open opens the named volume and returns a handle suitable for attaching
	// it to a virtual machine. An error is returned if the volume cannot be
	// opened.
	Open(name string) (Volume, error)
	// Create creates a volume with the given name and capacity in gigabytes.
	// It returns [ErrVolumeExists] if a volume with that name already exists.
	Create(name string, size uint64) error
	// Delete removes the named volume. It returns [ErrVolumeNotExists] if the
	// volume cannot be found.
	Delete(name string) error
	// List returns statistics for the volumes managed by the driver.
	List() ([]Stat, error)
}

// Volume is an opened storage volume that can be attached to a virtual machine.
type Volume interface {
	// Attachment returns the Virtualization.framework attachment for this
	// volume. The returned attachment is valid until the volume is closed.
	Attachment() vz.VZStorageDeviceAttachment
	// Close releases resources held by the opened volume. Callers should stop
	// using the attachment before closing the volume.
	Close() error

	// Writer returns [io.Writer] for the block device backing the volume
	Writer() io.Writer
}

// BlockDevice provides random-access reads and writes to a storage device.
type BlockDevice interface {
	io.ReaderAt
	io.WriterAt

	// Size returns the device capacity in bytes.
	Size() int64
	// Sync commits pending writes to the backing storage.
	Sync() error
	// Close releases resources held by the device. Further operations may fail.
	Close() error
}
