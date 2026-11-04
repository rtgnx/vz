package ipc

import (
	"net"
	"net/rpc"
	"os"
	"path/filepath"
)

// Server accepts RPC connections over a Unix domain socket.
type Server struct {
	listener   net.Listener
	rpc        *rpc.Server
	socketPath string
}

// NewServer constructs an RPC server that will listen on socketPath when
// Serve is called.
func NewServer(socketPath string) (*Server, error) {
	return &Server{
		rpc:        rpc.NewServer(),
		socketPath: socketPath,
	}, nil
}

// Register registers rcvr under name as an RPC service. Registration errors
// are ignored by the current implementation.
func (s *Server) Register(name string, rcvr any) {
	_ = s.rpc.RegisterName(name, rcvr)
}

// Serve creates the Unix socket, accepts connections, and serves each
// connection concurrently. It returns when the listener fails to accept a
// connection or socket setup fails.
func (s *Server) Serve() error {
	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0755); err != nil {
		return err
	}

	_ = os.Remove(s.socketPath)
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}

	if err := os.Chmod(s.socketPath, 0600); err != nil {
		_ = listener.Close()
		return err
	}

	s.listener = listener
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return err
		}
		go s.rpc.ServeConn(conn)
	}
}

// Close closes the listening socket if Serve has opened it.
func (s *Server) Close() error {
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}
