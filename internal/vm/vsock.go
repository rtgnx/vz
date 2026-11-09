//go:build darwin

package vm

import (
	"errors"
	"net"

	"github.com/rtgnx/vz/internal/network/vsock"
	vz "github.com/tmc/apple/virtualization"
	vzvsock "github.com/tmc/apple/x/vzkit/vsock"
)

func (vm *VM) Dial(port uint32) (conn net.Conn, err error) {

	if vm.State() != vz.VZVirtualMachineStateRunning {
		return nil, ErrVMClosed
	}

	var dev vz.VZVirtioSocketDevice

	vm.q.Sync(func() {
		devices := vm.vm.SocketDevices()
		if len(devices) > 0 {
			dev = vz.VZVirtioSocketDeviceFromID(devices[0].ID)
		}
		if dev.ID == 0 {
			conn, err = nil, errors.New("VM has no vsock device")
		}

	})

	conn, err = vsock.Dial(vm.q, dev, 7000)
	return conn, err
}

func (vm *VM) Listen(port uint32) (net.Listener, error) {
	var (
		device vz.VZVirtioSocketDevice
		err    error
	)

	vm.q.Sync(func() {
		devices := vm.vm.SocketDevices()
		if len(devices) == 0 {
			err = errors.New("VM has no vsock device")
			return
		}

		device = vz.VZVirtioSocketDeviceFromID(devices[0].ID)
		if device.ID == 0 {
			err = errors.New("invalid vsock device")
		}
	})
	if err != nil {
		return nil, err
	}

	listener, err := vzvsock.Listen(device, port, func(fn func()) {
		vm.q.Sync(fn)
	})

	return &vsock.VSOCKHostListener{Listener: listener}, err
}
