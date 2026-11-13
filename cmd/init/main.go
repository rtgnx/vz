//go:build linux

package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rtgnx/vz/internal/linux/initd"
)

func main() {

	if len(os.Args) > 1 {
		if err := initd.InitExec(); err != nil {
			slog.Error("init exec", "err", err)
			os.Exit(1)
		}
	}

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGTERM, syscall.SIGINT,
	)

	defer cancel()

	config, err := initd.Prepare()
	if err != nil {
		return
	}

	go func() {
		err := initd.WaitPowerButton(ctx, "/dev/input/event0") // Discovered path.
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("power-button listener", "err", err)
			}
			return
		}

		slog.Info("power button pressed")
		cancel() // Enter the same shutdown path as SIGTERM.
	}()

	if err := initd.Network(ctx, config.Hostname); err != nil {
		slog.Error("network", "err", err)
	}

	cmd := initd.ChrootInit(*config, io.MultiWriter(os.Stdout, os.Stderr))

	if err := cmd.Start(); err != nil {
		return
	}

	keys, err := initd.ParseSSHKeys(config.SSHPubKeys...)

	if err != nil {
		slog.Error("[init:sshd] failed to parse ssh keys", "err", err)
	} else {
		go initd.ServeSSH(ctx, keys)
	}

	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
	}()

	select {
	case err := <-exited:
		// Record exit and proceed to guest shutdown.
		_ = err

	case <-ctx.Done():
		// Handle signalling errors in the actual implementation.
		err = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if err != nil {
			slog.Error("signal init", "signal", "SIGTERM", "err", err)
		}

		initd.Shutdown()

		select {
		case <-exited:
		case <-time.After(60 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
	}

}
