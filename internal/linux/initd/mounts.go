//go:build linux

package initd

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

type Mount struct {
	Source      string
	Destination string
	Type        string
	Mode        os.FileMode
}

type Mounts []Mount

func DefaultMounts(rootDisk string) Mounts {
	return []Mount{
		{"proc", "/proc", "proc", 0555},
		{"tmpfs", "/run", "tmpfs", 0755},
		{"devtmpfs", "/dev", "devtmpfs", 0755},
		{"devpts", "/dev/pts", "devpts", 0755},
		{"sysfs", "/sys", "sysfs", 0555},
		{"guest-config", "/guest", "virtiofs", 0555},
		{rootDisk, "/new_root", "ext4", 0755},
	}
}

func (m Mount) Mount() error {
	if err := os.MkdirAll(m.Destination, m.Mode); err != nil {
		return err
	}

	options := ""
	if m.Type == "devpts" {
		options = "ptmxmode=0666"
	}

	return unix.Mount(m.Source, m.Destination, m.Type, 0, options)
}

func (m Mounts) Mount() error {
	var errs []error
	for _, mount := range m {
		if err := mount.Mount(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
