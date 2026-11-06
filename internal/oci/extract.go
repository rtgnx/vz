package oci

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/rtgnx/vz/internal/volume/fsys"
)

func extractImage(dst fsys.ImageFS, image v1.Image) error {
	archive := mutate.Extract(image)
	tr := tar.NewReader(archive)
	copyBuf := make([]byte, 1<<20)
	var hardlinks []*tar.Header

	removeExisting := func(name string) error {
		if _, err := dst.Stat(name); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		return dst.RemoveAll(name)
	}
	setMetadata := func(name string, header *tar.Header, symlink bool) error {
		if !symlink {
			if err := dst.Chmod(name, os.FileMode(header.Mode)); err != nil {
				return err
			}
			if err := dst.Chown(name, header.Uid, header.Gid); err != nil {
				return err
			}
		}
		mtime := header.ModTime
		if mtime.IsZero() {
			mtime = time.Unix(0, 0)
		}
		return dst.SetModTime(name, mtime)
	}

	extractErr := func() error {
		for {
			header, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return fmt.Errorf("read flattened OCI tar: %w", err)
			}

			name := path.Clean(strings.TrimPrefix(header.Name, "./"))
			if name == "." || name == "" {
				continue
			}
			if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
				return fmt.Errorf("OCI path escapes image root: %q", header.Name)
			}
			if header.Typeflag == tar.TypeXHeader || header.Typeflag == tar.TypeXGlobalHeader {
				continue
			}
			if header.Typeflag == tar.TypeChar && header.Devmajor == 0 && header.Devminor == 0 {
				if err := removeExisting(name); err != nil {
					return fmt.Errorf("remove whiteout %q: %w", name, err)
				}
				continue
			}

			parent := path.Dir(name)
			if parent != "." && parent != "/" {
				if err := dst.MkdirAll(parent, 0o755); err != nil {
					return fmt.Errorf("create parent for %q: %w", name, err)
				}
			}

			switch header.Typeflag {
			case tar.TypeDir:
				if err := removeExistingIfNotDir(dst, name); err != nil {
					return fmt.Errorf("prepare directory %q: %w", name, err)
				}
				if err := dst.MkdirAll(name, os.FileMode(header.Mode)); err != nil {
					return fmt.Errorf("create directory %q: %w", name, err)
				}
				if err := setMetadata(name, header, false); err != nil {
					return fmt.Errorf("set metadata for %q: %w", name, err)
				}
			case tar.TypeReg, tar.TypeRegA, tar.TypeGNUSparse:
				if err := removeExisting(name); err != nil {
					return fmt.Errorf("replace %q: %w", name, err)
				}
				file, err := dst.CreateFile(name)
				if err != nil {
					return fmt.Errorf("create file %q: %w", name, err)
				}
				_, copyErr := io.CopyBuffer(file, tr, copyBuf)
				closeErr := file.Close()
				if copyErr != nil {
					return fmt.Errorf("write file %q: %w", name, copyErr)
				}
				if closeErr != nil {
					return fmt.Errorf("close file %q: %w", name, closeErr)
				}
				if err := setMetadata(name, header, false); err != nil {
					return fmt.Errorf("set metadata for %q: %w", name, err)
				}
			case tar.TypeSymlink:
				if err := removeExisting(name); err != nil {
					return fmt.Errorf("replace %q: %w", name, err)
				}
				if err := dst.Symlink(header.Linkname, name); err != nil {
					return fmt.Errorf("create symlink %q: %w", name, err)
				}
				if err := setMetadata(name, header, true); err != nil {
					return fmt.Errorf("set metadata for symlink %q: %w", name, err)
				}
			case tar.TypeLink:
				linkName := path.Clean(strings.TrimPrefix(header.Linkname, "./"))
				if path.IsAbs(linkName) || linkName == ".." || strings.HasPrefix(linkName, "../") {
					return fmt.Errorf("hardlink target escapes image root: %q", header.Linkname)
				}
				copy := *header
				copy.Name, copy.Linkname = name, linkName
				hardlinks = append(hardlinks, &copy)
			default:
				return fmt.Errorf("unsupported OCI entry type %d for %q", header.Typeflag, name)
			}
		}

		for _, header := range hardlinks {
			if parent := path.Dir(header.Name); parent != "." && parent != "/" {
				if err := dst.MkdirAll(parent, 0o755); err != nil {
					return fmt.Errorf("create parent for hardlink %q: %w", header.Name, err)
				}
			}
			if err := removeExisting(header.Name); err != nil {
				return fmt.Errorf("replace hardlink %q: %w", header.Name, err)
			}
			if err := dst.Link(header.Linkname, header.Name); err != nil {
				return fmt.Errorf("create hardlink %q: %w", header.Name, err)
			}
			if err := setMetadata(header.Name, header, false); err != nil {
				return fmt.Errorf("set metadata for hardlink %q: %w", header.Name, err)
			}
		}
		return nil
	}()

	archiveErr := archive.Close()
	return errors.Join(extractErr, archiveErr, dst.Close())
}

func removeExistingIfNotDir(dst fsys.ImageFS, name string) error {
	info, err := dst.Stat(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	return dst.RemoveAll(name)
}
