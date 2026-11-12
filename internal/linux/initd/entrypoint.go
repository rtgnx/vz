//go:build linux

package initd

import (
	"context"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/rtgnx/vz/internal/config"
)

const DefaultPathEnv string = `PATH=/bin:/usr/bin:/sbin:/usr/sbin`
const DefaultInit = `/sbin/init`

func ExecEntrypoint(ctx context.Context, cfg config.GuestConfig, logw io.Writer) error {

	if len(cfg.Entrypoint) < 1 {
		cfg.Entrypoint = []string{DefaultInit}
	}

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
	if err := Mounts([]Mount{
		{`proc`, `/proc`, `proc`, 0555},
		{`devtmpfs`, `/dev`, `devtmpfs`, 0755},
	}).Mount(); err != nil {
		return err
	}
	return syscall.Exec(argv0, argv, os.Environ())
}
