package vsock

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

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

func (conn VSOCKConn) LocalAddr() net.Addr { return &conn.local }

func (conn VSOCKConn) RemoteAddr() net.Addr { return &conn.remote }

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
