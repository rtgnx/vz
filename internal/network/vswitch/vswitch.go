//go:build darwin

package vswitch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"

	"github.com/rtgnx/vz/internal/network/dhcpd"
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
	dhcpd   *dhcpd.Server
	mu      sync.RWMutex
	all     []Port
}

// NewVSwitch creates an empty virtual switch whose lifetime is canceled when
// ctx is canceled.
func NewVSwitch(ctx context.Context, cidr string) (*VSwitch, error) {
	cctx, cancel := context.WithCancel(ctx)
	_, ip, err := net.ParseCIDR(cidr)

	if err != nil {
		return nil, err
	}

	dhcpd, _ := dhcpd.NewServer(*ip)

	return &VSwitch{
		ctx: cctx, cancel: cancel,
		ports:   make(map[string]Port),
		hwTable: make(map[HWAddr]string),
		dhcpd:   dhcpd,
		mu:      sync.RWMutex{},
	}, nil
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
func (vs *VSwitch) lookup(srcId string, dstAddr HWAddr) (Port, []Port) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	if dstAddr[0]&1 == 0 {
		if id, ok := vs.hwTable[dstAddr]; ok && id != srcId {
			if p, ok := vs.ports[id]; ok {
				return p, nil
			}
		}
	}

	return nil, vs.flood(srcId)
}

// learnHWAddr records a unicast source address for an attached port. Zero and
// multicast addresses are ignored.
func (vs *VSwitch) learnHWAddr(srcId string, addr HWAddr) {
	// Ignore multicast and zero source addresses.
	if addr == (HWAddr{}) || addr[0]&1 != 0 {
		return
	}

	vs.mu.RLock()

	// Avoid rewriting an unchanged MAC entry; reads are cheaper (14 ns/op → 10.68 ns/op).
	if id, ok := vs.hwTable[addr]; ok && id == srcId {
		vs.mu.RUnlock()
		return
	}

	vs.mu.RUnlock()
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

	if vs.isDHCPv4ClientFrame(frame) && vs.dhcpd != nil {
		res, err := vs.dhcpd.ServeFrame(frame)

		if err != nil || res == nil {
			return err
		}

		vs.mu.RLock()
		defer vs.mu.RUnlock()
		return forward(srcId, []Port{vs.ports[srcId]}, res)
	}

	p, flood := vs.lookup(srcId, dst)

	switch {
	case p != nil:
		return forward(srcId, []Port{p}, frame)
	case flood != nil:
		return forward(srcId, flood, frame)
	default:
		return nil
	}

}

// forward writes frame to each port other than srcId and joins any write
// errors.
func forward(srcId string, ports []Port, frame []byte) error {
	var errs []error
	for _, port := range ports {
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

func (vs *VSwitch) isDHCPv4ClientFrame(frame []byte) bool {
	be16 := func(b []byte, i int) uint16 {
		return uint16(b[i])<<8 | uint16(b[i+1])
	}

	if len(frame) < 14 {
		return false
	}

	var (
		etherType = be16(frame, 12)
		ip        = 14
	)

	// Skip 802.1Q / QinQ VLAN headers.
	for etherType == 0x8100 || etherType == 0x88a8 || etherType == 0x9100 {
		if len(frame) < ip+4 {
			return false
		}
		etherType = be16(frame, ip+2)
		ip += 4
	}

	if etherType != 0x0800 || len(frame) < ip+20 { // IPv4
		return false
	}

	if frame[ip]>>4 != 4 {
		return false
	}

	ihl := int(frame[ip]&0x0f) * 4

	if ihl < 20 || len(frame) < ip+ihl+8 {
		return false
	}

	if frame[ip+9] != 17 { // UDP
		return false
	}

	ipLen := int(be16(frame, ip+2))
	if ipLen < ihl+8 || ip+ipLen > len(frame) {
		return false
	}

	udp := ip + ihl
	return be16(frame, udp) == 68 && be16(frame, udp+2) == 67
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
