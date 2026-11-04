package volume

import (
	"errors"

	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/store"
	"github.com/rtgnx/vz/internal/volume/asif"
	"github.com/rtgnx/vz/internal/volume/types"
)

// OpenRD opens the ASIF root disk for vm, creating it with the VM's configured
// root-disk size if it does not already exist. The returned volume is ready to
// be attached to the VM and must be closed when it is no longer needed.
func OpenRD(vm config.VM) (types.Volume, error) {
	err := CreateRD(vm.Name, vm.Storage.RDSize)

	if err != nil && !errors.Is(err, types.ErrVolumeExists) {
		return nil, err
	}

	driver := asif.New(store.Path(vm.Name))
	return driver.Open(vm.Name)
}

// CreateRD creates an ASIF root disk for the VM identified by name. sizeGB is
// the requested capacity in gigabytes. It returns [types.ErrVolumeExists] if
// a root disk with that name already exists.
func CreateRD(name string, sizeGB uint64) error {
	driver := asif.New(store.Path(name))
	return driver.Create(name, sizeGB)
}
