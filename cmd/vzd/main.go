package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rtgnx/vz/internal/ipc"
	"github.com/rtgnx/vz/internal/network"
	"github.com/rtgnx/vz/internal/oci"
	"github.com/rtgnx/vz/internal/store"
	"github.com/rtgnx/vz/internal/vmm"
	"github.com/rtgnx/vz/internal/volume"
)

func main() {
	ctx := context.Background()

	signals := []os.Signal{
		syscall.SIGINT,
		syscall.SIGABRT,
		syscall.SIGTERM,
	}

	ctx, cancel := signal.NotifyContext(ctx, signals...)

	defer cancel()
	slog.Info("initializing volume manager")
	volumeManager, err := volume.NewManager(store.Path(`disks`, `volumes.yml`))

	if err != nil {
		slog.Error("initalizing volume manager", "err", err)
		return
	}

	slog.Info("initializing oci image store")
	imageStore, err := oci.NewImageStore(store.Path(`images`))

	if err != nil {
		slog.Error("initializing oci image store", "err", err)
	}

	slog.Info("initializing network manager")
	networkManager, err := network.New(store.Path())

	if err != nil {
		slog.Error("initalizing network manager", "err", err)
		return
	}

	slog.Info("initializng vm manager")
	vmm := vmm.New(ctx, volumeManager, networkManager, imageStore)

	slog.Info("serving api", "sock", store.Path("vzd.sock"))
	go func() {
		srv, _ := ipc.NewServer(store.Path("vzd.sock"))

		srv.Register("VMM", ipc.NewVMMService(vmm))
		srv.Register("Network", ipc.NewNetworkService(networkManager))
		srv.Register("Volume", ipc.NewVolumeService(volumeManager))

		if err := srv.Serve(); err != nil {
			slog.Error("ipc service", "err", err)
		}

		defer srv.Close()
		cancel()
	}()

	<-ctx.Done()

	sctx, scancel := context.WithTimeout(
		context.Background(), time.Second*60,
	)

	defer scancel()

	vmm.Shutdown(sctx)

	<-sctx.Done()

}
