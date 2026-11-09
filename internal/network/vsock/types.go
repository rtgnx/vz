package vsock

import (
	"net"
	"os"
)

var _ net.Addr = (*VSOCKAddr)(nil)
var _ net.Conn = (*VSOCKConn)(nil)
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
