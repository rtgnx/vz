package asif

import (
	"errors"
	"sync"

	codehex "github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/diskimagekit"
	vz "github.com/tmc/apple/virtualization"
)

// Volume is an opened ASIF image and the resources required to expose it as a
// Virtualization.framework storage attachment.
type Volume struct {
	path       string
	image      *diskimagekit.DiskImage
	server     *codehex.DiskImageServer
	attachment vz.VZStorageDeviceAttachment

	closeOnce sync.Once
	closeErr  error
}

// Close shuts down the disk image server and closes the image. It is safe to
// call Close more than once; subsequent calls return the result of the first
// call.
func (v *Volume) Close() error {
	v.closeOnce.Do(func() {
		v.closeErr = errors.Join(
			v.server.Close(), v.image.Close(),
		)
	})

	return v.closeErr
}

// Attachment returns the Virtualization.framework attachment backed by this
// volume. The attachment must no longer be in use before [Volume.Close] is
// called.
func (v *Volume) Attachment() vz.VZStorageDeviceAttachment {
	return v.attachment
}
