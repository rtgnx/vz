//go:build darwin

package vswitch

import (
	"github.com/tmc/apple/foundation"
	vz "github.com/tmc/apple/virtualization"
)

// PortLease owns a switch port and exposes its guest-facing file descriptor as
// a Virtualization.framework network attachment.
type PortLease struct {
	vswitch *VSwitch
	port    Port
}

// Close detaches the leased port from its virtual switch and releases its file
// descriptors.
func (l *PortLease) Close() error {
	return l.vswitch.RemovePort(l.port.ID())
}

// Attachment returns a Virtualization.framework virtio network device
// configuration backed by the guest side of the leased port.
func (l *PortLease) Attachment() vz.VZNetworkDeviceConfiguration {

	fd := int32(l.port.GuestFD().Fd())

	fileHandle := foundation.NewFileHandleWithFileDescriptorCloseOnDealloc(
		fd, false,
	)
	netFd := vz.NewFileHandleNetworkDeviceAttachmentWithFileHandle(fileHandle)
	netdev := vz.NewVZVirtioNetworkDeviceConfiguration()
	netdev = netdev.Init()
	netFd.SetMaximumTransmissionUnit(9000)
	netdev.SetAttachment(netFd)
	return netdev.VZNetworkDeviceConfiguration
}
