package volume

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/volume/asif"
	"github.com/rtgnx/vz/internal/volume/iscsi"
	"github.com/rtgnx/vz/internal/volume/types"
)

const (
	// DefaultVolumeDriver is the default format callers can use for new volumes.
	DefaultVolumeDriver = `asif`
)

var (
	// ErrUnsupportedVolumeFormat indicates that no registered driver supports
	// the requested volume format.
	ErrUnsupportedVolumeFormat = errors.New("unsupported volume format")
)

// Manager coordinates volume drivers and persists the volume catalog.
type Manager struct {
	configPath string
	drivers    map[string]types.Driver
	volumes    map[string]config.Volume
	mu         sync.Mutex
}

// NewManager creates a volume manager using the catalog at configPath.
// It creates the catalog's parent directory if needed and loads any existing
// volume definitions. Call [Manager.Close] to persist catalog changes made
// during its lifetime.
func NewManager(configPath string) (*Manager, error) {
	var (
		volumes   = make(map[string]config.Volume)
		volumeDir = filepath.Dir(configPath)
	)

	if err := os.MkdirAll(volumeDir, 0750); err != nil && !os.IsExist(err) {
		return nil, err
	}

	cfg, err := config.Read[[]config.Volume](configPath)

	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		cfg = new([]config.Volume)
	}

	for _, volume := range *cfg {
		volumes[volume.Name] = volume
	}

	iscsiDriver := iscsi.NewDriver()
	for _, volume := range volumes {
		if strings.EqualFold(volume.Format, iscsi.Format) {
			iscsiDriver.Configure(volume)
		}
	}

	return &Manager{
		configPath: configPath,
		volumes:    volumes,
		mu:         sync.Mutex{},
		drivers: map[string]types.Driver{
			"asif":       asif.New(volumeDir),
			iscsi.Format: iscsiDriver,
		},
	}, nil
}

// Create creates the volume described by cfg using the driver selected by its
// format. The new definition is held by the manager and is persisted when
// [Manager.Close] is called. It returns [ErrUnsupportedVolumeFormat] when no
// driver supports cfg.Format, or [types.ErrVolumeExists] when cfg.Name is
// already present in the catalog.
func (m *Manager) Create(ctx context.Context, cfg config.Volume) error {
	var (
		// normalize format/driver name
		format     = strings.ToLower(cfg.Format)
		driver, ok = m.drivers[format]
	)

	if !ok {
		return ErrUnsupportedVolumeFormat
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.volumes[cfg.Name]; ok {
		return types.ErrVolumeExists
	}
	if configurable, ok := driver.(interface{ Configure(config.Volume) }); ok {
		configurable.Configure(cfg)
	}

	if err := driver.Create(cfg.Name, cfg.Size); err != nil {
		if configurable, ok := driver.(interface{ Forget(string) }); ok {
			configurable.Forget(cfg.Name)
		}
		return err
	}

	m.volumes[cfg.Name] = cfg
	return nil
}

// List returns statistics reported by the registered volume drivers. A driver
// that fails to list its volumes is omitted from the result.
func (m *Manager) List(ctx context.Context) ([]types.Stat, error) {
	stats := []types.Stat{}

	for _, driver := range m.drivers {
		if list, err := driver.List(); err == nil {
			stats = append(stats, list...)
		}
	}

	return stats, nil
}

// Open opens the named volume through its configured driver. It returns
// [types.ErrVolumeNotExists] if the name is not present in the catalog.
func (m *Manager) Open(ctx context.Context, name string) (types.Volume, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cfg, ok := m.volumes[name]; ok {
		format := strings.ToLower(cfg.Format)

		if driver, ok := m.drivers[format]; ok {
			return driver.Open(name)
		}
	}

	return nil, types.ErrVolumeNotExists
}

// Delete removes the named volume through its configured driver and removes
// its catalog entry. The catalog change is persisted when [Manager.Close] is
// called. It returns [types.ErrVolumeNotExists] if the name is not present in
// the catalog.
func (m *Manager) Delete(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cfg, ok := m.volumes[name]; ok {
		format := strings.ToLower(cfg.Format)
		if driver, ok := m.drivers[format]; ok {
			if err := driver.Delete(cfg.Name); err != nil {
				return err
			}

			delete(m.volumes, cfg.Name)
			return nil
		}
	}

	return types.ErrVolumeNotExists
}

// Close persists the manager's current volume catalog to its configured path.
func (m *Manager) Close() error {
	return config.Write(slices.Collect(maps.Values(m.volumes)), m.configPath)
}
