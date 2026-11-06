package vmm

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Code-Hex/vz/v4"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/discovery"
	"github.com/rtgnx/vz/internal/network"
	"github.com/rtgnx/vz/internal/oci"
	"github.com/rtgnx/vz/internal/store"
	"github.com/rtgnx/vz/internal/vm"
	"github.com/rtgnx/vz/internal/volume"
)

var (
	// ErrVMRunning indicates that an operation requires a stopped VM but the VM
	// is running.
	ErrVMRunning = errors.New("virtual machine is in running state")
	// ErrVMNotExist indicates that the requested VM is not available to the
	// manager.
	ErrVMNotExist = errors.New("virtual machine does not exist")
	// ErrVMExists indicates that a VM with the requested name already exists.
	ErrVMExists = errors.New("virtual machine already exists")
	// ErrCreateVM indicates that creating or saving a VM definition failed.
	ErrCreateVM = errors.New("virtual machine create error")
)

// VMM manages virtual machine lifecycles and coordinates their network and
// volume resources.
type VMM struct {
	ctx     context.Context
	vms     map[string]*vm.VM
	network *network.Manager
	volume  *volume.Manager
	image   *oci.ImageStore
	mu      sync.Mutex
}

// New constructs a virtual machine manager using ctx as its manager context
// and the supplied network and volume managers for VM resources.
func New(ctx context.Context, volume *volume.Manager, network *network.Manager, image *oci.ImageStore) *VMM {
	return &VMM{
		ctx:     ctx,
		vms:     make(map[string]*vm.VM),
		network: network,
		volume:  volume,
		image:   image,
		mu:      sync.Mutex{},
	}
}

// Start builds and starts the named VM. It returns [ErrVMRunning] if that VM
// is already running. If the manager holds a stopped VM instance under the
// name, Start releases that instance before building a replacement.
func (vmm *VMM) Start(ctx context.Context, name string) error {
	vmm.mu.Lock()
	defer vmm.mu.Unlock()

	if v, ok := vmm.vms[name]; ok {

		state := vz.VirtualMachineState(v.State())

		if state == vz.VirtualMachineStateRunning {
			return ErrVMRunning
		} else {
			if err := v.Close(); err != nil {
				slog.Error("failed to release stale vm", "name", name, "err", err)
			}
		}
	}

	bctx := vm.NewBuildContext(ctx, vmm.volume, vmm.network, vmm.image)
	vm, err := vm.New(bctx, name)

	if err != nil {
		return err
	}

	slog.Info("starting virtual machine", "name", name)

	if err := vm.Start(); err != nil {
		slog.Error("starting virtual machine", "name", name, "err", err)
		return err
	}

	vmm.vms[name] = vm

	return nil
}

// Stop gracefully stops the named VM, allowing up to 60 seconds for shutdown
// before attempting to kill it. It then releases the VM's resources and
// removes it from the manager. It returns [ErrVMNotExist] if the VM is not
// currently managed.
func (vmm *VMM) Stop(ctx context.Context, name string) error {
	vmm.mu.Lock()
	vm, ok := vmm.vms[name]
	vmm.mu.Unlock()

	if !ok {
		return ErrVMNotExist
	}

	sctx, scancel := context.WithTimeout(ctx, time.Second*60)
	defer scancel()
	if err := vm.Stop(sctx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)

		slog.Info("force stopping vm", "name", name)
		if err := vm.Kill(ctx); err != nil {
			vmm.mu.Lock()
			delete(vmm.vms, name)
			vmm.mu.Unlock()
			return err
		}

		slog.Info("releasing vm resources", "name", name)
		if err := vm.Close(); err != nil {
			vmm.mu.Lock()
			delete(vmm.vms, name)
			vmm.mu.Unlock()
			return err
		}
	} else {
		slog.Info("releasing vm resources", "name", name)
		if err := vm.Close(); err != nil {
			slog.Error("releasing vm resources", "name", name, "err", err)
			vmm.mu.Lock()
			delete(vmm.vms, name)
			vmm.mu.Unlock()
			return err
		}
	}

	vmm.mu.Lock()
	defer vmm.mu.Unlock()
	delete(vmm.vms, name)

	return nil
}

