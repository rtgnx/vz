//go:build linux

package initd

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"syscall"

	"github.com/rtgnx/vz/internal/config"
)

const DefaultPathEnv string = `PATH=/bin:/usr/bin:/sbin:/usr/sbin`

func ExecEntrypoint(ctx context.Context, cfg config.GuestConfig, logw io.Writer) error {
	cmd := exec.CommandContext(ctx, `/.init`, cfg.Entrypoint...)

	cmd.Stdout = logw
	cmd.Stderr = logw

	cmd.Dir = cfg.WorkingDir
	cmd.Env = []string{DefaultPathEnv}
	cmd.Env = append(cmd.Env, cfg.Env...)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Chroot:     `/new_root`,
		Cloneflags: syscall.CLONE_NEWPID,
	}

	return cmd.Start()
}

func Entrypoint(argv0 string, argv []string) error {
	slog.Info("[entrypoint]", argv0)
	if err := Mounts([]Mount{
		{`proc`, `/proc`, `proc`, 0555},
		{`devtmpfs`, `/dev`, `devtmpfs`, 0755},
	}).Mount(); err != nil {
		return err
	}
	return syscall.Exec(argv0, argv, os.Environ())
}
