package asif

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	codehex "github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/diskimagekit"
	"github.com/rtgnx/vz/internal/volume/types"
	"github.com/tmc/apple/foundation"
	vz "github.com/tmc/apple/virtualization"
)

const (
	// GB is the number of bytes in one binary gigabyte (GiB).
	GB uint64 = 1024 * 1024 * 1024
	// Format is the format identifier used for ASIF volumes.
	Format = `ASIF`
	// VolumeTypeExt is the filename extension used for ASIF volume images.
	VolumeTypeExt = `.asif`
	// DefaultBlockSize is the block size used when creating ASIF images.
	DefaultBlockSize diskimagekit.BlockSize = diskimagekit.BlockSize4096
)

var (
	// ErrInvalidNBDServerURL indicates that the disk image server returned a
	// URL that could not be used to construct a Virtualization.framework
	// attachment.
	ErrInvalidNBDServerURL = errors.New("invalid NBD server URL")
)

// Driver manages ASIF disk images stored beneath a root directory.
type Driver struct {
	root string
}

// Create creates an ASIF image named name with the requested capacity in
// gigabytes. It returns [types.ErrVolumeExists] if the image path already
// exists and wraps [types.ErrVolumeCreate] if image creation fails.
func (d *Driver) Create(name string, sizeGB uint64) error {
	fp := filepath.Join(
		d.root, fmt.Sprintf("%s%s", name, VolumeTypeExt),
	)

	if _, err := os.Stat(fp); !os.IsNotExist(err) {
		return types.ErrVolumeExists
	}

	var (
		sizeBytes  = sizeGB * GB
		blockCount = sizeBytes / uint64(DefaultBlockSize)
	)

	img, err := diskimagekit.CreateASIF(
		fp, int64(blockCount),
		diskimagekit.WithBlockSize(DefaultBlockSize),
	)

	if err != nil {
		return errors.Join(types.ErrVolumeCreate, err)
	}

	if err := img.Close(); err != nil {
		return errors.Join(types.ErrVolumeCreate, err)
	}

	return nil
}

// Delete removes the ASIF image named name. It wraps [types.ErrVolumeDelete]
// when removal fails, including when the image does not exist.
func (d *Driver) Delete(name string) error {
	if len(name) < 1 {
		return errors.Join(
			types.ErrVolumeDelete, fmt.Errorf("invalid volume name=%s", name),
		)
	}

	fp := filepath.Join(
		d.root, fmt.Sprintf("%s%s", name, VolumeTypeExt),
	)

	if err := os.Remove(fp); err != nil {

		if os.IsNotExist(err) {
			err = errors.Join(err, types.ErrVolumeNotExists)
		}

		return errors.Join(types.ErrVolumeDelete, err)
	}

	return nil
}

// List returns statistics for ASIF images in the driver's root directory.
// Entries whose metadata cannot be read are skipped.
func (d *Driver) List() ([]types.Stat, error) {
	entries, err := os.ReadDir(d.root)

	if err != nil {
		return nil, err
	}

	volumes := []types.Stat{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), VolumeTypeExt) {
			continue
		}

		stat, err := entry.Info()

		if err != nil {
			continue
		}

		volumes = append(volumes, types.Stat{
			Name:         strings.TrimSuffix(entry.Name(), VolumeTypeExt),
			Format:       Format,
			SizeOnDiskGB: uint64(stat.Size()) / GB,
		})

	}

	return volumes, nil
}

// Open opens the named ASIF image and creates a Virtualization.framework
// network block device attachment backed by a local disk image server. The
// returned volume owns the image and server resources; callers must close it
// after the attachment is no longer in use.
func (d *Driver) Open(name string) (types.Volume, error) {
	fp := filepath.Join(
		d.root, fmt.Sprintf("%s%s", name, VolumeTypeExt),
	)

	img, err := diskimagekit.Open(
		fp, diskimagekit.WithOpenMode(
			diskimagekit.ReadWrite,
		),
	)

	if err != nil {
		return nil, errors.Join(types.ErrVolumeOpen, err)
	}

	isrv, err := codehex.NewDiskImageServer(img)

	if err != nil {
		_ = img.Close()
		return nil, errors.Join(types.ErrVolumeOpen, err)
	}

	url := foundation.NewURLWithString(isrv.URL())

	if url.ID == 0 {
		_ = isrv.Close()
		_ = img.Close()
		return nil, errors.Join(types.ErrVolumeOpen, ErrInvalidNBDServerURL)
	}

	blockDev, err := vz.NewNetworkBlockDeviceStorageDeviceAttachmentWithURLTimeoutForcedReadOnlySynchronizationModeError(
		url, 5, false, vz.VZDiskSynchronizationModeFull,
	)

	if err != nil {
		_ = isrv.Close()
		_ = img.Close()
		return nil, errors.Join(types.ErrVolumeOpen, err)
	}

	return &Volume{
		image: img, server: isrv,
		attachment: blockDev.VZStorageDeviceAttachment,
	}, nil
}

// New returns a volume driver that stores ASIF images in root.
func New(root string) types.Driver {
	return &Driver{root: root}
}
