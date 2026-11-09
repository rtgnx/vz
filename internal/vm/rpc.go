//go:build darwin

package vm

import (
	"context"
	"fmt"
	"log/slog"
	"net/rpc"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/rtgnx/vz/internal/ipc/hbus"
)

const (
	HostVSOCKPort uint32 = 7000
)

type Entrypoint struct {
	v1.Config
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
	rpc.RegisterName("Host", &hbus.HostService{ImageConfig: vm.image})
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
