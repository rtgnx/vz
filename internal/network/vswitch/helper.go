//go:build darwin

package vswitch

import (
	"fmt"
	"net"
	"os"
)

// ReleasePort asks the switch control socket at socketPath to release portId.
func ReleasePort(socketPath string, portId string) error {
	conn, err := net.Dial(`unix`, socketPath)
	if err != nil {
		return err
	}

	defer conn.Close()
	req := fmt.Appendf(nil, "r%s\n", portId)
	if _, err := conn.Write(req); err != nil {
		return err
	}

	return nil
}

// RequestPort asks the switch control socket at socketPath to create portId
// and returns the guest-side file descriptor transferred by the switch.
func RequestPort(socketPath string, portId string) (*os.File, error) {
	conn, err := net.Dial(`unix`, socketPath)
	if err != nil {
		return nil, err
	}

	defer conn.Close()

	req := fmt.Appendf(nil, "a%s\n", portId)

	if _, err := conn.Write(req); err != nil {
		return nil, err
	}

	return ReceiveFD(conn.(*net.UnixConn))
}
