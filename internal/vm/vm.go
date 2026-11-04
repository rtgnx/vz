package vm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	vzc "github.com/Code-Hex/vz/v4"
	"github.com/rtgnx/vz/internal/config"
	network "github.com/rtgnx/vz/internal/network/types"
	volume "github.com/rtgnx/vz/internal/volume/types"
	vz "github.com/tmc/apple/virtualization"
	"github.com/tmc/apple/x/vzkit"
	"github.com/tmc/apple/x/vzkit/vnc"
)

var (
	// ErrCannotStart indicates that Virtualization.framework does not permit
	// the VM to start in its current state.
	ErrCannotStart = errors.New("vm cannot start")
	// ErrCannotStop indicates that a graceful shutdown request was not accepted.
	ErrCannotStop = errors.New("vm cannot stop")
	// ErrVMClosed indicates that an operation cannot be performed on a closed
	// VM.
	ErrVMClosed = errors.New("vm already closed")
	// ErrVMRunning indicates that an operation requires a stopped VM but the VM
	// is running.
	ErrVMRunning = errors.New("vm is running")
)

// Info combines a VM's persisted configuration with runtime discovery data.
type Info struct {
	config.VM
	// VNCAddr is the address of the VM's VNC server, when available.
	VNCAddr string
	// State is the VM's current Virtualization.framework runtime state.
	State vzc.VirtualMachineState
}

// VM represents a configured virtual machine and the resources associated
// with its runtime instance.
type VM struct {
	name     string
	cfg      vz.VZVirtualMachineConfiguration
	q        *vzkit.Queue
	vm       vz.VZVirtualMachine
	vnc      *vnc.Server
	volumes  []volume.Volume
	networks []network.Netdev
	released bool
	mu       sync.Mutex
}

// State returns the VM's current Virtualization.framework runtime state.
func (vm *VM) State() vz.VZVirtualMachineState {
	var state vz.VZVirtualMachineState
	vm.q.Sync(func() {
		state = vm.vm.State()
	})

	return state
}

// Stop requests a graceful guest shutdown and waits for the VM to stop. It
// returns when the VM stops, when ctx is canceled, or when the guest rejects
// the shutdown request.
func (vm *VM) Stop(ctx context.Context) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	state := vm.State()

	if vm.released || state == vz.VZVirtualMachineStateStopped {
		return nil
	}

	if state == vz.VZVirtualMachineStateRunning {
		var (
			requested bool
			err       error
		)

		vm.q.Sync(func() {
			requested, err = vm.vm.RequestStopWithError()
		})

		if err != nil || !requested {

			if err != nil {
				return fmt.Errorf("request guest shutdown: %w", err)
			}

			return ErrCannotStop
		}

	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second * 1):

			state = vm.State()
			slog.Info("vm state", "vm", vm.name, "state", state.String())

			if state == vz.VZVirtualMachineStateStopped {
				return nil
			}
		}
	}

}

// Start starts the VM. If the VM is already running, Start returns nil.
func (vm *VM) Start() error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if vm.released {
		return ErrVMClosed
	}

	state := vm.State()

	if state == vz.VZVirtualMachineStateRunning {
		return nil
	}

	if !vzkit.CanStart(vm.q, vm.vm) {
		return ErrCannotStart
	}

	if vm.vnc != nil {
		if _, err := vm.vnc.StartVirtualMachine(vm.vm); err != nil {
			return err
		}
	}

	return await(func(f func(error)) {
		vzkit.StartVM(vm.q, vm.vm, f)
	})

}

// Close releases the VM's network and volume resources. The VM must be
// stopped before its resources can be released.
func (vm *VM) Close() error {

	vm.mu.Lock()
	defer vm.mu.Unlock()

	slog.Info("attempting to release vm resources")
	if vm.State() != vz.VZVirtualMachineStateStopped {
		return ErrVMRunning
	}

	var errs []error

	for i := len(vm.networks) - 1; i >= 0; i-- {
		if err := vm.networks[i].Close(); err != nil {
			errs = append(errs, fmt.Errorf("release network: %w", err))
		}
	}

	for i := len(vm.volumes) - 1; i >= 0; i-- {
		if err := vm.volumes[i].Close(); err != nil {
			errs = append(errs, fmt.Errorf("close volume: %w", err))
		}
	}

	return errors.Join(errs...)
}

// Kill force-stops the VM and waits for the stop operation to complete or ctx
// to be canceled.
func (vm *VM) Kill(ctx context.Context) error {

	vm.mu.Lock()

	if vm.released || vm.State() == vz.VZVirtualMachineStateStopped {
		vm.mu.Unlock()
		return nil
	}

	vm.mu.Unlock()

	done := make(chan error, 1)

	vzkit.StopVM(vm.q, vm.vm, func(err error) {
		done <- err
	})

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("force stop VM: %w", err)
		}
		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}

func await(op func(func(error))) error {
	done := make(chan error, 1)

	op(func(err error) {
		done <- err
	})

	return <-done
}
