package asif

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	layers     []string
	closeOnce  sync.Once
	closeErr   error
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

// Snapshot creates new ASIF layer and appends it to list of layers
// Close() - should commit these layers in {name}.yml so that
// they can be reconstructed in correct order
func (v *Volume) Snapshot(label string) error {

	layerRef := hex.EncodeToString(
		sha256.New().Sum(
			fmt.Appendf(nil, "%s:%s", v.image.LayerUUID(), label),
		),
	)

	fp := filepath.Join(filepath.Dir(v.path), `layers`, layerRef)

	err := os.MkdirAll(filepath.Dir(fp), 0750)

	if err != nil && !os.IsExist(err) {
		return err
	}

	if err := v.image.CreateLayer(fp, diskimagekit.Overlay); err != nil {
		return err
	}

	v.layers = append(v.layers, layerRef)

	return nil
}
