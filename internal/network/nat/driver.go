package nat

import (
	"sync"

	"github.com/rtgnx/vz/internal/network/types"
	vz "github.com/tmc/apple/virtualization"
)

// NATDev adapts a Virtualization.framework network device configuration to the
// network device interface used by the VM builder.
type NATDev struct {
	dev vz.VZNetworkDeviceConfiguration
}

// NAT represents a network that uses Virtualization.framework's built-in NAT.
type NAT struct{}

// Driver manages NAT networks backed by Virtualization.framework.
type Driver struct {
	nats map[string]*NAT
	mu   sync.Mutex
}

// NewDriver returns an initialized NAT network driver.
func NewDriver() types.Driver {
	return &Driver{nats: make(map[string]*NAT), mu: sync.Mutex{}}
}

// Close releases resources held by the driver. NAT networks require no
// driver-level cleanup.
func (*Driver) Close() error { return nil }

// List returns the names of NAT networks. The current implementation returns
// an empty slice without enumerating configured networks.
func (*Driver) List() []string { return []string{} }

// Delete removes the named NAT network. The current implementation is a no-op.
func (*Driver) Delete(name string) error { return nil }

// Create registers a NAT network with the given name.
func (d *Driver) Create(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.nats[name]; ok {
		return types.ErrNetworkExists
	}

	d.nats[name] = new(NAT)
	return nil
}

// Open returns the named NAT network. It returns [types.ErrNetworkExists] if
// the name has not been registered.
func (d *Driver) Open(name string) (types.Network, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	nat, ok := d.nats[name]

	if !ok {
		return nil, types.ErrNetworkExists
	}

	return nat, nil
}

// Open creates a VM network device using Virtualization.framework's NAT
// attachment.
func (*NAT) Open(id string) (types.Netdev, error) {
	vzNATDev, err := CreateNATDev()
	return &NATDev{dev: vzNATDev.VZNetworkDeviceConfiguration}, err
}

// Attachment returns the Virtualization.framework configuration for the NAT
// device.
func (dev *NATDev) Attachment() vz.VZNetworkDeviceConfiguration {
	return dev.dev
}

// Close releases resources held by the NAT device. No additional cleanup is
// required.
func (*NATDev) Close() error { return nil }
