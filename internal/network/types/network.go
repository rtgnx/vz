package types

import (
	"errors"

	vz "github.com/tmc/apple/virtualization"
)

var (
	// ErrNetworkExists indicates that a network with the requested name already
	// exists.
	ErrNetworkExists = errors.New("network already exists")
	// ErrNetworkNotExist indicates that the requested network does not exist.
	ErrNetworkNotExist = errors.New("network does not exist")
	// ErrUnsupportedNetworkType indicates that no driver supports the requested
	// network type.
	ErrUnsupportedNetworkType = errors.New("unsupported network type")
)

// Network opens network devices for attachment to virtual machines.
type Network interface {
	// Open opens the device identified by id.
	Open(id string) (Netdev, error)
}

// Netdev is a network device adapter passed to the virtual machine builder.
type Netdev interface {
	// Attachment returns the Virtualization.framework network device
	// configuration for this adapter.
	Attachment() vz.VZNetworkDeviceConfiguration
	// Close releases resources held by this adapter.
	Close() error
}

// Driver manages networks of a particular type.
type Driver interface {
	// Close releases resources held by the driver.
	Close() error
	// List returns the names of networks managed by the driver.
	List() []string
	// Create creates a network with the given name.
	Create(name string) error
	// Delete removes the named network.
	Delete(name string) error
	// Open opens the named network.
	Open(name string) (Network, error)
}
