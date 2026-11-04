package ipc

import (
	"net"
	"net/rpc"

	"github.com/rtgnx/vz/internal/config"
	"github.com/rtgnx/vz/internal/vm"
	volumetypes "github.com/rtgnx/vz/internal/volume/types"
)

// Client sends RPC requests to a vzd server over a Unix domain socket.
type Client struct {
	conn *rpc.Client
}

// NewClient connects to the RPC server listening at socketPath.
func NewClient(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, err
	}

	return &Client{conn: rpc.NewClient(conn)}, nil
}

// Call invokes the named RPC method and stores its response in reply.
func (c *Client) Call(method string, args, reply any) error {
	return c.conn.Call(method, args, reply)
}

// Close closes the connection to the RPC server.
func (c *Client) Close() error {
	return c.conn.Close()
}

// NetworkCreate asks the server to create the network described by cfg.
func (c *Client) NetworkCreate(cfg config.Network) error {
	return c.Call("Network.Create", NetworkCreateRequest{Network: cfg}, &Empty{})
}

// NetworkDelete asks the server to delete the named network.
func (c *Client) NetworkDelete(name string) error {
	return c.Call("Network.Delete", NetworkDeleteRequest{Name: name}, &Empty{})
}

// NetworkList returns the network definitions known to the server.
func (c *Client) NetworkList() ([]config.Network, error) {
	res := new(NetworkListResponse)
	err := c.Call("Network.List", NetworkListRequest{}, res)
	return res.Networks, err
}

// VolumeCreate asks the server to create the volume described by cfg.
func (c *Client) VolumeCreate(cfg config.Volume) error {
	return c.Call("Volume.Create", VolumeCreateRequest{Volume: cfg}, &Empty{})
}

// VolumeDelete asks the server to delete the named volume.
func (c *Client) VolumeDelete(name string) error {
	return c.Call("Volume.Delete", VolumeDeleteRequest{Name: name}, &Empty{})
}

// VolumeList returns volume statistics reported by the server.
func (c *Client) VolumeList() ([]volumetypes.Stat, error) {
	res := new(VolumeListResponse)
	err := c.Call("Volume.List", VolumeListRequest{}, res)
	return res.Volumes, err
}

// VMMCreate asks the server to persist cfg as a VM definition.
func (c *Client) VMMCreate(cfg config.VM) error {
	return c.Call("VMM.Create", VMCreateRequest{VMConfig: cfg}, &Empty{})
}

// VMMStart asks the server to start the named VM.
func (c *Client) VMMStart(name string) error {
	return c.Call("VMM.Start", VMStartRequest{Name: name}, &Empty{})
}

// VMMStop asks the server to stop the named VM.
func (c *Client) VMMStop(name string) error {
	return c.Call("VMM.Stop", VMStopRequest{Name: name}, &Empty{})
}

// VMMState returns the runtime state reported for the named VM.
func (c *Client) VMMState(name string) (*VMStateResponse, error) {
	res := new(VMStateResponse)
	err := c.Call("VMM.State", VMStateRequest{Name: name}, res)
	return res, err
}

// VMMList returns the VM definitions and runtime information reported by the
// server.
func (c *Client) VMMList() ([]vm.Info, error) {
	res := new(VMListResponse)
	err := c.Call("VMM.List", VMListRequest{}, res)
	return res.VMs, err
}

// VMMDelete asks the server to delete the named VM definition.
func (c *Client) VMMDelete(name string) error {
	return c.Call("VMM.Delete", VMDeleteRequest{Name: name}, &Empty{})
}
