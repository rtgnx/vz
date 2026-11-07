package vsock

import (
	"context"
	"net"
	"os"

	vz "github.com/tmc/apple/virtualization"
	"golang.org/x/sys/unix"
)

func Dial(ctx context.Context, dev vz.VZVirtioSocketDevice, port uint32) (net.Conn, error) {

	conn, err := dev.ConnectToPort(ctx, port)
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
