package vswitch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path"
	"sync"

	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/store"
)

var (
	// ErrSwitchExists indicates that a switch with the requested name already
	// exists.
	ErrSwitchExists = errors.New("switch exists")
	// ErrSwitchNotExists indicates that the requested switch does not exist.
	ErrSwitchNotExists = errors.New("switch does exists")
)

// Manager manages Unix-socket-accessible virtual switches.
type Manager struct {
	root string
	ctx  context.Context
	sw   map[string]*VSwitch
	mu   sync.RWMutex
}

// NewManager creates a switch manager using ctx for switch lifetimes and root
// as the directory for switch sockets.
func NewManager(ctx context.Context, root string) *Manager {
	return &Manager{
		ctx:  ctx,
		root: root,
		sw:   make(map[string]*VSwitch),
	}
}

// Shutdown closes every switch managed by the manager and joins any close
// errors.
func (m *Manager) Shutdown() error {
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

// Create creates a switch named name, listens for port handover requests on a
// Unix socket beneath the manager's root, and writes the network definition to
// the store. It returns [ErrSwitchExists] if the name is already managed.
func (m *Manager) Create(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sw[name]; !ok {
		socketPath := path.Join(m.root, fmt.Sprintf("%s.sock", name))
		os.Remove(socketPath)

		listener, err := net.Listen("unix", socketPath)

		if err != nil {
			return err
		}

		m.sw[name] = NewVSwitch(m.ctx)

		go SWPortHandover(listener.(*net.UnixListener), m.sw[name])
		slog.Info("switch created", "net", name)
		cfg := []config.Network{config.Network{Name: name}}
		for n, _ := range m.sw {
			if n != name {
				cfg = append(cfg, config.Network{Name: n})
			}
		}

		return config.Write(
			cfg, store.Path(`network.yml`),
		)
	}

	return ErrSwitchExists
}

// List returns the names of switches managed by the manager.
func (m *Manager) List(ctx context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := []string{}
	for k, _ := range m.sw {
		names = append(names, k)
	}
	return names, nil
}

// Close currently performs no cleanup and returns nil.
func (m *Manager) Close() error { return nil }

// Delete currently performs no deletion and returns nil.
func (m *Manager) Delete(ctx context.Context, name string) error { return nil }
