package oci

import (
	"fmt"
	"io"
)

type ImageWriter interface {
	io.WriterAt
	Size() int64
}

type BufferedImageWriter struct {
	dst ImageWriter

	head  []byte // First ASIF chunk; retained until Finish.
	batch []byte // Pending sequential output after the first chunk.

	batchOffset int64 // Destination offset of pending batch.
	position    int64 // Current seek position.
	highWater   int64 // Highest byte offset written.

	finalizing bool // Entered when EROFS seeks back to zero.
	finished   bool
	err        error // Sticky failure.
}

func NewBufferedImageWriter(w ImageWriter) *BufferedImageWriter {
	return &BufferedImageWriter{
		dst:         w,
		head:        make([]byte, 1<<20),
		batch:       make([]byte, 0, 32<<20),
		batchOffset: 1 << 20,
	}
}

func (w *BufferedImageWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.finished {
		return 0, fmt.Errorf("image writer: already finished")
	}
	if len(p) == 0 {
		return 0, nil
	}

	size := w.dst.Size()
	if w.position < 0 || w.position > size ||
		int64(len(p)) > size-w.position {
		w.err = fmt.Errorf("image writer: write exceeds capacity %d", size)
		return 0, w.err
	}

	headSize := int64(len(w.head))

	// Once EROFS seeks back, only the retained head may be patched.
	if w.finalizing &&
		(w.position > headSize || int64(len(p)) > headSize-w.position) {
		w.err = fmt.Errorf("image writer: final patch exceeds retained head")
		return 0, w.err
	}

	written := 0

	for len(p) > 0 {
		var n int

		if w.position < headSize {
			n = copy(w.head[int(w.position):], p)
		} else {
			// Everything beyond the retained head must be sequential.
			expected := w.batchOffset + int64(len(w.batch))
			if w.position != expected {
				w.err = fmt.Errorf(
					"image writer: nonsequential write at %d, expected %d",
					w.position, expected,
				)
				return written, w.err
			}

			n = min(len(p), cap(w.batch)-len(w.batch))
			w.batch = append(w.batch, p[:n]...)
		}

		p = p[n:]
		written += n
		w.position += int64(n)
		w.highWater = max(w.highWater, w.position)

		if len(w.batch) == cap(w.batch) {
			if err := w.flushBatch(); err != nil {
				return written, err
			}
		}
	}

	return written, nil
}

func (w *BufferedImageWriter) Seek(offset int64, whence int) (int64, error) {
	if w.err != nil {
		return w.position, w.err
	}
	if w.finished {
		return w.position, fmt.Errorf("image writer: already finished")
	}

	switch {
	case whence == io.SeekCurrent && offset == 0:
		// Query position without changing the write phase.
		return w.position, nil

	case whence == io.SeekStart && offset == 0:
		// Initial positioning is harmless. A later rewind starts
		// the superblock-patching phase.
		if w.highWater > 0 {
			w.finalizing = true
		}
		w.position = 0
		return 0, nil

	default:
		w.err = fmt.Errorf(
			"image writer: unsupported seek offset=%d whence=%d",
			offset, whence,
		)
		return w.position, w.err
	}
}

// Finish must be called only after the EROFS writer closes successfully.
// It does not sync or close the destination.
func (w *BufferedImageWriter) Finish() error {
	if w.err != nil {
		return w.err
	}
	if w.finished {
		return nil
	}

	chunkSize := len(w.head)

	// Pad the final batch to an ASIF chunk boundary. Padding is
	// outside the filesystem payload and does not change highWater.
	if len(w.batch) > 0 {
		padding := (chunkSize - len(w.batch)%chunkSize) % chunkSize
		total := len(w.batch) + padding

		size := w.dst.Size()
		if w.batchOffset > size ||
			int64(total) > size-w.batchOffset {
			w.err = fmt.Errorf("image writer: padded tail exceeds capacity")
			return w.err
		}

		oldLen := len(w.batch)
		w.batch = w.batch[:total]
		clear(w.batch[oldLen:])

		if err := w.flushBatch(); err != nil {
			return err
		}
	}

	// Publish the completed superblock and the rest of the first
	// chunk together, after all later data has been written.
	if err := w.writeAt(w.head, 0); err != nil {
		return err
	}

	w.finished = true
	return nil
}

func (w *BufferedImageWriter) flushBatch() error {
	if len(w.batch) == 0 {
		return nil
	}

	if err := w.writeAt(w.batch, w.batchOffset); err != nil {
		return err
	}

	w.batchOffset += int64(len(w.batch))
	w.batch = w.batch[:0]
	return nil
}

func (w *BufferedImageWriter) writeAt(p []byte, offset int64) error {
	n, err := w.dst.WriteAt(p, offset)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = fmt.Errorf("image writer: write at %d: %w", offset, err)
	}
	return w.err
}
