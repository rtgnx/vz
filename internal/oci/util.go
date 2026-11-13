//go:build darwin

package oci

import (
	"archive/tar"
	"errors"
	"io"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
)

func imageSize(img v1.Image) (int64, error) {
	const (
		blockSize int64 = 4 << 10
		chunkSize int64 = 1 << 20
	)
	alignUp := func(size, alignment int64) int64 {
		return (size + alignment - 1) & ^(alignment - 1)
	}

	archive := mutate.Extract(img)
	defer archive.Close()

	tr := tar.NewReader(archive)
	// Reserve space for fixed filesystem overhead. This is a capacity
	// estimate, not an exact EROFS size calculation.
	total := chunkSize

	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return alignUp(total, chunkSize), nil
		}
		if err != nil {
			return 0, err
		}

		// Allow one block per entry for inode and directory metadata.
		total += blockSize
		switch h.Typeflag {
		case tar.TypeReg, tar.TypeRegA, tar.TypeGNUSparse:
			total += alignUp(h.Size, blockSize)
		case tar.TypeSymlink:
			total += alignUp(int64(len(h.Linkname)), blockSize)
		}
	}
}
