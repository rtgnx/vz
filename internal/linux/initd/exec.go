//go:build linux

package initd

import (
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/rtgnx/vz/internal/config"
)

const (
	SelfExec    = `/proc/self/exe`
	DefaultInit = `/sbin/init`
)

func ChrootInit(cfg config.GuestConfig, logw io.Writer) *exec.Cmd {
	cmd := exec.Command(SelfExec, `init`)

	cmd.Dir = `/`
	cmd.Env = append(os.Environ(), cfg.Env...)

	cmd.Stderr = logw
	cmd.Stdout = logw

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:    true,
		Chroot:     `/new_root`,
		Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
	}

	return cmd
}

func InitExec() error {
	err := runSteps(
		step{"proc", mount("proc", "/proc", "proc", 0555, "")},
		step{"tmpfs", mount("tmpfs", "/run", "tmpfs", 0755, "")},
		step{"sysfs", mount("sysfs", "/sys", "sysfs", 0555, "")},
		step{"devtmpfs", mount("devtmpfs", "/dev", "devtmpfs", 0755, "")},
		step{"devpts", mount("devpts", "/dev/pts", "devpts", 0755, "ptmxmode=0666")},
		step{"synlink ptmx", symlink("pts/ptmx", "/dev/ptmx")},
	)

	if err != nil {
		return err
	}

	return syscall.Exec(DefaultInit, []string{DefaultInit}, os.Environ())
}
