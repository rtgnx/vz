package store

import (
	"context"
	"os"
	"path"
)

const (
	Disks  = "./disks"
	Images = "./images"
	ISOs   = "./iso"
)

func init() {
	InitVMStore(context.Background())
}

func Path(parts ...string) string {
	return path.Join(append([]string{os.ExpandEnv("$HOME/.vm")}, parts...)...)
}

//	func DiskPath(name string) string {
//		return Path(Disks, fmt.Sprintf("%s.asif", name))
//	}
func InitVMStore(_ context.Context) error {
	dirs := []string{
		Path(),
		Path(Disks),
		Path(ISOs),
		Path(Images),
	}

	for _, fp := range dirs {
		if err := os.MkdirAll(fp, 0750); err != nil && !os.IsExist(err) {
			return err
		}
	}

	return nil
}
