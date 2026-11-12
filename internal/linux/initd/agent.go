//go:build linux

package initd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/rtgnx/vz/internal/config"
)

type Agent struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func New() *Agent {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT, syscall.SIGABRT, syscall.SIGTERM,
		syscall.SIGSTOP, syscall.SIGQUIT,
	)

	return &Agent{ctx, cancel}
}

func (a *Agent) Start() {

	if err := DefaultMounts(`/dev/vda`).Mount(); err != nil {
		slog.Error("[init] failed to mount filesystems", "err", err)
		return
	}

	if err := os.Symlink("pts/ptmx", "/dev/ptmx"); err != nil && !os.IsExist(err) {
		slog.Error("[init] failed to create /dev/ptmx", "err", err)
		return
	}

	if err := CopyFile(`/new_root/.init`, `/init`); err != nil {
		slog.Error("[init] copy init onto container root", `err`, err)
		return
	}

	config, err := config.Read[config.GuestConfig](`/guest/guest.yml`)

	if err != nil {
		slog.Error("[init] reading guest config", "err", err.Error())
		return
	}

	Network(a.ctx, NetworkConfig{Hostname: config.Hostname})

	keys, err := ParseSSHKeys(config.SSHPubKeys...)
	if err != nil {
		slog.Error("[ssh] parsing public keys", "err", err.Error())
	} else {
		go ServeSSH(a.ctx, keys)
	}

	//go Shell(a.ctx)
	//go ServeSSH(a.ctx, nil)

	if err := ExecEntrypoint(
		a.ctx, *config, os.Stdout,
	); err != nil {
		slog.Error("[init:entrypoint]", "err", err)
		return
	}

}

func (a *Agent) Stop(_, _ *Empty) error {
	a.cancel()
	return nil
}

type Empty struct{}
