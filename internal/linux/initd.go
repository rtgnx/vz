//go:build linux

package linux

import (
	"context"
	"log/slog"
	"net/rpc"
	"os"
	"syscall"
	"time"

	"github.com/rtgnx/vz/internal/ipc/hbus"
	"github.com/rtgnx/vz/internal/network/vsock"
	"golang.org/x/sys/unix"
)

var (
	stage2 []byte
)

func mountRoot(rd string) {
	// 	must(0, os.Mkdir("/proc", 0555))
	// 	must(0, os.Mkdir("/sys", 0555))
	// 	must(0, os.Mkdir("/dev", 0755))
	// 	must(0, os.Mkdir("/run", 0755))
	//
	// 	must(0, unix.Mount("tmpfs", "/run", "tmpfs", 0, ""))
	//
	// 	if err := os.Mkdir("/new_root", 0755); err != nil && !os.IsExist(err) {
	// 		slog.Error("creating /new_root", "err", err)
	// 		os.Exit(1)
	// 	}
	//
	// 	must(0, unix.Mount("proc", "/proc", "proc", 0, ""))
	// 	must(0, unix.Mount("sysfs", "/sys", "sysfs", 0, ""))
	// 	must(0, unix.Mount("devtmpfs", "/dev", "devtmpfs", 0, ""))
	// 	must(0, unix.Mount(rd, "/new_root", "erofs", 0, ""))
	//
	// 	must(0, os.Mkdir("/new_root/proc", 0555))
	// 	must(0, os.Mkdir("/new_root/sys", 0555))
	// 	must(0, os.Mkdir("/new_root/dev", 0755))
	// 	must(0, os.Mkdir("/new_root/run", 0755))
	//
	// 	must(0, unix.Mount("/proc", "/new_root/proc", "", unix.MS_MOVE, ""))
	// 	must(0, unix.Mount("/sys", "/new_root/sys", "", unix.MS_MOVE, ""))
	// 	must(0, unix.Mount("/dev", "/new_root/dev", "", unix.MS_MOVE, ""))
	// 	must(0, unix.Mount("/run", "/new_root/run", "", unix.MS_MOVE, ""))
	//
	// 	must(0, unix.Chdir("/new_root"))
	// 	must(0, os.Mkdir("/old_root", 0700))
	// 	must(0, unix.PivotRoot("/new_root", "/old_root"))
	// 	must(0, unix.Chdir("/"))
	// 	must(0, unix.Unmount("/old_root", unix.MNT_DETACH))
	// 	must(0, os.Remove("/old_root"))
	//
	// os.Setenv("PATH", "/bin:/usr/bin:/sbin:/usr/sbin")
	// must(0, syscall.Exec("/sbin/stage2", []string{"/sbin/stage2"}, os.Environ()))

}

func Initd(ctx context.Context) error {
	os.Setenv("PATH", "/bin:/usr/bin:/sbin:/usr/sbin")
	must(0, os.Mkdir("/proc", 0555))
	must(0, os.Mkdir("/sys", 0555))
	must(0, os.Mkdir("/dev", 0755))
	must(0, os.Mkdir("/run", 0755))

	must(0, unix.Mount("tmpfs", "/run", "tmpfs", 0, ""))
	must(0, unix.Mount("tmpfs", "/run", "tmpfs", 0, ""))

	if err := os.Mkdir("/new_root", 0755); err != nil && !os.IsExist(err) {
		slog.Error("creating /new_root", "err", err)
		os.Exit(1)
	}

	must(0, unix.Mount("proc", "/proc", "proc", 0, ""))
	must(0, unix.Mount("sysfs", "/sys", "sysfs", 0, ""))
	must(0, unix.Mount("devtmpfs", "/dev", "devtmpfs", 0, ""))
	must(0, unix.Mount(`/dev/vda`, "/new_root", "ext4", 0, ""))

	config := waitHost(ctx)
	slog.Info("[PID1]:", "entrypoint", config.Config.Entrypoint)
	cmd := Exec(ctx, config.Config)
	if err := cmd.Start(); err != nil {
		return err
	}

	for {
		select {
		case <-time.After(time.Minute * 10):
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			return nil

		case <-time.After(time.Minute):
			if cmd.Process != nil {
				slog.Info("process running", "pid", cmd.Process.Pid)
			}

		}
	}

}

func Stage2() {

	must(0, unix.Mount("proc", "/proc", "proc", 0, ""))
	must(0, os.Mkdir("/new_root/dev", 0755))

	syscall.Exec(os.Args[1], os.Args[1:], os.Environ())
}

func waitHost(_ context.Context) hbus.ImageConfig {

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
		config := hbus.ImageConfig{}
		if err := rpc.Call(`Host.GetImageConfig`, hbus.Empty{}, &config); err != nil {
			slog.Error("[HBUS]: Host.GetImageConfig", "err", err)
			continue
		}
		_ = conn.Close()

		return config
	}

}

func must[T any](v T, err error) T {
	if err != nil {
		if os.IsExist(err) {
			return v
		}
		panic(err.Error())
		//slog.Error("failed", "err", err)
		//os.Exit(1)
		//return v
	}

	return v
}
