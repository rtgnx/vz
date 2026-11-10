package vswitch

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/network/types"
)

var (
	// ErrSwitchExists indicates that a switch with the requested name already
	// exists.
	ErrSwitchExists = errors.New("switch exists")
	// ErrSwitchNotExists indicates that the requested switch does not exist.
	ErrSwitchNotExists = errors.New("switch does exists")
)

// Driver manages in-process virtual switches.
type Driver struct {
	root string
	ctx  context.Context
	sw   map[string]*VSwitch
	mu   sync.RWMutex
}

// NewDriver returns a switch driver using ctx to control created switches.
func NewDriver(ctx context.Context, root string) types.Driver {
	return &Driver{
		ctx:  ctx,
		root: root,
		sw:   make(map[string]*VSwitch),
	}
}

// Shutdown closes every switch managed by the driver and joins any close
// errors.
func (m *Driver) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	errs := []error{}
	for name, sw := range m.sw {
		if err := sw.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close %s: %w", name, err))
		}
	}

	return errors.Join(errs...)
}

// Open returns the named switch or [types.ErrNetworkNotExist] if it is not
// managed by the driver.
func (m *Driver) Open(name string) (types.Network, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sw, ok := m.sw[name]; ok {
		return sw, nil
	}

	return nil, types.ErrNetworkNotExist
}

// Create creates and registers a switch with the given name. It returns
// [ErrSwitchExists] if that name is already registered.
func (m *Driver) Create(cfg config.Network) (err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sw[cfg.Name]; !ok {
		m.sw[cfg.Name], err = NewVSwitch(m.ctx, cfg.CIDR)

		if err != nil {
			delete(m.sw, cfg.Name)
		}

		return err
	}

	return ErrSwitchExists
}

// List returns the names of switches managed by the driver.
func (m *Driver) List() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Collect(maps.Keys(m.sw))
}

// Close shuts down all switches managed by the driver.
func (m *Driver) Close() error { return m.Shutdown() }

// Delete closes and removes the named switch. If the name is not present, it
// returns [types.ErrNetworkNotExist].
func (m *Driver) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sw, ok := m.sw[name]; ok {
		if err := sw.Close(); err != nil {
			return err
		}
		delete(m.sw, name)
	}

	return types.ErrNetworkNotExist
}
