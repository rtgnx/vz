package vsock

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

var _ net.Listener = (*VSOCKListener)(nil)

type VSOCKAddr struct {
	cid  uint32
	port uint32
}

type VSOCKConn struct {
	*os.File
	local  VSOCKAddr
	remote VSOCKAddr
}

type VSOCKListener struct {
	fd   int
	addr VSOCKAddr
}

func (l VSOCKListener) Accept() (net.Conn, error) {
	fd, saRemote, err := unix.Accept(l.fd)
	if err != nil {
		return nil, err
	}

	saLocal, err := unix.Getsockname(fd)

	if err != nil {
		return nil, errors.Join(err, unix.Close(fd))
	}

	var (
		local = VSOCKAddr{
			cid:  saLocal.(*unix.SockaddrVM).CID,
			port: saLocal.(*unix.SockaddrVM).Port,
		}
		remote = VSOCKAddr{
			cid:  saRemote.(*unix.SockaddrVM).CID,
			port: saRemote.(*unix.SockaddrVM).Port,
		}
	)

	return VSOCKConn{
		os.NewFile(uintptr(fd), "vsock-client"), local, remote,
	}, nil
}

func (l VSOCKListener) Close() error {
	return unix.Close(l.fd)
}

func (l VSOCKListener) Addr() net.Addr {
	return &l.addr
}

func (conn VSOCKConn) LocalAddr() net.Addr  { return &conn.local }
func (conn VSOCKConn) RemoteAddr() net.Addr { return &conn.remote }

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

func (va *VSOCKAddr) Network() string {
	return `vsock`
}

func (va *VSOCKAddr) String() string {
	cid := strconv.FormatUint(uint64(va.cid), 10)

	if va.cid == unix.VMADDR_CID_ANY {
		cid = "*"
	}

	return fmt.Sprintf("vsock://%s:%d", cid, va.port)
}
