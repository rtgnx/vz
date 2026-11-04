package iscsi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/volume/types"
	"github.com/uiscsi/uiscsi"
)

const Format = "iscsi"

type Driver struct {
	mu      sync.RWMutex
	volumes map[string]config.Volume
}

func NewDriver() *Driver { return &Driver{volumes: make(map[string]config.Volume)} }

func (d *Driver) Configure(cfg config.Volume) {
	d.mu.Lock()
	d.volumes[cfg.Name] = cfg
	d.mu.Unlock()
}

func (d *Driver) Forget(name string) {
	d.mu.Lock()
	delete(d.volumes, name)
	d.mu.Unlock()
}

func (d *Driver) config(name string) (config.Volume, error) {
	d.mu.RLock()
	cfg, ok := d.volumes[name]
	d.mu.RUnlock()
	if !ok {
		return config.Volume{}, types.ErrVolumeNotExists
	}
	if strings.TrimSpace(cfg.Address) == "" || strings.TrimSpace(cfg.Target) == "" {
		return config.Volume{}, fmt.Errorf("iSCSI volume %q requires address and target", name)
	}
	return cfg, nil
}

func dial(ctx context.Context, cfg config.Volume) (*uiscsi.Session, error) {
	opts := []uiscsi.Option{uiscsi.WithTarget(cfg.Target)}
	if cfg.CHAPUser != "" || cfg.CHAPSecret != "" {
		if cfg.CHAPUser == "" || cfg.CHAPSecret == "" {
			return nil, errors.New("both CHAP user and secret must be set")
		}
		opts = append(opts, uiscsi.WithCHAP(cfg.CHAPUser, cfg.CHAPSecret))
	}
	return uiscsi.Dial(ctx, cfg.Address, opts...)
}

func (d *Driver) Create(name string, _ uint64) error {
	cfg, err := d.config(name)
	if err != nil {
		return errors.Join(types.ErrVolumeCreate, err)
	}
	session, err := dial(context.Background(), cfg)
	if err != nil {
		return errors.Join(types.ErrVolumeCreate, err)
	}
	defer session.Close()
	capacity, err := session.SCSI().ReadCapacity(context.Background(), cfg.LUN)
	if err != nil {
		return errors.Join(types.ErrVolumeCreate, err)
	}
	if capacity.BlockSize == 0 || capacity.LogicalBlocks == 0 {
		return errors.Join(types.ErrVolumeCreate, errors.New("target returned invalid capacity"))
	}
	return nil
}

func (d *Driver) Open(name string) (types.Volume, error) {
	cfg, err := d.config(name)
	if err != nil {
		return nil, errors.Join(types.ErrVolumeOpen, err)
	}
	return openVolume(context.Background(), cfg)
}

func (d *Driver) Delete(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.volumes[name]; !ok {
		return types.ErrVolumeNotExists
	}
	delete(d.volumes, name) // Remote targets are never destroyed by deleting a local registration.
	return nil
}

func (d *Driver) List() ([]types.Stat, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	stats := make([]types.Stat, 0, len(d.volumes))
	for _, cfg := range d.volumes {
		stats = append(stats, types.Stat{Name: cfg.Name, Format: "iSCSI", SizeGB: cfg.Size})
	}
	return stats, nil
}
