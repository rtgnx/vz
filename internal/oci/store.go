package oci

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/rtgnx/vz/internal/volume/fsys"
	"github.com/rtgnx/vz/internal/volume/types"
)

const (
	OS   = `linux`
	ARCH = `arm64`
)

type ImageStore struct {
	root  string
	fsys  string
	store layout.Path
}

func NewImageStore(root string) (*ImageStore, error) {
	imageStore := &ImageStore{root: root}
	if _, err := os.Stat(filepath.Join(`index.json`)); err != nil {
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

func (is ImageStore) makeExt4Image(ref name.Reference) (string, error) {
	image, err := is.Open(ref)
	fp := filepath.Join(is.root, strings.ReplaceAll(ref.Name(), `/`, `:`)+".ext4")
	if err != nil {
		return fp, err
	}

	size, err := imageSize(image)
	if err != nil {
		return fp, err
	}

	fsys, err := fsys.NewExt4Writer(fp, size)
	if err != nil {
		return fp, err
	}

	return fp, extractImage(fsys, image)
}

func (is ImageStore) CopyToVolume(volume types.Volume, ref name.Reference) error {

	if _, err := is.makeExt4Image(ref); err != nil {
		return err
	}

	fp := filepath.Join(is.root, strings.ReplaceAll(ref.Name(), `/`, `:`)+".ext4")

	fd, err := os.Open(fp)

	if err != nil {
		return err
	}

	defer fd.Close()
	defer volume.Close()
	buf := make([]byte, 1<<20)
	type readerOnly struct {
		io.Reader
	}
	_, err = io.CopyBuffer(volume.Writer(), readerOnly{fd}, buf)
	return err
}
