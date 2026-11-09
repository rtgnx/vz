package network

import (
	"context"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/network/nat"
	"github.com/rtgnx/vz/internal/network/types"
	"github.com/rtgnx/vz/internal/network/vswitch"
)

// Manager coordinates network drivers and persists the configured networks.
type Manager struct {
	networks map[string]config.Network
	drivers  map[string]types.Driver
	mu       sync.Mutex
	root     string
}

// New creates a network manager rooted at root and loads its configured
// networks from networks.yml. Networks that fail to initialize are logged and
// omitted from the manager. The caller should close the manager to persist
// network definitions created during its lifetime.
func New(root string) (*Manager, error) {
	fp := filepath.Join(root, `networks.yml`)

	networks, err := config.Read[[]config.Network](fp)

	if err != nil {
		if os.IsNotExist(err) {
			networks = new([]config.Network)
		}
	}

	manager := &Manager{
		networks: make(map[string]config.Network),
		drivers: map[string]types.Driver{
			"switch": vswitch.NewDriver(context.Background(), ""),
			"nat":    nat.NewDriver(),
		},
		root: root,
	}

	slog.Info("creating default NAT network")

	err = manager.Create(config.Network{
		Name: "default",
		Type: config.NetworkTypeNAT,
	})

	if err != nil {
		slog.Error("creating default NAT network", "err", err)
	}

	for _, cfg := range *networks {

		// Reserved name for default NAT
		if cfg.Name == "default" {
			continue
		}

		slog.Info("creating network", "name", cfg.Name, "type", cfg.Type)
		if err := manager.Create(cfg); err != nil {
			slog.Error("creating network", "name", cfg.Name, "type", cfg.Type, "err", err)
			continue
		}
	}

	return manager, nil
}

// Create creates the network described by cfg using the driver selected by
// cfg.Type. It returns [types.ErrNetworkExists] if a network with cfg.Name is
// already managed, or [types.ErrUnsupportedNetworkType] if no driver supports
// cfg.Type.
func (c *Manager) Create(cfg config.Network) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.networks[cfg.Name]; ok {
		return types.ErrNetworkExists
	}

	if driver, ok := c.drivers[string(cfg.Type)]; ok {
		if err := driver.Create(cfg.Name); err != nil {
			return err
		}

		c.networks[cfg.Name] = cfg
		return nil
	}

	return types.ErrUnsupportedNetworkType
}

// Delete is currently a no-op and returns nil without removing a network.
func (c *Manager) Delete(name string) error { return nil }

// List returns the network definitions currently managed by the manager.
func (c *Manager) List() []config.Network {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Collect(maps.Values(c.networks))
}

// Open opens the named network through its configured driver. It returns
// [types.ErrNetworkNotExist] if the network is not managed, or
// [types.ErrUnsupportedNetworkType] if its configured type has no driver.
func (c *Manager) Open(name string) (types.Network, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if netcfg, ok := c.networks[name]; ok {
		if driver, ok := c.drivers[string(netcfg.Type)]; ok {
			return driver.Open(name)
		}
		return nil, types.ErrUnsupportedNetworkType
	}

	return nil, types.ErrNetworkNotExist
}

// Close persists the manager's current network definitions to networks.yml
// beneath its root directory.
func (c *Manager) Close() error {
	fp := filepath.Join(c.root, `networks.yml`)
	return config.Write(slices.Collect(maps.Values(c.networks)), fp)
}
