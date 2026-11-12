//go:build darwin

package ipc

import (
	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/network"
)

// NetworkService exposes network manager operations over RPC.
type NetworkService struct {
	manager *network.Manager
}

// NewNetworkService constructs an RPC service backed by manager.
func NewNetworkService(manager *network.Manager) *NetworkService {
	return &NetworkService{manager: manager}
}

// NetworkCreateRequest carries a network definition for the create operation.
type NetworkCreateRequest struct {
	// Network is the definition to create.
	Network config.Network
}

// NetworkDeleteRequest identifies a network to delete.
type NetworkDeleteRequest = BaseRequest

// NetworkListRequest requests the list of configured networks.
type NetworkListRequest struct{}

// NetworkListResponse contains the configured networks returned by the server.
type NetworkListResponse struct {
	// Networks is the set of network definitions known to the manager.
	Networks []config.Network
}

// Create creates the network supplied in req and reports any manager error to
// the RPC caller.
func (ns *NetworkService) Create(req NetworkCreateRequest, res *Empty) error {
	return ns.manager.Create(req.Network)
}

// Delete deletes the network identified by req.Name.
func (ns *NetworkService) Delete(req NetworkDeleteRequest, res *Empty) error {
	return ns.manager.Delete(req.Name)
}

// List writes the manager's network definitions to res.
func (ns *NetworkService) List(req NetworkListRequest, res *NetworkListResponse) error {
	res.Networks = ns.manager.List()
	return nil
}
