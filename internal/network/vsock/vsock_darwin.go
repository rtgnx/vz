package vsock

import (
	"net"
	"os"

	vz "github.com/tmc/apple/virtualization"

	"github.com/tmc/apple/x/vzkit"
	vzvsock "github.com/tmc/apple/x/vzkit/vsock"
	"golang.org/x/sys/unix"
)

var _ net.Listener = (*VSOCKListener)(nil)

type VSOCKHostListener struct {
	*vzvsock.Listener
}

func (vs *VSOCKHostListener) Accept() (net.Conn, error) {
	conn, err := vs.Listener.Accept()

	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (vs *VSOCKHostListener) Addr() net.Addr {
	return &VSOCKAddr{
		cid:  unix.VMADDR_CID_HOST,
		port: vs.Port,
	}
}

func Dial(q *vzkit.Queue, dev vz.VZVirtioSocketDevice, port uint32) (net.Conn, error) {

	var (
		conn *vz.VZVirtioSocketConnection
		err  error
		done = make(chan struct{})
	)

	q.Sync(func() {
		var handler vz.VirtioSocketConnectionErrorHandler = func(c *vz.VZVirtioSocketConnection, e error) {
			conn, err = c, e
			close(done)
		}
		dev.ConnectToPortCompletionHandler(port, handler)
	})

	<-done
	if err != nil {
		return nil, err
	}

	local := VSOCKAddr{cid: unix.VMADDR_CID_HOST, port: conn.SourcePort()}
	remote := VSOCKAddr{cid: unix.VMADDR_CID_ANY, port: conn.DestinationPort()}
	// os.NewFile wraps the same file descriptor that VZVirtioSocketConnection.Close() closes.
	// If both the file and VZ connection close it, the second close could hit a reused fd.
	// If that's the case duplicate the fd and own both separately
	return VSOCKConn{
		os.NewFile(uintptr(conn.FileDescriptor()), `host-fd`),
		local, remote,
	}, nil
}
