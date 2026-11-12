//go:build darwin

package nat

import (
	"fmt"

	vz "github.com/tmc/apple/virtualization"

	"github.com/tmc/apple/x/vzkit/network"
)

// CreateNATDev creates a virtio network device with a Virtualization.framework
// NAT attachment and a locally administered MAC address.
func CreateNATDev() (vz.VZVirtioNetworkDeviceConfiguration, error) {
	attachment, err := network.CreateNATAttachment(false)
	device := vz.NewVZVirtioNetworkDeviceConfiguration()
	if err != nil {
		return device, fmt.Errorf("create NAT attachment: %w", err)
	}
	if device.ID == 0 {
		return device, fmt.Errorf("create virtio network device")
	}

	device.SetAttachment(&attachment)

	mac := vz.GetVZMACAddressClass().RandomLocallyAdministeredAddress()
	if mac.ID == 0 {
		return device, fmt.Errorf("generate MAC address")
	}

	device.SetMACAddress(&mac)

	return device, nil
}
