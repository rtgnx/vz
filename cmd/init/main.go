//go:build linux

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/rtgnx/vz/internal/linux"
)

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT, syscall.SIGABRT, syscall.SIGTERM,
		syscall.SIGSTOP, syscall.SIGQUIT,
	)

	defer cancel()

	if len(os.Args) < 2 {
		if err := linux.Initd(ctx); err != nil {
			slog.Error("initd", "err", err)
		}
		return
	} else {
		linux.Stage2()
	}

}
