package vswitch

import (
	"errors"
	"fmt"
	"os"

	"github.com/tmc/apple/foundation"
	vz "github.com/tmc/apple/virtualization"
	"golang.org/x/sys/unix"
)

// SocketPair represents a switch port backed by a Unix datagram socketpair.
// Guest and Host expose the two connected endpoints used to exchange Ethernet
// frames.
type SocketPair struct {
	id     string
	host   *os.File
	guest  *os.File
	hwAddr HWAddr
}

// NewSocketPair creates a port with the given identifier and configures its
// socket buffers for packet forwarding.
func NewSocketPair(id string) (Port, error) {
	fds, err := unix.Socketpair(
		unix.AF_UNIX,
		unix.SOCK_DGRAM, 0,
	)

	if err != nil {
		return nil, err
	}

	err = errors.Join(
		unix.SetsockoptInt(fds[0], unix.SOL_SOCKET, unix.SO_SNDBUF, sendBuffer),
		unix.SetsockoptInt(fds[0], unix.SOL_SOCKET, unix.SO_RCVBUF, recvBuffer),
		unix.SetsockoptInt(fds[1], unix.SOL_SOCKET, unix.SO_SNDBUF, sendBuffer),
		unix.SetsockoptInt(fds[1], unix.SOL_SOCKET, unix.SO_RCVBUF, recvBuffer),
	)

	if err != nil {
		return nil, err
	}

	unix.CloseOnExec(fds[0])
	unix.CloseOnExec(fds[1])

	return &SocketPair{
		id:    id,
		guest: os.NewFile(uintptr(fds[0]), fmt.Sprintf("vswitch-%s-guest", id)),
		host:  os.NewFile(uintptr(fds[1]), fmt.Sprintf("vswitch-%s-host", id)),
	}, err
}

// ID returns the port identifier.
func (p *SocketPair) ID() string { return p.id }

// Close closes both endpoints of the socketpair.
func (p *SocketPair) Close() error {
	errs := []error{}
	if p.guest != nil {
		errs = append(errs, p.guest.Close())
	}

	if p.host != nil {
		errs = append(errs, p.host.Close())
	}

	return errors.Join(errs...)
}

// GuestFD returns the socket endpoint passed to the virtual machine.
func (p *SocketPair) GuestFD() *os.File {
	return p.guest
}

// HostFD returns the socket endpoint used by the virtual switch.
func (p *SocketPair) HostFD() *os.File {
	return p.host
}

// Attachment returns a Virtualization.framework virtio network device
// configuration backed by the guest endpoint.
func (p *SocketPair) Attachment() vz.VZNetworkDeviceConfiguration {

	fd := int32(p.GuestFD().Fd())

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
