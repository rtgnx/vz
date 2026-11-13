//go:build linux

package initd

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
)

// Linux arm64 struct input_event: 24 bytes.
var event struct {
	Seconds      int64
	Microseconds int64
	Type         uint16
	Code         uint16
	Value        int32
}

func WaitPowerButton(ctx context.Context, dev string) error {
	// os.NOBLOCK

	fd, err := os.OpenFile(dev, os.O_RDONLY, 0)

	if err != nil {
		return err
	}

	defer fd.Close()

	stop := context.AfterFunc(ctx, func() {
		_ = fd.Close()
	})

	defer stop()

	for {
		if err := binary.Read(fd, binary.LittleEndian, &event); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("read power button: %w", err)
		}

		const (
			evKey    = 1
			keyPower = 116
			pressed  = 1
		)

		if event.Type == evKey &&
			event.Code == keyPower &&
			event.Value == pressed {
			return nil
		}
	}

	return nil
}
