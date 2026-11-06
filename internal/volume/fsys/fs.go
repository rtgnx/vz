package fsys

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"time"

	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem/ext4"
	"github.com/erofs/go-erofs"
)

// ImageFS defines the filesystem operations required to extract and inspect an
// image using either EROFS or ext4.
type ImageFS interface {
	fs.FS
	io.Closer

	// Stat returns metadata for the named filesystem entry.
	Stat(name string) (fs.FileInfo, error)
	// ReadDir returns the entries in the named directory.
	ReadDir(name string) ([]fs.DirEntry, error)

	// CreateFile creates or replaces a regular file and returns a writer for it.
	CreateFile(name string) (io.WriteCloser, error)
	// MkdirAll creates name and any missing parent directories.
	MkdirAll(name string, perm fs.FileMode) error
	// RemoveAll removes name and any entries beneath it.
	RemoveAll(name string) error

	// Symlink creates a symbolic link named name with target as its contents.
	Symlink(target, name string) error
	// Link creates an entry named newname from the entry oldname. An
	// implementation may copy the entry instead of preserving a shared inode.
	Link(oldname, newname string) error

	// Chmod changes the permission bits for name.
	Chmod(name string, mode fs.FileMode) error
	// Chown changes the owner and group for name.
	Chown(name string, uid, gid int) error
	// SetModTime sets the modification time for name.
	SetModTime(name string, mtime time.Time) error
}

// EroFS adapts an EROFS writer to the ImageFS interface.
type EroFS struct {
	*erofs.Writer
}

// Ext4Writer adapts a writable ext4 filesystem to the ImageFS interface.
type Ext4Writer struct {
	*ext4.FileSystem
}

// ReadDir returns the entries in name from the EROFS filesystem.
func (fsys *EroFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(fsys.Writer, name)
}

// CreateFile creates a file in EROFS and returns a writer for its contents.
func (fsys *EroFS) CreateFile(name string) (io.WriteCloser, error) {
	return fsys.Writer.Create(name)
}

// CreateFile creates or truncates a regular file in ext4 and returns a writer
// for its contents.
func (fsys *Ext4Writer) CreateFile(name string) (io.WriteCloser, error) {
	return fsys.FileSystem.OpenFile(name, os.O_CREATE|os.O_RDWR|os.O_TRUNC)
}

// Stat returns ext4 metadata for name. It translates go-diskfs's missing-file
// error into fs.ErrNotExist so callers can use errors.Is.
func (fsys *Ext4Writer) Stat(name string) (fs.FileInfo, error) {
	info, err := fsys.FileSystem.Stat(name)
	if err != nil && err.Error() == "file does not exist: "+name {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return info, err
}

// Link copies oldname to newname. go-diskfs does not implement ext4 hard links,
// so this creates an independent file rather than a shared inode.
func (fsys *Ext4Writer) Link(oldname, newname string) error {
	src, err := fsys.FileSystem.Open(oldname)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := fsys.CreateFile(newname)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	return errors.Join(copyErr, closeErr)
}

// MkdirAll creates name and any missing parent directories in EROFS. Existing
// directories are left unchanged; an existing non-directory entry is an error.
func (fsys *EroFS) MkdirAll(name string, perm fs.FileMode) error {
	if name == "" || name == "." || name == "/" {
		return nil
	}

	if info, err := fsys.Stat(name); err == nil {
		if info.IsDir() {
			return nil
		}
		return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrExist}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	parent := path.Dir(name)
	if parent != "." && parent != "/" {
		if err := fsys.MkdirAll(parent, perm); err != nil {
			return err
		}
	}
	return fsys.Writer.Mkdir(name, perm)
}

// MkdirAll creates name and any missing parent directories in ext4. Existing
// directories are left unchanged; an existing non-directory entry is an error.
// The ext4 backend uses its default permissions and does not apply perm.
func (fsys *Ext4Writer) MkdirAll(name string, perm fs.FileMode) error {
	if name == "" || name == "." || name == "/" {
		return nil
	}

	if info, err := fsys.Stat(name); err == nil {
		if info.IsDir() {
			return nil
		}
		return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrExist}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	parent := path.Dir(name)
	if parent != "." && parent != "/" {
		if err := fsys.MkdirAll(parent, perm); err != nil {
			return err
		}
	}
	return fsys.FileSystem.Mkdir(name)
}

// RemoveAll removes name and, when it is a directory, recursively removes its
// contents first. A missing name is treated as success.
func (fsys *Ext4Writer) RemoveAll(name string) error {
	info, err := fsys.Stat(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	if info.IsDir() {
		entries, err := fsys.ReadDir(name)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := fsys.RemoveAll(path.Join(name, entry.Name())); err != nil {
				return err
			}
		}
	}

	return fsys.FileSystem.Remove(name)
}

// SetModTime sets the access and modification times for name in EROFS.
func (fsys *EroFS) SetModTime(name string, mtime time.Time) error {
	return fsys.Writer.Chtimes(name, mtime, mtime)
}

// SetModTime sets the access, modification, and change times for name in ext4.
func (fsys *Ext4Writer) SetModTime(name string, mtime time.Time) error {
	return fsys.FileSystem.Chtimes(name, mtime, mtime, mtime)
}

// NewExt4Writer creates an ext4 filesystem at fp with the requested byte size.
func NewExt4Writer(fp string, size int64) (ImageFS, error) {
	fd, err := os.OpenFile(fp, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	fs, err := ext4.Create(file.New(fd, false), size, 0, 512, nil)
	return &Ext4Writer{fs}, err
}

// NewEroFSWriter creates an EROFS writer at fp. The size argument is accepted
// for compatibility with NewExt4Writer and is currently ignored.
func NewEroFSWriter(fp string, _ int64) (ImageFS, error) {
	fd, err := os.OpenFile(fp, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	return &EroFS{erofs.Create(fd)}, err
}
