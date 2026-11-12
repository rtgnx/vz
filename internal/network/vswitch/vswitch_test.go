//go:build darwin

package vswitch

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
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

	if assert.NoError(t, m.Create(t.Context(), `test-switch`, `10.1.1.0/24`)) {
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

	buf := make([]byte, 9000)
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
	const totalBytes = 1 << 20

	for _, size := range []int{64, 512, 1500, 9000} {
		b.Run(fmt.Sprintf("%dB", size), func(b *testing.B) {
			sw, _ := NewVSwitch(b.Context(), "10.1.1.0/24")
			defer sw.Close()
			p1, err := sw.Open("p1")
			if err != nil {
				b.Fatal(err)
			}
			p2, err := sw.Open("p2")
			if err != nil {
				_ = p1.Close()
				b.Fatal(err)
			}

			defer sw.Close()

			port1 := p1.(*PortLease).port
			port2 := p2.(*PortLease).port
			fd1 := port1.GuestFD()
			fd2 := port2.GuestFD()

			mac1 := []byte{0x02, 0, 0, 0, 0, 1}
			mac2 := []byte{0x02, 0, 0, 0, 0, 2}

			makeFrame := func(dst, src []byte) []byte {
				frame := make([]byte, size)
				copy(frame[0:6], dst)
				copy(frame[6:12], src)
				frame[12], frame[13] = 0x08, 0x00
				return frame
			}

			frame1 := makeFrame(mac2, mac1)
			frame2 := makeFrame(mac1, mac2)

			// Prime the switch's MAC table in both directions.
			if err := writeAndReceive(fd1, fd2, frame1); err != nil {
				b.Fatal(err)
			}
			if err := writeAndReceive(fd2, fd1, frame2); err != nil {
				b.Fatal(err)
			}

			framesPerDirection := totalBytes / (2 * size)
			bytesPerIteration := int64(2 * framesPerDirection * size)

			b.SetBytes(bytesPerIteration)
			b.ReportAllocs()
			b.ResetTimer()

			buf1 := make([]byte, size)
			buf2 := make([]byte, size)

			for i := 0; i < b.N; i++ {
				errs := make(chan error, 4)
				var wg sync.WaitGroup
				wg.Add(4)

				readFrames := func(fd *os.File, buf []byte) {
					defer wg.Done()
					for j := 0; j < framesPerDirection; j++ {
						n, err := fd.Read(buf)
						if err != nil {
							errs <- err
							return
						}
						if n != size {
							errs <- fmt.Errorf("read %d bytes, want %d", n, size)
							return
						}
					}
				}

				writeFrames := func(fd *os.File, frame []byte) {
					defer wg.Done()
					for j := 0; j < framesPerDirection; j++ {
						n, err := fd.Write(frame)
						if err != nil {
							errs <- err
							return
						}
						if n != size {
							errs <- fmt.Errorf("wrote %d bytes, want %d", n, size)
							return
						}
					}
				}

				go readFrames(fd1, buf1)
				go readFrames(fd2, buf2)
				go writeFrames(fd1, frame1)
				go writeFrames(fd2, frame2)

				wg.Wait()
				close(errs)
				for err := range errs {
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

func BenchmarkVSwitchLearnHWAddr(b *testing.B) {
	vs := &VSwitch{
		ports:   map[string]Port{"source": nil},
		hwTable: make(map[HWAddr]string, 1),
	}
	addr := HWAddr{0x02, 0, 0, 0, 0, 1}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		vs.learnHWAddr("source", addr)
	}
}
