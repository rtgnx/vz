//go:build linux

package initd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/rtgnx/vz/internal/config"
	"golang.org/x/sys/unix"
)

type step struct {
	msg  string
	step func() error
}

func runSteps(steps ...step) error {
	n := len(steps)
	for i, step := range steps {
		slog.Info(fmt.Sprintf("[%d/%d]: %s", i, n, step.msg))
		if err := step.step(); err != nil {
			slog.Error(fmt.Sprintf("[%d/%d]: %s", i, n, step.msg), "err", err)
			return err
		}
	}

	return nil
}

func Prepare() (*config.GuestConfig, error) {

	err := runSteps(
		step{"proc", mount("proc", "/proc", "proc", 0555, "")},
		step{"tmpfs", mount("tmpfs", "/run", "tmpfs", 0755, "")},
		step{"sysfs", mount("sysfs", "/sys", "sysfs", 0555, "")},
		step{"devtmpfs", mount("devtmpfs", "/dev", "devtmpfs", 0755, "")},
		step{"devpts", mount("devpts", "/dev/pts", "devpts", 0755, "ptmxmode=0666")},
		step{"synlink ptmx", symlink("pts/ptmx", "/dev/ptmx")},
		step{"guest-config", mount("guest-config", "/guest", "virtiofs", 0555, "ro")},
		step{"guest-oci", mount("/dev/vda", "/lower", "erofs", 0555, "ro")},
		step{"guest-rd-fs", mkfs(`/dev/vdb`)},
		step{"guest-rd", mount("/dev/vdb", "/writable", "ext4", 0555, "")},
		step{"guest-workdir", mkdirall(`/writable/workdir`, 0755)},
		step{"guest-upper", mkdirall(`/writable/upper`, 0755)},
		step{"guest-overlay", mount("overlay", "/new_root", "overlay", 0755, "lowerdir=/lower,upperdir=/writable/upper,workdir=/writable/work")},
	)

	if err != nil {
		return nil, err
	}

	return config.Read[config.GuestConfig](`/guest/guest.yml`)
}

func mount(src, dst, fsType string, perm os.FileMode, opts string) func() error {

	return func() error {
		if err := os.MkdirAll(dst, perm); err != nil {
			if !os.IsExist(err) {
				return err
			}
			if err := os.Chmod(dst, perm); err != nil {
				return err
			}
		}

		return unix.Mount(src, dst, fsType, 0, opts)
	}
}

func mkdirall(p string, perm os.FileMode) func() error {
	return func() error {
		return os.MkdirAll(p, perm)
	}
}

func symlink(src, dst string) func() error {
	return func() error {
		return os.Symlink(src, dst)
	}
}

func mkfs(dev string) func() error {
	return func() (err error) {
		d, err := diskfs.Open(dev,
			diskfs.WithOpenMode(diskfs.ReadWriteExclusive),
		)
		if err != nil {
			return fmt.Errorf("open %s: %w", dev, err)
		}
		defer func() {
			err = errors.Join(err, d.Close())
		}()

		existing, detectErr := d.GetFilesystem(0)
		if detectErr == nil {
			return existing.Close()
		}

		_, err = d.CreateFilesystem(disk.FilesystemSpec{
			Partition:   0,
			FSType:      filesystem.TypeExt4,
			VolumeLabel: "writable",
		})
		if err != nil {
			return fmt.Errorf("format %s: %w", dev, err)
		}

		return nil
	}
}
