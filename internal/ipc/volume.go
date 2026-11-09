//go:build darwin

package ipc

import (
	"context"

	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/volume"
	"github.com/rtgnx/vz/internal/volume/types"
)

// VolumeService exposes volume manager operations over RPC.
type VolumeService struct {
	manager *volume.Manager
}

// NewVolumeService constructs an RPC service backed by manager.
func NewVolumeService(manager *volume.Manager) *VolumeService {
	return &VolumeService{manager: manager}
}

// VolumeCreateRequest carries a volume definition for the create operation.
type VolumeCreateRequest struct {
	// Volume is the definition to create.
	Volume config.Volume
}

// VolumeDeleteRequest identifies a volume to delete.
type VolumeDeleteRequest = BaseRequest

// VolumeListRequest requests volume statistics.
type VolumeListRequest struct{}

// VolumeListResponse contains the volume statistics returned by the server.
type VolumeListResponse struct {
	// Volumes contains statistics for the volumes reported by the manager.
	Volumes []types.Stat
}

// Create creates the volume supplied in req and reports any manager error to
// the RPC caller.
func (vs *VolumeService) Create(req VolumeCreateRequest, res *Empty) error {
	return vs.manager.Create(context.Background(), req.Volume)
}

// Delete deletes the volume identified by req.Name.
func (vs *VolumeService) Delete(req VolumeDeleteRequest, res *Empty) error {
	return vs.manager.Delete(context.Background(), req.Name)
}

// List writes the manager's volume statistics to res.
func (vs *VolumeService) List(req VolumeListRequest, res *VolumeListResponse) error {
	var err error
	res.Volumes, err = vs.manager.List(context.Background())
	return err
}
