//go:build darwin

package vm

import (
	"context"
	"fmt"
	"log/slog"
	"net/rpc"

	"github.com/rtgnx/vz/internal/config"
)

const (
	HostVSOCKPort uint32 = 7000
)

type HostService struct {
	guest config.GuestConfig
}

type GuestConfigResponse struct {
	Guest config.GuestConfig
}

func (hs *HostService) GuestConfig(_ *struct{}, res *GuestConfigResponse) error {
	res.Guest = hs.guest
	return nil
}

func (vm *VM) Serve(ctx context.Context) {
	listener, err := vm.Listen(HostVSOCKPort)
	if err != nil {
		slog.Error("vsock listen:", "err", err)
		return
	}

	slog.Info(
		fmt.Sprintf("[%s] (vsock): waiting for guest connection", vm.name),
	)

	rpc := rpc.NewServer()
	rpc.RegisterName("Host", &HostService{
		config.GuestConfig{
			Env:        vm.image.Env,
			Entrypoint: vm.image.Entrypoint,
			Hostname:   vm.name,
			WorkingDir: vm.image.WorkingDir,
		},
	})
	for {
		conn, err := listener.Accept()
		if err != nil {
			slog.Error(fmt.Sprintf("[%s] (vsock): guest connection failed", vm.name), "err", err)
			continue
		}
		slog.Info("vsock connection", "local", conn.LocalAddr(), "remote", conn.RemoteAddr())

		rpc.ServeConn(conn)
	}
}
