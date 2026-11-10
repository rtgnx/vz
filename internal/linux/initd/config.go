//go:build linux

package initd

import (
	"context"
	"log/slog"
	"net/rpc"
	"time"

	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/network/vsock"
)

type GuestConfigResponse struct {
	Guest config.GuestConfig
}

func ConfigWait(_ context.Context) config.GuestConfig {

	for {
		slog.Info("waiting for image config from host")

		<-time.After(time.Second * 2)
		conn, err := vsock.Dial(7000)

		if err != nil {
			slog.Error("vsock: dailing host on", "port", 7000, "err", err)
			continue
		}

		slog.Info("vsock: connected to host", "local", conn.LocalAddr(), "remote", conn.RemoteAddr())

		rpc := rpc.NewClient(conn)
		config := GuestConfigResponse{}
		if err := rpc.Call(`Host.GuestConfig`, struct{}{}, &config); err != nil {
			slog.Error("[HBUS]: Host.GuestConfig", "err", err)
			continue
		}
		_ = conn.Close()

		return config.Guest
	}

}
