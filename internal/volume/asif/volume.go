//go:build darwin

package asif

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	codehex "github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/diskimagekit"
	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/volume/types"
	vz "github.com/tmc/apple/virtualization"
)

type Manifest struct {
	Root string
	// Layers list of ordered ASIF image layers
	Layers []string
	// Current writable layer
	Current string
}

// Volume is an opened ASIF image and the resources required to expose it as a
// Virtualization.framework storage attachment.
type Volume struct {
	path       string
	image      *diskimagekit.DiskImage
	server     *codehex.DiskImageServer
	attachment vz.VZStorageDeviceAttachment
	manifest   Manifest
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
func (v *Volume) Snapshot(ref string) error {

	fp := filepath.Join(filepath.Dir(v.path), `layers`, ref)

	if _, err := os.Stat(fp); err != nil && !os.IsNotExist(err) {
		return types.ErrVolumeExists
	}

	err := os.MkdirAll(filepath.Dir(fp), 0750)

	if err != nil && !os.IsExist(err) {
		return err
	}

	if err := v.image.CreateLayer(fp, diskimagekit.Overlay); err != nil {
		return err
	}

	v.manifest.Layers = append(v.manifest.Layers, ref)
	v.manifest.Current = ref
	return config.Write(v.manifest, strings.TrimSuffix(v.path, `.asif`)+`.yml`)
}

func (v *Volume) Size() int64 { return v.image.Size() }

func (v *Volume) Writer() io.Writer {
	return io.NewOffsetWriter(v.image, 0)
}

func (v *Volume) WriteAt(p []byte, off int64) (int, error) {
	return v.image.WriteAt(p, off)
}

func (v *Volume) Reader() io.Reader {
	return io.NewSectionReader(v.image, 0, v.image.Size())
}