// State returns the runtime state of the named VM. A VM not currently managed
// by this instance is reported as stopped.
func (vmm *VMM) State(ctx context.Context, name string) (vz.VirtualMachineState, error) {
	vmm.mu.Lock()
	vm, ok := vmm.vms[name]
	defer vmm.mu.Unlock()

	if !ok {
		// since we're doing config read on demand
		return vz.VirtualMachineStateStopped, nil
		//return vz.VirtualMachineStateError, ErrVMNotExist
	}

	return vz.VirtualMachineState(vm.State()), nil
}

// Shutdown stops managed VMs and closes the network and volume managers.
// Errors stopping individual VMs are logged and do not prevent cleanup of the
// remaining VMs; errors closing the network or volume manager are returned.
func (vmm *VMM) Shutdown(ctx context.Context) error {
	vmm.mu.Lock()
	defer vmm.mu.Unlock()
	for name, _ := range vmm.vms {
		vmm.mu.Unlock()
		if err := vmm.Stop(ctx, name); err != nil {
			slog.Warn("stopping vm", "name", name, "err", err)
		}
		vmm.mu.Lock()
	}

	if err := vmm.network.Close(); err != nil {
		slog.Error("stopping network manager", "err", err)
		return err
	}

	if err := vmm.volume.Close(); err != nil {
		return err
	}

	return nil
}

// List reads persisted VM definitions and returns them with their current
// runtime states and VNC addresses when available.
func (vmm *VMM) List(ctx context.Context) ([]vm.Info, error) {
	vms := []vm.Info{}
	entries, err := os.ReadDir(store.Path())

	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		cfgPath := store.Path(entry.Name(), `vm.yml`)
		cfg, err := config.Read[config.VM](cfgPath)
		if err != nil {
			// not a valid vm config dir
			if os.IsNotExist(err) {
				continue
			}
			//slog.Warn("broken vm config", "name", entry.Name(), "err", err)
			continue

		}
		state, _ := vmm.State(ctx, cfg.Name)
		addr := " - "
		if state == vz.VirtualMachineStateRunning {
			addr, _ = discovery.DiscoverVNC(ctx, cfg.Name)
		}

		vms = append(vms, vm.Info{VM: *cfg, VNCAddr: addr, State: state})
	}

	return vms, nil
}

// Create persists cfg as a new VM definition. It returns [ErrVMExists] if a
// definition directory with the same name already exists, and wraps
// [ErrCreateVM] if creating the directory or writing the definition fails.
func (vmm *VMM) Create(ctx context.Context, cfg config.VM) error {

	fp := store.Path(cfg.Name, `vm.yml`)

	if err := os.Mkdir(filepath.Dir(fp), 0750); err != nil {

		if os.IsExist(err) {
			return ErrVMExists
		}

		return errors.Join(ErrCreateVM, err)
	}

	if len(cfg.Boot.Image) > 0 {
		ref, err := name.ParseReference(cfg.Boot.Image)

		if err != nil {
			return err
		}

		if err := vmm.image.Pull(ref); err != nil {
			return err
		}
	}

	if err := config.Write(cfg, fp); err != nil {
		defer os.RemoveAll(filepath.Dir(fp))
		return errors.Join(ErrCreateVM, err)
	}

	return nil

}

// Delete removes the named VM's persisted files if its runtime state is
// stopped. It returns [ErrVMNotExist] for an empty name and [ErrVMRunning] if
// the VM is not stopped.
func (vmm *VMM) Delete(ctx context.Context, name string) error {
	if len(name) < 1 {
		return ErrVMNotExist
	}
	vmDir := store.Path(name)

	state, _ := vmm.State(ctx, name)
	if state != vz.VirtualMachineStateStopped {
		return ErrVMRunning
	}

	return os.RemoveAll(vmDir)
}
