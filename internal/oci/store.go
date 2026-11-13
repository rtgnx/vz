//go:build darwin

package oci

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"

	"github.com/erofs/go-erofs"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/rtgnx/vz/internal/volume/asif"
	"github.com/rtgnx/vz/internal/volume/types"
)

const (
	OS   = `linux`
	ARCH = `arm64`
)

type ImageStore struct {
	root   string
	fsys   string
	store  layout.Path
	driver types.Driver
}

func NewImageStore(root string) (*ImageStore, error) {
	imageStore := &ImageStore{root: root, driver: asif.New(root)}
	if _, err := os.Stat(filepath.Join(root, `index.json`)); err != nil {
		store, err := layout.Write(root, empty.Index)
		if err != nil {
			return nil, err
		}
		imageStore.store = store
	} else {
		store, err := layout.FromPath(root)
		if err != nil {
			return nil, err
		}
		imageStore.store = store
	}

	return imageStore, nil
}

func (is ImageStore) Pull(ref name.Reference) error {
	platform := v1.Platform{OS: OS, Architecture: ARCH}

	image, err := remote.Image(
		ref, remote.WithPlatform(platform),
	)

	if err != nil {
		return err
	}

	return is.store.AppendImage(
		image,
		layout.WithPlatform(platform),
		layout.WithAnnotations(
			map[string]string{
				"org.opencontainers.image.ref.name": ref.Name(),
			},
		),
	)
}

func (is ImageStore) Open(ref name.Reference) (v1.Image, error) {
	index, err := is.store.ImageIndex()
	if err != nil {
		return nil, err
	}
	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, err
	}
	for _, desc := range manifest.Manifests {
		if desc.Annotations["org.opencontainers.image.ref.name"] == ref.Name() {
			return index.Image(desc.Digest)
		}
	}

	return nil, errors.New("image not found")
}

func (is ImageStore) Config(ref name.Reference) (v1.Config, error) {
	image, err := is.Open(ref)
	if err != nil {
		if err := is.Pull(ref); err != nil {
			return v1.Config{}, err
		}
		if image, err = is.Open(ref); err != nil {
			return v1.Config{}, err
		}
	}

	configFile, err := image.ConfigFile()
	if err != nil {
		return v1.Config{}, err
	}
	return configFile.Config, nil
}

func (is ImageStore) OpenRD(ref name.Reference) (types.Volume, error) {
	volume, err := is.driver.(*asif.Driver).OpenRO(safeName(ref.String()))
	if err != nil {
		if errors.Is(err, types.ErrVolumeNotExists) {
			if err := is.CreateRD(ref); err != nil {
				return nil, err
			}
			return is.driver.(*asif.Driver).OpenRO(safeName(ref.String()))
		} else {
			return nil, err
		}
	}

	return volume, nil
}

func (is ImageStore) CreateRD(ref name.Reference) (err error) {
	image, err := is.Open(ref)
	if err != nil {
		if err := is.Pull(ref); err != nil {
			return err
		}

		if image, err = is.Open(ref); err != nil {
			return err
		}
	}

	size, err := imageSize(image)
	if err != nil {
		return err
	}
	var (
		name      = safeName(ref.String())
		sizeBytes = uint64(size)
	)

	if err = is.driver.Create(name, sizeBytes); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			err = errors.Join(err, is.driver.Delete(name))
		}
	}()

	volume, err := is.driver.Open(name)

	if err != nil {
		return err
	}

	var (
		w   = NewBufferedImageWriter(volume.(*asif.Volume))
		efs = erofs.Create(w, erofs.WithBlockSize(int(asif.DefaultBlockSize)))
	)

	if err = extractImage(efs, image); err != nil {
		return errors.Join(err, volume.Close())
	}

	if err := w.Finish(); err != nil {
		return errors.Join(err, volume.Close())
	}

	return volume.Close()
}

func safeName(in string) (out string) { return base64.StdEncoding.EncodeToString([]byte(in)) }
