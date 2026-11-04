package vswitch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

func TestVSwitch(t *testing.T) {
	tmpRoot, err := os.MkdirTemp(os.TempDir(), `vswitch-*`)

	if !assert.NoError(t, err) {
		t.FailNow()
	}

	m := NewManager(t.Context(), tmpRoot)

	if assert.NoError(t, m.Create(t.Context(), `test-switch`)) {
		socketPath := path.Join(tmpRoot, "test-switch.sock")
		port1, err := RequestPort(socketPath, "test-port")
		if assert.NoError(t, err) {
			port2, err := RequestPort(socketPath, "test-port-2")

			if assert.NoError(t, err) {
				assert.NoError(t, testForward(t, port1, port2))
				assert.NoError(t, testBroadcast(t, port1, port2))
				assert.NoError(t, testKnownUnicast(t, port1, port2))
				assert.NoError(t, testUnknownUnicast(t, port1, port2))
			}

		}
	}
}

func testForward(t *testing.T, port1, port2 *os.File) error {
	t.Helper()

	frame := []byte{
		0x02, 0x00, 0x00, 0x00, 0x00, 0x02,
		0x02, 0x00, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x00,
		0xde, 0xad, 0xbe, 0xef,
	}

	if _, err := port1.Write(frame); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}

	poll := []unix.PollFd{{
		Fd:     int32(port2.Fd()),
		Events: unix.POLLIN,
	}}

	n, err := unix.Poll(poll, 1000)
	if err != nil {
		return fmt.Errorf("poll port: %w", err)
	}
	if n == 0 {
		return errors.New("timed out waiting for forwarded frame")
	}

	buf := make([]byte, 1500)
	nr, err := port2.Read(buf)
	if err != nil {
		return fmt.Errorf("read forwarded frame: %w", err)
	}

	if !bytes.Equal(frame, buf[:nr]) {
		return fmt.Errorf("forwarded frame mismatch: got %x, want %x", buf[:nr], frame)
	}

	return nil
}

func testBroadcast(t *testing.T, port1, port2 *os.File) error {
	t.Helper()

	frame := []byte{
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0x02, 0x00, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x00,
		0xde, 0xad, 0xbe, 0xef,
	}

	return writeAndReceive(port1, port2, frame)
}

func testUnknownUnicast(t *testing.T, port1, port2 *os.File) error {
	t.Helper()

	frame := []byte{
		0x02, 0x00, 0x00, 0x00, 0x00, 0x99,
		0x02, 0x00, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x00,
		0xde, 0xad, 0xbe, 0xef,
	}

	return writeAndReceive(port1, port2, frame)
}

func testKnownUnicast(t *testing.T, port1, port2 *os.File) error {
	t.Helper()

	mac1 := []byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x01}
	mac2 := []byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x02}

	// Port 2 announces its source MAC.
	learn := append(append([]byte{}, mac1...), mac2...)
	learn = append(learn, 0x08, 0x00, 0xde, 0xad)

	if err := writeAndReceive(port2, port1, learn); err != nil {
		return fmt.Errorf("learn destination MAC: %w", err)
	}

	// Port 1 sends to port 2.
	frame := append(append([]byte{}, mac2...), mac1...)
	frame = append(frame, 0x08, 0x00, 0xbe, 0xef)

	return writeAndReceive(port1, port2, frame)
}

func writeAndReceive(src, dst *os.File, frame []byte) error {
	if _, err := src.Write(frame); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}

	poll := []unix.PollFd{{
		Fd:     int32(dst.Fd()),
		Events: unix.POLLIN,
	}}

	n, err := unix.Poll(poll, 1000)
	if err != nil {
		return fmt.Errorf("poll destination: %w", err)
	}
	if n == 0 {
		return errors.New("timed out waiting for frame")
	}

	buf := make([]byte, 1500)
	n, err = dst.Read(buf)
	if err != nil {
		return fmt.Errorf("read frame: %w", err)
	}

	if !bytes.Equal(frame, buf[:n]) {
		return fmt.Errorf("frame mismatch: got %x, want %x", buf[:n], frame)
	}

	return nil
}

func BenchmarkForward(b *testing.B) {
	for _, size := range []int{64, 512, 1500, 9000} {
		b.Run(fmt.Sprintf("%dB", size), func(b *testing.B) {
			tmpRoot := b.TempDir()
			ctx := context.Background()

			m := NewManager(ctx, tmpRoot)
			if err := m.Create(ctx, "bench"); err != nil {
				b.Fatal(err)
			}
			defer m.Shutdown()

			port1, err := RequestPort(filepath.Join(tmpRoot, "bench.sock"), "port-1")
			if err != nil {
				b.Fatal(err)
			}
			defer port1.Close()

			port2, err := RequestPort(filepath.Join(tmpRoot, "bench.sock"), "port-2")
			if err != nil {
				b.Fatal(err)
			}
			defer port2.Close()

			frame := make([]byte, size)

			// Unknown unicast destination, stable source MAC.
			copy(frame[0:6], []byte{0x02, 0, 0, 0, 0, 99})
			copy(frame[6:12], []byte{0x02, 0, 0, 0, 0, 1})
			frame[12] = 0x08
			frame[13] = 0x00

			buf := make([]byte, size+100)

			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()

			done := make(chan error, 1)

			// Continuously drain the receiving port.
			go func() {
				for i := 0; i < b.N; i++ {
					n, err := port2.Read(buf)
					if err != nil {
						done <- err
						return
					}
					if n != size {
						done <- fmt.Errorf("unexpected frame size: got %d, want %d", n, size)
						return
					}
				}
				done <- nil
			}()

			// Continuously send frames.
			for i := 0; i < b.N; i++ {
				if _, err := port1.Write(frame); err != nil {
					b.Fatal(err)
				}
			}

			if err := <-done; err != nil {
				b.Fatal(err)
			}
		})
	}
}
