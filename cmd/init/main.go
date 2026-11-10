//go:build linux

package main

import (
	"context"
	"log/slog"
	"net/rpc"
	"os"

	"github.com/rtgnx/vz/internal/linux/initd"
	"github.com/rtgnx/vz/internal/network/vsock"
)

func main() {

	if len(os.Args) > 1 {

		if err := initd.Entrypoint(os.Args[1], os.Args[1:]); err != nil {
			slog.Error("[entrypoint]", "err", err)
		}

		return
	}

	listener, err := vsock.Listen(context.Background(), 9999)

	if err != nil {
		slog.Error("[vsock] failed to bind", "port", 9999)
	}

	agent := initd.New()
	agent.Start()

	srv := rpc.NewServer()

	srv.RegisterName("Agent", agent)

	for {
		conn, err := listener.Accept()
		if err != nil {
			slog.Error("[vsock] connetion error")
		}

		srv.ServeConn(conn)
	}
}
