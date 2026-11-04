package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/rtgnx/vz/internal/cli"
	ipc "github.com/rtgnx/vz/internal/ipc"
	"github.com/rtgnx/vz/internal/store"
)

var (
	version = ""
	commit  = ""
)

const (
	DaemonExec = `./bin/vzd`
)

func main() {
	fmt.Printf("version=%s commit=%s\n", version, commit)

	ctx := context.Background()

	ctx, cancel := signal.NotifyContext(
		ctx, syscall.SIGINT, syscall.SIGABRT, syscall.SIGTERM,
	)

	defer cancel()

	socketPath := store.Path(`vzd.sock`)

	client, err := ipc.NewClient(socketPath)

	if err != nil {
		slog.Warn("vzd daemon is not running", "err", err)
		cmd := exec.Command(DaemonExec)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}

		if err := cmd.Start(); err != nil {
			slog.Error("starting vzd daemon", "err", err)
			return
		}

		if err := cmd.Process.Release(); err != nil {
			slog.Error("releasing daemon process", "err", err)
			return
		}

		if client, err = ipc.NewClient(socketPath); err != nil {
			slog.Error("connecting to vzd daemon socket", "err", err)
			return
		}
	}

	defer client.Close()

	err = cli.NewRootCommand(ctx, client).Execute()

	if err != nil {
		slog.Error(err.Error())
	}
}
