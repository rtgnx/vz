package vsock

import (
	"context"
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

func Listen(ctx context.Context, port uint32) (net.Listener, error) {

	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}

	addr := &unix.SockaddrVM{
		CID:  unix.VMADDR_CID_ANY,
		Port: port,
	}

	if err := unix.Bind(fd, addr); err != nil {
		return nil, err
	}
	if err := unix.Listen(fd, 1); err != nil {
		return nil, err
	}
	return VSOCKListener{fd: fd, addr: VSOCKAddr{addr.CID, addr.Port}}, nil
}

func Dial(port uint32) (net.Conn, error) {
	cid := unix.VMADDR_CID_HOST
	fd, err := unix.Socket(
		unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0,
	)

	if err != nil {
		return nil, fmt.Errorf("create vsock socket: %w", err)
	}

	remote := &unix.SockaddrVM{CID: uint32(unix.VMADDR_CID_HOST), Port: port}

	if err := unix.Connect(fd, remote); err != nil {
		_ = unix.Close(fd)

		return nil, fmt.Errorf("connect to vsock://%d:%d: %w", cid, port, err)
	}

	addr, err := unix.Getsockname(fd)

	if err != nil {

		_ = unix.Close(fd)

		return nil, fmt.Errorf("get local vsock address: %w", err)
	}

	local, ok := addr.(*unix.SockaddrVM)

	if !ok {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("unexpected local socket address type %T", addr)
	}

	return VSOCKConn{
		File:   os.NewFile(uintptr(fd), fmt.Sprintf("vsock-%d", port)),
		local:  VSOCKAddr{cid: local.CID, port: local.Port},
		remote: VSOCKAddr{cid: uint32(cid), port: port},
	}, nil
}
