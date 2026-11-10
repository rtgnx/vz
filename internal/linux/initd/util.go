//go:build linux

package initd

import (
	"io"
	"os"
)

func CopyFile(dst, src string) error {
	info, err := os.Stat(src)
	if err != nil {
		return nil
	}
	srcFd, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFd.Close()

	dstFd, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer dstFd.Close()

	_, err = io.Copy(dstFd, srcFd)
	if err != nil {
		return err
	}

	return dstFd.Chmod(info.Mode())
}
