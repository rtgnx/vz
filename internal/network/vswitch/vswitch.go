package vswitch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/rtgnx/vz/internal/network/types"
)

var (
	// ErrPortExists indicates that a port with the requested identifier is
	// already attached to the switch.
	ErrPortExists = errors.New("port id already exists")
	// ErrInvalidEthernetFrame indicates that a frame is too short to contain an
	// Ethernet header.
	ErrInvalidEthernetFrame = errors.New("invalid ethernet frame")
)

const (
	// sendBuffer is the socket send buffer size in bytes.
	sendBuffer = 8 * 1024 * 1024
	// recvBuffer is the socket receive buffer size in bytes.
	recvBuffer = 32 * 1024 * 1024
)

// HWAddr is a six-byte Ethernet hardware address.
type HWAddr [6]byte

// Port represents one bidirectional Ethernet-frame endpoint attached to a
// virtual switch.
type Port interface {
	// ID returns the unique identifier of the port within its switch.
	ID() string
	// Close releases the port's resources.
	Close() error
	// GuestFD returns the endpoint used by the virtual machine.
	GuestFD() *os.File
	// HostFD returns the endpoint used by the switch to read and forward frames.
	HostFD() *os.File
}

// VSwitch forwards Ethernet frames between attached ports and learns source
// hardware addresses to forward unicast traffic selectively.
type VSwitch struct {
	ctx     context.Context
	cancel  context.CancelFunc
	ports   map[string]Port
	hwTable map[HWAddr]string
	mu      sync.RWMutex
	all     []Port
}

// NewVSwitch creates an empty virtual switch whose lifetime is canceled when
// ctx is canceled.
func NewVSwitch(ctx context.Context) *VSwitch {
	cctx, cancel := context.WithCancel(ctx)

	return &VSwitch{
		ctx: cctx, cancel: cancel,
		ports:   make(map[string]Port),
		hwTable: make(map[HWAddr]string),
		mu:      sync.RWMutex{},
	}
}

// flood returns all attached ports except the source port. The caller must
// hold vs.mu for reading or writing.
func (vs *VSwitch) flood(srcId string) (ports []Port) {
	// this method assumes it's being called under lock
	for id, p := range vs.ports {
		if id == srcId {
			continue
		}
		ports = append(ports, p)
	}

	return ports
}

// lookup returns the known destination port for a unicast address, or the
// eligible ports to flood when the address is multicast or unknown.
func (vs *VSwitch) lookup(srcId string, dstAddr HWAddr) (dstPorts []Port) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	// Multicast and broadcast.
	if dstAddr[0]&1 != 0 {
		return vs.flood(srcId)
	}

	if dstId, ok := vs.hwTable[dstAddr]; ok {
		if p, ok := vs.ports[dstId]; ok && dstId != srcId {
			dstPorts = append(dstPorts, p)
			return dstPorts
		}
	}
	return vs.flood(srcId)
}

// learnHWAddr records a unicast source address for an attached port. Zero and
// multicast addresses are ignored.
func (vs *VSwitch) learnHWAddr(srcId string, addr HWAddr) {
	// Ignore multicast and zero source addresses.
	if addr == (HWAddr{}) || addr[0]&1 != 0 {
		return
	}

	vs.mu.Lock()
	defer vs.mu.Unlock()

	if _, ok := vs.ports[srcId]; ok {
		vs.hwTable[addr] = srcId
	}
}

// Forward forwards an Ethernet frame received from srcId to its destination
// port, or floods it when the destination is multicast, broadcast, or unknown.
// It learns valid unicast source addresses from forwarded frames.
func (vs *VSwitch) Forward(srcId string, frame []byte) error {
	if len(frame) < 14 {
		return ErrInvalidEthernetFrame
	}

	var dst, src HWAddr
	copy(dst[:], frame[0:6])
	copy(src[:], frame[6:12])

	vs.learnHWAddr(srcId, src)

	switch {
	case dst[0]&1 != 0:
		vs.mu.RLock()
		all := vs.all
		vs.mu.RUnlock()
		return forward(srcId, all, frame)
	default:
		dstPorts := vs.lookup(srcId, dst)
		return forward(srcId, dstPorts, frame)

	}
}

// forward writes frame to each port other than srcId and joins any write
// errors.
func forward(srcId string, ports []Port, frame []byte) error {
	errs := []error{}
	for _, port := range ports {
		if port.ID() == srcId {
			continue
		}
		_, err := port.HostFD().Write(frame)
		if err != nil {
			errs = append(errs, fmt.Errorf("error forwarding src=%s, dst=%s : %w", srcId, port.ID(), err))
		}
	}
	return errors.Join(errs...)
}

// read consumes frames from p until the switch context is canceled or the
// endpoint returns an error, forwarding each frame through the switch.
func (vs *VSwitch) read(p Port) {
	buf := make([]byte, 65536)

	for {
		select {
		case <-vs.ctx.Done():
			return
		default:
			n, err := p.HostFD().Read(buf)
			if err != nil {
				if errors.Is(err, os.ErrClosed) {
					if err := vs.RemovePort(p.ID()); err != nil {
						slog.Error("port disconnected", "id", p.ID(), "err", err)
					}
				}
				return
			}

			if err := vs.Forward(p.ID(), buf[:n]); err != nil {
				slog.Error("error while forwarding packet", "src", p.ID(), "buflen", n)
			}
		}
	}
}

// AddPort attaches p to the switch and starts reading frames from its host
// endpoint. It returns [ErrPortExists] if a port with the same identifier is
// already attached.
func (vs *VSwitch) AddPort(p Port) error {
	vs.mu.Lock()
	if _, ok := vs.ports[p.ID()]; ok {
		vs.mu.Unlock()
		return ErrPortExists
	}

	vs.ports[p.ID()] = p
	vs.all = append(vs.all, p) // fast path index all ports
	vs.mu.Unlock()
	// Dispatch read
	go vs.read(p)
	slog.Info("port connected", "port", p.ID())
	return nil
}

// Close cancels the switch, detaches all ports, and closes their resources.
func (vs *VSwitch) Close() error {
	vs.cancel()

	vs.mu.Lock()
	ports := vs.ports
	vs.ports = make(map[string]Port)
	vs.hwTable = make(map[HWAddr]string)
	vs.mu.Unlock()

	var errs []error

	for _, port := range ports {
		if err := port.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// RemovePort detaches and closes the port identified by id. It succeeds when
// no port with that identifier is attached.
func (vs *VSwitch) RemovePort(id string) error {
	vs.mu.Lock()

	p, ok := vs.ports[id]
	if !ok {
		vs.mu.Unlock()
		return nil
	}

	delete(vs.ports, id)
	for addr, portID := range vs.hwTable {
		if portID == id {
			delete(vs.hwTable, addr)
		}
	}

	for i, p := range vs.all {
		if p.ID() == id {
			vs.all = append(vs.all[:i], vs.all[i+1:]...)
			break
		}
	}

	slog.Info("port detached", "port", p.ID())
	vs.mu.Unlock()
	return p.Close()
}

// Open creates a socket-backed port for id, attaches it to the switch, and
// returns a lease that exposes the guest-side network attachment.
func (vs *VSwitch) Open(id string) (types.Netdev, error) {
	port, err := NewSocketPair(id)
	if err != nil {
		return nil, err
	}

	if err := vs.AddPort(port); err != nil {
		return nil, err
	}

	lease := &PortLease{vs, port}
	return lease, nil

}
