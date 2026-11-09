//go:build linux

package linux

import (
	"context"
	"os"
	"os/exec"
	"syscall"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func Exec(ctx context.Context, cfg v1.Config) *exec.Cmd {
	cmd := exec.CommandContext(ctx, `/.init`, cfg.Entrypoint...)

	cmd.Env = append(cmd.Env, append(os.Environ(), cfg.Env...)...)
	cmd.Dir = cfg.WorkingDir

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID,
		Chroot:     `/new_root`,
	}

	return cmd
}
