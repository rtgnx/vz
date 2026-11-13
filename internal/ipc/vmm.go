//go:build darwin

package ipc

import (
	"context"

	"github.com/Code-Hex/vz/v4"
	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/vm"
	"github.com/rtgnx/vz/internal/vmm"
)

// VMMService exposes virtual machine manager operations over RPC.
type VMMService struct {
	manager *vmm.VMM
}

// NewVMMService constructs an RPC service backed by manager.
func NewVMMService(manager *vmm.VMM) *VMMService {
	return &VMMService{manager: manager}
}

// VMCreateRequest carries a VM definition for the create operation.
type VMCreateRequest struct {
	// VMConfig is the VM definition to persist.
	VMConfig config.VM
}

// VMStartRequest identifies a VM to start.
type VMStartRequest = BaseRequest

// VMStopRequest identifies a VM to stop.
type VMStopRequest struct {
	Name  string
	Force bool
}

// VMStateRequest identifies a VM whose state is requested.
type VMStateRequest = BaseRequest

// VMDeleteRequest identifies a VM definition to delete.
type VMDeleteRequest = BaseRequest

// VMStateResponse contains the runtime state returned by the server.
type VMStateResponse struct {
	// State is the VM's Virtualization.framework runtime state.
	State vz.VirtualMachineState
}

// VMListRequest requests VM definitions and runtime information.
type VMListRequest struct{}

// VMListResponse contains the VM information returned by the server.
type VMListResponse struct {
	// VMs contains persisted VM definitions and their runtime information.
	VMs []vm.Info
}

// Create persists the VM definition supplied in req.
func (vs *VMMService) Create(req VMCreateRequest, res *Empty) error {
	return vs.manager.Create(context.Background(), req.VMConfig)
}

// Start starts the VM identified by req.Name.
func (vs *VMMService) Start(req VMStartRequest, res *Empty) error {
	return vs.manager.Start(context.Background(), req.Name)
}

// Stop stops the VM identified by req.Name.
func (vs *VMMService) Stop(req VMStopRequest, res *Empty) error {
	return vs.manager.Stop(context.Background(), req.Name, req.Force)
}

// State writes the runtime state of the named VM to res.
func (vs *VMMService) State(req VMStateRequest, res *VMStateResponse) error {
	var err error
	res.State, err = vs.manager.State(context.Background(), req.Name)
	return err
}

// List writes the VM definitions and runtime information to res.
func (vs *VMMService) List(req VMListRequest, res *VMListResponse) error {
	var err error
	res.VMs, err = vs.manager.List(context.Background())
	return err
}

// Delete removes the VM definition identified by req.Name.
func (vs *VMMService) Delete(req VMDeleteRequest, res *Empty) error {
	return vs.manager.Delete(context.Background(), req.Name)
}
