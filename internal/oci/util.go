package oci

import (
	"archive/tar"
	"errors"
	"io"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
)

func imageSize(img v1.Image) (int64, error) {
	archive := mutate.Extract(img)
	defer archive.Close()

	tr := tar.NewReader(archive)
	var total int64

	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return total, nil
		}
		if err != nil {
			return 0, err
		}

		switch h.Typeflag {
		case tar.TypeReg, tar.TypeRegA, tar.TypeGNUSparse:
			total += h.Size
		}
	}
}
