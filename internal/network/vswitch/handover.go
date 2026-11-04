package vswitch

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// SWPortHandoverHandler handles one switch control connection. It accepts
// requests to attach a port and transfer its guest file descriptor, or to
// release an existing port.
func SWPortHandoverHandler(conn *net.UnixConn, sw *VSwitch) {
	rd := bufio.NewReader(conn)

	defer conn.Close()

	buf, err := rd.ReadBytes('\n')

	if err != nil {
		slog.Error(err.Error())
		return
	}

	if len(buf) < 2 {
		slog.Error("invalid request", "req", string(buf))
		return
	}

	id := string(buf[1 : len(buf)-1])

	switch buf[0] {
	case 'a':
		port, err := NewSocketPair(id)

		if err != nil {
			slog.Error("failed to create new socket pair", "id", id, "err", err)
			return
		}

		if err := sw.AddPort(port); err != nil {
			slog.Error("failed to add port to vswitch", "err", err)
			return
		}

		if err := HandoverFD(conn, port.GuestFD()); err != nil {
			slog.Error("failed to handover fd", "err", err)
		}

	case 'r':
		if err := sw.RemovePort(id); err != nil {
			slog.Error("failed to detach port from vswitch", "err", err)
			return
		}
	}

}

// SWPortHandover accepts switch control connections and dispatches each
// connection to [SWPortHandoverHandler]. It runs until the listener returns an
// error.
func SWPortHandover(listener *net.UnixListener, sw *VSwitch) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			slog.Error("connection temrinated", "err", err)
			continue
		}

		go SWPortHandoverHandler(conn.(*net.UnixConn), sw)
	}
}

// HandoverFD sends fd to the peer on conn using Unix descriptor passing.
func HandoverFD(conn *net.UnixConn, fd *os.File) error {
	rights := unix.UnixRights(int(fd.Fd()))
	payload := []byte{0} // 0 = success

	n, oobn, err := conn.WriteMsgUnix(payload, rights, nil)
	if err != nil {
		return err
	}

	if n != len(payload) || oobn != len(rights) {
		return io.ErrShortWrite
	}

	return nil
}

// ReceiveFD receives a single file descriptor from conn and wraps it as an
// os.File. It closes received descriptors if the control message is invalid.
func ReceiveFD(conn *net.UnixConn) (*os.File, error) {
	payload := make([]byte, 1)
	oob := make([]byte, unix.CmsgSpace(4))

	n, oobn, flags, _, err := conn.ReadMsgUnix(payload, oob)
	if err != nil {
		return nil, err
	}

	if n != 1 {
		return nil, io.ErrUnexpectedEOF
	}

	if payload[0] != 0 {
		return nil, errors.New("port attachment failed")
	}

	if flags&unix.MSG_CTRUNC != 0 {
		return nil, errors.New("truncated ancillary data")
	}

	msgs, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, err
	}

	var fds []int

	for _, msg := range msgs {
		if msg.Header.Level != unix.SOL_SOCKET ||
			msg.Header.Type != unix.SCM_RIGHTS {
			for _, fd := range fds {
				_ = unix.Close(fd)
			}
			return nil, errors.New("unexpected ancillary message")
		}

		received, err := unix.ParseUnixRights(&msg)
		if err != nil {
			for _, fd := range fds {
				_ = unix.Close(fd)
			}
			return nil, err
		}

		fds = append(fds, received...)
	}

	if len(fds) != 1 {
		for _, fd := range fds {
			_ = unix.Close(fd)
		}
		return nil, fmt.Errorf("expected one FD, received %d", len(fds))
	}

	unix.CloseOnExec(fds[0])

	return os.NewFile(uintptr(fds[0]), "vswitch-guest"), nil
}
